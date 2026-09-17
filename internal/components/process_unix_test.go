//go:build !windows

package components

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKillTerminalProcessStopsChildJobs(t *testing.T) {
	cmd := exec.Command("bash", "-c", "set -m; sleep 60 & echo $!; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	killTerminalProcess(cmd.Process)
	_ = cmd.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for {
		output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if err != nil || strings.TrimSpace(string(output)) == "" || strings.HasPrefix(strings.TrimSpace(string(output)), "Z") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d is still running: %s", pid, output)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStopAllStopsEveryTerminal(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	var sessions []*TerminalSession
	for _, id := range []string{"active-project", "hidden-project"} {
		s, err := tm.Start(id, "sleep 60", t.TempDir(), true)
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, s)
	}
	tm.StopAll()
	for _, s := range sessions {
		if tm.session(s.ID) != nil || !s.isClosed() {
			t.Fatalf("terminal %s survived StopAll", s.ID)
		}
		deadline := time.Now().Add(2 * time.Second)
		for syscall.Kill(s.cmd.Process.Pid, 0) == nil {
			if time.Now().After(deadline) {
				t.Fatalf("process %d survived StopAll", s.cmd.Process.Pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestProjectCommandRestartPreservesOtherTerminals(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	cwd := t.TempDir()
	other, err := tm.Start("shell", "", cwd, true)
	if err != nil {
		t.Fatal(err)
	}
	command := "pwd > command-cwd; printf PROJECT_READY; sleep 60"
	first, err := tm.Start("project", command, cwd, true)
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput := func(s *TerminalSession) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			s.mu.Lock()
			ready := strings.Contains(string(s.pendingOutput), "PROJECT_READY")
			s.mu.Unlock()
			if ready {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("startup output was lost")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForOutput(first)
	actual, err := os.ReadFile(filepath.Join(cwd, "command-cwd"))
	if err != nil || strings.TrimSpace(string(actual)) != cwd {
		t.Fatalf("wrong working directory: %q, %v", actual, err)
	}
	if err := tm.Restart("project", command, true, cwd); err != nil {
		t.Fatal(err)
	}
	second := tm.session("project")
	if second == first || !first.isClosed() {
		t.Fatal("restart did not replace process")
	}
	waitForOutput(second)
	tm.Stop("project")
	if !second.isClosed() || tm.session("project") != nil {
		t.Fatal("project still running")
	}
	if tm.session("shell") != other || other.isClosed() {
		t.Fatal("unrelated shell was stopped")
	}
}
