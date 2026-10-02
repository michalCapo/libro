package libro

import (
	"slices"
	"testing"
)

func TestRemoveAppSelection(t *testing.T) {
	agent := func(id string) Application {
		return Application{ID: id, Type: AppTypeTerminal, Command: "codex"}
	}
	tool := Application{ID: "tool", Type: AppTypeTerminal, Command: "bash"}
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
		t.Run(tc.name, func(t *testing.T) {
			sm := NewStateManager()
			s := &AppState{Apps: slices.Clone(tc.apps), SelectedIndex: tc.selected}
			sm.states["test"] = s
			id := s.Apps[tc.remove].ID
			removed := sm.RemoveAppByID("test", id)
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
	apps, err := sm.CloseProject("test", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 || len(s.Apps) != 0 || s.SelectedIndex != 0 {
		t.Fatalf("close did not return and clear all panels: %+v", s)
	}
	if s.ActiveProject != "work" || len(s.Projects) != 2 {
		t.Fatal("close changed the project list or active project")
	}
	if !s.closedWorkspaces["work"] {
		t.Fatal("closed workspace still available for shortcuts")
	}
	if !sm.SwitchProject("test", "home") || len(s.Apps) != 1 || s.Apps[0].ID != "other" {
		t.Fatal("close affected another project")
	}
	if !sm.SwitchProject("test", "work") || len(s.Apps) != 0 {
		t.Fatal("closed panels restored on switching back")
	}
	if s.closedWorkspaces["work"] {
		t.Fatal("reopened workspace still closed")
	}
	if apps, err := sm.CloseProject("missing", ""); err != nil || len(apps) != 0 {
		t.Fatal("missing session returned panels")
	}
}

func TestCloseInactiveBranchPreservesActiveWorkspace(t *testing.T) {
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "repo/feature",
		Projects:      []Project{{Name: "repo"}, {Name: "repo/feature", Virtual: true}},
		Apps:          []Application{{ID: "active-agent"}, {ID: "active-tool"}},
		SelectedIndex: 1,
		snapshots: map[string]*projectSnapshot{
			"repo": {Apps: []Application{{ID: "base-agent"}, {ID: "base-tool"}}, SelectedIndex: 1},
		},
	}
	manager.states["test"] = state
	apps, err := manager.CloseProject("test", "repo")
	if err != nil || len(apps) != 2 || apps[0].ID != "base-agent" {
		t.Fatalf("wrong panels closed: %v, %v", apps, err)
	}
	if state.ActiveProject != "repo/feature" || len(state.Apps) != 2 || state.SelectedIndex != 1 {
		t.Fatal("closing another branch changed the active workspace")
	}
	if !state.closedWorkspaces["repo"] || len(state.snapshots["repo"].Apps) != 0 || state.snapshots["repo"].SelectedIndex != 0 {
		t.Fatal("inactive branch was not cleared and closed")
	}
	if !manager.SwitchProject("test", "repo") || len(state.Apps) != 0 || state.closedWorkspaces["repo"] {
		t.Fatal("closed branch did not reopen empty")
	}
	if !manager.SwitchProject("test", "repo/feature") || len(state.Apps) != 2 || state.SelectedIndex != 1 {
		t.Fatal("active workspace was not preserved")
	}
}

func TestCloseUnknownBranchPreservesWorkspace(t *testing.T) {
	manager := NewStateManager()
	state := &AppState{ActiveProject: "repo", Projects: []Project{{Name: "repo"}}, Apps: []Application{{ID: "agent"}}}
	manager.states["test"] = state
	if _, err := manager.CloseProject("test", "unknown"); err == nil {
		t.Fatal("unknown branch accepted")
	}
	if len(state.Apps) != 1 || len(state.closedWorkspaces) != 0 {
		t.Fatal("unknown branch changed workspace state")
	}
}

func TestNewThreadAgentKeepsConfiguredWidth(t *testing.T) {
	manager := NewStateManager()
	state := &AppState{ActiveProject: "thread:test", Threads: []Thread{{ID: "thread:test"}}, Apps: []Application{{ID: "tool", Type: AppTypeURL, Dock: "right", Width: WidthLG}}}
	manager.states["test"] = state
	add := func(id string) {
		manager.InsertTerminalPlaceholder("test", id, WidthSM, "codex", true, "Agent", "", -1)
		manager.SetAppPlugin("test", id, "codex", "center")
	}
	add("first")
	if state.Apps[1].Width != WidthSM || state.Apps[0].Width != WidthLG {
		t.Fatal("agent should keep its configured width without resizing tools")
	}
}

func TestNewProjectAgentKeepsConfiguredWidth(t *testing.T) {
	manager := NewStateManager()
	state := &AppState{ActiveProject: "project"}
	manager.states["test"] = state
	for _, id := range []string{"first", "second"} {
		manager.InsertTerminalPlaceholder("test", id, WidthSM, "codex", true, "Agent", "", -1)
		manager.SetAppPlugin("test", id, "codex", "center")
		if state.Apps[len(state.Apps)-1].Width != WidthSM {
			t.Fatal("new project agent should keep its configured width")
		}
		// A manually maximized project panel must also survive opening an agent.
		if id == "first" {
			manager.SetAppWidthByID("test", "first", WidthFull)
		}
	}
	if state.Apps[0].Width != WidthFull {
		t.Fatal("opening a project agent changed a manually maximized panel")
	}
}

func TestSharedProjectAppsMoveBetweenProjectThreads(t *testing.T) {
	manager := NewStateManager()
	state := &AppState{
		ActiveProject: "thread:one",
		Projects:      []Project{{Name: "project", Path: "/project"}},
		Threads: []Thread{
			{ID: "thread:one", Project: "project", Path: "/project"},
			{ID: "thread:two", Project: "project", Path: "/project"},
			{ID: "thread:other", Project: "other", Path: "/other"},
		},
		Apps: []Application{
			{ID: "agent-one", PluginID: "codex", Dock: "center"},
			{ID: "browser", PluginID: "browser", Dock: "right"},
			{ID: "notes", PluginID: "notes", Dock: "right"},
			{ID: "app", PluginID: "project-command", Dock: "bottom"},
			{ID: "shell-one", PluginID: "terminal", Dock: "bottom"},
			{ID: "shell-two", PluginID: "terminal", Dock: "bottom"},
		},
		snapshots: map[string]*projectSnapshot{
			"thread:two":   {Apps: []Application{{ID: "agent-two", PluginID: "claude", Dock: "center"}}},
			"thread:other": {Apps: []Application{{ID: "agent-other", PluginID: "pi", Dock: "center"}}},
		},
	}
	manager.states["test"] = state

	moved := manager.MoveSharedProjectApps("test", "thread:two")
	if len(moved) != 4 || len(state.Apps) != 2 || state.Apps[0].ID != "agent-one" || state.Apps[1].ID != "browser" {
		t.Fatalf("thread-local panels moved: moved=%+v source=%+v", moved, state.Apps)
	}
	if !manager.SwitchProject("test", "thread:two") || len(state.Apps) != 5 {
		t.Fatalf("shared panels missing from target: %+v", state.Apps)
	}
	if state.Apps[0].ID != "agent-two" || state.Apps[1].ID != "notes" || state.Apps[2].ID != "app" || state.Apps[3].ID != "shell-one" || state.Apps[4].ID != "shell-two" {
		t.Fatalf("unexpected target panels: %+v", state.Apps)
	}
	if moved := manager.MoveSharedProjectApps("test", "thread:other"); len(moved) != 0 {
		t.Fatalf("shared panels crossed projects: %+v", moved)
	}
	if !manager.SwitchProject("test", "thread:other") {
		t.Fatal("could not switch to other project")
	}
	moved = manager.MoveSharedProjectApps("test", "thread:one")
	if len(moved) != 4 {
		t.Fatalf("shared panels were stranded in an inactive thread: %+v", moved)
	}
}
