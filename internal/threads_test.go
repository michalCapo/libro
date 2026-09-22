package libro

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneThreadPersistenceAndIsolation(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	DBSaveProject("project", t.TempDir())
	if _, err := db.Exec("INSERT INTO threads (id, name) VALUES ('thread:test', 'Fix server')"); err != nil {
		t.Fatal(err)
	}
	// Older versions persisted project threads; new sessions must ignore them.
	if _, err := db.Exec("INSERT INTO threads (id, name, project) VALUES ('thread:old-project', 'Old project thread', 'project')"); err != nil {
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

func TestProjectThreadKeepsItsProjectDirectory(t *testing.T) {
	projectPath := t.TempDir()
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "thread:test",
		Projects:      []Project{{Name: "project", Path: projectPath}},
		Threads:       []Thread{{ID: "thread:test", Project: "project", Path: projectPath}},
	}
	manager.states["test"] = state

	project, path := threadProjectContext(state, "thread:test")
	if project != "project" || path != projectPath {
		t.Fatalf("thread context = %q, %q", project, path)
	}
	if got := manager.GetActiveProjectPath("test"); got != projectPath {
		t.Fatalf("thread directory = %q", got)
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
	t.Cleanup(func() { _ = db.Close(); db = original })
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

func TestCloseThreadAgentKeepsSharedProjectApps(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if _, err := db.Exec("INSERT INTO threads (id, name, project, path) VALUES ('thread:test', 'Test', 'project', '/project')"); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "thread:test",
		Threads:       []Thread{{ID: "thread:test", Project: "project", Path: "/project"}},
		Apps: []Application{
			{ID: "agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center"},
			{ID: "browser", PluginID: "browser", Dock: "right"},
			{ID: "issues", PluginID: "notes", Dock: "right"},
			{ID: "app", PluginID: "project-command", Dock: "bottom"},
			{ID: "shell", PluginID: "terminal", Dock: "bottom"},
		},
	}
	manager.states["test"] = state

	closed, err := manager.CloseThreadAgent("test", "agent")
	if err != nil || len(closed) != 2 || len(state.Apps) != 3 {
		t.Fatalf("close result: closed=%+v remaining=%+v err=%v", closed, state.Apps, err)
	}
	if state.Apps[0].ID != "issues" || state.Apps[1].ID != "app" || state.Apps[2].ID != "shell" {
		t.Fatalf("shared apps were closed: %+v", state.Apps)
	}
}

func TestCloseThreadAgentPreservesPanelsOnArchiveFailure(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "missing-schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
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

func TestThreadSessionSurvivesRestartAndDefaultAgentChange(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	// Start with the schema used before session persistence was added.
	if _, err := db.Exec("CREATE TABLE threads (id TEXT PRIMARY KEY, name TEXT NOT NULL, archived INTEGER NOT NULL DEFAULT 0); INSERT INTO threads VALUES ('thread:test', 'Existing', 1)"); err != nil {
		t.Fatal(err)
	}
	createTables()
	createTables() // Migration must be safe on subsequent launches.
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.saveThreadSession(sid, "thread:test", "pi", "pi --model example", "saved-session")
	state := newAppStateFromDB()
	thread := state.thread("thread:test")
	if thread == nil || thread.Name != "Existing" || !thread.Archived || thread.SessionID != "saved-session" || thread.AgentID != "pi" || thread.AgentCommand != "pi --model example" {
		t.Fatalf("resume metadata lost: %+v", thread)
	}
	if err := setDefaultThreadAgent("codex"); err != nil {
		t.Fatal(err)
	}
	state.ActiveProject = "thread:test"
	if launch := projectAutolaunchJS(state, sid); !strings.Contains(launch, `"plugin":"pi"`) {
		t.Fatalf("wrong agent on reopen: %s", launch)
	}
	state.Apps = []Application{{Type: AppTypeTerminal, PluginID: "pi"}}
	if projectAutolaunchJS(state, sid) != "" {
		t.Fatal("reopening a running thread started a second agent")
	}
	manager.saveThreadSession(sid, "thread:test", "pi", "pi --model example", "next-session")
	if loadThreads()[0].SessionID != "next-session" {
		t.Fatal("in-agent session switch was not persisted")
	}
}

func TestProjectAgentLaunchCreatesSiblingThreads(t *testing.T) {
	agent := Application{Type: AppTypeTerminal, PluginID: "codex", Dock: "center"}
	for _, workspace := range []string{"project", "thread:project", "thread:standalone"} {
		for _, occupied := range []bool{false, true} {
			state := &AppState{
				ActiveProject: workspace,
				Projects:      []Project{{Name: "project", Path: "/project"}},
				Threads:       []Thread{{ID: "thread:project", Project: "project"}, {ID: "thread:standalone"}},
				Apps:          []Application{{PluginID: "browser", Type: AppTypeURL, Dock: "right"}},
			}
			if occupied {
				state.Apps = append(state.Apps, agent)
			}
			want := workspace == "project" || workspace == "thread:project" && occupied
			if state.needsProjectThread(agent) != want {
				t.Fatalf("workspace=%s occupied=%t: wrong agent destination", workspace, occupied)
			}
			if state.needsProjectThread(state.Apps[0]) {
				t.Fatal("opening a browser must stay in its thread")
			}
		}
	}
}

func TestSelectingProjectReusesOpenThread(t *testing.T) {
	state := &AppState{ActiveProject: "other", Threads: []Thread{
		{ID: "thread:one", Project: "project"},
		{ID: "thread:two", Project: "project"},
		{ID: "thread:archived", Project: "project", Archived: true},
	}}
	if state.projectWorkspace("project") != "thread:two" {
		t.Fatal("project should select its newest open thread")
	}
	state.ActiveProject = "thread:one"
	if state.projectWorkspace("project") != "thread:one" {
		t.Fatal("selecting the active project should keep its thread")
	}
	if state.projectWorkspace("thread:archived") != "thread:archived" || state.projectWorkspace("other") != "other" {
		t.Fatal("explicit workspace selection changed")
	}
}

func TestProjectThreadChoosesInstalledAgentWithoutChangingStandaloneDefault(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if got := defaultProjectThreadAgent(); got != "" {
		t.Fatalf("selected an unavailable agent: %s", got)
	}
	for _, name := range []string{"codex", "pi"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := defaultProjectThreadAgent(); got == "" {
		t.Fatal("new project threads need an installed agent even without a configured default")
	}
	if defaultThreadAgent() != "" {
		t.Fatal("project fallback changed the standalone default")
	}
	if err := setDefaultThreadAgent("pi"); err != nil {
		t.Fatal(err)
	}
	if got := defaultProjectThreadAgent(); got != "pi" {
		t.Fatalf("configured agent was not preferred: %s", got)
	}
}
