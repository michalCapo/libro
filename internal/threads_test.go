package libro

import (
	"database/sql"
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
	manager.SwitchProject(sid, "project")
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
	manager.SwitchProject(sid, "thread:test")
	manager.AddTerminalApp(sid, "agent", "pi --model example", 0, true, WidthFull, "Pi", "")
	manager.saveThreadSession(sid, "thread:test", "agent", "pi", "pi --model example", "saved-session")
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
	manager.saveThreadSession(sid, "thread:test", "agent", "pi", "pi --model example", "next-session")
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
			want := (workspace == "project" || workspace == "thread:project") && occupied
			if state.needsProjectThread(agent) != want {
				t.Fatalf("workspace=%s occupied=%t: wrong agent destination", workspace, occupied)
			}
			if state.needsProjectThread(state.Apps[0]) {
				t.Fatal("opening a browser must stay in its thread")
			}
		}
	}
}

func TestCloseProjectArchivesThreadAndClosesTools(t *testing.T) {
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
	apps, err := manager.CloseProject("test")
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

func TestCloseProjectPreservesPanelsOnArchiveFailure(t *testing.T) {
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
	if _, err := manager.CloseProject("test"); err == nil {
		t.Fatal("expected archive failure")
	}
	if state.Threads[0].Archived || len(state.Apps) != 1 {
		t.Fatal("archive failure changed the workspace")
	}
}

func TestAdjacentProjectThread(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active string
		want   string
	}{
		{"previous", "third", "second"},
		{"prefer previous", "second", "first"},
		{"next", "first", "second"},
		{"only thread", "other", ""},
		{"project without thread", "project", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &AppState{
				ActiveProject: tc.active,
				Threads: []Thread{
					{ID: "first", Project: "project"},
					{ID: "second", Project: "project"},
					{ID: "archived", Project: "project", Archived: true},
					{ID: "other", Project: "another"},
					{ID: "third", Project: "project"},
				},
			}
			if got := state.adjacentProjectThread(); got != tc.want {
				t.Fatalf("adjacent thread = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCloseWorkspaceAppSelectsAdjacentThread(t *testing.T) {
	oldDB, oldSM := db, sm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db, sm = oldDB, oldSM })
	createTables()
	for _, tc := range []struct {
		name, active, closeID, want string
	}{
		{"previous", "second", "agent", "first"},
		{"next", "first", "agent", "second"},
		{"tool keeps thread", "second", "browser", "second"},
		{"last thread stays in project", "other", "agent", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sm = NewStateManager()
			state := &AppState{
				ActiveProject: tc.active,
				Threads: []Thread{
					{ID: "first", Project: "project"},
					{ID: "other", Project: "another"},
					{ID: "second", Project: "project"},
				},
				Apps: []Application{
					{ID: "agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center"},
					{ID: "browser", Type: AppTypeURL, Dock: "right"},
					{ID: "issues", PluginID: "notes", Dock: "right"},
				},
				snapshots: map[string]*projectSnapshot{},
			}
			if tc.active != tc.want {
				state.snapshots[tc.want] = &projectSnapshot{Apps: []Application{{ID: "sibling-agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center"}}}
			}
			sm.states["test"] = state
			js := closeWorkspaceApp("test", tc.closeID)
			if state.ActiveProject != tc.want {
				t.Fatalf("active thread = %q, want %q", state.ActiveProject, tc.want)
			}
			if tc.closeID == "agent" && tc.active == tc.want {
				if !state.thread(tc.active).Archived || len(state.Apps) != 1 || state.Apps[0].ID != "issues" {
					t.Fatalf("last thread close changed shared panels: %+v", state.Apps)
				}
			} else if tc.closeID == "agent" {
				if !state.thread(tc.active).Archived || len(state.Apps) != 2 || state.Apps[0].ID != "sibling-agent" || state.Apps[1].ID != "issues" {
					t.Fatalf("thread switch lost sibling or shared panels: %+v", state.Apps)
				}
				if !strings.Contains(js, projectMainID(tc.want)) {
					t.Fatal("response did not show the selected thread")
				}
			} else if state.thread(tc.active).Archived {
				t.Fatal("closing a tool archived the thread")
			}
		})
	}
}

func TestReplaceThreadAgentStartsFreshAndKeepsTools(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if _, err := db.Exec("INSERT INTO threads (id, name, session_id, agent_id, archived) VALUES ('thread:test', 'Test', 'missing-session', 'pi', 1)"); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.SwitchProject(sid, "thread:test")
	manager.AddTerminalApp(sid, "old", "pi", 0, true, WidthFull, "Pi", "")
	manager.SetAppPlugin(sid, "old", "pi", "center")
	manager.AddApp(sid, "https://example.com", WidthMD, "Browser")
	state := manager.Get(sid)
	browserID := state.Apps[state.SelectedIndex].ID
	removed, err := manager.ReplaceThreadAgent(sid, "pi", "pi --model new")
	if err != nil || len(removed) != 1 || removed[0].ID != "old" {
		t.Fatalf("replacement = %v, %v", removed, err)
	}
	if len(state.Apps) != 1 || state.Apps[0].ID != browserID || !state.canStartThreadApp(removed[0]) {
		t.Fatal("replacement lost tools or blocked the new agent")
	}
	manager.saveThreadSession(sid, "thread:test", "old", "pi", "pi", "stale-session")
	thread := loadThreads()[0]
	if thread.SessionID != "" || thread.AgentCommand != "pi --model new" || thread.Archived {
		t.Fatalf("replacement retained resume metadata: %+v", thread)
	}
	manager.AddTerminalApp(sid, "new", "pi --model new", 0, true, WidthFull, "Pi", "")
	manager.saveThreadSession(sid, "thread:test", "new", "pi", "pi --model new", "new-session")
	if loadThreads()[0].SessionID != "new-session" {
		t.Fatal("new panel session was not saved")
	}
}
