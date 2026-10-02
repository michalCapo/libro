package components

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTerminalLogs(t *testing.T) {
	tm := NewTerminalManager()
	log := &terminalLog{}
	tm.logs["app"] = log
	session := &TerminalSession{log: log, connected: true}
	session.broadcastOutput([]byte("stdout\r\nstderr\r\n"))
	for range 2 {
		if output, truncated := tm.Logs("app"); output != "stdout\r\nstderr\r\n" || truncated {
			t.Fatalf("logs are missing or consumed: %q %v", output, truncated)
		}
	}
	session.broadcastOutput([]byte(strings.Repeat("x", terminalLogLimit)))
	session.broadcastOutput([]byte("end"))
	if output, truncated := tm.Logs("app"); len(output) != terminalLogLimit || !strings.HasSuffix(output, "end") || !truncated {
		t.Fatal("logs must retain a bounded tail and report truncation")
	}
	tm.Stop("app")
	if output, truncated := tm.Logs("app"); output != "" || truncated {
		t.Fatal("stop must clear logs even after the process exits")
	}
}

func TestTerminalLogsSurviveExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX test command")
	}
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	session, err := tm.StartWithEnvironment("app", "printf stdout; printf stderr >&2; exit 0", t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	output, truncated := tm.Logs("app")
	if tm.IsRunning("app") || !strings.Contains(output, "stdout") || !strings.Contains(output, "stderr") || truncated {
		t.Fatalf("exited process lost output: %q %v", output, truncated)
	}
	if other, _ := tm.Logs("other"); other != "" {
		t.Fatal("logs leaked to another terminal")
	}
}
