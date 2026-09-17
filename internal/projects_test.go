package libro

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestHomeProjectIsOptional(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
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
	if project, ok := DBFindProjectByPath(filepath.Join(home, "child")); !ok || project.Name != "home" {
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
	if !sm.AddProject("test", "home", t.TempDir()) || !sm.SwitchProject("test", "home") {
		t.Fatal("cannot add home after removing last project")
	}
}
