package libro

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStandaloneThreadPersistenceAndIsolation(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	createTables()
	DBSaveProject("project", t.TempDir())
	if _, err := db.Exec("INSERT INTO threads (id, name) VALUES ('thread:test', 'Fix server')"); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	sid := manager.NewSession()
	state := manager.Get(sid)
	manager.AddApp(sid, "https://example.com", WidthMD, "Project browser")
	if !manager.SwitchProject(sid, "thread:test") {
		t.Fatal("cannot switch to thread")
	}
	if len(state.Projects) != 1 || len(state.Apps) != 0 {
		t.Fatal("thread mixed with project")
	}
	if manager.GetActiveProjectPath(sid) != defaultHomeDir() {
		t.Fatal("thread inherited project directory")
	}
	if workspaceProjectLabel(state) != "Fix server" {
		t.Fatal("thread label missing")
	}
	manager.AddApp(sid, "https://example.org", WidthMD, "Thread browser")
	if !manager.SwitchProject(sid, "project") || len(state.Apps) != 1 || state.Apps[0].Name != "Project browser" {
		t.Fatal("project panels lost")
	}
	if _, err := db.Exec("UPDATE threads SET archived = 1 WHERE id = 'thread:test'"); err != nil {
		t.Fatal(err)
	}
	loaded := newAppStateFromDB()
	if len(loaded.Threads) != 1 || !loaded.Threads[0].Archived {
		t.Fatal("archive not persisted")
	}
	if !manager.SwitchProject(sid, "thread:test") || len(state.Apps) != 1 || state.Apps[0].Name != "Thread browser" {
		t.Fatal("thread panels lost")
	}
	if manager.SwitchProject(sid, "thread:missing") {
		t.Fatal("unknown thread accepted")
	}
}
