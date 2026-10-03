package components

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/net/websocket"
)

func TestAgentWindowTitles(t *testing.T) {
	const id = "01a0c304-7225-78c3-b807-123456789abc"
	for _, tt := range []struct{ kind, input, want string }{
		{"codex", "Working | " + id + " ⠋ | Fix login", "Fix login"},
		{"codex", "Ready | " + id, ""},
		{"codex", "Working | " + id + " ⠋", ""},
		{"codex", "Ready | " + id + " | 01a0c304-7225-78c3-b807-12345...", ""},
		{"codex", "Ready | " + id + " | renaming...", ""},
		{"codex", "Ready | Fix login", "Fix login"},
		{"pi", "π - Fix login - project", "Fix login"},
		{"pi", "π - project", ""},
		{"claude", "✳ Fix login", "Fix login"},
		{"claude", "✳ Claude Code", ""},
		{"claude", "capo@fedora:~/project", ""},
		{"", "Fix login", ""},
	} {
		if got := agentWindowTitle(tt.kind, tt.input); got != tt.want {
			t.Errorf("%s %q = %q, want %q", tt.kind, tt.input, got, tt.want)
		}
	}
	if got := cleanAgentTitle(strings.Repeat("ž", 250)); !utf8.ValidString(got) || len([]rune(got)) != 200 {
		t.Fatal("title truncation corrupted Unicode")
	}
}

func TestCodexTitleMetadata(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "older schema", true: "current schema"}[current], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", home)
			db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			if _, err := db.Exec(`CREATE TABLE threads (id TEXT, title TEXT, first_user_message TEXT);
INSERT INTO threads VALUES ('one', 'Fix sidebar descriptions', 'First prompt'), ('two', '', 'Second session');`); err != nil {
				t.Fatal(err)
			}
			s := &TerminalSession{activity: &agentActivity{kind: "codex"}}
			s.setAgentStatus("session:one")
			s.readCodexTitle()
			if s.agentTitle != "Fix sidebar descriptions" {
				t.Fatalf("missing first-prompt fallback: %q", s.agentTitle)
			}
			if current {
				if _, err := db.Exec(`ALTER TABLE threads ADD COLUMN name TEXT; UPDATE threads SET name = 'Named task' WHERE id = 'one'`); err != nil {
					t.Fatal(err)
				}
				s.readCodexTitle()
				s.setAgentStatus("title:Old window title")
				if s.agentTitle != "Named task" {
					t.Fatalf("saved name not preferred: %q", s.agentTitle)
				}
			}
			s.setAgentStatus("session:two")
			s.readCodexTitle()
			if s.agentTitle != "Second session" {
				t.Fatalf("new session kept old title: %q", s.agentTitle)
			}
		})
	}
}

func TestCodexAbbreviatedSessionTitle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	const id = "01a1008a-b13f-74f0-88cb-1b46fd1303c4"
	if _, err := db.Exec(`CREATE TABLE threads (id TEXT, title TEXT, first_user_message TEXT);
INSERT INTO threads VALUES (?, 'Verify Codex sidebar descriptions', 'Original prompt')`, id); err != nil {
		t.Fatal(err)
	}
	var reported string
	for _, hint := range []string{id, "01a1008a-b13f-74f0-88cb-1b46f..."} {
		if title := RecoverCodexTitle(hint); title != "Verify Codex sidebar descriptions" {
			t.Fatalf("saved UUID was not recovered: %q", title)
		}
	}
	if title := RecoverCodexTitle("Keep my name"); title != "Keep my name" {
		t.Fatal("human name changed")
	}
	s := &TerminalSession{activity: &agentActivity{kind: "codex"}, reportSession: func(id string) { reported = id }}
	for _, title := range []string{"Ready | 01a1008a-b13f-74f0-88cb-1b46f...", "Working | 01a1008a-b13f-74f0-88cb-1b46f... ⠇ | ⠇"} {
		s.activity.output([]byte("\x1b]0;"+title+"\x07"), s.setAgentStatus)
		s.readCodexTitle()
		if reported != id || s.agentTitle != "Verify Codex sidebar descriptions" {
			t.Fatalf("abbreviated session = %q, title = %q", reported, s.agentTitle)
		}
	}
	if _, err := db.Exec(`INSERT INTO threads VALUES ('01a1008a-b13f-74f0-88cb-1b46faaaaaaa', 'Other session', '')`); err != nil {
		t.Fatal(err)
	}
	s.agentSessionID, reported = "", ""
	s.readCodexTitle()
	if reported != "" {
		t.Fatal("ambiguous prefix matched another session")
	}
	if hint := "01a1008a-b13f-74f0-88cb-1b46f..."; RecoverCodexTitle(hint) != hint {
		t.Fatal("ambiguous saved ID was renamed")
	}
}

func TestClaudeTitleFromHooksAndResume(t *testing.T) {
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
	s := &TerminalSession{activity: activity}
	hook := func(event, payload string) {
		t.Helper()
		cmd := exec.Command("sh", "-c", config.Hooks[event][0].Hooks[0].Command)
		cmd.Stdin = strings.NewReader(payload)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("hook: %v: %s", err, output)
		}
		s.readAgentSession()
	}
	hook("UserPromptSubmit", `{"session_id":"one","prompt":"Fix\n login"}`)
	hook("UserPromptSubmit", `{"session_id":"one","prompt":"Second prompt"}`)
	if s.agentTitle != "Fix login" {
		t.Fatalf("prompt title = %q", s.agentTitle)
	}
	s.activity.output([]byte("\x1b]0;✳ Login repair\x07"), s.setAgentStatus)
	hook("UserPromptSubmit", `{"session_id":"one","prompt":"Third prompt"}`)
	if s.agentTitle != "Login repair" {
		t.Fatalf("native title replaced: %q", s.agentTitle)
	}
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"user","isMeta":true,"message":{"role":"user","content":"Injected instructions"}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Ignore this"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"image"},{"type":"text","text":"Restored task"}]}}
`), 0600); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"session_id": "two", "transcript_path": transcript})
	hook("SessionStart", string(payload))
	if s.agentTitle != "Restored task" {
		t.Fatalf("resume title = %q", s.agentTitle)
	}
}

func TestAgentTitleReplayedOnReconnect(t *testing.T) {
	s := &TerminalSession{clients: make(map[*terminalClient]bool), connected: true}
	s.setAgentStatus("title:Fix sidebar descriptions")
	server := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		client := &terminalClient{conn: conn}
		s.addClient(client)
		s.removeClient(client)
	}))
	defer server.Close()
	for range 2 {
		conn, err := websocket.Dial("ws"+strings.TrimPrefix(server.URL, "http"), "", server.URL)
		if err != nil {
			t.Fatal(err)
		}
		var msg terminalWSMessage
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
		if msg.Type != "agent-title" || msg.Data != "Fix sidebar descriptions" {
			t.Fatalf("reconnected client missed title: %+v", msg)
		}
	}
}
