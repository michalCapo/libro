package libro

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

var toolKeys = []struct{ ID, Name, Key string }{
	{"terminal", "Terminal", "Ctrl+T"},
	{"browser", "Browser", "Ctrl+B"},
	{"new-browser", "New browser panel", "Ctrl+Shift+B"},
	{"previous-browser", "Previous browser panel", "Ctrl+["},
	{"next-browser", "Next browser panel", "Ctrl+]"},
	{"files", "Files", "Ctrl+F"},
	{"notes", "Issues", "Ctrl+I"},
	{"nvim", "Nvim", "Ctrl+E"},
	{"lazyrepo", "Git", "Ctrl+G"},
	{"lazydata", "Database", "Ctrl+D"},
	{"close-panel", "Close panel", "Ctrl+Q"},
	{"close-project", "Close project", "Ctrl+Shift+Q"},
	{"run-project", "Start / restart project", "Ctrl+Shift+R"},
	{"stop-project", "Stop project command", "Ctrl+Shift+T"},
	{"panel-size-down", "Decrease panel size", "Ctrl+."},
	{"panel-size-up", "Increase panel size", "Ctrl+,"},
	{"panel-size-max", "Toggle panel size to MAX", "Ctrl+M"},
	{"settings", "Settings", "Ctrl+Shift+S"},
	{"voice", "Voice typing (toggle)", "CapsLock"},
	{"project-picker", "Switch project", "Ctrl+Shift+P"},
	{"new-agent", "New agent", ""},
	{"new-thread", "New thread", ""},
	{"finish-thread", "Finish current thread…", ""},
	{"thread-actions", "Thread actions", "Ctrl+;"},
	{"replace-agent", "Replace agent", "Ctrl+Shift+A"},
	{"previous-agent", "Previous panel", "Ctrl+H"},
	{"next-agent", "Next panel", "Ctrl+L"},
	{"command-palette", "Command palette", "Meta+;"},
	{"zoom-in", "Zoom in", "Ctrl+="},
	{"zoom-out", "Zoom out", "Ctrl+-"},
	{"zoom-reset", "Reset zoom", "Ctrl+0"},
}
var shortcutPattern = regexp.MustCompile(`^(Ctrl\+)?(Alt\+)?(Shift\+)?(Meta\+)?[A-Z0-9=,.;\[\]\-]$`)

var reservedNavigationShortcut = regexp.MustCompile(`^Ctrl\+(Shift\+)?[1-9]$`)

func defaultToolKeybindings() map[string]string {
	result := map[string]string{}
	for _, tool := range toolKeys {
		result[tool.ID] = tool.Key
	}
	return result
}

func validateToolKeybindings(bindings map[string]string) error {
	if len(bindings) < len(toolKeys) {
		return fmt.Errorf("provide a shortcut for each tool")
	}
	used := map[string]bool{}
	for _, tool := range toolKeys {
		_, ok := bindings[tool.ID]
		if !ok {
			return fmt.Errorf("missing shortcut for %s", tool.Name)
		}
	}
	defaults := defaultToolKeybindings()
	for _, plugin := range installedPlugins {
		if configurableTool(plugin) {
			defaults[plugin.ID] = ""
		}
	}
	for id, key := range bindings {
		if _, known := defaults[id]; !known && (!strings.HasPrefix(id, "custom-tool-") || !pluginIDPattern.MatchString(id)) {
			return fmt.Errorf("unknown shortcut target %s", id)
		}
		if key == "" {
			continue
		}
		if key == "Ctrl+A" || reservedNavigationShortcut.MatchString(key) {
			return fmt.Errorf("%s is reserved for workspace navigation", key)
		}
		if (id != "voice" || key != "CapsLock") && (!shortcutPattern.MatchString(key) || (!strings.Contains(key, "Ctrl+") && !strings.Contains(key, "Alt+") && !strings.Contains(key, "Meta+"))) {
			return fmt.Errorf("use CapsLock for voice typing, or Ctrl, Alt, or Meta with a letter, number, comma, period, semicolon, brackets, = or -")
		}
		if used[key] {
			return fmt.Errorf("%s is assigned more than once", key)
		}
		used[key] = true
	}
	return nil
}

func toolKeybindings() map[string]string {
	dbMu.Lock()
	defer dbMu.Unlock()
	result := defaultToolKeybindings()
	if db != nil {
		var raw string
		var saved map[string]string
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'tool_keybindings'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &saved) == nil {
			if saved["voice"] == "Ctrl+Alt+V" || saved["voice"] == "Tab" {
				saved["voice"] = "CapsLock"
			}
			if saved["notes"] == "Ctrl+O" {
				available := true
				for _, key := range saved {
					if key == "Ctrl+I" {
						available = false
					}
				}
				if available {
					saved["notes"] = "Ctrl+I"
				}
			}
			if _, exists := saved["new-agent"]; saved["nvim"] == "Ctrl+Alt+N" || (!exists && saved["nvim"] == "Ctrl+N") {
				saved["nvim"] = "Ctrl+E"
				for id, key := range saved {
					if id != "nvim" && key == "Ctrl+E" {
						saved["nvim"] = ""
						break
					}
				}
			}
			if _, exists := saved["replace-agent"]; !exists {
				for id, key := range saved {
					if key == "Ctrl+Shift+N" {
						saved[id] = ""
					}
				}
				saved["replace-agent"] = "Ctrl+Shift+N"
			}
			if saved["new-agent"] == "Ctrl+N" {
				saved["new-agent"] = ""
			}
			// The command-palette field marks settings saved with the new defaults.
			if _, current := saved["command-palette"]; !current {
				if saved["new-thread"] == "Ctrl+N" {
					saved["new-thread"] = ""
				}
				if saved["project-picker"] == "Ctrl+P" {
					delete(saved, "project-picker")
				}
			}
			delete(saved, "toggle-projects")
			if saved["replace-agent"] == "Ctrl+Shift+N" {
				saved["replace-agent"] = ""
				available := true
				for _, key := range saved {
					if key == "Ctrl+Shift+A" {
						available = false
					}
				}
				if available {
					saved["replace-agent"] = "Ctrl+Shift+A"
				}
			}
			for _, shortcut := range toolKeys {
				if _, exists := saved[shortcut.ID]; exists {
					continue
				}
				value := shortcut.Key
				for _, key := range saved {
					if key == value {
						value = ""
						break
					}
				}
				saved[shortcut.ID] = value
			}
			if validateToolKeybindings(saved) == nil {
				return saved
			}
		}
	}
	return result
}

func setToolKeybindings(bindings map[string]string) error {
	if err := validateToolKeybindings(bindings); err != nil {
		return err
	}
	raw, err := json.Marshal(bindings)
	if err != nil {
		return err
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES ('tool_keybindings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	return err
}

func registerKeybindingActions(app *r.App) {
	registerAction(app, "settings.tool-keys", func(ctx *r.Context) string {
		raw, _ := json.Marshal(ctx.WsData()["bindings"])
		var bindings map[string]string
		err := json.Unmarshal(raw, &bindings)
		if err == nil {
			err = setToolKeybindings(bindings)
		}
		if err != nil {
			return fmt.Sprintf("libroWorkspace.toolKeysSaved(null,%s);", components.JSString(err.Error()))
		}
		encoded, _ := json.Marshal(bindings)
		return fmt.Sprintf("libroWorkspace.toolKeysSaved(%s,'Saved. Shortcuts are active now.');", encoded)
	})
}

func renderToolKeybindings() *r.Node {
	rows := []*r.Node{}
	for _, tool := range toolKeys {
		if tool.ID == "nvim" || tool.ID == "lazyrepo" || tool.ID == "lazydata" {
			continue
		}
		id := "tool-key-" + tool.ID
		rows = append(rows, r.Div("ws-settings-row ws-agent-command-row").Render(
			r.El("label", "").Attr("for", id).Text(tool.Name),
			r.Input("ws-agent-command").ID(id).Attr("data-tool-key", tool.ID).Attr("readonly", "").Attr("placeholder", "Press shortcut").Attr("aria-describedby", "tool-key-help"),
			workspaceButton("Clear "+tool.Name+" shortcut", "close", "document.getElementById('"+id+"').value=''"),
		))
	}
	rows = append(rows, r.Div("ws-settings-row").Render(r.Button("ws-launch").Attr("type", "submit").Text("Save shortcuts"), r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.resetToolKeys()")).Text("Restore defaults")))
	return r.El("form", "").ID("tool-key-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveToolKeys()")).Render(
		r.El("h2", "ws-shortcut-heading").Text("Keyboard shortcuts"),
		r.P("ws-settings-status").ID("tool-key-help").Text("Select a field and press Ctrl, Alt, or Meta with a letter, number, comma, period, semicolon, brackets, = or -. Voice typing also accepts CapsLock. Press once to listen and again to transcribe. Clear a field to disable its shortcut. Ctrl+1–9 switches project agent threads and unarchived standalone threads in sidebar order."),
		r.Div("ws-settings-group").Render(rows...),
		r.P("ws-settings-status").ID("tool-key-status").Attr("role", "status"),
	)
}
