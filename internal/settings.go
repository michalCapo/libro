package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"strings"
)

func defaultPanelWidths() []Width {
	return []Width{WidthXS, WidthSM, WidthMD, WidthLG, WidthXL, Width2XL}
}

func validDefaultPanelWidth(width Width) bool {
	for _, candidate := range defaultPanelWidths() {
		if width == candidate {
			return true
		}
	}
	return false
}

func DBDefaultPanelWidth() Width {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return WidthMD
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'default_panel_width'`).Scan(&value); err != nil || !validDefaultPanelWidth(Width(value)) {
		return WidthMD
	}
	return Width(value)
}

func DBSetDefaultPanelWidth(width Width) error {
	if !validDefaultPanelWidth(width) {
		return fmt.Errorf("invalid default panel width: %s", width)
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('default_panel_width', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(width))
	return err
}

// Agent command overrides are global and apply only to new sessions.
func agentCommand(plugin Plugin) string {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil && plugin.Dock == "center" {
		var command string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "agent_command."+plugin.ID).Scan(&command); err == nil && strings.TrimSpace(command) != "" {
			return command
		}
	}
	return plugin.Command
}

func setAgentCommand(id, command string) error {
	return setAgentCommands(map[string]string{id: command})
}

func setAgentCommands(commands map[string]string) error {
	return saveAgentConfig(commands, nil, nil, nil)
}

func saveAgentConfig(commands map[string]string, disabled map[string]bool, custom []Plugin, names map[string]string, removals ...map[string]bool) error {
	if len(commands) == 0 && len(removals) == 0 {
		return fmt.Errorf("no agent commands supplied")
	}
	removed := map[string]bool{}
	if len(removals) > 0 {
		removed = removals[0]
	}
	agents := map[string]bool{}
	for _, plugin := range plugins() {
		if plugin.Dock == "center" && plugin.Type == AppTypeTerminal {
			agents[plugin.ID] = true
			if removed[plugin.ID] && plugin.Custom {
				found := false
				for _, entry := range custom {
					if entry.ID == plugin.ID {
						found = true
					}
				}
				if !found {
					custom = append(custom, plugin)
				}
			}
		}
	}
	for _, plugin := range custom {
		if !strings.HasPrefix(plugin.ID, "custom-") || !pluginIDPattern.MatchString(plugin.ID) || strings.TrimSpace(plugin.Name) == "" || plugin.Type != AppTypeTerminal || plugin.Dock != "center" || !plugin.Custom {
			return fmt.Errorf("invalid custom agent")
		}
		agents[plugin.ID] = true
	}
	for id, drop := range removed {
		if drop && (!agents[id] || (disabled != nil && !disabled[id])) {
			return fmt.Errorf("only disabled agents can be removed")
		}
	}
	for id, name := range names {
		if !agents[id] || strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) {
			return fmt.Errorf("enter a valid agent name")
		}
	}
	for id := range disabled {
		if !agents[id] {
			return fmt.Errorf("unknown agent")
		}
	}
	for id, command := range commands {
		if !agents[id] {
			return fmt.Errorf("unknown agent")
		}
		if strings.TrimSpace(command) == "" || strings.ContainsRune(command, 0) {
			return fmt.Errorf("enter a CLI command")
		}
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, command := range commands {
		if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, "agent_command."+id, strings.TrimSpace(command)); err != nil {
			return err
		}
	}
	if disabled != nil {
		raw, err := json.Marshal(disabled)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('disabled_agents',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
			return err
		}
	}
	if custom != nil {
		raw, err := json.Marshal(custom)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('custom_agents',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
			return err
		}
	}
	if names != nil {
		raw, err := json.Marshal(names)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('agent_names',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
			return err
		}
	}
	if len(removals) > 0 {
		raw, err := json.Marshal(removed)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('removed_agents',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
			return err
		}
		for id, drop := range removed {
			if drop {
				if _, err := tx.Exec(`DELETE FROM settings WHERE key = ?`, "agent_command."+id); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func renderAgentCommands() *r.Node {
	return r.El("form", "").ID("agent-commands-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveAgentCommand(this)")).Render(
		r.Div("ws-settings-group").Render(
			r.Div("").ID("agent-command-rows"),
			r.Div("ws-settings-row").Render(
				r.Button("ws-launch").Attr("type", "submit").Text("Save agents"),
				r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addCustomAgent()")).Text("Add custom agent"),
			),
		),
		r.P("ws-settings-status").Attr("role", "status"),
	)
}

func registerSettingsActions(app *r.App) {
	registerKeybindingActions(app)
	registerProjectCommandActions(app)
	registerToolSettings(app)
	registerAction(app, "settings.agent-command", func(ctx *r.Context) string {
		raw, _ := json.Marshal(ctx.WsData()["commands"])
		var commands map[string]string
		err := json.Unmarshal(raw, &commands)
		if err == nil {
			var disabled map[string]bool
			var custom []Plugin
			disabledRaw, _ := json.Marshal(ctx.WsData()["disabled"])
			customRaw, _ := json.Marshal(ctx.WsData()["custom"])
			err = json.Unmarshal(disabledRaw, &disabled)
			if err == nil {
				err = json.Unmarshal(customRaw, &custom)
			}
			if err == nil {
				var names map[string]string
				namesRaw, _ := json.Marshal(ctx.WsData()["names"])
				err = json.Unmarshal(namesRaw, &names)
				if err == nil {
					var removed map[string]bool
					raw, _ := json.Marshal(ctx.WsData()["removed"])
					err = json.Unmarshal(raw, &removed)
					if err == nil {
						err = saveAgentConfig(commands, disabled, custom, names, removed)
					}
				}
			}
		}
		message := "Saved. Applies to new sessions."
		if err != nil {
			message = "Could not save. Enter a valid command for each agent and try again."
		}
		list, _ := json.Marshal(plugins())
		return fmt.Sprintf("libroWorkspace.agentCommandSaved(%s,%s);", components.JSString(message), list)
	})
	registerAction(app, "settings.open", func(ctx *r.Context) string {
		commands := map[string]string{}
		for _, plugin := range plugins() {
			if plugin.Dock == "center" && plugin.Type == AppTypeTerminal {
				commands[plugin.ID] = agentCommand(plugin)
			}
		}
		encoded, _ := json.Marshal(commands)
		keys, _ := json.Marshal(toolKeybindings())
		list, _ := json.Marshal(plugins())
		return fmt.Sprintf("window.__libroPlugins=%s;libroWorkspace.showSettings(%s,%s,%s);", list, components.JSString(string(DBDefaultPanelWidth())), encoded, keys)
	})
	registerAction(app, "settings.width", func(ctx *r.Context) string {
		value, _ := ctx.WsData()["width"].(string)
		if err := DBSetDefaultPanelWidth(Width(value)); err != nil {
			return "libroWorkspace.settingsSaved(false);"
		}
		return fmt.Sprintf("libroWorkspace.settingsSaved(true,%s);", components.JSString(Width(value).PixelWidth()))
	})
}

func renderWorkspaceSettings() *r.Node {
	options := []*r.Node{}
	for _, width := range defaultPanelWidths() {
		options = append(options, r.El("option", "").Attr("value", string(width)).Text(width.Label()))
	}
	return r.El("section", "ws-settings").ID("workspace-settings").Attr("hidden", "").Attr("aria-labelledby", "workspace-settings-title").Render(
		r.Div("ws-settings-header").Render(
			r.El("h1", "").ID("workspace-settings-title").Attr("tabindex", "-1").Text("Settings"),
		),
		r.Div("ws-settings-content").Render(
			r.El("h2", "ws-shortcut-heading").Text("Appearance"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "workspace-theme").Text("Theme"),
						r.P("").ID("workspace-theme-help").Text("Auto follows your operating system. Changes apply immediately."),
					),
					r.El("select", "ws-settings-select").ID("workspace-theme").Attr("aria-describedby", "workspace-theme-help").On("change", r.JS("libroWorkspace.saveTheme(event.target.value)")).Render(
						r.El("option", "").Attr("value", "system").Text("Auto"),
						r.El("option", "").Attr("value", "light").Text("Light"),
						r.El("option", "").Attr("value", "dark").Text("Dark"),
					),
				),
			),
			r.P("ws-settings-status").ID("workspace-theme-status").Attr("role", "status"),
			r.El("h2", "ws-shortcut-heading").Text("Notifications"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "notification-sound").Text("Agent done sound"),
						r.P("").ID("notification-sound-help").Text("Play a short sound whenever an agent finishes a task, in any project."),
					),
					r.El("select", "ws-settings-select").ID("notification-sound").Attr("aria-describedby", "notification-sound-help").On("change", r.JS("libroWorkspace.saveNotificationSound(event.target.value)")).Render(
						r.El("option", "").Attr("value", "on").Text("On"),
						r.El("option", "").Attr("value", "off").Text("Off"),
					),
				),
			),
			r.P("ws-settings-status").ID("notification-sound-status").Attr("role", "status"),
			r.El("h2", "ws-shortcut-heading").Text("Panels"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "default-panel-width").Text("Default panel width"),
						r.P("").ID("default-panel-width-help").Text("The width of new panels in every project. Existing panels keep their current width."),
					),
					r.El("select", "ws-settings-select").ID("default-panel-width").Attr("aria-describedby", "default-panel-width-help").On("change", r.JS("libroWorkspace.saveSettings(event.target.value)")).Render(options...),
				),
			),
			r.P("ws-settings-status").ID("workspace-settings-status").Attr("role", "status").Attr("aria-live", "polite"),
			r.El("h2", "ws-shortcut-heading").Text("Agent commands"),
			r.P("ws-settings-status").Text("CLI commands used to start agents in every project. Include any flags you need. Running sessions are unchanged."),
			renderAgentCommands(),
			renderToolSettings(),
			renderToolKeybindings(),
		),
	)
}
