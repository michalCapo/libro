package libro

import "testing"

func TestBrowserScopeFollowsThreadOwner(t *testing.T) {
	original := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = original })
	sm.states["test"] = &AppState{
		ActiveProject: "first",
		Threads:       []Thread{{ID: "first", Project: "same"}, {ID: "second", Project: "same"}},
		Apps:          []Application{{ID: "browser"}, {ID: "agent"}},
		snapshots:     map[string]*projectSnapshot{"second": {Apps: []Application{{ID: "other"}}}},
	}
	first := browserScope("test", "browser")
	if first != browserScope("test", "agent") {
		t.Fatal("agent and browser must share their thread scope")
	}
	if first == browserScope("test", "other") {
		t.Fatal("threads in the same project must have separate profiles")
	}
	state := sm.states["test"]
	state.snapshots["first"] = &projectSnapshot{Apps: state.Apps}
	state.Apps = state.snapshots["second"].Apps
	state.ActiveProject = "second"
	if first != browserScope("test", "browser") {
		t.Fatal("scope changed when switching threads")
	}
	if first == browserScope("test", "missing") {
		t.Fatal("unknown panel inherited active scope")
	}
}
