package components

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ManagedStatus describes a finite agent attempt, including silent hangs.
type ManagedStatus struct {
	Status       string    `json:"status"`
	LastActivity time.Time `json:"lastActivity"`
	ExitCode     *int      `json:"exitCode"`
}

type managedTerminal struct {
	mu        sync.Mutex
	state     ManagedStatus
	file      *os.File
	statePath string
	saved     time.Time
	redactor  secretRedactor
}

// BashEnvironment loads login and interactive startup files privately. Neither
// startup output nor environment values are sent to terminals or error messages.
func BashEnvironment() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lic", `set +x; if [ -r "$HOME/.bashrc" ]; then source "$HOME/.bashrc"; fi; set +x; printf '\000LIBRO_ENV\000'; env -0`)
	cmd.Stderr = io.Discard
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not load Bash provider environment")
	}
	marker := []byte("\x00LIBRO_ENV\x00")
	i := bytes.LastIndex(data, marker)
	if i < 0 {
		return nil, fmt.Errorf("bash provider environment unavailable")
	}
	return strings.Split(strings.TrimSuffix(string(data[i+len(marker):]), "\x00"), "\x00"), nil
}

// StartManaged uses existing agent integrations but exits with the agent rather
// than opening another interactive shell. Binding overrides are applied last.
func (tm *TerminalManager) StartManaged(generation uint64, id, command, cwd, logPath string, environment []string, sensitive ...string) (*TerminalSession, error) {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	m := &managedTerminal{state: ManagedStatus{Status: "running", LastActivity: time.Now().UTC()}, file: file, statePath: logPath + ".json"}
	for _, entry := range mergeEnvironment(os.Environ(), environment) {
		key, value, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if value != "" && (strings.Contains(upper, "KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "CREDENTIAL")) {
			m.redactor.secrets = append(m.redactor.secrets, []byte(value))
		}
	}
	for _, value := range sensitive {
		if value != "" {
			m.redactor.secrets = append(m.redactor.secrets, []byte(value))
		}
	}
	session, err := tm.startTerminal(generation, id, command, cwd, true, environment, nil, m)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return session, nil
}

func (m *managedTerminal) touch() {
	m.mu.Lock()
	m.state.LastActivity = time.Now().UTC()
	m.persist(false)
	m.mu.Unlock()
}
func (m *managedTerminal) write(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.file.Write(data)
}
func (m *managedTerminal) finish(code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.ExitCode = &code
	m.state.Status = "completed"
	if code != 0 {
		m.state.Status = "failed"
	}
	m.state.LastActivity = time.Now().UTC()
	_ = m.file.Close()
	m.persist(true)
}

// persist is called under the attempt lock. It contains no command or environment.
func (m *managedTerminal) persist(force bool) {
	if !force && time.Since(m.saved) < time.Second {
		return
	}
	data, _ := json.Marshal(m.state)
	if os.WriteFile(m.statePath+".tmp", data, 0o600) == nil {
		_ = os.Rename(m.statePath+".tmp", m.statePath)
	}
	m.saved = time.Now()
}
func (m *managedTerminal) activity(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.ExitCode != nil {
		return
	}
	m.state.Status = "running"
	if status == "idle" || status == "done" {
		m.state.Status = "waiting"
	}
	m.state.LastActivity = time.Now().UTC()
	m.persist(true)
}

// ManagedStatus retains exit information after a process is reaped.
func (tm *TerminalManager) ManagedStatus(id string, stall time.Duration) (ManagedStatus, bool) {
	tm.mu.Lock()
	log := tm.logs[id]
	tm.mu.Unlock()
	if log == nil || log.managed == nil {
		return ManagedStatus{}, false
	}
	m := log.managed
	m.mu.Lock()
	state := m.state
	m.mu.Unlock()
	if state.ExitCode == nil {
		if state.Status == "running" && time.Since(state.LastActivity) >= stall {
			state.Status = "stalled"
		}
	}
	return state, true
}

// InterruptManaged stops only this owned attempt and retains its log and exit.
func (tm *TerminalManager) InterruptManaged(id string) {
	if session := tm.session(id); session != nil && session.managed != nil {
		session.close(true)
	}
}

// Retain a possible secret prefix across reads, so split writes cannot leak it.
type secretRedactor struct {
	secrets [][]byte
	pending []byte
}

func (r *secretRedactor) feed(data []byte, final bool) []byte {
	r.pending = append(r.pending, data...)
	var out []byte
	for len(r.pending) > 0 {
		partial := false
		matched := 0
		for _, secret := range r.secrets {
			if bytes.HasPrefix(r.pending, secret) && len(secret) > matched {
				matched = len(secret)
			}
			if !final && len(r.pending) < len(secret) && bytes.HasPrefix(secret, r.pending) {
				partial = true
			}
		}
		if partial {
			break
		}
		if matched > 0 {
			out = append(out, []byte("[REDACTED]")...)
			r.pending = r.pending[matched:]
			continue
		}
		out = append(out, r.pending[0])
		r.pending = r.pending[1:]
	}
	return out
}
