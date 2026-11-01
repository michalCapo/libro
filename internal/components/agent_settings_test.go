package components

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRecoverAgentSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexHome := filepath.Join(home, "codex-custom")
	t.Setenv("CODEX_HOME", filepath.Join(home, "wrong-home"))
	const session = "01a0c304-7225-78c3-b807-123456789abc"
	for _, test := range []struct{ name, command, path, data, model, effort string }{
		{"codex latest turn", "codex", filepath.Join(codexHome, "sessions", "2026", "10", "05", "rollout-date-"+session+".jsonl"),
			`{"type":"turn_context","payload":{"model":"old","effort":"low"}}` + "\n" +
				`{"type":"turn_context","payload":{"model":"new","effort":"medium"}}` + "\n" +
				`{"type":"event_msg","payload":{"model":"ignore","effort":"ignore"}}` + "\n" + `{partial`, "new", "medium"},
		{"codex reasoning_effort", "codex", filepath.Join(codexHome, "sessions", "rollout-date-"+session+".jsonl"),
			`{"type":"turn_context","payload":{"model":"new","reasoning_effort":"high"}}`, "new", "high"},
		{"claude latest assistant", "/usr/bin/claude", filepath.Join(home, ".claude", "projects", "-repo-path", session+".jsonl"),
			`{"type":"assistant","message":{"model":"old"}}` + "\n" +
				`{"type":"assistant","message":{"model":"new","effort":"high"}}` + "\n" +
				`{"type":"user","message":{"model":"ignore"}}`, "new", "high"},
		{"claude synthetic message", "claude", filepath.Join(home, ".claude", "projects", "-repo-path", session+".jsonl"),
			`{"type":"assistant","message":{"model":"new","effort":"high"}}` + "\n" +
				`{"type":"assistant","message":{"model":"<synthetic>"}}`, "new", "high"},
		{"claude without effort", "claude", filepath.Join(home, ".claude", "projects", "-repo-path", session+".jsonl"),
			`{"type":"assistant","message":{"model":"new"}}`, "new", ""},
		{"claude top-level effort", "claude", filepath.Join(home, ".claude", "projects", "-repo-path", session+".jsonl"),
			`{"type":"assistant","effort":"medium","message":{"model":"new"}}`, "new", "medium"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(test.path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(test.path, []byte(test.data), 0600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Remove(test.path) })
			model, effort := RecoverAgentSettings(test.command, session, codexHome)
			if model != test.model || effort != test.effort {
				t.Fatalf("got %q %q, want %q %q", model, effort, test.model, test.effort)
			}
		})
	}
	for _, command := range []string{"codex", "claude", "pi", "opencode", "ollama launch claude"} {
		if model, effort := RecoverAgentSettings(command, "missing-session", codexHome); model != "" || effort != "" {
			t.Fatalf("missing session: %q %q", model, effort)
		}
	}
	if model, effort := RecoverAgentSettings("claude", "../session", ""); model != "" || effort != "" {
		t.Fatal("path traversal accepted")
	}
}

func TestResumeAgentSettingsArguments(t *testing.T) {
	dir := t.TempDir()
	for _, kind := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(dir, kind), []byte("#!/bin/sh\nprintf '%s\\0' \"$@\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, kind, flags string
		want              []string
	}{
		{"claude saved settings", "claude", "", []string{"--model", "saved model", "--effort", "medium", "--resume", "session-123"}},
		{"claude existing model", "claude", " --model configured", []string{"--model", "configured", "--effort", "medium", "--resume", "session-123"}},
		{"claude equals flags", "claude", " --model=configured --effort=high", []string{"--model=configured", "--effort=high", "--resume", "session-123"}},
		{"codex saved settings", "codex", "", []string{"resume", "session-123", "-c", `model="saved model"`, "-c", `model_reasoning_effort="medium"`}},
		{"codex model flag", "codex", " -m configured", []string{"resume", "session-123", "-m", "configured", "-c", `model_reasoning_effort="medium"`}},
		{"claude prompt mentions flags", "claude", " --append-system-prompt 'Avoid --model and --effort'", []string{"--append-system-prompt", "Avoid --model and --effort", "--model", "saved model", "--effort", "medium", "--resume", "session-123"}},
		{"codex other config", "codex", ` -c 'profile="model=x"'`, []string{"resume", "session-123", "-c", `profile="model=x"`, "-c", `model="saved model"`, "-c", `model_reasoning_effort="medium"`}},
		{"codex config flags", "codex", ` -c 'model="configured"' --config='model_reasoning_effort="high"'`, []string{"resume", "session-123", "-c", `model="configured"`, `--config=model_reasoning_effort="high"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := ResumeAgentCommand(filepath.Join(dir, test.kind)+test.flags, "session-123", "saved model", "medium")
			data, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
			if !reflect.DeepEqual(args, test.want) {
				t.Fatalf("args: %q, want %q", args, test.want)
			}
		})
	}
}
