package libro

import (
	"fmt"
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
	registerAction(app, "project.command.save", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name, _ := ctx.WsData()["name"].(string)
		command, _ := ctx.WsData()["command"].(string)
		restoreWorktreeProject(sm, sid, name)
		for _, project := range sm.Get(sid).Projects {
			if project.Name != name {
				continue
			}
			if err := setProjectCommand(project.Path, command); err != nil {
				return r.Notify("error", "Could not save command: "+err.Error())
			}
			return projectsJS(sm.Get(sid)) + `document.getElementById('project-command-dialog')?.close();`
		}
		return r.Notify("error", "Project not found")
	})
	registerAction(app, "project.command.run", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		path := sm.GetActiveProjectPath(sid)
		command := projectCommand(path)
		if state.ActiveProject == "" {
			return r.Notify("error", "Open a project first")
		}
		if command == "" {
			return fmt.Sprintf(`libroWorkspace.projectSettings(%s);`, components.JSString(state.ActiveProject))
		}
		js := stopProjectCommand(sid)
		id := sm.NextAppID()
		sm.InsertTerminalPlaceholder(sid, id, WidthFull, command, true, "Project command", "", -1)
		sm.SetAppPlugin(sid, id, "project-command", "bottom")
		state = sm.Get(sid)
		frame := renderAppFramePlaceholder(state.Apps[state.SelectedIndex], state.SelectedIndex, true, sid)
		result := js + insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + projectsJS(state) + hydrateAppAfterScrollJS(id, sidData(sid, "id", id)) + fmt.Sprintf(`libroWorkspace.select(%s);`, components.JSString(id))
		if js != "" {
			return "libroWorkspace.restartProject(function(){" + result + "});"
		}
		return result
	})
	registerAction(app, "project.command.stop", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		js := stopProjectCommand(sid)
		return js + navigateJS(sm.Get(sid), sid) + projectsJS(sm.Get(sid))
	})
}

func stopProjectCommand(sid string) string {
	for _, app := range sm.Get(sid).Apps {
		if app.PluginID == "project-command" {
			tm.Stop(app.ID)
			sm.RemoveAppByID(sid, app.ID)
			return removeAppJS(app.ID)
		}
	}
	return ""
}
