package components

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	r "github.com/michalCapo/g-sui/ui"
	"golang.org/x/net/websocket"
)

// WatchGnomeTheme starts a background goroutine that calls onChange every
// time GNOME's color-scheme setting flips between light and dark variants.
// No-op if gsettings is unavailable (non-GNOME desktops).
func WatchGnomeTheme(onChange func()) {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return
	}
	go func() {
		cmd := exec.Command("gsettings", "monitor", "org.gnome.desktop.interface", "color-scheme")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("terminal theme watcher: stdout pipe failed: %v", err)
			return
		}
		if err := cmd.Start(); err != nil {
			log.Printf("terminal theme watcher: start failed: %v", err)
			return
		}
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				onChange()
			}
			if err != nil {
				return
			}
		}
	}()
}

// userShell returns the user's $SHELL if set and available, otherwise "bash".
func userShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		if _, err := exec.LookPath(sh); err == nil {
			return sh
		}
	}
	return "bash"
}

// UserShellBase returns the basename of the user's shell (e.g. "zsh", "fish").
func UserShellBase() string {
	return filepath.Base(userShell())
}

// shellQuote escapes a string for safe use as a single shell argument.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// TerminalManager manages native PTY-backed terminal sessions.
type TerminalManager struct {
	mu       sync.Mutex
	sessions map[string]*TerminalSession
	launchMu sync.Mutex // serializes launches with StopAll
}

// TerminalSession is one PTY process plus its connected browser clients.
type TerminalSession struct {
	ID       string
	AppID    string
	Command  string
	Cwd      string
	Writable bool

	cmd         *exec.Cmd
	ptyFile     *os.File
	outputDone  chan struct{}
	processDone chan struct{}

	mu            sync.Mutex
	clients       map[*terminalClient]bool
	closed        bool
	pendingOutput []byte // startup output retained until the first client connects
	connected     bool
	cols          uint16
	rows          uint16
	activity      *agentActivity
	agentStatus   string
	agentWorked   bool
	agentEnded    bool
	agentMu       sync.Mutex
	processStatus string
}

type terminalClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type terminalWSMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Cols    uint16 `json:"cols,omitempty"`
	Rows    uint16 `json:"rows,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// terminalBinaryDataFrame prefixes binary WebSocket payloads that carry raw
// terminal bytes. Control messages stay JSON text, but PTY input/output use
// this fast path to avoid JSON marshal/parse on every keystroke and echo.
const terminalBinaryDataFrame byte = 0

const (
	// Coalesce PTY bursts into fewer WebSocket frames. A tiny delay keeps
	// keystroke echo responsive while avoiding thousands of sends during
	// full-screen TUI redraws, grep output, or large pastes.
	terminalOutputFlushDelay       = time.Millisecond
	terminalOutputExitFlushTimeout = 75 * time.Millisecond
	terminalOutputMaxBatch         = 64 * 1024
	terminalOutputQueueSize        = 128
)

// NewTerminalManager creates a PTY terminal manager.
func NewTerminalManager() *TerminalManager {
	return &TerminalManager{sessions: make(map[string]*TerminalSession)}
}

// Start launches (or returns) a PTY session for the given app.
func (tm *TerminalManager) Start(appID, command, cwd string, writable bool) (*TerminalSession, error) {
	return tm.StartWithEnvironment(appID, command, cwd, writable, nil)
}

// StartWithEnvironment launches a PTY session with additional environment
// variables. Entries override variables inherited from Libro.
func (tm *TerminalManager) StartWithEnvironment(appID, command, cwd string, writable bool, environment []string) (*TerminalSession, error) {
	tm.launchMu.Lock()
	defer tm.launchMu.Unlock()
	if appID == "" {
		return nil, fmt.Errorf("empty terminal id")
	}
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	if cwd == "" {
		cwd = "/"
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	tm.mu.Lock()
	if existing := tm.sessions[appID]; existing != nil && !existing.isClosed() {
		tm.mu.Unlock()
		return existing, nil
	}
	tm.mu.Unlock()

	shell := userShell()
	launchCommand, activity, err := prepareAgentActivity(command)
	if err != nil {
		return nil, fmt.Errorf("prepare agent activity: %w", err)
	}
	cmd := terminalCommand(launchCommand, shell)
	cmd.Dir = cwd
	cmd.Env = mergeEnvironment(os.Environ(), append(environment, "TERM=xterm-256color", "COLORTERM=truecolor"))
	if activity != nil {
		cmd.Env = append(cmd.Env, activity.env...)
	}

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 30})
	if err != nil {
		activity.cleanup()
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	s := &TerminalSession{
		ID:          appID,
		AppID:       appID,
		Command:     command,
		Cwd:         cwd,
		Writable:    writable,
		cmd:         cmd,
		ptyFile:     ptyFile,
		outputDone:  make(chan struct{}),
		processDone: make(chan struct{}),
		clients:     make(map[*terminalClient]bool),
		cols:        100,
		rows:        30,
		activity:    activity,
		agentStatus: "idle",
	}

	tm.mu.Lock()
	tm.sessions[appID] = s
	tm.mu.Unlock()

	log.Printf("terminal started for app %s (writable=%v): %s", appID, writable, command)
	go tm.readLoop(s)
	go tm.waitLoop(s)
	go s.watchAgentActivity()
	go s.watchProcessActivity()
	return s, nil
}

func mergeEnvironment(base, overrides []string) []string {
	lastOverride := make(map[string]int, len(overrides))
	for i, entry := range overrides {
		if name, _, ok := strings.Cut(entry, "="); ok {
			lastOverride[name] = i
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		if _, replaced := lastOverride[name]; !replaced {
			result = append(result, entry)
		}
	}
	for i, entry := range overrides {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || lastOverride[name] == i {
			result = append(result, entry)
		}
	}
	return result
}

// The shell stays alive after commands finish, so session existence is not activity.
func (s *TerminalSession) watchProcessActivity() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status := "idle"
		if terminalHasChildren(s.cmd.Process.Pid) {
			status = "running"
		}
		s.mu.Lock()
		changed := s.processStatus != status
		s.processStatus = status
		s.mu.Unlock()
		if changed {
			s.broadcast(terminalWSMessage{Type: "process-status", Data: status})
		}
		select {
		case <-s.processDone:
			return
		case <-ticker.C:
		}
	}
}

func terminalCommand(command, shell string) *exec.Cmd {
	if strings.TrimSpace(command) == "" {
		return exec.Command(shell)
	}
	return exec.Command(shell, "-lc", terminalShellScript(command, shell))
}

func terminalShellScript(command, shell string) string {
	return command + "; exec " + shellQuote(shell)
}

func (tm *TerminalManager) readLoop(s *TerminalSession) {
	chunks := make(chan []byte, terminalOutputQueueSize)
	go func() {
		s.outputLoop(chunks)
		close(s.outputDone)
	}()
	defer close(chunks)

	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptyFile.Read(buf)
		if n > 0 {
			s.activity.output(buf[:n], s.setAgentStatus)
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			chunks <- chunk
		}
		if err != nil {
			if err != io.EOF && !s.isClosed() {
				log.Printf("terminal read failed for app %s: %v", s.ID, err)
			}
			return
		}
	}
}

func (s *TerminalSession) outputLoop(chunks <-chan []byte) {
	var batch []byte
	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	startTimer := func() {
		if timer != nil {
			return
		}
		timer = time.NewTimer(terminalOutputFlushDelay)
		timerC = timer.C
	}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		s.broadcastOutput(batch)
		batch = batch[:0]
	}
	defer stopTimer()

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				flush()
				return
			}
			if len(chunk) == 0 {
				continue
			}
			if len(batch) == 0 {
				startTimer()
			}
			batch = append(batch, chunk...)
			if len(batch) >= terminalOutputMaxBatch {
				stopTimer()
				flush()
			}
		case <-timerC:
			timer = nil
			timerC = nil
			flush()
		}
	}
}

func (s *TerminalSession) waitForOutputFlush(timeout time.Duration) {
	if s.outputDone == nil {
		return
	}
	select {
	case <-s.outputDone:
	case <-time.After(timeout):
	}
}

func (tm *TerminalManager) waitLoop(s *TerminalSession) {
	defer close(s.processDone)
	err := s.cmd.Wait()
	code := 0
	if err != nil {
		code = 1
	}
	if s.cmd.ProcessState != nil {
		code = s.cmd.ProcessState.ExitCode()
	}
	s.waitForOutputFlush(terminalOutputExitFlushTimeout)
	s.broadcast(terminalWSMessage{Type: "exit", Code: code})
	tm.removeSession(s.ID, s)
	s.close(false)
	log.Printf("terminal process for app %s exited with code %d", s.ID, code)
}

func (tm *TerminalManager) removeSession(id string, s *TerminalSession) {
	tm.mu.Lock()
	if tm.sessions[id] == s {
		delete(tm.sessions, id)
	}
	tm.mu.Unlock()
}

// Stop terminates a terminal session.
func (tm *TerminalManager) Stop(appID string) {
	tm.mu.Lock()
	s := tm.sessions[appID]
	if s != nil {
		delete(tm.sessions, appID)
	}
	tm.mu.Unlock()
	if s != nil {
		s.close(true)
		log.Printf("terminal stopped for app %s", appID)
	}
}

// Restart kills the PTY session for an app, then starts it again.
func (tm *TerminalManager) Restart(appID, command string, writable bool, cwd string) error {
	return tm.RestartWithEnvironment(appID, command, writable, cwd, nil)
}

// RestartWithEnvironment restarts a PTY session with additional environment variables.
func (tm *TerminalManager) RestartWithEnvironment(appID, command string, writable bool, cwd string, environment []string) error {
	tm.Stop(appID)
	_, err := tm.StartWithEnvironment(appID, command, cwd, writable, environment)
	return err
}

// StopAll terminates all sessions managed by this TerminalManager.
func (tm *TerminalManager) StopAll() {
	tm.launchMu.Lock()
	defer tm.launchMu.Unlock()
	tm.mu.Lock()
	ids := make([]string, 0, len(tm.sessions))
	for id := range tm.sessions {
		ids = append(ids, id)
	}
	tm.mu.Unlock()
	for _, id := range ids {
		tm.Stop(id)
	}
}

func (tm *TerminalManager) session(id string) *TerminalSession {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.sessions[id]
}

func (s *TerminalSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *TerminalSession) close(killProcess bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	clients := make([]*terminalClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
		delete(s.clients, c)
	}
	ptyFile := s.ptyFile
	cmd := s.cmd
	s.mu.Unlock()

	for _, c := range clients {
		_ = c.conn.Close()
	}
	if killProcess && cmd != nil && cmd.Process != nil {
		killTerminalProcess(cmd.Process)
	}
	if ptyFile != nil {
		_ = ptyFile.Close()
	}
	s.activity.cleanup()
	if killProcess && s.processDone != nil {
		<-s.processDone
	}
}

func (s *TerminalSession) addClient(c *terminalClient) {
	s.mu.Lock()
	if len(s.pendingOutput) > 0 {
		payload := append([]byte{terminalBinaryDataFrame}, s.pendingOutput...)
		_ = c.sendBinary(payload)
		s.pendingOutput = nil
	}
	s.connected = true
	s.clients[c] = true
	cols, rows := s.cols, s.rows
	status := s.agentStatus
	if s.processStatus != "" {
		_ = c.send(terminalWSMessage{Type: "process-status", Data: s.processStatus})
	}
	if status != "" {
		_ = c.send(terminalWSMessage{Type: "agent-status", Data: status})
	}
	s.mu.Unlock()
	if cols > 0 && rows > 0 && s.ptyFile != nil {
		_ = pty.Setsize(s.ptyFile, &pty.Winsize{Cols: cols, Rows: rows})
	}
}

func (s *TerminalSession) removeClient(c *terminalClient) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

func (s *TerminalSession) broadcast(msg terminalWSMessage) {
	s.mu.Lock()
	clients := make([]*terminalClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()
	for _, c := range clients {
		if err := c.send(msg); err != nil {
			s.removeClient(c)
			_ = c.conn.Close()
		}
	}
}

func (s *TerminalSession) broadcastOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	if !s.connected {
		s.pendingOutput = append(s.pendingOutput, data...)
		if len(s.pendingOutput) > terminalOutputMaxBatch {
			s.pendingOutput = s.pendingOutput[len(s.pendingOutput)-terminalOutputMaxBatch:]
		}
	}
	clients := make([]*terminalClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()
	payload := make([]byte, len(data)+1)
	payload[0] = terminalBinaryDataFrame
	copy(payload[1:], data)
	for _, c := range clients {
		if err := c.sendBinary(payload); err != nil {
			s.removeClient(c)
			_ = c.conn.Close()
		}
	}
}

func (c *terminalClient) send(msg terminalWSMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return websocket.Message.Send(c.conn, string(b))
}

func (c *terminalClient) sendBinary(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return websocket.Message.Send(c.conn, data)
}

func (s *TerminalSession) handleInputBytes(data []byte) {
	if len(data) == 0 {
		return
	}
	s.mu.Lock()
	closed := s.closed
	writable := s.Writable
	ptyFile := s.ptyFile
	s.mu.Unlock()
	if closed || !writable || ptyFile == nil {
		return
	}
	_, _ = ptyFile.Write(data)
}

func (s *TerminalSession) handleMessage(msg terminalWSMessage) {
	s.mu.Lock()
	closed := s.closed
	writable := s.Writable
	ptyFile := s.ptyFile
	s.mu.Unlock()
	if closed || ptyFile == nil {
		return
	}
	switch msg.Type {
	case "input":
		if writable && msg.Data != "" {
			_, _ = ptyFile.Write([]byte(msg.Data))
		}
	case "resize":
		if msg.Cols > 0 && msg.Rows > 0 {
			s.mu.Lock()
			if s.cols == msg.Cols && s.rows == msg.Rows {
				s.mu.Unlock()
				return
			}
			s.cols, s.rows = msg.Cols, msg.Rows
			s.mu.Unlock()
			_ = pty.Setsize(ptyFile, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
		}
	case "ping":
		// Client keepalive; no response required.
	}
}

// RegisterTerminalRoutes mounts the native PTY WebSocket endpoint.
func RegisterTerminalRoutes(app *r.App, tm *TerminalManager, allowed func(sid, terminalID string) bool) {
	app.GET("/terminal/ws/", func(w http.ResponseWriter, req *http.Request) {
		terminalID := terminalIDFromPath(req.URL.Path)
		sid := req.URL.Query().Get("sid")
		if terminalID == "" {
			http.Error(w, "missing terminal id", http.StatusBadRequest)
			return
		}
		// Check the live pty first. If a terminal session is actually running,
		// allow the connection regardless of session-membership bookkeeping
		// state. The 403/404 split then reflects the truth: 404 = the pty is
		// not running, 403 = the client asked for an id we have never seen.
		if s := tm.session(terminalID); s != nil && !s.isClosed() {
			serveTerminalWS(w, req, s)
			return
		}
		if allowed != nil && !allowed(sid, terminalID) {
			http.Error(w, "terminal not found", http.StatusForbidden)
			return
		}
		http.Error(w, "terminal is not running", http.StatusNotFound)
	})
}

func serveTerminalWS(w http.ResponseWriter, req *http.Request, s *TerminalSession) {
	websocket.Handler(func(conn *websocket.Conn) {
		client := &terminalClient{conn: conn}
		s.addClient(client)
		defer s.removeClient(client)
		_ = client.send(terminalWSMessage{Type: "ready"})
		for {
			var raw []byte
			if err := websocket.Message.Receive(conn, &raw); err != nil {
				return
			}
			if len(raw) > 0 && raw[0] == terminalBinaryDataFrame {
				s.handleInputBytes(raw[1:])
				continue
			}
			var msg terminalWSMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				_ = client.send(terminalWSMessage{Type: "error", Message: "invalid terminal message"})
				continue
			}
			s.handleMessage(msg)
		}
	}).ServeHTTP(w, req)
}

func terminalIDFromPath(path string) string {
	id := strings.TrimPrefix(path, "/terminal/ws/")
	id = strings.Trim(id, "/")
	if slash := strings.IndexByte(id, '/'); slash >= 0 {
		id = id[:slash]
	}
	return id
}
