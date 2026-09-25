package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"net/url"
	"strings"
)

func configurableTool(p Plugin) bool {
	return p.Dock == "right" && (p.Type == AppTypeTerminal || p.Type == AppTypeURL) && p.ID != "terminal" && p.ID != "browser" && p.ID != "files" && p.ID != "notes"
}

func saveTools(list []Plugin, shortcuts ...map[string]string) error {
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
		p.URL = strings.TrimSpace(p.URL)
		if seen[p.ID] || !pluginIDPattern.MatchString(p.ID) || (!known[p.ID] && !strings.HasPrefix(p.ID, "custom-tool-")) || !configurableTool(*p) || p.Name == "" || (p.Removed && !p.Disabled) {
			return fmt.Errorf("each tool needs a unique ID and name; disable it before removing")
		}
		if p.Type == AppTypeURL {
			u, err := url.Parse(p.URL)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("%s needs a valid website URL starting with http:// or https://", p.Name)
			}
			p.Command = ""
		} else {
			if p.Command == "" || strings.ContainsRune(p.Command, 0) {
				return fmt.Errorf("%s needs a command", p.Name)
			}
			p.URL = ""
		}
		seen[p.ID] = true
	}
	var keys []byte
	if len(shortcuts) > 0 {
		if err := validateToolKeybindings(shortcuts[0]); err != nil {
			return err
		}
		keys, _ = json.Marshal(shortcuts[0])
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
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`INSERT INTO settings (key,value) VALUES ('tool_configs',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	if err != nil {
		return err
	}
	if keys != nil {
		if _, err = tx.Exec(`INSERT INTO settings (key,value) VALUES ('tool_keybindings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(keys)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func registerToolSettings(app *r.App) {
	registerAction(app, "settings.tools", func(ctx *r.Context) string {
		raw, _ := json.Marshal(ctx.WsData()["tools"])
		var list []Plugin
		err := json.Unmarshal(raw, &list)
		if err == nil {
			keyJSON, _ := json.Marshal(ctx.WsData()["bindings"])
			var bindings map[string]string
			err = json.Unmarshal(keyJSON, &bindings)
			if err == nil {
				err = saveTools(list, bindings)
			}
		}
		if err != nil {
			return fmt.Sprintf("libroWorkspace.toolsSaved(null,%s);", components.JSString(err.Error()))
		}
		updated, _ := json.Marshal(plugins())
		keys, _ := json.Marshal(toolKeybindings())
		return fmt.Sprintf("libroWorkspace.toolsSaved(%s,'Saved. Shortcuts are active now. Other changes apply to new sessions.',%s);", updated, keys)
	})
}

func renderToolSettings() *r.Node {
	return r.El("section", "ws-tool-settings").Render(
		r.El("h2", "ws-shortcut-heading").Text("Tools"),
		r.P("ws-settings-status").Text("Configure Nvim, Git, Database, and custom CLI tools and websites. Changes apply across projects to new sessions."),
		r.P("ws-settings-status").ID("tool-shortcut-help").Text("Select a shortcut field and press Ctrl, Alt, or Meta with a letter, number, comma, period, brackets, = or -. Clear it to disable the shortcut."),
		r.El("form", "").ID("tool-commands-form").On("submit", r.JS("event.preventDefault();libroWorkspace.saveAllSettings()")).Render(
			r.Div("ws-settings-group").Render(
				r.Div("").ID("tool-command-rows"),
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-actions").Render(
						r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addCustomTool()")).Text("Add custom tool"),
						r.Button("ws-launch").Attr("type", "button").OnClick(r.JS("libroWorkspace.addCustomTool('url')")).Text("Add website"),
					),
				),
			), r.P("ws-settings-status").Attr("role", "status"),
		),
	)
}
