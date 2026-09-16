package libro

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginDiscovery(t *testing.T) {
	dir := t.TempDir()
	for name, manifest := range map[string]string{
		"custom":    `{"id":"my-agent","name":"My agent","type":"terminal","command":"agent --interactive","dock":"center"}`,
		"invalid":   `{"id":"bad","name":"Bad","type":"script","dock":"right"}`,
		"duplicate": `{"id":"terminal","name":"Override","type":"terminal","command":"something-else","dock":"center"}`,
	} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := loadPlugins(dir)
	if len(got) != len(builtinPlugins)+1 {
		t.Fatalf("got %d plugins, wanted builtins plus one valid plugin", len(got))
	}
	custom := got[len(got)-1]
	if custom.ID != "my-agent" || custom.Command != "agent --interactive" || custom.Dock != "center" {
		t.Fatalf("custom manifest changed: %+v", custom)
	}
}

func TestAppDockDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		app  Application
		dock string
	}{
		{Application{Type: AppTypeURL}, "right"},
		{Application{Type: AppTypeTerminal, Command: "codex --resume"}, "center"},
		{Application{Type: AppTypeTerminal, Command: "bash"}, "right"},
		{Application{Type: AppTypeTerminal, Command: "nvim"}, "right"},
		{Application{Type: AppTypeTerminal, Command: "lazyrepo"}, "right"},
		{Application{Type: AppTypeTerminal, Command: "pi", Dock: "right"}, "right"},
	} {
		if got := appDock(tc.app); got != tc.dock {
			t.Errorf("appDock(%+v)=%s, want %s", tc.app, got, tc.dock)
		}
	}
}

func TestDockChangePreservesSession(t *testing.T) {
	m := NewStateManager()
	m.states["test"] = &AppState{Apps: []Application{{ID: "app-1", PluginID: "pi", TerminalID: "pty-1", TerminalReady: true}}}
	m.SetAppPlugin("test", "app-1", "pi", "right")
	a := m.Get("test").Apps[0]
	if a.Dock != "right" || a.TerminalID != "pty-1" || !a.TerminalReady {
		t.Fatalf("moving app changed its runtime: %+v", a)
	}
}

func TestMainAreaRejectsTools(t *testing.T) {
	for _, app := range []Application{
		{Type: AppTypeURL, Dock: "center"},
		{Type: AppTypeTerminal, Command: "bash", Dock: "center"},
		{Type: AppTypeTerminal, Command: "nvim", Dock: "center"},
		{Type: AppTypeTerminal, Command: "lazyrepo", Dock: "center"},
	} {
		if isAgentApp(app) || appDock(app) != "right" {
			t.Errorf("tool allowed in main area: %+v", app)
		}
	}
	for _, command := range []string{"codex", "claude", "pi"} {
		app := Application{Type: AppTypeTerminal, Command: command, Dock: "center"}
		if !isAgentApp(app) || appDock(app) != "center" {
			t.Errorf("agent rejected: %s", command)
		}
	}
}
