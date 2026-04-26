package components

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

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

// TtydManager manages ttyd processes
type TtydManager struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd // keyed by app ID
}

// NewTtydManager creates a new ttyd process manager.
func NewTtydManager() *TtydManager {
	return &TtydManager{
		processes: make(map[string]*exec.Cmd),
	}
}

// KillStaleTtyd kills any ttyd processes left over from a previous run.
// Should be called once at startup before allocating ports.
func KillStaleTtyd() {
	_ = exec.Command("pkill", "-f", "^ttyd ").Run()
	time.Sleep(200 * time.Millisecond)
	_ = exec.Command("pkill", "-9", "-f", "^ttyd ").Run()
	time.Sleep(100 * time.Millisecond)
}

// PortFree returns true if nothing is listening on the given TCP port.
func PortFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// waitForPort polls until something is listening on port, or timeout elapses.
func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// Start launches a ttyd process for the given app.
// If pwd is non-empty, the command is prefixed with cd to that directory.
func (tm *TtydManager) Start(appID string, port int, command string, writable bool, pwd string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !PortFree(port) {
		return fmt.Errorf("port %d is already in use", port)
	}

	args := []string{
		"-p", strconv.Itoa(port),
		"-t", "fontSize=18",
	}
	if writable {
		args = append(args, "--writable")
	}
	// Use $SHELL (falling back to bash). Wrap with --norc --noprofile
	// to avoid user profile scripts (e.g. .bashrc launching editors).
	shell := userShell()
	shellCmd := command + "; exec " + shell + " --norc --noprofile"
	if pwd != "" {
		shellCmd = fmt.Sprintf("cd %s && %s", shellQuote(pwd), shellCmd)
	}
	args = append(args, shell, "--norc", "--noprofile", "-c", shellCmd)

	cmd := exec.Command("ttyd", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ttyd: %w", err)
	}

	tm.processes[appID] = cmd
	log.Printf("ttyd started for app %s on port %d (writable=%v): %s", appID, port, writable, command)

	go func() {
		_ = cmd.Wait()
		tm.mu.Lock()
		delete(tm.processes, appID)
		tm.mu.Unlock()
		log.Printf("ttyd process for app %s exited", appID)
	}()

	if !waitForPort(port, 3*time.Second) {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		delete(tm.processes, appID)
		return fmt.Errorf("ttyd on port %d failed to start listening", port)
	}

	return nil
}

// Stop kills the ttyd process for the given app
func (tm *TtydManager) Stop(appID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if cmd, ok := tm.processes[appID]; ok {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		delete(tm.processes, appID)
		log.Printf("ttyd stopped for app %s", appID)
	}
}

// StopAll kills all running ttyd processes
func (tm *TtydManager) StopAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for id, cmd := range tm.processes {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		delete(tm.processes, id)
	}
}

// RegisterTtydProxy mounts the /ttyd/ HTTP+WebSocket reverse proxy that
// forwards browser traffic to the per-app ttyd processes.
func RegisterTtydProxy(app *r.App) {
	app.GET("/ttyd/", handleTtydProxy)
}

// parseTtydPath extracts port and remainder from a ttyd proxy path.
// Input: /ttyd/{port}/rest/of/path → port, /rest/of/path
func parseTtydPath(urlPath string) (int, string, error) {
	path := strings.TrimPrefix(urlPath, "/ttyd/")
	slashIdx := strings.Index(path, "/")
	var portStr, remainder string
	if slashIdx >= 0 {
		portStr = path[:slashIdx]
		remainder = path[slashIdx:]
	} else {
		portStr = path
		remainder = "/"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("invalid port: %s", portStr)
	}
	return port, remainder, nil
}

func handleTtydProxy(w http.ResponseWriter, req *http.Request) {
	port, remainder, err := parseTtydPath(req.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if isWebSocketUpgrade(req) {
		proxyWebSocket(w, req, port, remainder)
		return
	}

	target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	rawQuery := req.URL.RawQuery

	proxy := &httputil.ReverseProxy{
		Director: func(outReq *http.Request) {
			outReq.URL.Scheme = target.Scheme
			outReq.URL.Host = target.Host
			outReq.URL.Path = remainder
			outReq.URL.RawQuery = rawQuery
			outReq.Host = target.Host
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Content-Security-Policy")
			return nil
		},
	}

	proxy.ServeHTTP(w, req)
}

func isWebSocketUpgrade(req *http.Request) bool {
	for _, v := range req.Header["Upgrade"] {
		if strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}

// proxyWebSocket hijacks the client connection and pipes raw bytes
// to/from the ttyd backend, which is more reliable than relying on
// httputil.ReverseProxy for WebSocket upgrades.
func proxyWebSocket(w http.ResponseWriter, req *http.Request, port int, path string) {
	backend := fmt.Sprintf("localhost:%d", port)

	backConn, err := net.Dial("tcp", backend)
	if err != nil {
		http.Error(w, "ttyd backend unavailable", http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		backConn.Close()
		http.Error(w, "websocket hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		backConn.Close()
		return
	}

	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s%s HTTP/1.1\r\n", req.Method, path, queryString(req))
	req.Header.Set("Host", backend)
	_ = req.Header.Write(&reqBuf)
	reqBuf.WriteString("\r\n")
	_, _ = backConn.Write(reqBuf.Bytes())

	if clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		_, _ = clientBuf.Read(buffered)
		_, _ = backConn.Write(buffered)
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(backConn, clientConn); done <- struct{}{} }()
	go func() { io.Copy(clientConn, backConn); done <- struct{}{} }()
	<-done
	clientConn.Close()
	backConn.Close()
}

func queryString(req *http.Request) string {
	if req.URL.RawQuery != "" {
		return "?" + req.URL.RawQuery
	}
	return ""
}
