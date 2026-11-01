package components

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAgentSessions(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	environment := map[string]string{"CLAUDE_CONFIG_DIR": claude, "CODEX_HOME": codex}
	dir := "/home/me/my.project"
	project := filepath.Join(claude, "projects", "-home-me-my-project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	for i, file := range []struct{ id, content string }{
		{"renamed", `{"type":"ai-title","aiTitle":"Generated"}` + "\n" + `{"type":"custom-title","customTitle":"Renamed"}` + "\n" + `{"type":"ai-title","aiTitle":"Generated again"}`},
		{"prompt", `{"type":"user","cwd":"/home/me/my.project/sub","message":{"role":"user","content":"Fix the login"}}`},
		{"empty", `{"type":"mode","mode":"normal"}`},
		{"other-folder", `{"type":"user","cwd":"/home/me/my/project","message":{"role":"user","content":"Same encoded name"}}`},
	} {
		path := filepath.Join(project, file.id+".jsonl")
		if err := os.WriteFile(path, []byte(file.content+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now, now.Add(-time.Duration(i*2)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(codex, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE threads (id TEXT, cwd TEXT, title TEXT, first_user_message TEXT, updated_at INTEGER, archived INTEGER, source TEXT);
INSERT INTO threads VALUES
	('codex-cli', ?1, 'Codex task', 'prompt', ?2, 0, 'cli'),
	('codex-exec', ?1, 'Exec run', 'prompt', ?2, 0, 'exec'),
	('codex-archived', ?1, 'Archived', 'prompt', ?2, 1, 'cli'),
	('codex-other', '/elsewhere', 'Other folder', 'prompt', ?2, 0, 'cli')`, dir, now.Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	want := []AgentSession{
		{Kind: "claude", ID: "renamed", Title: "Renamed", Updated: now.UnixMilli()},
		{Kind: "codex", ID: "codex-cli", Title: "Codex task", Updated: now.Add(-time.Hour).UnixMilli()},
		{Kind: "claude", ID: "prompt", Title: "Fix the login", Updated: now.Add(-2 * time.Hour).UnixMilli()},
	}
	if got := AgentSessions(dir, environment); !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions:\n got %+v\nwant %+v", got, want)
	}
}
