package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type environmentInput struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	OriginalName string `json:"originalName"`
}

func agentEnvironment() map[string]string {
	result := map[string]string{}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return result
	}
	var raw string
	if db.QueryRow(`SELECT value FROM settings WHERE key = 'agent_environment'`).Scan(&raw) == nil {
		_ = json.Unmarshal([]byte(raw), &result)
	}
	return result
}

func agentEnvironmentNames() []string {
	environment := agentEnvironment()
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func agentEnvironmentList() []string {
	environment := agentEnvironment()
	result := make([]string, 0, len(environment))
	for name, value := range environment {
		result = append(result, name+"="+value)
	}
	sort.Strings(result)
	return result
}

func setAgentEnvironment(inputs []environmentInput) error {
	existing := agentEnvironment()
	next := make(map[string]string, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if !environmentNamePattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name: %s", name)
		}
		if _, duplicate := next[name]; duplicate {
			return fmt.Errorf("duplicate environment variable: %s", name)
		}
		value := input.Value
		if value == "" && input.OriginalName != "" && name == input.OriginalName {
			value = existing[input.OriginalName]
		}
		if value == "" || strings.ContainsRune(value, 0) {
			return fmt.Errorf("enter a value for %s", name)
		}
		next[name] = value
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES ('agent_environment',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	return err
}

func defaultPanelWidths() []Width {
	return []Width{WidthXS, WidthSM, WidthMD, WidthLG, WidthXL, Width2XL, WidthFull}
}

func validDefaultPanelWidth(width Width) bool {
	return slices.Contains(defaultPanelWidths(), width)
}

func DBDefaultPanelWidth() Width {
	return dbPanelWidth("default_panel_width", WidthMD)
}

func DBDefaultToolPanelWidth() Width {
	return dbPanelWidth("default_tool_panel_width", WidthLG)
}

func defaultAppWidth(app Application) Width {
	if isAgentApp(app) {
		return DBDefaultPanelWidth()
	}
	return DBDefaultToolPanelWidth()
}

func dbPanelWidth(key string, fallback Width) Width {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fallback
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil || !validDefaultPanelWidth(Width(value)) {
		return fallback
	}
	return Width(value)
}

func DBSetDefaultPanelWidth(width Width) error {
	return dbSetPanelWidth("default_panel_width", width)
}

func DBSetDefaultToolPanelWidth(width Width) error {
	return dbSetPanelWidth("default_tool_panel_width", width)
}

func browserPageToolsAutoExecute() bool {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return false
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'browser_page_tools_autoexecute'`).Scan(&value); err != nil {
		return false
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func setBrowserPageToolsAutoExecute(enabled bool) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	value := "0"
	if enabled {
		value = "1"
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES ('browser_page_tools_autoexecute',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, value)
	return err
}

func dbSetPanelWidth(key string, width Width) error {
	if !validDefaultPanelWidth(width) {
		return fmt.Errorf("invalid default panel width: %s", width)
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(width))
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
	var removed map[string]bool
	if len(removals) > 0 {
		removed = removals[0]
	}
	return saveAgentSettings(commands, disabled, custom, names, removed, nil, nil)
}

func saveAgentSettings(commands map[string]string, disabled map[string]bool, custom []Plugin, names map[string]string, removed map[string]bool, autolaunch *string, order []string) error {
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
	if autolaunch != nil && *autolaunch != "" && (!agents[*autolaunch] || disabled[*autolaunch] || removed[*autolaunch]) {
		return fmt.Errorf("autolaunch requires an enabled agent")
	}
	seen := map[string]bool{}
	for _, id := range order {
		if !agents[id] || removed[id] || seen[id] {
			return fmt.Errorf("invalid agent order")
		}
		seen[id] = true
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
	defer func() { _ = tx.Rollback() }()
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
	if removed != nil {
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
	if autolaunch != nil {
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('autolaunch_agent',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, *autolaunch); err != nil {
			return err
		}
	}
	if order != nil {
		raw, err := json.Marshal(order)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings (key,value) VALUES ('agent_order',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func defaultThreadAgent() string {
	dbMu.Lock()
	defer dbMu.Unlock()
	var id string
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'default_thread_agent'`).Scan(&id)
	}
	return id
}

func setDefaultThreadAgent(id string) error {
	if id != "" {
		valid := false
		for _, plugin := range plugins() {
			if plugin.ID == id && plugin.Dock == "center" && plugin.Type == AppTypeTerminal && !plugin.Disabled && !plugin.Removed {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("choose an enabled agent")
		}
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES ('default_thread_agent',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, id)
	return err
}

func renderAgentCommands() *r.Node {
	return r.El("form", "").ID("agent-commands-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveAllSettings()")).Render(
		r.Div("ws-settings-group").Render(
			r.Div("").ID("agent-command-rows"),
			r.Div("ws-settings-row").Render(
				r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addCustomAgent()")).Text("Add custom agent"),
			),
		),
		r.P("ws-settings-status").Attr("role", "status"),
	)
}

func registerSettingsActions(app *r.App) {
	registerAction(app, "settings.agent-environment", func(ctx *r.Context) string {
		raw, _ := json.Marshal(ctx.WsData()["entries"])
		var entries []environmentInput
		err := json.Unmarshal(raw, &entries)
		if err == nil {
			err = setAgentEnvironment(entries)
		}
		message := "Saved. Applies to new agent sessions."
		if err != nil {
			message = "Could not save: " + err.Error()
		}
		names, _ := json.Marshal(agentEnvironmentNames())
		return fmt.Sprintf("libroWorkspace.agentEnvironmentSaved(%t,%s,%s);", err == nil, components.JSString(message), names)
	})
	registerAction(app, "settings.thread-agent", func(ctx *r.Context) string {
		id, ok := ctx.WsData()["agent"].(string)
		saved := ok && setDefaultThreadAgent(id) == nil
		return fmt.Sprintf("libroWorkspace.threadAgentSaved(%t,%s);", saved, components.JSString(defaultThreadAgent()))
	})
	registerAction(app, "settings.page-tools", func(ctx *r.Context) string {
		enabled, ok := ctx.WsData()["autoexecute"].(bool)
		saved := ok && setBrowserPageToolsAutoExecute(enabled) == nil
		return fmt.Sprintf("libroWorkspace.pageToolsSaved(%t,%t);", saved, browserPageToolsAutoExecute())
	})
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
						autolaunch, ok := ctx.WsData()["autolaunch"].(string)
						if !ok {
							err = fmt.Errorf("invalid autolaunch agent")
						} else {
							var order []string
							raw, _ := json.Marshal(ctx.WsData()["order"])
							err = json.Unmarshal(raw, &order)
							if err == nil {
								err = saveAgentSettings(commands, disabled, custom, names, removed, &autolaunch, order)
							}
						}
					}
				}
			}
		}
		message := "Saved. Applies to new sessions."
		if err != nil {
			message = "Could not save: " + err.Error()
		}
		list, _ := json.Marshal(plugins())
		return fmt.Sprintf("libroWorkspace.agentCommandSaved(%s,%s,%t);", components.JSString(message), list, err == nil)
	})
	registerAction(app, "settings.open", func(_ *r.Context) string {
		commands := map[string]string{}
		for _, plugin := range plugins() {
			if plugin.Dock == "center" && plugin.Type == AppTypeTerminal {
				commands[plugin.ID] = agentCommand(plugin)
			}
		}
		encoded, _ := json.Marshal(commands)
		keys, _ := json.Marshal(toolKeybindings())
		list, _ := json.Marshal(plugins())
		environment, _ := json.Marshal(agentEnvironmentNames())
		return fmt.Sprintf("window.__libroPlugins=%s;libroWorkspace.showSettings(%s,%s,%s,%s,%s,%t,%s);", list, components.JSString(string(DBDefaultPanelWidth())), encoded, keys, components.JSString(string(DBDefaultToolPanelWidth())), components.JSString(defaultThreadAgent()), browserPageToolsAutoExecute(), environment)
	})
	registerAction(app, "settings.width", func(ctx *r.Context) string {
		value, _ := ctx.WsData()["width"].(string)
		tool, _ := ctx.WsData()["tool"].(bool)
		setter := DBSetDefaultPanelWidth
		if tool {
			setter = DBSetDefaultToolPanelWidth
		}
		if err := setter(Width(value)); err != nil {
			return fmt.Sprintf("libroWorkspace.settingsSaved(false,%t);", tool)
		}
		return fmt.Sprintf("libroWorkspace.settingsSaved(true,%t);", tool)
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
						r.P("").ID("workspace-theme-help").Text("Auto follows your operating system."),
					),
					r.El("select", "ws-settings-select").ID("workspace-theme").Attr("aria-describedby", "workspace-theme-help").Render(
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
					r.El("select", "ws-settings-select").ID("notification-sound").Attr("aria-describedby", "notification-sound-help").Render(
						r.El("option", "").Attr("value", "on").Text("On"),
						r.El("option", "").Attr("value", "off").Text("Off"),
					),
				),
			),
			r.P("ws-settings-status").ID("notification-sound-status").Attr("role", "status"),
			r.El("h2", "ws-shortcut-heading").Text("Page tools"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "page-tools-autoexecute").Text("Autoexecute page tool prompts"),
						r.P("").ID("page-tools-autoexecute-help").Text("Use A to annotate an element, D to annotate an area, or P to annotate the whole page, then send the prompt to the active agent. When on, it is submitted immediately."),
					),
					r.El("select", "ws-settings-select").ID("page-tools-autoexecute").Attr("aria-describedby", "page-tools-autoexecute-help").Render(
						r.El("option", "").Attr("value", "off").Text("Off — paste only"),
						r.El("option", "").Attr("value", "on").Text("On — send and run"),
					),
				),
			),
			r.P("ws-settings-status").ID("page-tools-autoexecute-status").Attr("role", "status"),
			r.El("h2", "ws-shortcut-heading").Text("Agent environment"),
			r.P("ws-settings-status").Text("Environment variables passed to new agent sessions. Saved values stay hidden in Settings."),
			r.El("form", "").ID("agent-environment-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveAllSettings()")).Render(
				r.Div("ws-settings-group").Render(
					r.Div("").ID("agent-environment-rows"),
					r.Div("ws-settings-row").Render(
						r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addAgentEnvironment()")).Text("Add variable"),
					),
				),
				r.P("ws-settings-status").Attr("role", "status"),
			),
			r.El("h2", "ws-shortcut-heading").Text("Panels"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "default-panel-width").Text("Agent panel width"),
						r.P("").ID("default-panel-width-help").Text("Default width for the agent panel in new threads. Choose MAX to start at full width."),
					),
					r.El("select", "ws-settings-select").ID("default-panel-width").Attr("aria-describedby", "default-panel-width-help").Render(options...),
				),
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "default-tool-panel-width").Text("Tool panel width"),
						r.P("").ID("default-tool-panel-width-help").Text("Default width for new tool panels, such as Files and Browser."),
					),
					r.El("select", "ws-settings-select").ID("default-tool-panel-width").Attr("aria-describedby", "default-tool-panel-width-help").Render(options...),
				),
			),
			r.P("ws-settings-status").Text("Existing panels keep their current width."),
			r.P("ws-settings-status").ID("workspace-settings-status").Attr("role", "status").Attr("aria-live", "polite"),
			r.El("h2", "ws-shortcut-heading").Text("Threads"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "default-thread-agent").Text("Default agent"),
						r.P("").ID("default-thread-agent-help").Text("Start this agent in new threads."),
					),
					r.El("select", "ws-settings-select").ID("default-thread-agent").Attr("aria-describedby", "default-thread-agent-help"),
				),
			),
			r.P("ws-settings-status").ID("default-thread-agent-status").Attr("role", "status"),
			r.El("h2", "ws-shortcut-heading").Text("Autolaunch"),
			r.Div("ws-settings-group").Render(
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-copy").Render(
						r.El("label", "").Attr("for", "autolaunch-agent").Text("Autolaunch agent"),
						r.P("").ID("autolaunch-agent-help").Text("Start this agent when you open a project with no agent panels. Choose Off to start manually."),
					),
					r.El("select", "ws-settings-select").ID("autolaunch-agent").Attr("form", "agent-commands-form").Attr("aria-describedby", "autolaunch-agent-help"),
				),
			),
			r.El("h2", "ws-shortcut-heading").Text("Agent commands"),
			r.P("ws-settings-status").Text("CLI commands used to start agents in new threads. Drag the handles or use the arrows to reorder agents, then save. The first three enabled agents appear on the welcome screen. Include any flags you need. Running sessions are unchanged."),
			renderAgentCommands(),
			renderToolSettings(),
			renderToolKeybindings(),
		),
		r.Div("ws-settings-footer").Render(
			r.P("ws-settings-status").ID("settings-save-status").Attr("role", "status").Attr("aria-live", "polite"),
			r.Button("ws-launch").ID("settings-cancel").Attr("type", "button").OnClick(r.JS("libroWorkspace.closeSettings()")).Text("Cancel"),
			r.Button("ws-launch").ID("settings-save").Attr("type", "button").OnClick(r.JS("libroWorkspace.saveAllSettings()")).Text("Save"),
		),
	)
}
