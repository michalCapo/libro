package libro

import "testing"

func TestRemoveAppSelection(t *testing.T) {
	agent := func(id string) Application {
		return Application{ID: id, Type: AppTypeTerminal, Command: "codex"}
	}
	tool := Application{ID: "tool", Type: AppTypeTerminal, Command: "bash"}
	for _, byID := range []bool{false, true} {
		for _, tc := range []struct {
			name             string
			apps             []Application
			selected, remove int
			want             string
		}{
			{"left agent", []Application{agent("a"), agent("b"), tool}, 1, 1, "a"},
			{"skip tool on left", []Application{agent("a"), tool, agent("b")}, 2, 2, "a"},
			{"right agent", []Application{agent("a"), tool, agent("b")}, 0, 0, "b"},
			{"prefer left", []Application{agent("a"), agent("b"), agent("c"), tool}, 1, 1, "a"},
			{"last agent", []Application{agent("a"), tool}, 0, 0, "tool"},
			{"last panel", []Application{agent("a")}, 0, 0, ""},
			{"keep selected agent", []Application{agent("a"), agent("b"), tool}, 1, 0, "b"},
			{"keep selected tool", []Application{agent("a"), tool}, 1, 0, "tool"},
			{"close tool", []Application{agent("a"), tool}, 1, 1, "a"},
		} {
			t.Run(tc.name+map[bool]string{false: "/index", true: "/id"}[byID], func(t *testing.T) {
				sm := NewStateManager()
				s := &AppState{Apps: cloneApplications(tc.apps), SelectedIndex: tc.selected}
				sm.states["test"] = s
				id := s.Apps[tc.remove].ID
				var removed *Application
				if byID {
					removed = sm.RemoveAppByID("test", id)
				} else {
					removed = sm.RemoveApp("test", tc.remove)
				}
				if removed == nil || removed.ID != id {
					t.Fatalf("removed = %v, want %s", removed, id)
				}
				if tc.want == "" {
					if len(s.Apps) != 0 || s.SelectedIndex != 0 {
						t.Fatal("invalid empty state")
					}
				} else if got := s.Apps[s.SelectedIndex].ID; got != tc.want {
					t.Fatalf("selected = %s, want %s", got, tc.want)
				}
			})
		}
	}
}

func TestCloseProjectOnlyClearsActiveProject(t *testing.T) {
	sm := NewStateManager()
	s := &AppState{
		ActiveProject: "work",
		Projects:      []Project{{Name: "home"}, {Name: "work"}},
		Apps:          []Application{{ID: "terminal", Type: AppTypeTerminal}, {ID: "browser", Type: AppTypeURL}},
		SelectedIndex: 1,
		snapshots: map[string]*projectSnapshot{
			"home": {Apps: []Application{{ID: "other"}}},
		},
	}
	sm.states["test"] = s
	apps := sm.CloseProject("test")
	if len(apps) != 2 || len(s.Apps) != 0 || s.SelectedIndex != 0 {
		t.Fatalf("close did not return and clear all panels: %+v", s)
	}
	if s.ActiveProject != "work" || len(s.Projects) != 2 {
		t.Fatal("close changed the project list or active project")
	}
	if !sm.SwitchProject("test", "home") || len(s.Apps) != 1 || s.Apps[0].ID != "other" {
		t.Fatal("close affected another project")
	}
	if !sm.SwitchProject("test", "work") || len(s.Apps) != 0 {
		t.Fatal("closed panels restored on switching back")
	}
	if len(sm.CloseProject("missing")) != 0 {
		t.Fatal("missing session returned panels")
	}
}
