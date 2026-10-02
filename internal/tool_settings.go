package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"net/url"
	"strings"
)

func configurableTool(p Plugin) bool {
	return p.Dock == "right" && (p.Type == AppTypeTerminal || p.Type == AppTypeURL) && p.ID != "terminal" && p.ID != "browser" && p.ID != "files" && p.ID != "notes"
}

func editorToolID() string {
	dbMu.Lock()
	defer dbMu.Unlock()
	value := "nvim"
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'editor_tool'`).Scan(&value)
	}
	return value
}

func editorTool(p Plugin) bool {
	return configurableTool(p) && p.Type == AppTypeTerminal && !p.Disabled && !p.Removed
}

func saveToolsWithEditor(list []Plugin, editor string, saveEditor bool, shortcuts ...map[string]string) error {
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
	if saveEditor && editor != "" {
		valid := false
		for _, p := range list {
			if p.ID == editor && editorTool(p) {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("select an enabled CLI tool as the editor")
		}
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
	if saveEditor {
		if _, err = tx.Exec(`INSERT INTO settings (key,value) VALUES ('editor_tool',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, editor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func registerToolSettings(app *r.App) {
	r.RegisterAction(app, "settings.tools", func(_ *r.Context, in actionSettingsToolsInput) (r.Result, error) {
		editor, saveEditor := inputField(in.Editor)
		err := saveToolsWithEditor(in.Tools, editor, saveEditor, in.Bindings)
		if err != nil {
			return clientScript("libroWorkspace.toolsSaved(null,props[0]);", err.Error()), nil
		}
		return clientScript("libroWorkspace.toolsSaved(props[0],'Saved. Shortcuts are active now. Other changes apply to new sessions.',props[1]);", plugins(), toolKeybindings()), nil
	})
}

func renderToolSettings() *r.Node {
	return r.El("section", "ws-tool-settings").Render(
		r.El("h2", "ws-shortcut-heading").Text("Tools"),
		r.P("ws-settings-status").Text("Configure Nvim, Git, Database, and custom CLI tools and websites. Changes apply across projects to new sessions."),
		r.P("ws-settings-status").ID("tool-shortcut-help").Text("Select a shortcut field and press Ctrl, Alt, or Meta with a letter, number, comma, period, brackets, = or -. Clear it to disable the shortcut."),
		r.El("form", "").ID("tool-commands-form").OnSubmit(r.UnsafeJS("libroWorkspace.saveAllSettings()")).Render(
			r.Div("ws-settings-group").Render(
				r.Div("").ID("tool-command-rows"),
				r.Div("ws-settings-row").Render(
					r.Div("ws-settings-actions").Render(
						r.Button("ws-launch").Attr("type", "button").OnClick(r.UnsafeJS("libroWorkspace.addCustomTool()")).Text("Add custom tool"),
						r.Button("ws-launch").Attr("type", "button").OnClick(r.UnsafeJS("libroWorkspace.addCustomTool('url')")).Text("Add website"),
					),
				),
			), r.P("ws-settings-status").Attr("role", "status"),
		),
	)
}

func renderEditorSettings() *r.Node {
	return r.El("section", "").Render(
		r.El("h2", "ws-shortcut-heading").Text("Editor"),
		r.Div("ws-settings-group").Render(r.Div("ws-settings-row").Render(
			r.Div("ws-settings-copy").Render(
				r.El("label", "").Attr("for", "editor-tool").Text("File editor"),
				r.P("").ID("editor-tool-help").Text("Press e in Files to open the current file in this tool. The file path is passed to its CLI command.")),
			r.El("select", "ws-settings-select").ID("editor-tool").Attr("aria-describedby", "editor-tool-help"))),
	)
}
