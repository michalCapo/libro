package libro

import (
	"database/sql"
	"encoding/json"
	"libro/internal/components"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func layoutTestDB(t *testing.T) {
	t.Helper()
	previous := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "layout.db"))
	if err != nil {
		t.Fatal(err)
	}
	createTables()
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = db.Close(); db = previous })
}

func TestLayoutRoundTrip(t *testing.T) {
	layoutTestDB(t)
	DBSaveProject("first", t.TempDir())
	DBSaveProject("second", t.TempDir())
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.SwitchProject(sid, "first")
	manager.InsertApp(sid, "https://example.com", WidthMD, "Browser", 0)
	manager.SetAppPlugin(sid, manager.Get(sid).Apps[0].ID, "browser", "right")
	manager.InsertTerminalPlaceholder(sid, "app-41", WidthLG, "bash", false, "Shell", "icon.svg", 1)
	manager.SetAppPlugin(sid, "app-41", "terminal", "bottom")
	manager.SetAppWidthByID(sid, "app-41", WidthFull)
	manager.HydrateTerminalByID(sid, "app-41", "old-pty")
	manager.SelectApp(sid, 1)
	first := append([]Application(nil), manager.Get(sid).Apps...)
	first[1].TerminalID, first[1].TerminalReady = "", false
	manager.SwitchProject(sid, "second")
	manager.InsertApp(sid, "https://second.example", WidthSM, "Second", 0)
	transientPath := t.TempDir()
	manager.AddProjectWithOptions(sid, "folder", transientPath, true)
	manager.SwitchProject(sid, "folder")
	manager.InsertApp(sid, "https://folder.example", WidthLG, "Folder", 0)
	manager.Get(sid).worktreeOrder = map[string]int{"path-b": 0, "path-a": 1}
	manager.saveLayout(sid)

	restored := NewStateManager()
	newSID := restored.NewSession()
	restored.restoreLayout(newSID)
	t.Cleanup(restored.flushLayout)
	state := restored.Get(newSID)
	if state.ActiveProject != "folder" || len(state.Apps) != 1 || state.Apps[0].Name != "Folder" {
		t.Fatalf("active workspace: %+v", state)
	}
	if snapshot := state.snapshots["first"]; snapshot == nil || snapshot.SelectedIndex != 1 || !reflect.DeepEqual(snapshot.Apps, first) {
		t.Fatalf("first workspace: %+v, want %+v", snapshot, first)
	}
	if snapshot := state.snapshots["second"]; snapshot == nil || len(snapshot.Apps) != 1 || snapshot.Apps[0].Name != "Second" {
		t.Fatalf("second workspace: %+v", snapshot)
	}
	if state.renderedProjects["first"] || !state.renderedProjects["folder"] {
		t.Fatalf("rendered: %v", state.renderedProjects)
	}
	if len(state.Projects) != 3 || !state.Projects[2].Transient || state.Projects[2].Path != transientPath {
		t.Fatalf("transient: %+v", state.Projects)
	}
	if !reflect.DeepEqual(state.worktreeOrder, map[string]int{"path-b": 0, "path-a": 1}) {
		t.Fatal("sidebar order changed")
	}
	id, err := strconv.Atoi(strings.TrimPrefix(restored.NextAppID(), "app-"))
	if err != nil || id <= 41 {
		t.Fatalf("new ID: %d, %v", id, err)
	}
	// A second window may not reuse any restored terminal IDs.
	otherSID := restored.NewSession()
	restored.restoreLayout(otherSID)
	other := restored.Get(otherSID)
	if other.ActiveProject != "" || len(other.Apps) != 0 || len(other.snapshots) != 0 {
		t.Fatal("layout restored twice")
	}
	restored.InsertApp(otherSID, "https://other.example", WidthMD, "Other", 0)
	restored.flushLayout()
	var raw string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = 'layout'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var layout savedLayout
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		t.Fatal(err)
	}
	if layout.ActiveProject != "folder" || len(layout.Open) != 4 {
		t.Fatal("second window overwrote layout on shutdown")
	}
}

func TestLayoutDropsUnavailableWorkspacesAndApplication(t *testing.T) {
	layoutTestDB(t)
	DBSaveProject("good", t.TempDir())
	missingPath := filepath.Join(t.TempDir(), "removed")
	DBSaveProject("missing-path", missingPath)
	if _, err := db.Exec(`INSERT INTO threads (id,name,archived,path) VALUES
		('thread:archived','Archived',1,''),('thread:missing-path','Missing path',0,?)`, missingPath); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	sid := manager.NewSession()
	state := manager.Get(sid)
	state.ActiveProject = "good"
	state.Apps = []Application{
		{ID: "app-50", Type: AppTypeTerminal, PluginID: "project-command", ApplicationPort: 9100, TerminalReady: true},
		{ID: "app-51", Type: AppTypeURL, URL: "https://example.com"},
		{ID: "app-52", Type: AppTypeTerminal, Command: "bash", TerminalReady: true, TerminalID: "pty"},
	}
	state.SelectedIndex = 2
	for _, name := range []string{"gone", "missing-path", "thread:archived", "thread:gone", "thread:missing-path", "closed"} {
		state.snapshots[name] = &projectSnapshot{Apps: []Application{{ID: name}}}
	}
	state.closedWorkspaces = map[string]bool{"closed": true}
	state.Projects = append(state.Projects, Project{Name: "transient-gone", Path: missingPath, Transient: true})
	manager.saveLayout(sid)
	restored := NewStateManager()
	rsid := restored.NewSession()
	restored.restoreLayout(rsid)
	t.Cleanup(restored.flushLayout)
	state = restored.Get(rsid)
	if len(state.snapshots) != 0 || state.ActiveProject != "good" || len(state.Apps) != 2 {
		t.Fatalf("restored: %+v", state)
	}
	if state.SelectedIndex != 1 || state.Apps[1].TerminalReady || state.Apps[1].TerminalID != "" || state.Apps[1].ApplicationPort != 0 {
		t.Fatalf("runtime state or selection: %+v", state.Apps)
	}
	for _, project := range state.Projects {
		if project.Name == "transient-gone" {
			t.Fatal("missing folder restored")
		}
	}
}

func TestLayoutUnknownVersionAndInvalidJSON(t *testing.T) {
	layoutTestDB(t)
	for _, raw := range []string{`{"V":2,"ActiveProject":"invalid"}`, `{broken`} {
		if _, err := db.Exec("INSERT OR REPLACE INTO settings VALUES ('layout', ?)", raw); err != nil {
			t.Fatal(err)
		}
		manager := NewStateManager()
		sid := manager.NewSession()
		manager.restoreLayout(sid)
		state := manager.Get(sid)
		if state.ActiveProject != "" || len(state.Apps) != 0 {
			t.Fatalf("invalid layout restored: %+v", state)
		}
		manager.flushLayout()
	}
}

func TestLayoutDebounceAndShutdownFlush(t *testing.T) {
	layoutTestDB(t)
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.restoreLayout(sid)
	t.Cleanup(manager.flushLayout)
	manager.InsertApp(sid, "https://one.example", WidthMD, "One", 0)
	manager.InsertApp(sid, "https://two.example", WidthLG, "Two", 1)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var raw string
		if db.QueryRow("SELECT value FROM settings WHERE key = 'layout'").Scan(&raw) == nil {
			var layout savedLayout
			if json.Unmarshal([]byte(raw), &layout) != nil || len(layout.Open) != 1 || len(layout.Open[0].Apps) != 2 {
				t.Fatalf("debounced layout: %s", raw)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("layout was not saved")
		}
		time.Sleep(20 * time.Millisecond)
	}
	manager.RemoveAppByID(sid, manager.Get(sid).Apps[0].ID)
	manager.flushLayout()
	var raw string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = 'layout'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var layout savedLayout
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		t.Fatal(err)
	}
	if len(layout.Open[0].Apps) != 1 || layout.Open[0].Apps[0].Name != "Two" {
		t.Fatalf("shutdown did not flush: %+v", layout)
	}
}

func TestProjectAgentSessionRestoresWithoutThreadRow(t *testing.T) {
	layoutTestDB(t)
	oldSM := sm
	t.Cleanup(func() { sm = oldSM })
	DBSaveProject("project", t.TempDir())
	sm = NewStateManager()
	sid := sm.NewSession()
	sm.SwitchProject(sid, "project")
	sm.InsertTerminalPlaceholder(sid, "app-9", WidthFull, "codex", true, "Codex", "", 0)
	sm.SetAppPlugin(sid, "app-9", "codex", "center")
	_, report := agentLaunch(sid, sm.Get(sid).Apps[0])
	report("saved-session")
	sm.saveLayout(sid)
	sm = NewStateManager()
	sid = sm.NewSession()
	sm.restoreLayout(sid)
	t.Cleanup(sm.flushLayout)
	panel := sm.Get(sid).Apps[0]
	if panel.SessionID != "saved-session" {
		t.Fatalf("session lost: %+v", panel)
	}
	command, report := agentLaunch(sid, panel)
	if command == "codex" || report == nil {
		t.Fatal("project agent was not resumed")
	}
	// A report arriving while the workspace is hidden still updates its panel.
	sm.AddProjectWithOptions(sid, "other", t.TempDir(), true)
	sm.SwitchProject(sid, "other")
	report("next-session")
	if sm.Get(sid).snapshots["project"].Apps[0].SessionID != "next-session" {
		t.Fatal("hidden workspace report lost")
	}
	app, workspace, path, found := sm.workspaceApp(sid, panel.ID)
	if !found || workspace != "project" || path == defaultHomeDir() || app.TerminalReady {
		t.Fatal("hydration used active workspace")
	}
}

func TestLayoutRestoresWorktreeWorkspace(t *testing.T) {
	layoutTestDB(t)
	_, root, worktree := finishFixture(t)
	DBSaveProject("project", root)
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.AddVirtualProject(sid, "project/feature", worktree, "project")
	manager.SwitchProject(sid, "project/feature")
	manager.InsertApp(sid, "https://example.com", WidthMD, "Browser", 0)
	manager.saveLayout(sid)
	restored := NewStateManager()
	rsid := restored.NewSession()
	restored.restoreLayout(rsid)
	t.Cleanup(restored.flushLayout)
	state := restored.Get(rsid)
	if state.ActiveProject != "project/feature" || len(state.Apps) != 1 || restored.GetActiveProjectPath(rsid) != worktree {
		t.Fatalf("worktree not restored: %+v", state)
	}
}

func TestResumeReadsChangedModelAndIgnoresLateReports(t *testing.T) {
	layoutTestDB(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	if _, err := db.Exec("INSERT INTO threads (id,name) VALUES ('thread:test','Test')"); err != nil {
		t.Fatal(err)
	}
	previous := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = previous })
	sid := sm.NewSession()
	sm.restoreLayout(sid)
	t.Cleanup(sm.flushLayout)
	sm.SwitchProject(sid, "thread:test")
	sm.InsertTerminalPlaceholder(sid, "app-2", WidthFull, "codex", true, "Codex", "", 0)
	sm.SetAppPlugin(sid, "app-2", "codex", "center")
	sm.saveThreadSession(sid, "thread:test", "app-2", "codex", "codex", "saved-session")
	// The model changes inside the agent after its session was reported.
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "rollout-date-saved-session.jsonl"), []byte(`{"type":"turn_context","payload":{"model":"changed-model","effort":"high"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	// A crash skips shutdown, so the resume reads the session file itself.
	command, _ := agentLaunch(sid, sm.Get(sid).Apps[0])
	if command != components.ResumeAgentCommand("codex", "saved-session", "changed-model", "high") {
		t.Fatalf("resume command: %s", command)
	}
	sm.flushLayout()
	sm.saveThreadSession(sid, "thread:test", "app-2", "codex", "codex", "late-session")
	if thread := loadThreads()[0]; thread.SessionID != "saved-session" {
		t.Fatalf("late report saved: %+v", thread)
	}
}

func TestRestoredTerminalsHydrateInTheirOwnWorkspace(t *testing.T) {
	layoutTestDB(t)
	previousSM, previousTM := sm, tm
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); sm.flushLayout(); sm, tm = previousSM, previousTM })
	first, second := t.TempDir(), t.TempDir()
	DBSaveProject("first", first)
	DBSaveProject("second", second)
	sid := sm.NewSession()
	sm.SwitchProject(sid, "first")
	sm.InsertTerminalPlaceholder(sid, "app-4", WidthFull, "pwd > started-cwd; sleep 30", true, "Shell", "", 0)
	sm.SwitchProject(sid, "second")
	sm.InsertTerminalPlaceholder(sid, "app-5", WidthFull, "pwd > started-cwd; sleep 30", true, "Shell", "", 0)
	sm.saveLayout(sid)
	sm = NewStateManager()
	sid = sm.NewSession()
	sm.restoreLayout(sid)
	if tm.IsRunning("app-4") || tm.IsRunning("app-5") {
		t.Fatal("restore started a terminal eagerly")
	}
	// Initial render hydrates the active panel through the same action as a new panel.
	hydrateApp(sid, "app-5", false)
	if !tm.IsRunning("app-5") || tm.IsRunning("app-4") {
		t.Fatal("hidden terminal started before showing workspace")
	}
	sm.SwitchProject(sid, "first")
	hydrateApp(sid, "app-4", false)
	// A delayed duplicate from a hidden workspace uses its owner, not the active path.
	hydrateApp(sid, "app-5", false)
	deadline := time.Now().Add(3 * time.Second)
	for {
		a, ea := os.ReadFile(filepath.Join(first, "started-cwd"))
		b, eb := os.ReadFile(filepath.Join(second, "started-cwd"))
		if ea == nil && eb == nil {
			if strings.TrimSpace(string(a)) != first || strings.TrimSpace(string(b)) != second {
				t.Fatal("terminals started in the wrong workspace")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminals did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sm.Get(sid).Apps[0].TerminalReady || !sm.Get(sid).snapshots["second"].Apps[0].TerminalReady {
		t.Fatal("terminal state was not hydrated")
	}
}

func TestMovedAgentStillReportsItsSession(t *testing.T) {
	layoutTestDB(t)
	previous := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = previous })
	DBSaveProject("first", t.TempDir())
	DBSaveProject("second", t.TempDir())
	sid := sm.NewSession()
	sm.SwitchProject(sid, "first")
	sm.InsertTerminalPlaceholder(sid, "app-2", WidthFull, "codex", true, "Codex", "", 0)
	sm.SetAppPlugin(sid, "app-2", "codex", "center")
	_, report := agentLaunch(sid, sm.Get(sid).Apps[0])
	if _, ok := sm.MoveSelectedAppToProject(sid, "second"); !ok {
		t.Fatal("move failed")
	}
	report("moved-session")
	if sm.Get(sid).Apps[0].SessionID != "moved-session" {
		t.Fatal("moved panel lost its session report")
	}
	report("")
	if sm.Get(sid).Apps[0].SessionID != "moved-session" {
		t.Fatal("temporary empty report erased confirmed session")
	}
}
