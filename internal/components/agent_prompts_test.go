package components

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestClaudePromptsFollowTranscript(t *testing.T) {
	_, activity, err := prepareAgentActivity("claude")
	if err != nil {
		t.Fatal(err)
	}
	defer activity.cleanup()
	data, _ := os.ReadFile(filepath.Join(activity.dir, "claude.json"))
	var config struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	s := &TerminalSession{activity: activity, clients: make(map[*terminalClient]bool)}
	hook := func(event, payload string) {
		t.Helper()
		cmd := exec.Command("sh", "-c", config.Hooks[event][0].Hooks[0].Command)
		cmd.Stdin = strings.NewReader(payload)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hook: %v: %s", err, output)
		}
		s.readAgentSession()
		s.readAgentPrompts()
	}
	check := func(want ...string) {
		t.Helper()
		if !slices.Equal(s.agentPrompts, want) {
			t.Fatalf("prompts = %q, want %q", s.agentPrompts, want)
		}
	}
	transcript := filepath.Join(t.TempDir(), "one.jsonl")
	appendFile(t, transcript, `{"type":"user","isMeta":true,"message":{"role":"user","content":"Injected instructions"}}
{"type":"user","message":{"role":"user","content":"Fix\n  login"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Ignore this"}]}}
{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name>"}}
{"type":"user","message":{"role":"user","content":"<div> is too wide"}}
{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"Summary"}}
{"type":"user","message":{"role":"user","content":"[Request interrupted by user]"}}
{"type":"user","message":{"role":"user","content":[{"type":"image"},{"type":"text","text":"Check this"}]}}
{"type":"user","message":{"role":"user","content":"Half`)
	payload, _ := json.Marshal(map[string]string{"session_id": "one", "transcript_path": transcript})
	hook("SessionStart", string(payload))
	check("Fix\n  login", "<div> is too wide", "Check this")
	// A line is listed once Claude finishes writing it.
	appendFile(t, transcript, " written\"}}\n")
	s.readAgentPrompts()
	check("Fix\n  login", "<div> is too wide", "Check this", "Half written")
	// /clear starts a new transcript; the list starts again.
	cleared := filepath.Join(t.TempDir(), "two.jsonl")
	appendFile(t, cleared, "")
	payload, _ = json.Marshal(map[string]string{"session_id": "two", "transcript_path": cleared})
	hook("SessionStart", string(payload))
	check()
}

func TestCodexPromptsFromRollout(t *testing.T) {
	rollout := filepath.Join(t.TempDir(), "rollout.jsonl")
	appendFile(t, rollout, `{"type":"session_meta","payload":{"id":"one"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions"}]}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"local_image","path":"/tmp/a.png"},{"type":"text","text":"Make the buttons bigger"}]}}}
{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","content":[{"type":"text","text":"UserMessage"}]}}}
{"type":"event_msg","payload":{"type":"user_message","message":"Older Codex prompt"}}
`)
	s := &TerminalSession{activity: &agentActivity{kind: "codex"}, clients: make(map[*terminalClient]bool), agentSessionID: "one"}
	s.promptLog.source, s.promptLog.session = rollout, "one"
	check := func(want ...string) {
		t.Helper()
		s.readAgentPrompts()
		if !slices.Equal(s.agentPrompts, want) {
			t.Fatalf("prompts = %q, want %q", s.agentPrompts, want)
		}
	}
	check("Make the buttons bigger", "Older Codex prompt")
	// A file replaced by a longer one is read from the start.
	replacement := rollout + ".new"
	appendFile(t, replacement, `{"type":"event_msg","payload":{"type":"user_message","message":"Replacement prompt with a much longer text than before, so the file grows past the old offset"}}
{"type":"event_msg","payload":{"type":"user_message","message":"Second replacement prompt"}}
{"type":"event_msg","payload":{"type":"user_message","message":"Third replacement prompt"}}
`)
	if err := os.Rename(replacement, rollout); err != nil {
		t.Fatal(err)
	}
	check("Replacement prompt with a much longer text than before, so the file grows past the old offset", "Second replacement prompt", "Third replacement prompt")
	// /new reports another session; the old rollout is hidden before the next lookup.
	s.agentSessionID = "two"
	check()
	// After the agent exits, the panel runs a shell and lists nothing.
	s.agentSessionID = "one"
	check("Replacement prompt with a much longer text than before, so the file grows past the old offset", "Second replacement prompt", "Third replacement prompt")
	s.setAgentStatus("exited")
	if len(s.agentPrompts) != 0 {
		t.Fatalf("prompts kept after the agent exited: %q", s.agentPrompts)
	}
}
