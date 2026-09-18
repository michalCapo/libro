package libro

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPanelWidthPersistence(t *testing.T) {
	original := db
	t.Cleanup(func() {
		if db != nil {
			db.Close()
		}
		db = original
	})
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	createTables()
	if got := DBDefaultPanelWidth(); got != WidthMD {
		t.Fatalf("initial width = %s", got)
	}
	if got := DBDefaultToolPanelWidth(); got != WidthLG {
		t.Fatalf("initial tool width = %s", got)
	}
	if err := DBSetDefaultToolPanelWidth(WidthXL); err != nil {
		t.Fatal(err)
	}
	if err := DBSetDefaultToolPanelWidth(Width("invalid")); err == nil {
		t.Fatal("accepted invalid tool width")
	}
	if err := DBSetDefaultPanelWidth(WidthSM); err != nil {
		t.Fatal(err)
	}
	if err := DBSetDefaultPanelWidth(Width("invalid")); err == nil {
		t.Fatal("accepted invalid width")
	}
	if err := setAgentCommand("codex", "codex --model custom-model"); err != nil {
		t.Fatal(err)
	}
	if err := setAgentCommand("codex", "  "); err == nil {
		t.Fatal("accepted empty command")
	}
	if err := setAgentCommand("browser", "anything"); err == nil {
		t.Fatal("accepted tool override")
	}
	if got := agentCommand(builtinPlugins[1]); got != "pi" {
		t.Fatalf("default command = %q", got)
	}
	keys := defaultToolKeybindings()
	keys["terminal"] = "Ctrl+Shift+J"
	keys["browser"] = ""
	if err := setToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	custom := Plugin{ID: "custom-test", Name: "My agent", Type: AppTypeTerminal, Dock: "center", Command: "my-agent --chat", Custom: true}
	if err := saveAgentConfig(map[string]string{"custom-test": custom.Command}, map[string]bool{"codex": true, "custom-test": true}, []Plugin{custom}, map[string]string{"codex": "My Codex", "custom-test": "My agent"}); err != nil {
		t.Fatal(err)
	}
	if err := saveAgentConfig(map[string]string{"custom-test": ""}, map[string]bool{"codex": false}, []Plugin{custom}, map[string]string{"codex": "My Codex", "custom-test": "My agent"}); err == nil {
		t.Fatal("accepted empty custom command")
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, plugin := range plugins() {
		if plugin.ID == "codex" && (!plugin.Disabled || plugin.Name != "My Codex") {
			t.Fatal("disabled state lost")
		}
		if plugin.ID == "custom-test" {
			found = plugin.Disabled && plugin.Custom && plugin.Name == "My agent" && agentCommand(plugin) == "my-agent --chat"
		}
	}
	if !found {
		t.Fatal("custom agent not persisted")
	}
	if got := toolKeybindings(); got["terminal"] != "Ctrl+Shift+J" || got["browser"] != "" {
		t.Fatalf("shortcuts not persisted: %v", got)
	}
	if got := agentCommand(builtinPlugins[0]); got != "codex --model custom-model" {
		t.Fatalf("saved command = %q", got)
	}
	if got := DBDefaultPanelWidth(); got != WidthSM {
		t.Fatalf("saved width after reopening = %s", got)
	}
	if got := DBDefaultToolPanelWidth(); got != WidthXL {
		t.Fatalf("saved tool width after reopening = %s", got)
	}
	for _, app := range []Application{
		{Type: AppTypeTerminal, PluginID: "codex"},
		{Type: AppTypeTerminal, PluginID: "custom-test"},
		{Type: AppTypeURL, PluginID: "files"},
		{Type: AppTypeURL, PluginID: "browser"},
		{Type: AppTypeTerminal, PluginID: "nvim"},
		{Type: AppTypeTerminal, PluginID: "terminal"},
	} {
		want := WidthXL
		if app.PluginID == "codex" || app.PluginID == "custom-test" {
			want = WidthSM
		}
		if got := defaultAppWidth(app); got != want {
			t.Errorf("%s width = %s, want %s", app.PluginID, got, want)
		}
	}
	if err := saveAgentConfig(nil, map[string]bool{"codex": false}, nil, nil, map[string]bool{"codex": true}); err == nil {
		t.Fatal("removed enabled agent")
	}
	if err := saveAgentConfig(nil, map[string]bool{"codex": true, "custom-test": true}, []Plugin{}, nil, map[string]bool{"codex": true, "custom-test": true}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	removedCount := 0
	for _, plugin := range plugins() {
		if (plugin.ID == "codex" || plugin.ID == "custom-test") && plugin.Removed {
			removedCount++
		}
	}
	if removedCount != 2 {
		t.Fatal("agent removals did not persist")
	}

	tools := []Plugin{{ID: "nvim", Name: "Editor", Command: "nvim -u NONE", Dock: "right", Type: AppTypeTerminal}, {ID: "custom-tool-monitor", Name: "Monitor", Command: "top", Dock: "right", Type: AppTypeTerminal, Custom: true, Disabled: true}}
	if err := saveTools(tools); err != nil {
		t.Fatal(err)
	}
	if err := saveTools([]Plugin{{ID: "codex", Name: "Bad", Command: "bad", Dock: "right", Type: AppTypeTerminal}}); err == nil {
		t.Fatal("allowed tool to replace agent")
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	foundEditor, foundCustom := false, false
	for _, p := range plugins() {
		if p.ID == "nvim" {
			foundEditor = p.Name == "Editor" && p.Command == "nvim -u NONE"
		}
		if p.ID == "custom-tool-monitor" {
			foundCustom = p.Disabled && p.Custom && p.Command == "top"
		}
	}
	if !foundEditor || !foundCustom {
		t.Fatal("tool settings did not persist")
	}

}

func TestAgentAutolaunch(t *testing.T) {
	original := db
	var err error
	path := filepath.Join(t.TempDir(), "settings.db")
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	createTables()
	state := &AppState{ActiveProject: "project"}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("default should not autolaunch")
	}
	save := func(id string, disabled map[string]bool) error {
		return saveAgentSettings(nil, disabled, nil, nil, nil, &id, nil)
	}
	for _, id := range []string{"browser", "missing"} {
		if save(id, nil) == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	if save("codex", map[string]bool{"codex": true}) == nil {
		t.Fatal("accepted disabled agent")
	}
	if err := save("codex", nil); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"codex"`) {
		t.Fatalf("missing persisted agent: %s", got)
	}
	state.Apps = []Application{{Type: AppTypeTerminal, PluginID: "pi", Dock: "center"}}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("launched with an existing agent")
	}
	state.Apps = []Application{{Type: AppTypeTerminal, PluginID: "terminal", Dock: "bottom"}}
	if projectAutolaunchJS(state, "test") == "" {
		t.Fatal("bottom shell blocked agent")
	}
	if err := save("pi", nil); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, p := range plugins() {
		if p.Autolaunch {
			count++
			if p.ID != "pi" {
				t.Fatal("old choice retained")
			}
		}
	}
	if count != 1 {
		t.Fatalf("selected %d agents", count)
	}
	if err := save("", nil); err != nil {
		t.Fatal(err)
	}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("clearing choice did not restore default")
	}
}

func TestAgentOrderPersistence(t *testing.T) {
	original := db
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	createTables()
	custom := Plugin{ID: "custom-first", Name: "First", Type: AppTypeTerminal, Dock: "center", Command: "first", Custom: true}
	order := []string{"custom-first", "claude", "codex", "pi"}
	if err := saveAgentSettings(nil, map[string]bool{"claude": true}, []Plugin{custom}, nil, nil, nil, order); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]string{{"codex", "codex"}, {"browser"}, {"missing"}} {
		if saveAgentSettings(nil, nil, nil, nil, nil, nil, invalid) == nil {
			t.Fatalf("accepted invalid order %v", invalid)
		}
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var agents, enabled []string
	for _, p := range plugins() {
		if p.Dock == "center" && p.Type == AppTypeTerminal {
			agents = append(agents, p.ID)
			if !p.Disabled && !p.Removed {
				enabled = append(enabled, p.ID)
			}
		}
	}
	if len(agents) < 5 || strings.Join(agents[:4], ",") != strings.Join(order, ",") {
		t.Fatalf("saved order lost: %v", agents)
	}
	if strings.Join(enabled[:3], ",") != "custom-first,codex,pi" {
		t.Fatalf("enabled order = %v", enabled)
	}
	if err := setAgentCommand("codex", "codex --help"); err != nil {
		t.Fatal(err)
	}
	if plugins()[0].ID != "custom-first" {
		t.Fatal("command update reset order")
	}
}

func TestDefaultThreadAgent(t *testing.T) {
	original := db
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); db = original })
	createTables()
	state := &AppState{ActiveProject: "thread:test", Threads: []Thread{{ID: "thread:test"}}}
	if defaultThreadAgent() != "" {
		t.Fatal("expected manual selection by default")
	}
	for _, id := range []string{"missing", "browser"} {
		if setDefaultThreadAgent(id) == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	projectAgent := "pi"
	if err := saveAgentSettings(nil, nil, nil, nil, nil, &projectAgent, nil); err != nil {
		t.Fatal(err)
	}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("thread used project preference")
	}
	if err := setDefaultThreadAgent("codex"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"codex"`) {
		t.Fatalf("thread preference not persisted: %s", got)
	}
	state.ActiveProject = "project"
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"pi"`) {
		t.Fatalf("project preference changed: %s", got)
	}
	state.ActiveProject = "thread:test"
	state.Apps = []Application{{Type: AppTypeTerminal, PluginID: "pi", Dock: "center"}}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("duplicate agent launched")
	}
	state.Apps = nil
	if err := saveAgentConfig(map[string]string{"codex": "codex"}, map[string]bool{"codex": true}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if setDefaultThreadAgent("codex") == nil {
		t.Fatal("accepted disabled agent")
	}
	if projectAutolaunchJS(state, "test") != "" {
		t.Fatal("launched disabled agent")
	}
	if err := setDefaultThreadAgent(""); err != nil {
		t.Fatal(err)
	}
	if defaultThreadAgent() != "" {
		t.Fatal("manual selection not saved")
	}
}
