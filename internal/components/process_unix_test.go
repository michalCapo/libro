//go:build !windows

package components

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func BenchmarkTerminalHasChildren(b *testing.B) {
	cmd := exec.Command("bash", "--noprofile", "--norc")
	if _, err := cmd.StdinPipe(); err != nil {
		b.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	b.ResetTimer()
	for b.Loop() {
		if terminalHasChildren(cmd.Process.Pid) {
			b.Fatal("idle shell reported as running a command")
		}
	}
}

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
	defer func() { _ = cmd.Process.Kill() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
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

func TestTerminalHasChildrenTracksCommandCompletion(t *testing.T) {
	cmd := exec.Command("bash", "--noprofile", "--norc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killTerminalProcess(cmd.Process); _ = cmd.Wait() })
	if terminalHasChildren(cmd.Process.Pid) {
		t.Fatal("idle shell reported as running a command")
	}
	if _, err := stdin.Write([]byte("sleep 60\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !terminalHasChildren(cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("running command was not detected")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := exec.Command("pkill", "-TERM", "-P", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		t.Fatal(err)
	}
	for terminalHasChildren(cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("completed command still reported as running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStopAllStopsEveryTerminal(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	var sessions []*TerminalSession
	for _, id := range []string{"active-project", "hidden-project"} {
		s, err := tm.StartWithEnvironment(id, "sleep 60", t.TempDir(), true, nil)
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

func TestLaunchAskedBeforeStopAllFailsAfterResume(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	old := tm.Generation()
	tm.StopAll()
	tm.Resume()
	if _, err := tm.StartWithSessionReporter(old, "late", "sleep 60", t.TempDir(), true, nil, nil); !errors.Is(err, ErrTerminalsStopped) {
		t.Fatalf("late launch: %v", err)
	}
	if tm.IsRunning("late") {
		t.Fatal("late launch started a terminal")
	}
	if _, err := tm.StartWithSessionReporter(tm.Generation(), "new", "sleep 60", t.TempDir(), true, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStopSessionKeepsNewerTerminal(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	old, err := tm.StartWithEnvironment("panel", "sleep 60", t.TempDir(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	tm.StopAll()
	tm.Resume()
	if _, err = tm.StartWithEnvironment("panel", "sleep 60", t.TempDir(), true, nil); err != nil {
		t.Fatal(err)
	}
	tm.StopSession(old)
	if !tm.IsRunning("panel") {
		t.Fatal("late cleanup stopped the newer terminal")
	}
}

func TestProjectCommandRestartPreservesOtherTerminals(t *testing.T) {
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	cwd := t.TempDir()
	other, err := tm.StartWithEnvironment("shell", "", cwd, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	command := "pwd > command-cwd; printf PROJECT_READY; sleep 60"
	first, err := tm.StartWithEnvironment("project", command, cwd, true, nil)
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
	tm.Stop("project")
	if _, err := tm.StartWithEnvironment("project", command, cwd, true, nil); err != nil {
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

func TestCommandWithFileKeepsPathAsOneArgument(t *testing.T) {
	path := "/tmp/a file's $(printf injected); `printf injected` __dir__.go"
	output, err := exec.Command("bash", "-c", CommandWithFile("printf '%s'", path)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != path {
		t.Fatalf("path changed: %q", output)
	}
}
