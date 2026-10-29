package libro

import (
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
)

func TestHomeProjectIsOptional(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := db.Exec("INSERT INTO projects (name, path, position) VALUES ('home', ?, 0)", home); err != nil {
		t.Fatal(err)
	}
	removeDefaultHomeProject()
	state := newAppStateFromDB()
	if len(state.Projects) != 0 || state.ActiveProject != "" {
		t.Fatalf("expected empty state: %+v", state)
	}
	DBSaveProject("home", home)
	removeDefaultHomeProject()
	if projects := DBLoadProjects(); len(projects) != 1 || projects[0].Name != "home" {
		t.Fatal("manually added home project is missing")
	}
	DBRemoveProject("home")
	if len(DBLoadProjects()) != 0 {
		t.Fatal("removed home project returned")
	}
}

func TestRemoveActiveProject(t *testing.T) {
	sm := NewStateManager()
	state := &AppState{
		Projects:      []Project{{Name: "home"}, {Name: "work"}},
		ActiveProject: "home",
		Apps:          []Application{{ID: "removed"}},
		snapshots:     map[string]*projectSnapshot{"work": {Apps: []Application{{ID: "kept"}, {ID: "selected"}}, SelectedIndex: 1}},
	}
	sm.states["test"] = state
	apps, ok := sm.RemoveProject("test", "home")
	if !ok || len(apps) != 1 || apps[0].ID != "removed" {
		t.Fatal("home project apps were not returned for cleanup")
	}
	if state.ActiveProject != "work" || len(state.Apps) != 2 || state.SelectedIndex != 1 {
		t.Fatalf("remaining project was not restored: %+v", state)
	}
	apps, ok = sm.RemoveProject("test", "work")
	if !ok || len(apps) != 2 || len(state.Projects) != 0 || len(state.Apps) != 0 || state.ActiveProject != "" || state.SelectedIndex != 0 {
		t.Fatalf("last project removal failed: %+v", state)
	}
	if !sm.AddProjectWithOptions("test", "home", t.TempDir(), false) || !sm.SwitchProject("test", "home") {
		t.Fatal("cannot add home after removing last project")
	}
}

func TestProjectBaseOpensOnNavigation(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	DBSaveProject("first", t.TempDir())
	DBSaveProject("second", t.TempDir())
	manager := NewStateManager()
	sid := manager.NewSession()
	state := manager.Get(sid)
	if state.ActiveProject != "" || slices.ContainsFunc(projectItems(state), func(p jsProject) bool { return p.BaseOpened }) {
		t.Fatal("projects opened before navigation")
	}
	for i, name := range []string{"second", "first", "second"} {
		if !manager.SwitchProject(sid, name) {
			t.Fatal("cannot open project")
		}
		want := min(i+1, 2)
		if got := len(slices.DeleteFunc(projectItems(state), func(p jsProject) bool { return !p.BaseOpened })); got != want {
			t.Fatalf("opened bases = %d, want %d", got, want)
		}
	}
	if _, err := manager.CloseProject(sid, ""); err != nil {
		t.Fatal(err)
	}
	if got := len(slices.DeleteFunc(projectItems(state), func(p jsProject) bool { return !p.BaseOpened })); got != 1 {
		t.Fatalf("closed base still numbered: %d open bases", got)
	}
	if !manager.SwitchProject(sid, "second") {
		t.Fatal("cannot reopen active base")
	}
	if got := len(slices.DeleteFunc(projectItems(state), func(p jsProject) bool { return !p.BaseOpened })); got != 2 {
		t.Fatalf("reopened base missing: %d open bases", got)
	}

}

func TestReorderProjects(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	for _, name := range []string{"first", "second", "third"} {
		DBSaveProject(name, t.TempDir())
	}
	manager := NewStateManager()
	sid := manager.NewSession()
	manager.Get(sid).Projects = append(manager.Get(sid).Projects, Project{Name: "folder", Transient: true})
	DBReorderProjects(manager.ReorderProjects(sid, []string{"third", "folder", "first"}))
	names := func(projects []Project) []string {
		var out []string
		for _, p := range projects {
			out = append(out, p.Name)
		}
		return out
	}
	if got := names(manager.Get(sid).Projects); !slices.Equal(got, []string{"third", "folder", "first", "second"}) {
		t.Fatalf("session order = %v", got)
	}
	if got := names(DBLoadProjects()); !slices.Equal(got, []string{"third", "first", "second"}) {
		t.Fatalf("saved order = %v", got)
	}
}

func TestRenameProjectKeepsPath(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	path := filepath.Join(t.TempDir(), "lukas")
	DBSaveProject("lukas", path)
	DBRenameProject("lukas", "Lukas shop")
	DBSaveProject("lukas", path)
	projects := DBLoadProjects()
	if len(projects) != 1 || projects[0].Name != "lukas" || projects[0].Path != path || projects[0].displayName() != "Lukas shop" {
		t.Fatalf("renamed project = %+v", projects)
	}
	DBRenameProject("lukas", "")
	if got := DBLoadProjects()[0].displayName(); got != "lukas" {
		t.Fatalf("blank name shows %q, want folder name", got)
	}
}
