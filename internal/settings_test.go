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
	if got := DBDefaultPanelWidth(); got != WidthSM {
		t.Fatalf("fixed width = %s", got)
	}
	if err := DBSetDefaultPanelWidth(WidthFull); err != nil {
		t.Fatal(err)
	}
	if err := DBSetDefaultPanelWidth(Width("invalid")); err == nil {
		t.Fatal("accepted invalid width")
	}
	if err := saveAgentSettings(map[string]string{"codex": "codex --model custom-model"}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := saveAgentSettings(map[string]string{"codex": "  "}, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("accepted empty command")
	}
	if err := saveAgentSettings(map[string]string{"browser": "anything"}, nil, nil, nil, nil, nil); err == nil {
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
	if err := saveAgentSettings(map[string]string{"custom-test": custom.Command}, map[string]bool{"codex": true, "custom-test": true}, []Plugin{custom}, map[string]string{"codex": "My Codex", "custom-test": "My agent"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := saveAgentSettings(map[string]string{"custom-test": ""}, map[string]bool{"codex": false}, []Plugin{custom}, map[string]string{"codex": "My Codex", "custom-test": "My agent"}, nil, nil); err == nil {
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
	if got := DBDefaultPanelWidth(); got != WidthFull {
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
			want = WidthFull
		}
		if got := defaultAppWidth(app); got != want {
			t.Errorf("%s width = %s, want %s", app.PluginID, got, want)
		}
	}
	if err := saveAgentSettings(nil, map[string]bool{"codex": false}, nil, nil, map[string]bool{"codex": true}, nil); err == nil {
		t.Fatal("removed enabled agent")
	}
	if err := saveAgentSettings(nil, map[string]bool{"codex": true, "custom-test": true}, []Plugin{}, nil, map[string]bool{"codex": true, "custom-test": true}, nil); err != nil {
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
	if err := saveToolsWithEditor(tools, "", false, keys); err != nil {
		t.Fatal(err)
	}
	if got := toolKeybindings(); got["custom-tool-docs"] != "Ctrl+Alt+D" || got["nvim"] != keys["nvim"] {
		t.Fatal("tool shortcuts did not persist")
	}
	keys["custom-tool-docs"] = keys["terminal"]
	tools[0].Name = "Should not save"
	if err := saveToolsWithEditor(tools, "", false, keys); err == nil {
		t.Fatal("accepted duplicate tool shortcut")
	}
	for _, p := range plugins() {
		if p.ID == "nvim" && p.Name == "Should not save" {
			t.Fatal("saved tool despite invalid shortcut")
		}
	}
	if err := saveToolsWithEditor([]Plugin{{ID: "codex", Name: "Bad", Command: "bad", Dock: "right", Type: AppTypeTerminal}}, "", false); err == nil {
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
	if err := saveAgentSettings(nil, map[string]bool{"claude": true}, []Plugin{custom}, nil, nil, order); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]string{{"codex", "codex"}, {"browser"}, {"missing"}} {
		if saveAgentSettings(nil, nil, nil, nil, nil, invalid) == nil {
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
	if err := saveAgentSettings(map[string]string{"codex": "codex --help"}, nil, nil, nil, nil, nil); err != nil {
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
	if got := projectAutolaunchPlugin(state); got == nil || got.ID != "codex" {
		t.Fatalf("thread preference not persisted: %+v", got)
	}
	state.ActiveProject = "project"
	if got := projectAutolaunchPlugin(state); got != nil {
		t.Fatalf("base workspace must start empty: %+v", got)
	}
	state.ActiveProject = "thread:test"
	state.Apps = []Application{{Type: AppTypeTerminal, PluginID: "pi", Dock: "center"}}
	if projectAutolaunchPlugin(state) != nil {
		t.Fatal("duplicate agent launched")
	}
	state.Apps = nil
	if err := saveAgentSettings(map[string]string{"codex": "codex"}, map[string]bool{"codex": true}, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if setDefaultThreadAgent("codex") == nil {
		t.Fatal("accepted disabled agent")
	}
	if projectAutolaunchPlugin(state) != nil {
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
		err := saveToolsWithEditor([]Plugin{{ID: "custom-tool-website", Name: "Website", Type: AppTypeURL, Dock: "right", URL: address}}, "", false)
		if err == nil || !strings.Contains(err.Error(), "valid website URL") {
			t.Errorf("URL %q: got %v", address, err)
		}
	}
	for _, id := range []string{"browser", "files", "terminal", "codex"} {
		if err := saveToolsWithEditor([]Plugin{{ID: id, Name: "Website", Type: AppTypeURL, Dock: "right", URL: "https://example.com"}}, "", false); err == nil {
			t.Errorf("allowed replacing %s", id)
		}
	}
}

func TestEditorToolPersistenceAndValidation(t *testing.T) {
	original := db
	t.Cleanup(func() { _ = db.Close(); db = original })
	var err error
	path := filepath.Join(t.TempDir(), "settings.db")
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	createTables()
	if editorToolID() != "nvim" {
		t.Fatal("expected Nvim by default")
	}
	tools := []Plugin{
		{ID: "custom-tool-editor", Name: "Editor", Command: "nvim -u NONE", Dock: "right", Type: AppTypeTerminal, Custom: true},
		{ID: "custom-tool-site", Name: "Website", URL: "https://example.com", Dock: "right", Type: AppTypeURL, Custom: true},
	}
	if err := saveToolsWithEditor(tools, tools[0].ID, true); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if editorToolID() != tools[0].ID {
		t.Fatal("editor did not persist")
	}
	for _, id := range []string{"missing", tools[1].ID} {
		if err := saveToolsWithEditor(tools, id, true); err == nil {
			t.Fatalf("accepted %s as editor", id)
		}
	}
	tools[0].Disabled = true
	if err := saveToolsWithEditor(tools, tools[0].ID, true); err == nil {
		t.Fatal("accepted disabled editor")
	}
	if editorToolID() != tools[0].ID {
		t.Fatal("invalid save changed editor")
	}
	if err := saveToolsWithEditor(tools, "", true); err != nil {
		t.Fatal(err)
	}
	if editorToolID() != "" {
		t.Fatal("editor did not turn off")
	}
}
