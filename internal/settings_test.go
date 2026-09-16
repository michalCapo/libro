package libro

import (
	"database/sql"
	"path/filepath"
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
