package libro

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
	// pkill sends SIGTERM to all matching processes; ignore errors (no matches is fine)
	_ = exec.Command("pkill", "-f", "^ttyd ").Run()
	// Give processes a moment to exit so their ports are released
	time.Sleep(200 * time.Millisecond)
	// Force-kill any that didn't exit
	_ = exec.Command("pkill", "-9", "-f", "^ttyd ").Run()
	time.Sleep(100 * time.Millisecond)
}

// portFree returns true if nothing is listening on the given TCP port.
func portFree(port int) bool {
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

	// Verify port is free before starting
	if !portFree(port) {
		return fmt.Errorf("port %d is already in use", port)
	}

	args := []string{
		"-p", strconv.Itoa(port),
		"-t", "fontSize=18",
	}
	if writable {
		args = append(args, "--writable")
	}
	// Wrap in bash --norc --noprofile to avoid user profile scripts
	// (e.g. .bashrc launching editors). Also supports complex commands with pipes.
	shellCmd := command + "; exec bash --norc --noprofile"
	if pwd != "" {
		shellCmd = fmt.Sprintf("cd %s && %s", shellQuote(pwd), shellCmd)
	}
	args = append(args, "bash", "--norc", "--noprofile", "-c", shellCmd)

	cmd := exec.Command("ttyd", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ttyd: %w", err)
	}

	tm.processes[appID] = cmd
	log.Printf("ttyd started for app %s on port %d (writable=%v): %s", appID, port, writable, command)

	// Wait for process in background to clean up zombie processes
	go func() {
		_ = cmd.Wait()
		tm.mu.Lock()
		delete(tm.processes, appID)
		tm.mu.Unlock()
		log.Printf("ttyd process for app %s exited", appID)
	}()

	// Verify ttyd is actually listening before returning
	if !waitForPort(port, 3*time.Second) {
		// Process started but never bound to port — clean up
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
