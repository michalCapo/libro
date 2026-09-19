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
	manager.AddTerminalApp(sid, "agent", "codex", 0, true, WidthFull, "Thread agent", "")
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
	if !manager.SwitchProject(sid, "thread:test") || len(state.Apps) != 1 || state.Apps[0].Name != "Thread agent" {
		t.Fatal("thread agent lost")
	}
	if manager.SwitchProject(sid, "thread:missing") {
		t.Fatal("unknown thread accepted")
	}
}

func TestThreadAllowsToolsAndOneAgent(t *testing.T) {
	state := &AppState{}
	agent := Application{Type: AppTypeTerminal, Command: "codex", PluginID: "codex"}
	tools := []Application{
		{Type: AppTypeURL, URL: "https://example.com", Dock: "right"},
		{Type: AppTypeTerminal, PluginID: "terminal", Dock: "bottom"},
		{Type: AppTypeURL, PluginID: "files", Dock: "right"},
	}
	for _, app := range tools {
		if !state.canStartThreadApp(app) {
			t.Fatal("thread rejected a tool panel")
		}
		state.Apps = append(state.Apps, app)
	}
	if !state.canStartThreadApp(agent) {
		t.Fatal("tools must not prevent starting the thread agent")
	}
	state.Apps = append(state.Apps, agent)
	if state.canStartThreadApp(agent) {
		t.Fatal("thread accepted a second agent")
	}
	for _, app := range tools {
		if !state.canStartThreadApp(app) {
			t.Fatal("running agent must not prevent opening tools")
		}
	}
}

func TestCloseThreadAgentArchivesAndClosesTools(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	createTables()
	if _, err := db.Exec("INSERT INTO threads (id, name) VALUES ('thread:test', 'Test')"); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "thread:test",
		Threads:       []Thread{{ID: "thread:test", Name: "Test"}},
		Apps: []Application{
			{ID: "agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center"},
			{ID: "browser", Type: AppTypeURL, Dock: "right"},
			{ID: "shell", Type: AppTypeTerminal, Dock: "bottom"},
		},
		snapshots: map[string]*projectSnapshot{"other": {Apps: []Application{{ID: "other-agent"}}}},
	}
	manager.states["test"] = state
	for _, id := range []string{"browser", "shell", "missing"} {
		apps, err := manager.CloseThreadAgent("test", id)
		if err != nil || apps != nil || state.Threads[0].Archived || len(state.Apps) != 3 {
			t.Fatalf("closing %s affected thread: %v", id, err)
		}
	}
	apps, err := manager.CloseThreadAgent("test", "agent")
	if err != nil || len(apps) != 3 || len(state.Apps) != 0 || !state.Threads[0].Archived {
		t.Fatalf("thread not closed and archived: %v", err)
	}
	if len(state.snapshots["other"].Apps) != 1 {
		t.Fatal("another workspace was closed")
	}
	if threads := loadThreads(); len(threads) != 1 || !threads[0].Archived {
		t.Fatal("archive was not persisted")
	}
}

func TestCloseThreadAgentPreservesPanelsOnArchiveFailure(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "missing-schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "thread:test",
		Threads:       []Thread{{ID: "thread:test"}},
		Apps:          []Application{{ID: "agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center"}},
	}
	manager.states["test"] = state
	if _, err := manager.CloseThreadAgent("test", "agent"); err == nil {
		t.Fatal("expected archive failure")
	}
	if state.Threads[0].Archived || len(state.Apps) != 1 {
		t.Fatal("archive failure changed the workspace")
	}
}
