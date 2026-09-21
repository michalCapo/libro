package libro

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

const applicationHelp = `Control the application's saved project command in Libro. Actions: status, start, restart, stop.
Use the application MCP tool or: libro application status|start|restart|stop [project-path].
The project defaults to the agent's working directory and must match Libro's active project path.
Start is idempotent. Restart replaces the project command terminal. Stop stops only that terminal.
Set the start command in project settings first. Commands cannot be supplied or changed through this tool.
A starting response means launch was requested; use status to check the process. Running does not guarantee server readiness.
`

// ApplicationCommand sends a project lifecycle command through the desktop bridge.
func ApplicationCommand(command json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Action  string `json:"action"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(command, &args); err != nil {
		return nil, err
	}
	if args.Action != "status" && args.Action != "start" && args.Action != "restart" && args.Action != "stop" {
		return nil, errors.New("unknown application action")
	}
	if args.Project == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		args.Project = cwd
	}
	path, err := filepath.Abs(args.Project)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{"action": "application", "operation": args.Action, "project": path})
	if err != nil {
		return nil, err
	}
	return BrowserCommand(payload)
}

// RunApplicationCLI controls the configured application without MCP support.
func RunApplicationCLI(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" {
		_, err := io.WriteString(out, applicationHelp)
		return err
	}
	if len(args) > 2 {
		return errors.New("expected action and optional project path; see libro application --help")
	}
	command := map[string]string{"action": args[0]}
	if len(args) == 2 {
		command["project"] = args[1]
	}
	data, err := json.Marshal(command)
	if err != nil {
		return err
	}
	result, err := ApplicationCommand(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}

func registerApplicationControl(app *r.App) {
	registerAction(app, "project.command.agent", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		id, _ := data["request"].(string)
		operation, _ := data["operation"].(string)
		project, _ := data["project"].(string)
		js, result, err := controlApplication(sid, project, operation)
		reply := map[string]any{"result": result}
		if err != nil {
			reply["error"] = err.Error()
		}
		encoded, _ := json.Marshal(reply)
		return js + fmt.Sprintf("window.libroWorkspace.applicationResult(%s,%s);", components.JSString(id), encoded)
	})
}

func controlApplication(sid, project, operation string) (string, map[string]any, error) {
	path := sm.GetActiveProjectPath(sid)
	if sm.Get(sid).ActiveProject == "" || project == "" || path == "" || filepath.Clean(project) != filepath.Clean(path) {
		return "", nil, errors.New("switch Libro to the requested project first")
	}
	if operation != "status" && operation != "start" && operation != "restart" && operation != "stop" {
		return "", nil, errors.New("unknown application action")
	}
	command := projectCommand(path)
	status := "stopped"
	for _, app := range sm.Get(sid).Apps {
		if app.PluginID == "project-command" {
			if !app.TerminalReady {
				status = "starting"
			} else if tm.IsRunning(app.ID) {
				status = "running"
			}
		}
	}
	js := ""
	switch operation {
	case "start", "restart":
		if command == "" {
			return "", nil, errors.New("set a start command in project settings first")
		}
		if operation == "restart" || status == "stopped" {
			js = runProjectCommand(sid, command)
			status = "starting"
		}
	case "stop":
		js = stopProjectCommand(sid) + navigateJS(sm.Get(sid), sid) + projectsJS(sm.Get(sid))
		status = "stopped"
	}
	return js, map[string]any{"project": path, "configured": command != "", "status": status}, nil
}
