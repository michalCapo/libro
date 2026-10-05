package components

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestManagedOwnershipAndRedaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY requires POSIX")
	}
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	dir := t.TempDir()
	const secret = "fake-test-key-12345"
	first, err := tm.StartManaged(tm.Generation(), "first", `printf '%s' "$TEST_API_KEY"; sleep 30`, dir, filepath.Join(dir, "first.log"), []string{"TEST_API_KEY=" + secret})
	if err != nil {
		t.Fatal(err)
	}
	second, err := tm.StartManaged(tm.Generation(), "second", `sleep 30`, dir, filepath.Join(dir, "second.log"), nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, _ := tm.Logs("first")
		if strings.Contains(output, "[REDACTED]") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("redacted output missing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	first.managed.activity("done")
	if state, _ := tm.ManagedStatus("first", time.Hour); state.Status != "waiting" {
		t.Fatal("missing waiting status")
	}
	first.managed.activity("working")
	first.managed.mu.Lock()
	first.managed.state.LastActivity = time.Now().Add(-time.Hour)
	first.managed.mu.Unlock()
	if state, _ := tm.ManagedStatus("first", time.Minute); state.Status != "stalled" {
		t.Fatal("missing stall status")
	}
	tm.InterruptManaged("first")
	if tm.IsRunning("first") || !tm.IsRunning("second") {
		t.Fatal("interrupt affected another owned process")
	}
	state, ok := tm.ManagedStatus("first", time.Minute)
	if !ok || state.Status != "failed" || state.ExitCode == nil || *state.ExitCode == 0 {
		t.Fatal("interrupted attempt has no exit status")
	}
	data, err := os.ReadFile(filepath.Join(dir, "first.log"))
	if err != nil || strings.Contains(string(data), secret) || !strings.Contains(string(data), "[REDACTED]") {
		t.Fatal("attempt log leaked or lost credential output")
	}
	tm.InterruptManaged("second")
	select {
	case <-second.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("owned process survived interrupt")
	}
}

func TestManagedAgentExitAndRecoveryMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY requires POSIX")
	}
	tm := NewTerminalManager()
	t.Cleanup(tm.StopAll)
	dir := t.TempDir()
	// Named pi exercises the lifecycle integration's exit marker removal.
	script := filepath.Join(dir, "pi")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf result\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	session, err := tm.StartManaged(tm.Generation(), "attempt", script+" -p", dir, filepath.Join(dir, "attempt.log"), nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not exit")
	}
	state, ok := tm.ManagedStatus("attempt", time.Minute)
	if !ok || state.Status != "failed" || state.ExitCode == nil || *state.ExitCode != 7 {
		t.Fatal("agent exit code was hidden by shell lifecycle")
	}
	data, err := os.ReadFile(filepath.Join(dir, "attempt.log.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved ManagedStatus
	if err = json.Unmarshal(data, &saved); err != nil || saved.ExitCode == nil || *saved.ExitCode != 7 {
		t.Fatal("exit metadata not recoverable")
	}
}

func TestSecretRedactorSplitWrites(t *testing.T) {
	r := secretRedactor{secrets: [][]byte{[]byte("fake-secret"), []byte("fake-secret-long")}}
	var output []byte
	for _, chunk := range []string{"before fa", "ke-sec", "ret-long then fake-sec", "ret after"} {
		output = append(output, r.feed([]byte(chunk), false)...)
	}
	output = append(output, r.feed(nil, true)...)
	if string(output) != "before [REDACTED] then [REDACTED] after" {
		t.Fatal("split or overlapping credential leaked")
	}
}

func TestBashStartupCredentialsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash requires POSIX")
	}
	dir := t.TempDir()
	const secret = "fake-startup-key"
	// Use a fixture startup file, without touching the user's Bash files.
	if err := os.WriteFile(filepath.Join(dir, ".bashrc"), []byte("export TEST_API_KEY="+secret+"\necho startup-noise\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\nexport HOME=" + shellQuote(dir) + "\nexec /bin/bash --noprofile --rcfile " + shellQuote(filepath.Join(dir, ".bashrc")) + " -ic \"$2\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bash"), []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	environment, err := BashEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range environment {
		if entry == "TEST_API_KEY="+secret {
			found = true
		}
		if strings.Contains(entry, "startup-noise") {
			t.Fatal("startup output mixed with credentials")
		}
	}
	if !found {
		t.Fatal("Bash startup credentials not loaded")
	}
}
