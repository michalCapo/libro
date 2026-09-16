package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"strings"
)

func configurableTool(p Plugin) bool {
	return p.Dock == "right" && p.Type == AppTypeTerminal && p.ID != "terminal"
}

func saveTools(list []Plugin) error {
	known := map[string]bool{}
	for _, p := range plugins() {
		if configurableTool(p) {
			known[p.ID] = true
		}
	}
	seen := map[string]bool{}
	for i := range list {
		p := &list[i]
		p.Name = strings.TrimSpace(p.Name)
		p.Command = strings.TrimSpace(p.Command)
		if seen[p.ID] || !pluginIDPattern.MatchString(p.ID) || (!known[p.ID] && !strings.HasPrefix(p.ID, "custom-tool-")) || !configurableTool(*p) || p.Name == "" || p.Command == "" || strings.ContainsRune(p.Command, 0) || (p.Removed && !p.Disabled) {
			return fmt.Errorf("each tool needs a unique ID, name and command; disable it before removing")
		}
		seen[p.ID] = true
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES ('tool_configs',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	return err
}

func registerToolSettings(app *r.App) {
	registerAction(app, "settings.tools", func(ctx *r.Context) string {
		raw, _ := json.Marshal(ctx.WsData()["tools"])
		var list []Plugin
		err := json.Unmarshal(raw, &list)
		if err == nil {
			err = saveTools(list)
		}
		if err != nil {
			return fmt.Sprintf("libroWorkspace.toolsSaved(null,%s);", components.JSString(err.Error()))
		}
		updated, _ := json.Marshal(plugins())
		return fmt.Sprintf("libroWorkspace.toolsSaved(%s,'Saved. Commands apply to new sessions.');", updated)
	})
}

func renderToolSettings() *r.Node {
	return r.El("section", "ws-tool-settings").Render(
		r.El("h2", "ws-shortcut-heading").Text("Tools"),
		r.P("ws-settings-status").Text("Configure Nvim, Git, Database, and custom CLI tools. Changes apply across projects to new sessions."),
		r.El("form", "").ID("tool-commands-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveTools(this)")).Render(
			r.Div("ws-settings-group").Render(
				r.Div("").ID("tool-command-rows"),
				r.Div("ws-settings-row").Render(r.Button("ws-launch").Attr("type", "submit").Text("Save tools"), r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addCustomTool()")).Text("Add custom tool")),
			), r.P("ws-settings-status").Attr("role", "status"),
		),
	)
}
