package components

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ApplicationInstructions is shared by startup prompts and application tool help.
const ApplicationInstructions = `Libro owns the application processes. Project settings choose shared or per-thread mode; status reports the mode and assigned URL.
Your application MCP tool and CLI are scoped to your assigned workspace. Only control that application; do not target another thread, project, PID, or port. In shared mode, that assigned application is shared with the project.
Before running or testing the app, call the Libro application tool with action status (CLI: libro application status).
If running, reuse it. If starting, wait and check status again. If stopped, use action start (CLI: libro application start), then check readiness.
Do not launch a separate application server from a shell, background task, subprocess, or another port. This includes npm/pnpm/bun dev or start, framework dev servers, and equivalent project commands. Do not kill a port owner to make room for your own server.
Use Libro application restart only when needed; it restarts only your assigned application (affecting other threads only in shared mode). Use stop only when requested or required by the task.
If Libro is unavailable or the start command is missing, report the problem and ask the user to configure it in Libro project settings; do not fall back to a separate server.
One-shot builds, lint checks, and tests that do not launch another application server can run normally.`

// AgentInstructions adds browser testing guidance to the shared application rules.
const AgentInstructions = ApplicationInstructions + `

Use the agent-browser CLI for browsing and browser testing. Libro's MCP provides application and notes tools; it does not provide browser automation.
Before first use, run agent-browser --help and read its bundled guide with agent-browser skills get core --full when available.
Choose a unique session name for this agent/thread and pass --session <name> on every browser command. Keep using that name for this task so concurrent agents do not share a browser. Do not use the default session or close other agents' sessions.
Typical flow: agent-browser --session <name> open <url>, then snapshot -i, click @ref or fill @ref "text", and screenshot <path>. Include --session <name> on each command and take a fresh snapshot after navigation or page changes.
Use the application managed by Libro for testing, following the application rules above. Agent-browser runs its own browser and does not share Libro panel logins.
When finished, close only your session with agent-browser --session <name> close. If agent-browser is not installed, report that it is required for browser testing.`

// Launch-local integrations never modify the user's agent settings. Only a
// status and session metadata are written to temporary activity files.
var agentCommandPattern = regexp.MustCompile(`^([a-zA-Z0-9_./-]+)(\s.*)?$`)
var agentSessionPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var codexSessionPattern = regexp.MustCompile(`(?i)^([0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}|[0-9a-f]{8}-[0-9a-f-]+\.\.\.)(?: [^|]*)?(?: \| |$)`)
var ollamaClaudePattern = regexp.MustCompile(`^(\s+launch\s+claude(?:\s+(?:--model(?:=|\s+)(?:"[^"]*"|'[^']*'|[^\s;&|]+)|--yes|-y))*)(?:\s+--(\s.*)?)?\s*$`)

type agentActivity struct {
	kind, dir, path string
	env             []string
	codexHome       string // CODEX_HOME seen by the launched agent
	codexSession    string // session ID shown in Codex's title
	osc             []byte
	escape, inOSC   bool
}

func prepareAgentActivity(command string) (string, *agentActivity, error) {
	parts := agentCommandPattern.FindStringSubmatch(strings.TrimSpace(command))
	if parts == nil {
		return command, nil, nil
	}
	kind := filepath.Base(parts[1])
	if kind == "ollama" {
		launch := ollamaClaudePattern.FindStringSubmatch(parts[2])
		if launch == nil {
			return command, nil, nil
		}
		// Ollama consumes its own flags; only arguments after -- reach Claude.
		parts[1] += launch[1] + " --"
		parts[2] = launch[2]
		kind = "claude"
	}
	if kind != "codex" && kind != "claude" && kind != "pi" && kind != "opencode" {
		return command, nil, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return command, nil, err
	}
	executableJSON, _ := json.Marshal(executable)
	instructionsJSON, _ := json.Marshal(AgentInstructions)
	libroMCP := map[string]any{"command": executable, "args": []string{"mcp"}}
	a := &agentActivity{kind: kind}
	if kind == "codex" {
		return agentExitCommand(parts[1] + ` -c 'tui.terminal_title=["run-state","session-id","thread-name"]'` + " -c " + shellQuote("mcp_servers.libro.command="+string(executableJSON)) + " -c " + shellQuote(`mcp_servers.libro.args=["mcp"]`) + " -c " + shellQuote(`mcp_servers.libro.env_vars=["LIBRO_INSTANCE","LIBRO_PORT","LIBRO_APPLICATION_PATH"]`) + " -c " + shellQuote("developer_instructions="+string(instructionsJSON)) + parts[2]), a, nil
	}
	dir, err := os.MkdirTemp("", "libro-agent-")
	if err != nil {
		return command, nil, err
	}
	a.dir, a.path = dir, filepath.Join(dir, "status")
	pathJSON, _ := json.Marshal(a.path)
	sessionJSON, _ := json.Marshal(filepath.Join(dir, "session"))
	var filename, content, args string
	switch kind {
	case "claude":
		hooks := map[string]any{}
		for event, state := range map[string]string{"UserPromptSubmit": "working", "PreToolUse": "working", "Stop": "done", "StopFailure": "error", "SessionEnd": "idle", "SessionStart": "idle"} {
			hookCommand := "printf '%s' " + shellQuote(state) + " > " + shellQuote(a.path)
			if event == "SessionStart" || event == "UserPromptSubmit" {
				hookCommand = "cat > " + shellQuote(filepath.Join(dir, "session")) + "; " + hookCommand
			}
			if event == "Stop" {
				// Background shells and agents wake Claude again when they finish.
				hookCommand = "if grep -q '\"background_tasks\":\\[[^]]'; then s=working; else s=done; fi; printf '%s' \"$s\" > " + shellQuote(a.path)
			}
			hooks[event] = []any{map[string]any{"hooks": []any{map[string]any{
				"type": "command", "command": hookCommand, "timeout": 2,
			}}}}
		}
		data, _ := json.Marshal(map[string]any{
			"hooks":       hooks,
			"permissions": map[string]any{"allow": []string{"mcp__libro__application", "mcp__libro__notes", "mcp__libro__children"}},
		})
		filename, content = "claude.json", string(data)
		mcpJSON, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"libro": libroMCP}})
		args = " --settings " + shellQuote(filepath.Join(dir, filename)) + " --mcp-config " + shellQuote(string(mcpJSON)) + " --append-system-prompt " + shellQuote(AgentInstructions)
	case "pi":
		filename = "pi.mjs"
		content = `import { writeFileSync } from 'node:fs';
export default function (pi) {
  const status = value => { try { writeFileSync(` + string(pathJSON) + `, value); } catch {} };
  const reportSession = ctx => {
    try { writeFileSync(` + string(sessionJSON) + `, JSON.stringify({session_id:ctx.sessionManager.getSessionId(), title:pi.getSessionName()})); } catch {}
  };
  const nameSession = text => {
    if (pi.getSessionName()) return;
    const name = text.replace(/[\x00-\x1f\x7f-\x9f]/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 160);
    if (name) pi.setSessionName(name);
  };
  pi.on('session_start', (_event, ctx) => {
    status('idle');
    const entry = ctx.sessionManager.getBranch().find(entry => entry.type === 'message' && entry.message.role === 'user');
    if (entry) {
      const content = entry.message.content;
      nameSession(typeof content === 'string' ? content : content.filter(part => part.type === 'text').map(part => part.text).join(' '));
    }
    reportSession(ctx);
  });
  pi.on('input', (event, ctx) => { if (event.source !== 'extension') { nameSession(event.text); reportSession(ctx); } });
  pi.on('session_info_changed', (_event, ctx) => reportSession(ctx));
  pi.on('agent_start', () => status('working'));
  pi.on('agent_end', event => { if (!event.willRetry) status('done'); });
  pi.on('session_shutdown', () => status('idle'));
}`
		args = " --extension " + shellQuote(filepath.Join(dir, filename)) + " --append-system-prompt " + shellQuote(AgentInstructions+"\nControl the saved project start command with libro application status|start|restart|stop|logs; see libro application --help. Application actions preserve the user’s active project and thread. Manage project notes with libro notes; see libro notes --help for create, list, read, set_status, and delete.")
	case "opencode":
		filename = "opencode.mjs"
		content = `import { writeFileSync } from 'node:fs';
export default async function () {
  const sessions = new Map();
  const status = () => {
    const states = [...sessions.values()];
    const value = states.includes('working') ? 'working' : states.includes('error') ? 'error' : states.length ? 'done' : 'idle';
    try { writeFileSync(` + string(pathJSON) + `, value); } catch {}
  };
  return { event: async ({ event }) => {
    const p = event.properties;
    if ((event.type === 'session.created' || event.type === 'session.updated') && !p.info.parentID) {
      try { writeFileSync(` + string(sessionJSON) + `, JSON.stringify({session_id:p.info.id})); } catch {}
    }
    if (event.type === 'session.status') {
      if (p.status.type !== 'idle') sessions.set(p.sessionID, 'working');
      else if (sessions.get(p.sessionID) !== 'error') sessions.set(p.sessionID, 'done');
    }
    else if (event.type === 'session.error') sessions.set(p.sessionID, 'error');
    else if (event.type === 'session.deleted') sessions.delete(p.info.id);
    else return;
    status();
  }};
}`
		// Inline configuration is merged with OpenCode's normal config. Preserve
		// existing inline settings and plugins, too.
		config := map[string]any{}
		if raw := os.Getenv("OPENCODE_CONFIG_CONTENT"); raw != "" {
			if err = json.Unmarshal([]byte(raw), &config); err != nil {
				break
			}
		}
		if config == nil {
			config = map[string]any{}
		}
		mcp, _ := config["mcp"].(map[string]any)
		if mcp == nil {
			mcp = map[string]any{}
		}
		mcp["libro"] = map[string]any{"type": "local", "command": []string{executable, "mcp"}, "enabled": true}
		config["mcp"] = mcp
		instructionsPath := filepath.Join(dir, "agent.md")
		if err = os.WriteFile(instructionsPath, []byte(AgentInstructions), 0600); err != nil {
			break
		}
		instructions, _ := config["instructions"].([]any)
		config["instructions"] = append(instructions, instructionsPath)
		plugins, _ := config["plugin"].([]any)
		config["plugin"] = append(plugins, "file://"+filepath.Join(dir, filename))
		encoded, _ := json.Marshal(config)
		a.env = []string{"OPENCODE_CONFIG_CONTENT=" + string(encoded)}
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600)
	}
	if err != nil {
		a.cleanup()
		return command, nil, err
	}
	prepared := parts[1] + args + parts[2]
	return agentExitCommand(prepared), a, nil
}

const agentExitMarker = `; printf '\033]777;libro;exited\007'`

func agentExitCommand(command string) string { return command + agentExitMarker }

func (a *agentActivity) cleanup() {
	if a != nil && a.dir != "" {
		_ = os.RemoveAll(a.dir)
	}
}

func (s *TerminalSession) watchAgentActivity() {
	if s.activity == nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var nextTitleRead time.Time
	for now := range ticker.C {
		s.mu.Lock()
		stopped := s.closed || s.agentEnded
		s.mu.Unlock()
		if stopped {
			return
		}
		s.readAgentSession()
		if s.activity.kind == "codex" && !now.Before(nextTitleRead) {
			s.readCodexTitle()
			nextTitleRead = now.Add(time.Second)
		}
		if data, err := os.ReadFile(s.activity.path); err == nil {
			s.setAgentStatus(string(data))
		}
	}
}

func (s *TerminalSession) readAgentSession() {
	if s.activity == nil || s.activity.dir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.activity.dir, "session"))
	var payload struct {
		SessionID      string `json:"session_id"`
		Title          string `json:"title"`
		Prompt         string `json:"prompt"`
		TranscriptPath string `json:"transcript_path"`
	}
	if err == nil && json.Unmarshal(data, &payload) == nil && agentSessionPattern.MatchString(payload.SessionID) {
		s.setAgentStatus("session:" + payload.SessionID)
		s.agentMu.Lock()
		defer s.agentMu.Unlock()
		s.setAgentTitle(payload.Title, 2)
		if s.activity.kind == "claude" {
			s.mu.Lock()
			unnamed := s.agentTitle == ""
			s.mu.Unlock()
			if unnamed {
				// A resumed session keeps its original prompt, not the next follow-up.
				s.setAgentTitle(claudeFirstPrompt(payload.TranscriptPath), 1)
				s.setAgentTitle(payload.Prompt, 1)
			}
		}
	}
}

func (s *TerminalSession) setAgentStatus(status string) {
	if title, ok := strings.CutPrefix(status, "title:"); ok {
		s.agentMu.Lock()
		defer s.agentMu.Unlock()
		s.setAgentTitle(title, 2)
		return
	}
	if id, ok := strings.CutPrefix(status, "session:"); ok {
		if id == "" {
			s.agentMu.Lock()
			defer s.agentMu.Unlock()
			s.agentSessionID, s.codexSessionPrefix = "", ""
			s.clearAgentTitle()
			return
		}
		if s.activity != nil && s.activity.kind == "codex" && strings.HasSuffix(id, "...") {
			s.agentMu.Lock()
			s.codexSessionPrefix = strings.TrimSuffix(id, "...")
			s.agentMu.Unlock()
			return
		}
		if !agentSessionPattern.MatchString(id) {
			return
		}
		s.agentMu.Lock()
		defer s.agentMu.Unlock()
		s.codexSessionPrefix = ""
		s.updateAgentSession(id)
		return
	}
	switch status {
	case "idle", "working", "done", "error", "exited":
	default:
		return
	}
	// The PTY exit marker and lifecycle file watcher can report concurrently.
	// Keep state changes and their websocket messages in the same order.
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	s.updateAgentStatus(status)
}

// updateAgentSession requires agentMu.
func (s *TerminalSession) updateAgentSession(id string) {
	if id == s.agentSessionID {
		return
	}
	// Titles seen before the first session ID belong to that session.
	if s.agentSessionID != "" {
		s.clearAgentTitle()
	}
	s.agentSessionID = id
	if s.reportSession != nil {
		s.reportSession(id)
	}
}

// updateAgentStatus requires agentMu.
func (s *TerminalSession) updateAgentStatus(status string) {
	codex := s.activity != nil && s.activity.kind == "codex"
	// Codex shows Ready while spawned agents run; they wake it when finished.
	waiting := codex && status == "done" && (s.codexWaiting || s.codexReported != "done" && codexAgentsRunning(s.activity.codexHome, s.agentSessionID))
	if codex {
		s.codexReported = status
	}
	s.mu.Lock()
	if s.closed || s.agentEnded {
		s.mu.Unlock()
		return
	}
	if status == "exited" {
		s.agentEnded = true
		status = "idle"
	}
	// A freshly opened prompt is idle, not a completed task.
	if status == "done" && s.activity != nil && s.activity.kind == "codex" && !s.agentWorked {
		status = "idle"
	}
	if waiting && status == "done" {
		status = "working"
		if !s.codexWaiting {
			s.codexWaiting = true
			go s.waitCodexAgents()
		}
	}
	switch status {
	case "idle":
		s.agentWorked = false
	case "working":
		s.agentWorked = true
	}
	if status == s.agentStatus {
		s.mu.Unlock()
		return
	}
	s.agentStatus = status
	s.mu.Unlock()
	if s.managed != nil {
		s.managed.activity(status)
	}
	s.broadcast(terminalWSMessage{Type: "agent-status", Data: status})
}

func (s *TerminalSession) waitCodexAgents() {
	for running := true; running; {
		time.Sleep(time.Second)
		s.agentMu.Lock()
		id, reported := s.agentSessionID, s.codexReported
		s.agentMu.Unlock()
		s.mu.Lock()
		stopped := s.closed || s.agentEnded
		s.mu.Unlock()
		running = !stopped && reported == "done" && codexAgentsRunning(s.activity.codexHome, id)
	}
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	s.codexWaiting = false
	if s.codexReported == "done" {
		s.updateAgentStatus("done")
	}
}

// codexAgentsRunning reports whether agents spawned by a Codex thread are in a turn.
func codexAgentsRunning(home, threadID string) bool {
	if threadID == "" {
		return false
	}
	db := openCodexStateDB(home)
	if db == nil {
		return false
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT t.rollout_path FROM thread_spawn_edges e JOIN threads t ON t.id = e.child_thread_id WHERE e.parent_thread_id = ? AND e.status = 'open'`, threadID)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var path string
		if rows.Scan(&path) != nil {
			continue
		}
		// Ignore rollouts abandoned mid-turn by a crashed or killed Codex.
		if info, err := os.Stat(path); err != nil || time.Since(info.ModTime()) > 15*time.Minute {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		started := bytes.LastIndex(data, []byte(`"type":"task_started"`))
		if started > bytes.LastIndex(data, []byte(`"type":"task_complete"`)) && started > bytes.LastIndex(data, []byte(`"type":"turn_aborted"`)) {
			return true
		}
	}
	return false
}

// openCodexStateDB uses the agent's CODEX_HOME, then Libro's, then ~/.codex.
func openCodexStateDB(home string) *sql.DB {
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		home = filepath.Join(userHome, ".codex")
	}
	// Codex versions its state database file name.
	paths, _ := filepath.Glob(filepath.Join(home, "state_*.sqlite"))
	var dbPath string
	var newest time.Time
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(newest) {
			dbPath, newest = path, info.ModTime()
		}
	}
	if dbPath == "" {
		return nil
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return nil
	}
	return db
}

var codexStates = map[string]bool{"Working": true, "Thinking": true, "Waiting": true, "Ready": true, "Starting": true, "": true}

// Parse only OSC title reports, including sequences split across PTY reads.
// Arbitrary terminal text and periods without output are never completion signals.
func (a *agentActivity) output(data []byte, report func(string)) {
	if a == nil {
		return
	}
	for _, b := range data {
		if a.inOSC {
			if b == 7 || (a.escape && b == '\\') {
				title := strings.TrimSuffix(string(a.osc), "\x1b")
				if title == "777;libro;exited" {
					report("exited")
				}
				state, threadTitle, _ := strings.Cut(title[min(len(title), 2):], " | ")
				// Only Codex titles carry its state. A shell title after Codex exits does not.
				if a.kind == "codex" && (strings.HasPrefix(title, "0;") || strings.HasPrefix(title, "2;")) && codexStates[state] {
					var session string
					if match := codexSessionPattern.FindStringSubmatch(threadTitle); match != nil {
						session = match[1]
					}
					// /new drops the session ID, then shows the new one. The thread
					// name may blink while working, so it is not a session signal.
					if !sameCodexSession(a.codexSession, session) {
						if a.codexSession != "" {
							report("session:")
							report("idle")
						}
						if session != "" {
							report("session:" + session)
						}
					}
					a.codexSession = session
					switch state {
					case "Working", "Thinking", "Waiting":
						report("working")
					case "Ready":
						report("done")
					case "Starting", "":
						report("idle")
					}
				}
				if strings.HasPrefix(title, "0;") || strings.HasPrefix(title, "2;") {
					if name := agentWindowTitle(a.kind, title[2:]); name != "" {
						report("title:" + name)
					}
				}
				a.inOSC, a.escape, a.osc = false, false, nil
				continue
			}
			if len(a.osc) >= 1024 {
				a.inOSC, a.osc = false, nil
			} else {
				a.osc = append(a.osc, b)
			}
		} else if a.escape && b == ']' {
			a.inOSC, a.osc = true, nil
		}
		a.escape = b == 27
	}
}

// sameCodexSession compares IDs that Codex may abbreviate with "...".
func sameCodexSession(a, b string) bool {
	a, b = strings.TrimSuffix(a, "..."), strings.TrimSuffix(b, "...")
	if a == "" || b == "" {
		return a == b
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// ResumeAgentCommand preserves configured flags and uses the agent's session selector.
func ResumeAgentCommand(command, sessionID, model, effort string) string {
	if sessionID == "" {
		return command
	}
	parts := agentCommandPattern.FindStringSubmatch(strings.TrimSpace(command))
	if parts == nil {
		return command
	}
	kind := filepath.Base(parts[1])
	if kind == "ollama" {
		launch := ollamaClaudePattern.FindStringSubmatch(parts[2])
		if launch == nil {
			return command
		}
		return parts[1] + launch[1] + " --" + launch[2] + " --resume " + shellQuote(sessionID)
	}
	switch kind {
	case "codex":
		if model != "" && !agentModelFlag.MatchString(command) && !codexModelConfig.MatchString(command) {
			parts[2] += " -c " + shellQuote("model="+configString(model))
		}
		if effort != "" && !codexEffortConfig.MatchString(command) {
			parts[2] += " -c " + shellQuote("model_reasoning_effort="+configString(effort))
		}
		return parts[1] + " resume " + shellQuote(sessionID) + parts[2]
	case "claude":
		if model != "" && !agentModelFlag.MatchString(command) {
			command += " --model " + shellQuote(model)
		}
		if effort != "" && !agentEffortFlag.MatchString(command) {
			command += " --effort " + shellQuote(effort)
		}
		return command + " --resume " + shellQuote(sessionID)
	case "pi", "opencode":
		return command + " --session " + shellQuote(sessionID)
	default:
		return command
	}
}

var agentModelFlag = regexp.MustCompile(`(?:^|\s)(?:--model|-m)(?:=|\s|$)`)
var agentEffortFlag = regexp.MustCompile(`(?:^|\s)--effort(?:=|\s|$)`)
var codexModelConfig = regexp.MustCompile(`(?:^|\s)(?:-c|--config)(?:=|\s+)['"]?model\s*=`)
var codexEffortConfig = regexp.MustCompile(`(?:^|\s)(?:-c|--config)(?:=|\s+)['"]?model_reasoning_effort\s*=`)

// JSON string quoting also produces TOML basic strings for Codex config values.
func configString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

// RecoverAgentSettings reads only model metadata from the exact agent session.
// Missing files and unsupported agents leave the launch command unchanged.
func RecoverAgentSettings(command, sessionID, codexHome string) (string, string) {
	parts := agentCommandPattern.FindStringSubmatch(strings.TrimSpace(command))
	if parts == nil || !agentSessionPattern.MatchString(sessionID) {
		return "", ""
	}
	var path string
	kind := filepath.Base(parts[1])
	switch kind {
	case "codex":
		if codexHome == "" {
			codexHome = os.Getenv("CODEX_HOME")
		}
		if codexHome == "" {
			home, _ := os.UserHomeDir()
			codexHome = filepath.Join(home, ".codex")
		}
		// Prefer Codex's exact index when available, then standard rollout names.
		if db := openCodexStateDB(codexHome); db != nil {
			_ = db.QueryRow("SELECT rollout_path FROM threads WHERE id = ?", sessionID).Scan(&path)
			_ = db.Close()
		}
		if path == "" {
			_ = filepath.WalkDir(filepath.Join(codexHome, "sessions"), func(candidate string, entry fs.DirEntry, err error) error {
				if err == nil && !entry.IsDir() && strings.HasSuffix(entry.Name(), "-"+sessionID+".jsonl") {
					path = candidate
					return fs.SkipAll
				}
				return nil
			})
		}
	case "claude":
		home, _ := os.UserHomeDir()
		paths, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
		if len(paths) == 1 {
			path = paths[0]
		}
	default:
		return "", ""
	}
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var model, effort string
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Effort  string `json:"effort"`
			Payload struct {
				Model           string `json:"model"`
				Effort          string `json:"effort"`
				ReasoningEffort string `json:"reasoning_effort"`
			} `json:"payload"`
			Message struct {
				Model  string `json:"model"`
				Effort string `json:"effort"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		if kind == "codex" && entry.Type == "turn_context" {
			model, effort = entry.Payload.Model, entry.Payload.Effort
			if effort == "" {
				effort = entry.Payload.ReasoningEffort
			}
		} else if kind == "claude" && entry.Type == "assistant" && entry.Message.Model != "" && entry.Message.Model != "<synthetic>" {
			model, effort = entry.Message.Model, entry.Message.Effort
			if effort == "" {
				effort = entry.Effort
			}
		}
	}
	if scanner.Err() != nil {
		return "", ""
	}
	return model, effort
}
