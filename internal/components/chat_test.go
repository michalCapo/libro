package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Lines as Claude 2.x writes them with --output-format stream-json
// --include-partial-messages (shortened).
var chatTurn = []string{
	`{"type":"system","subtype":"init","session_id":"s1"}`,
	`{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}},"parent_tool_use_id":null}`,
	`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text"}},"parent_tool_use_id":null}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me "}},"parent_tool_use_id":null}`,
	`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"check."}},"parent_tool_use_id":null}`,
	`{"type":"stream_event","event":{"type":"content_block_stop","index":0},"parent_tool_use_id":null}`,
	`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Let me check."}]},"parent_tool_use_id":null}`,
	`{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"Bash"}},"parent_tool_use_id":null}`,
	`{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls -la","description":"List files"}}]},"parent_tool_use_id":null}`,
	`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls -la"},"description":"List files","permission_suggestions":[{"type":"addRules"}]}}`,
	`{"type":"assistant","message":{"id":"m9","content":[{"type":"text","text":"subagent text"}]},"parent_tool_use_id":"tu1"}`,
	`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"tu1","type":"tool_result","content":"a.txt\nb.txt","is_error":false}]},"parent_tool_use_id":null}`,
	`{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"Two files."}]},"parent_tool_use_id":null}`,
	`{"type":"control_request","request_id":"r2","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"question":"Delete them?","options":[{"label":"Yes"},{"label":"No"}]}]}}}`,
	`{"type":"result","subtype":"success","is_error":false,"result":"Two files.","session_id":"s1"}`,
}

func TestChatTurn(t *testing.T) {
	c := &chatState{index: map[string]int{}, streamed: map[string][]string{}}
	var statuses []string
	for _, line := range chatTurn {
		if status := c.handle([]byte(line), false); status != "" {
			statuses = append(statuses, status)
		}
		// Tools run without asking; only questions and plans wait.
		if want := strings.Count(line, "AskUserQuestion"); strings.Contains(line, "control_request") && len(c.requests) != want {
			t.Fatalf("requests = %+v after %s", c.requests, line)
		}
	}
	if got := strings.Join(statuses, ","); got != "input,done" {
		t.Fatalf("statuses = %s", got)
	}
	var got []string
	for _, item := range c.items {
		got = append(got, item.Kind+":"+item.Text+":"+item.State+":"+item.Detail)
	}
	want := []string{"assistant:Let me check.::", "tool:ls -la:done:a.txt\nb.txt", "assistant:Two files.::"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %q", got)
	}
	if len(c.requests) != 0 {
		t.Fatal("result must close open requests")
	}
}

func TestChatReplayShowsOnlyTheConversation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s1.jsonl")
	transcript := strings.Join([]string{
		`{"type":"queue-operation","operation":"enqueue"}`,
		`{"type":"user","message":{"role":"user","content":"Fix the bug"}}`,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"Caveat: hidden"}}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>"}}`,
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"thinking","thinking":""},{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/a.go"}}]}}`,
		`{"type":"assistant","isSidechain":true,"message":{"id":"m5","content":[{"type":"text","text":"side"}]}}`,
		`{"type":"assistant","message":{"id":"m2","content":[{"type":"text","text":"Fixed."}]}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(transcript), 0600); err != nil {
		t.Fatal(err)
	}
	c := &chatState{index: map[string]int{}, streamed: map[string][]string{}}
	c.replay(path)
	var got []string
	for _, item := range c.items {
		got = append(got, item.Kind+":"+item.Text+":"+item.State)
	}
	// A tool without a saved result was cut off; it no longer runs.
	want := "user:Fix the bug:|tool:/a.go:done|assistant:Fixed.:"
	if strings.Join(got, "|") != want {
		t.Fatalf("items = %q", got)
	}
}

func TestChatResumeID(t *testing.T) {
	for command, want := range map[string]string{
		"claude --model opus --resume 'abc-1'": "abc-1",
		"claude --resume=abc":                  "abc",
		"claude":                               "",
	} {
		if got := chatResumeID(command); got != want {
			t.Errorf("chatResumeID(%q) = %q, want %q", command, got, want)
		}
	}
}
