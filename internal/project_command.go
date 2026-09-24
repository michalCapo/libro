package libro

import (
	"fmt"
	"strconv"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

func projectCommand(path string) string {
	dbMu.Lock()
	defer dbMu.Unlock()
	var command string
	if db != nil {
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "project-command:"+path).Scan(&command)
	}
	return command
}

func setProjectCommand(path, command string) error {
	command = strings.TrimSpace(command)
	if strings.ContainsRune(command, 0) {
		return fmt.Errorf("command cannot contain a null character")
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "project-command:"+path, command)
	return err
}

func registerProjectCommandActions(app *r.App) {
	registerApplicationControl(app)
	registerAction(app, "project.command.save", func(ctx *r.Context) string {
		applicationControlMu.Lock()
		defer applicationControlMu.Unlock()
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		command, _ := data["command"].(string)
		mode, _ := data["mode"].(string)
		portText, _ := data["port"].(string)
		port := 0
		if portText != "" {
			var err error
			port, err = strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return r.Notify("error", "Enter a port between 1 and 65535, or leave it blank")
			}
		}
		restoreWorktreeProject(sm, sid, name)
		state := sm.Get(sid)
		_, path := threadProjectContext(state, name)
		if path == "" {
			return r.Notify("error", "Project not found")
		}
		root := applicationRoot(state, name)
		settings := loadApplicationSettings(root)
		if mode == "" {
			mode = settings.Mode
		}
		if mode != "shared" && mode != "thread" {
			return r.Notify("error", "Choose shared or per-thread application mode")
		}
		if strings.ContainsRune(command, 0) {
			return r.Notify("error", "Command cannot contain a null character")
		}
		if settings.Mode != mode {
			for _, running := range sm.GetAllRunningApps(sid) {
				if applicationRoot(state, running.Name) != root {
					continue
				}
				for _, panel := range running.Apps {
					if panel.PluginID == "project-command" {
						return r.Notify("error", "Stop the project's applications before changing application mode")
					}
				}
			}
		}
		settings.Mode = mode
		if mode == "shared" {
			path = root
			port = 0
		}
		if path == root {
			settings.Port = port
		}
		if err := saveApplicationSettings(root, settings); err != nil {
			return r.Notify("error", err.Error())
		}
		if path != root {
			if err := saveApplicationSettings(path, applicationSettings{Mode: mode, Port: port}); err != nil {
				return r.Notify("error", err.Error())
			}
		}
		if err := setProjectCommand(path, command); err != nil {
			return r.Notify("error", "Could not save command: "+err.Error())
		}
		return projectsJS(state) + `document.getElementById('project-command-dialog')?.close();`
	})
	registerAction(app, "project.command.run", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if state.ActiveProject == "" {
			return r.Notify("error", "Open a project first")
		}
		_, command, _ := applicationConfiguration(state, state.ActiveProject)
		if command == "" {
			return fmt.Sprintf(`libroWorkspace.projectSettings(%s);`, components.JSString(state.ActiveProject))
		}
		hadCommand := false
		for _, panel := range state.Apps {
			if panel.PluginID == "project-command" {
				hadCommand = true
			}
		}
		js, _, err := controlApplication(sid, sm.GetActiveProjectPath(sid), "restart")
		if err != nil {
			return js + r.Notify("error", err.Error())
		}
		for _, panel := range sm.Get(sid).Apps {
			if panel.PluginID == "project-command" {
				js += navigateJS(sm.Get(sid), sid) + fmt.Sprintf(`libroWorkspace.select(%s);`, components.JSString(panel.ID))
				break
			}
		}
		if hadCommand {
			js = "libroWorkspace.restartProject(function(){" + js + "});"
		}
		return js
	})
	registerAction(app, "project.command.stop", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		js, _, err := controlApplication(sid, sm.GetActiveProjectPath(sid), "stop")
		if err != nil {
			return js + r.Notify("error", err.Error())
		}
		return js + r.Notify("info", "Application stopped")
	})
}
