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
			_ = db.Close()
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
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
	tools = append(tools, Plugin{ID: "custom-tool-docs", Name: "Docs", URL: " https://example.com/docs?q=go ", Dock: "right", Type: AppTypeURL, Custom: true})
	keys = toolKeybindings()
	keys["custom-tool-monitor"] = "Ctrl+Alt+M"
	keys["custom-tool-docs"] = "Ctrl+Alt+D"
	if err := saveTools(tools, keys); err != nil {
		t.Fatal(err)
	}
	if got := toolKeybindings(); got["custom-tool-docs"] != "Ctrl+Alt+D" || got["nvim"] != keys["nvim"] {
		t.Fatal("tool shortcuts did not persist")
	}
	keys["custom-tool-docs"] = keys["terminal"]
	tools[0].Name = "Should not save"
	if err := saveTools(tools, keys); err == nil {
		t.Fatal("accepted duplicate tool shortcut")
	}
	for _, p := range plugins() {
		if p.ID == "nvim" && p.Name == "Should not save" {
			t.Fatal("saved tool despite invalid shortcut")
		}
	}
	if err := saveTools([]Plugin{{ID: "codex", Name: "Bad", Command: "bad", Dock: "right", Type: AppTypeTerminal}}); err == nil {
		t.Fatal("allowed tool to replace agent")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	foundEditor, foundCustom, foundWebsite := false, false, false
	for _, p := range plugins() {
		if p.ID == "custom-tool-docs" {
			foundWebsite = p.Type == AppTypeURL && p.URL == "https://example.com/docs?q=go" && p.Command == "" && configurableTool(p)
		}
		if p.ID == "nvim" {
			foundEditor = p.Name == "Editor" && p.Command == "nvim -u NONE"
		}
		if p.ID == "custom-tool-monitor" {
			foundCustom = p.Disabled && p.Custom && p.Command == "top"
		}
	}
	if !foundEditor || !foundCustom || !foundWebsite {
		t.Fatal("tool settings did not persist")
	}

}

func TestBrowserPageToolsAutoExecutePersistence(t *testing.T) {
	original := db
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if browserPageToolsAutoExecute() {
		t.Fatal("page tools should be disabled by default")
	}
	if err := setBrowserPageToolsAutoExecute(true); err != nil {
		t.Fatal(err)
	}
	if !browserPageToolsAutoExecute() {
		t.Fatal("enabled page tools setting was not read")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if !browserPageToolsAutoExecute() {
		t.Fatal("page tools setting did not persist")
	}
	if err := setBrowserPageToolsAutoExecute(false); err != nil {
		t.Fatal(err)
	}
	if browserPageToolsAutoExecute() {
		t.Fatal("page tools setting did not disable")
	}
}

func TestAgentEnvironmentPersistence(t *testing.T) {
	original := db
	path := filepath.Join(t.TempDir(), "settings.db")
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()

	if err := setAgentEnvironment([]environmentInput{
		{Name: "OPENROUTER_API_KEY", Value: "secret"},
		{Name: "MODEL_NAME", Value: "openrouter/auto"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := agentEnvironment(); got["OPENROUTER_API_KEY"] != "secret" || got["MODEL_NAME"] != "openrouter/auto" {
		t.Fatalf("environment not saved: %v", got)
	}

	// An empty value for an unchanged saved row preserves the hidden value.
	if err := setAgentEnvironment([]environmentInput{{Name: "OPENROUTER_API_KEY", OriginalName: "OPENROUTER_API_KEY"}}); err != nil {
		t.Fatal(err)
	}
	if got := agentEnvironment(); len(got) != 1 || got["OPENROUTER_API_KEY"] != "secret" {
		t.Fatalf("hidden value was not preserved: %v", got)
	}

	for _, entries := range [][]environmentInput{
		{{Name: "BAD-NAME", Value: "value"}},
		{{Name: "DUP", Value: "one"}, {Name: "DUP", Value: "two"}},
		{{Name: "NEW"}},
	} {
		if err := setAgentEnvironment(entries); err == nil {
			t.Fatalf("accepted invalid environment: %+v", entries)
		}
	}
}

func TestSaveNewAgentWithQuotedCommand(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	custom := Plugin{
		ID: "custom-12345678-1234-4234-8234-123456789abc", Name: "luna",
		Command: `codex -m gpt-5.6-luna -c 'model_reasoning_effort="xhigh"'`,
		Type:    AppTypeTerminal, Dock: "center", Custom: true,
	}
	autolaunch := ""
	if err := saveAgentSettings(
		map[string]string{custom.ID: custom.Command}, map[string]bool{custom.ID: false},
		[]Plugin{custom}, map[string]string{custom.ID: custom.Name}, map[string]bool{},
		&autolaunch, []string{custom.ID},
	); err != nil {
		t.Fatal(err)
	}
	for _, plugin := range plugins() {
		if plugin.ID == custom.ID {
			if plugin.Name != "luna" || plugin.Disabled || !plugin.Custom || agentCommand(plugin) != custom.Command {
				t.Fatalf("new agent changed: %+v", plugin)
			}
			return
		}
	}
	t.Fatal("new agent was not saved")
}

func TestAgentAutolaunch(t *testing.T) {
	original := db
	var err error
	path := filepath.Join(t.TempDir(), "settings.db")
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"codex"`) || !strings.Contains(got, "'app.start'") {
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
	t.Cleanup(func() { _ = db.Close(); db = original })
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
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
	t.Cleanup(func() { _ = db.Close(); db = original })
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"codex"`) {
		t.Fatalf("thread preference not persisted: %s", got)
	}
	state.ActiveProject = "project"
	if got := projectAutolaunchJS(state, "test"); !strings.Contains(got, `"plugin":"pi"`) || !strings.Contains(got, "'app.start'") {
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

func TestWebsiteToolValidation(t *testing.T) {
	for _, address := range []string{"", "example.com", "https://", "javascript:alert(1)", "file:///tmp/test", "https://bad host"} {
		err := saveTools([]Plugin{{ID: "custom-tool-website", Name: "Website", Type: AppTypeURL, Dock: "right", URL: address}})
		if err == nil || !strings.Contains(err.Error(), "valid website URL") {
			t.Errorf("URL %q: got %v", address, err)
		}
	}
	for _, id := range []string{"browser", "files", "terminal", "codex"} {
		if err := saveTools([]Plugin{{ID: id, Name: "Website", Type: AppTypeURL, Dock: "right", URL: "https://example.com"}}); err == nil {
			t.Errorf("allowed replacing %s", id)
		}
	}
}

func TestReplaceAgentShortcutMigration(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('tool_keybindings', '{"new-agent":"Ctrl+N","new-thread":"Ctrl+Shift+N","terminal":"Ctrl+Alt+T"}')`); err != nil {
		t.Fatal(err)
	}
	keys := toolKeybindings()
	if keys["new-agent"] != "Ctrl+N" || keys["replace-agent"] != "Ctrl+Shift+N" || keys["new-thread"] != "" || keys["terminal"] != "Ctrl+Alt+T" {
		t.Fatalf("incorrect shortcut migration: %v", keys)
	}
	if err := validateToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	keys["replace-agent"] = "Ctrl+Alt+N"
	if err := setToolKeybindings(keys); err != nil {
		t.Fatal(err)
	}
	if toolKeybindings()["replace-agent"] != "Ctrl+Alt+N" {
		t.Fatal("migration overwrote a customized replacement shortcut")
	}
}
