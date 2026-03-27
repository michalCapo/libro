package main

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"sync"
)

// TtydManager manages ttyd processes
type TtydManager struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd // keyed by app ID
}

// NewTtydManager creates a new ttyd process manager
func NewTtydManager() *TtydManager {
	return &TtydManager{
		processes: make(map[string]*exec.Cmd),
	}
}

// Start launches a ttyd process for the given app.
// If pwd is non-empty, the command is prefixed with cd to that directory.
func (tm *TtydManager) Start(appID string, port int, command string, writable bool, pwd string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	args := []string{
		"-p", strconv.Itoa(port),
		"-t", "fontSize=18",
	}
	if writable {
		args = append(args, "--writable")
	}
	// Wrap in bash --norc --noprofile to avoid user profile scripts
	// (e.g. .bashrc launching editors). Also supports complex commands with pipes.
	shellCmd := command
	if pwd != "" {
		shellCmd = fmt.Sprintf("cd '%s' && %s", pwd, command)
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
