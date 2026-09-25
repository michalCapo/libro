//go:build !windows

package libro

import (
	"fmt"
	"net"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"libro/internal/components"
)

func TestKillApplicationPort(t *testing.T) {
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\n[ \"$*\" = '-nP -t -a -iTCP:43210 -sTCP:LISTEN' ] || exit 2\nprintf '%%s\\n' %d %d\n", child.Process.Pid, child.Process.Pid)
	if err := os.WriteFile(filepath.Join(dir, "lsof"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := killApplicationPort(43210); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("listener process was not killed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener process did not exit")
	}
}

func TestKillApplicationPortErrors(t *testing.T) {
	for _, test := range []struct{ name, script, want string }{
		{"self", fmt.Sprintf("echo %d", os.Getpid()), "refusing to kill"},
		{"invalid", "echo invalid", "refusing to kill"},
		{"init", "echo 1", "refusing to kill"},
		{"lookup failure", "echo denied >&2; exit 1", "exit status 1"},
		{"gone", "exit 1", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "lsof"), []byte("#!/bin/sh\n"+test.script+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			err := killApplicationPort(43210)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestAllocateOccupiedApplicationPort(t *testing.T) {
	oldSM, oldTM := sm, tm
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { sm, tm = oldSM, oldTM })
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	// An occupied port must reach cleanup, but never kill Libro itself.
	script := fmt.Sprintf("#!/bin/sh\necho %d\n", os.Getpid())
	if err := os.WriteFile(filepath.Join(dir, "lsof"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if _, err := allocateApplicationPort(port); err == nil || !strings.Contains(err.Error(), "refusing to kill") {
		t.Fatalf("occupied port did not attempt cleanup: %v", err)
	}
	sm.states["test"] = &AppState{Apps: []Application{{ApplicationPort: port}}}
	if _, err := allocateApplicationPort(port); err == nil || !strings.Contains(err.Error(), "already assigned") {
		t.Fatalf("reserved port reached cleanup: %v", err)
	}
	delete(sm.states, "test")
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := allocateApplicationPort(port); err != nil || got != port {
		t.Fatalf("free port %d was not preserved: %d, %v", port, got, err)
	}
}
