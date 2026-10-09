package libro

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"libro/internal/components"
)

func TestBrowserURLsIncludeAllApplicationTerminals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX test commands")
	}
	oldDB, oldTM := db, tm
	db, tm = nil, components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); db, tm = oldDB, oldTM })
	root, other := t.TempDir(), t.TempDir()
	state := &AppState{
		ActiveProject: "thread:one",
		Projects:      []Project{{Name: "project", Path: root}, {Name: "other", Path: other}},
		Threads:       []Thread{{ID: "thread:one", Project: "project", Path: root}, {ID: "thread:two", Project: "project", Path: root}},
		Apps: []Application{
			{ID: "custom", Type: AppTypeTerminal, PluginID: "custom-app", Dock: "right", TerminalReady: true},
			{ID: "agent", Type: AppTypeTerminal, PluginID: "codex", Dock: "center", TerminalReady: true},
			{ID: "exited", Type: AppTypeTerminal, PluginID: "terminal", TerminalReady: true},
		},
		snapshots: map[string]*projectSnapshot{
			"thread:two": {Apps: []Application{
				{ID: "shared", Type: AppTypeTerminal, PluginID: "terminal", Dock: "bottom", TerminalReady: true},
				{ID: "sibling", Type: AppTypeTerminal, PluginID: "custom-app", Dock: "right", TerminalReady: true},
			}},
			"other": {Apps: []Application{{ID: "other", Type: AppTypeTerminal, PluginID: "terminal", Dock: "bottom", TerminalReady: true}}},
		},
	}
	for _, test := range []struct{ id, command string }{
		{"custom", "printf 'http://localhost:3000/app/\\nhttp://localhost:3001/admin/\\n'"},
		{"agent", "printf 'http://localhost:4000/\\n'"},
		{"shared", "printf 'http://localhost:5000/\\nhttp://localhost:3001/admin/\\n'"},
		{"sibling", "printf 'http://localhost:6000/\\n'"},
		{"other", "printf 'http://localhost:7000/\\n'"},
		{"exited", "printf 'http://localhost:8000/\\n'; exit 0"},
	} {
		_, err := tm.StartWithEnvironment(test.id, test.command, root, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for len(tm.LocalURLs(test.id)) == 0 || test.id == "exited" && tm.IsRunning(test.id) {
			if time.Now().After(deadline) {
				t.Fatalf("terminal %s did not print URLs", test.id)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	want := []string{"http://localhost:3000/app/", "http://localhost:3001/admin/", "http://localhost:4000/", "http://localhost:5000/"}
	if got := applicationBrowserURLs(state); !reflect.DeepEqual(got, want) {
		t.Fatalf("browser URLs = %v, want %v", got, want)
	}
	tm.Stop("custom")
	want = []string{"http://localhost:4000/", "http://localhost:5000/", "http://localhost:3001/admin/"}
	if got := applicationBrowserURLs(state); !reflect.DeepEqual(got, want) {
		t.Fatalf("stopped application still suggested: %v, want %v", got, want)
	}
}
