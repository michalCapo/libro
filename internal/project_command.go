package libro

import (
	"fmt"
	"strconv"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
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
	registerChildControl(app)
	r.RegisterAction(app, "project.command.save", func(_ *r.Context, in actionProjectCommandSaveInput) (r.Result, error) {
		applicationControlMu.Lock()
		defer applicationControlMu.Unlock()
		sid := inputSID(in.SID)
		name := in.Name
		command := in.Command
		mode := in.Mode
		portText := in.Port
		port := 0
		if portText != "" {
			var err error
			port, err = strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return r.Result{}.Run(r.Notify("error", "Enter a port between 1 and 65535, or leave it blank")), nil
			}
		}
		restoreWorktreeProject(sm, sid, name)
		state := sm.Get(sid)
		_, path := threadProjectContext(state, name)
		if path == "" {
			return r.Result{}.Run(r.Notify("error", "Project not found")), nil
		}
		root := applicationRoot(state, name)
		settings := loadApplicationSettings(root)
		if mode == "" {
			mode = settings.Mode
		}
		if mode != "shared" && mode != "thread" {
			return r.Result{}.Run(r.Notify("error", "Choose shared or per-thread application mode")), nil
		}
		if strings.ContainsRune(command, 0) {
			return r.Result{}.Run(r.Notify("error", "Command cannot contain a null character")), nil
		}
		if settings.Mode != mode {
			for _, running := range sm.GetAllRunningApps(sid) {
				if applicationRoot(state, running.Name) != root {
					continue
				}
				for _, panel := range running.Apps {
					if panel.PluginID == "project-command" {
						return r.Result{}.Run(r.Notify("error", "Stop the project's applications before changing application mode")), nil
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
			return r.Result{}.Run(r.Notify("error", err.Error())), nil
		}
		if path != root {
			if err := saveApplicationSettings(path, applicationSettings{Mode: mode, Port: port}); err != nil {
				return r.Result{}.Run(r.Notify("error", err.Error())), nil
			}
		}
		if err := setProjectCommand(path, command); err != nil {
			return r.Result{}.Run(r.Notify("error", "Could not save command: "+err.Error())), nil
		}
		return r.Merge(projectsJS(state), r.Result{}.Run(r.CloseDialog("project-command-dialog"))), nil
	})
	r.RegisterAction(app, "project.command.run", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		state := sm.Get(sid)
		if state.ActiveProject == "" {
			return r.Result{}.Run(r.Notify("error", "Open a project first")), nil
		}
		_, command, _ := applicationConfiguration(state, state.ActiveProject)
		if command == "" {
			return clientScript("libroWorkspace.projectSettings(props[0]);", state.ActiveProject), nil
		}
		hadCommand := false
		for _, panel := range state.Apps {
			if panel.PluginID == "project-command" {
				hadCommand = true
			}
		}
		js, _, err := controlApplication(sid, sm.GetActiveProjectPath(sid), "restart")
		if err != nil {
			return r.Merge(js, r.Result{}.Run(r.Notify("error", err.Error()))), nil
		}
		for _, panel := range sm.Get(sid).Apps {
			if panel.PluginID == "project-command" {
				js = r.Merge(js, navigateJS(sm.Get(sid), sid), clientScript("libroWorkspace.select(props[0]);", panel.ID))
				break
			}
		}
		if hadCommand {
			js = r.Merge(trustedResponse("libroWorkspace.beginProjectRestart();"), js, trustedResponse("libroWorkspace.endProjectRestart();"))
		}
		return js, nil
	})
	r.RegisterAction(app, "project.command.stop", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		js, _, err := controlApplication(sid, sm.GetActiveProjectPath(sid), "stop")
		if err != nil {
			return r.Merge(js, r.Result{}.Run(r.Notify("error", err.Error()))), nil
		}
		return r.Merge(js, r.Result{}.Run(r.Notify("info", "Application stopped"))), nil
	})
}
