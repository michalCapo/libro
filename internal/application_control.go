package libro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

const applicationHelp = components.ApplicationInstructions + "\n\n" + `Control the application's saved project command in Libro. Actions: status, start, restart, stop.
Use the application MCP tool or: libro application status|start|restart|stop.
The MCP tool is bound to the workspace where its server was launched; it accepts only an action, never a target project, process ID, or command.
Agent CLI calls are bound to LIBRO_APPLICATION_PATH, set by Libro when launching the agent. A different project path is rejected.
Outside an agent session, the CLI accepts an optional project path and otherwise uses the current directory.
All actions preserve the visible project, thread, and panel selection.
Project settings choose a shared application or one application per thread. Browsers remain independent per thread.
Start is idempotent. Restart and stop affect the selected application scope. Status includes mode, port and URL when assigned.
Per-thread commands run in their worktree and receive PORT. The start command must use that port.
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
	if scope := os.Getenv("LIBRO_APPLICATION_PATH"); scope != "" {
		if args.Project != "" && applicationPath(args.Project) != applicationPath(scope) {
			return nil, errors.New("application control is restricted to this agent's workspace")
		}
		args.Project = scope
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
	return desktopCommand(payload)
}

// scopedApplicationCommand deliberately excludes target selection from the MCP API.
func scopedApplicationCommand(command json.RawMessage, scope string) (json.RawMessage, error) {
	var args struct {
		Action string `json:"action"`
	}
	decoder := json.NewDecoder(bytes.NewReader(command))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, errors.New("application workspace is unavailable; restart the agent from its Libro thread")
	}
	payload, err := json.Marshal(map[string]string{"action": args.Action, "project": scope})
	if err != nil {
		return nil, err
	}
	return ApplicationCommand(payload)
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

func applicationPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// applicationProject resolves the most specific registered root, preserving the
// active thread when several workspaces share the same application directory.
func applicationProject(state *AppState, requested string) (string, string, error) {
	if requested == "" {
		return "", "", errors.New("application project path is required")
	}
	requested = applicationPath(requested)
	name, path, best := "", "", -1
	consider := func(workspace, root string) {
		if root == "" {
			return
		}
		resolved := applicationPath(root)
		relative, err := filepath.Rel(resolved, requested)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return
		}
		if len(resolved) > best || (len(resolved) == best && workspace == state.ActiveProject) {
			name, path, best = workspace, root, len(resolved)
		}
	}
	for _, project := range state.Projects {
		consider(project.Name, project.Path)
	}
	for _, thread := range state.Threads {
		if !thread.Archived {
			consider(thread.ID, thread.Path)
		}
	}
	if name == "" {
		return "", "", fmt.Errorf("no Libro project matches %q; open the project in Libro first", requested)
	}
	return name, path, nil
}

var applicationControlMu sync.Mutex

func controlApplication(sid, project, operation string) (string, map[string]any, error) {
	applicationControlMu.Lock()
	defer applicationControlMu.Unlock()
	if operation != "status" && operation != "start" && operation != "restart" && operation != "stop" {
		return "", nil, errors.New("unknown application action")
	}
	workspace, _, err := applicationProject(sm.Get(sid), project)
	if err != nil {
		return "", nil, err
	}
	path, command, settings := applicationConfiguration(sm.Get(sid), workspace)
	port := settings.Port
	perThread := settings.Mode == "thread"
	if (operation == "start" || operation == "restart") && command == "" {
		return "", nil, errors.New("set a start command in project settings first")
	}
	status := "stopped"
	var previous []string
	for _, running := range sm.GetAllRunningApps(sid) {
		root := applicationRoot(sm.Get(sid), running.Name)
		if perThread && running.Name != workspace || !perThread && (root == "" || applicationPath(root) != applicationPath(path)) {
			continue
		}
		for _, app := range running.Apps {
			if app.PluginID == "project-command" && app.ApplicationPerThread == perThread {
				port = app.ApplicationPort
				previous = append(previous, app.ID)
				if !app.TerminalReady {
					status = "starting"
				} else if tm.IsRunning(app.ID) {
					status = "running"
				}
			}
		}
	}
	if operation == "status" || operation == "start" && status != "stopped" {
		return "", map[string]any{"project": path, "configured": command != "", "status": status, "mode": settings.Mode, "port": port, "url": applicationURL(port)}, nil
	}
	// Reject a conflicting new override before stopping a healthy application.
	if operation != "stop" && perThread && settings.Port != 0 && (len(previous) == 0 || settings.Port != port) {
		if _, err := allocateApplicationPort(settings.Port); err != nil {
			return "", nil, err
		}
	}
	var js strings.Builder
	for _, id := range previous {
		tm.Stop(id)
		sm.RemoveAppByID(sid, id)
		js.WriteString(removeAppJS(id))
	}
	status = "stopped"
	if operation != "stop" {
		if perThread {
			requested := settings.Port
			if requested == 0 {
				requested = port
			}
			port, err = allocateApplicationPort(requested)
			if err != nil && settings.Port == 0 {
				port, err = allocateApplicationPort(0)
			}
			if err != nil {
				return js.String() + projectsJS(sm.Get(sid)), nil, err
			}
		}
		panel, index := sm.insertProjectCommand(sid, workspace, command, port, perThread, path)
		js.WriteString(insertAppJS(renderAppFramePlaceholder(panel, index, false, sid).Attr("data-dock-seen", "true"), false, workspace))
		// Hydration also works when this workspace has never been shown.
		js.WriteString(hydrateAppAfterScrollJS(panel.ID, sidData(sid, "id", panel.ID)))
		status = "starting"
	}
	js.WriteString(projectsJS(sm.Get(sid)))
	return js.String(), map[string]any{"project": path, "configured": command != "", "status": status, "mode": settings.Mode, "port": port, "url": applicationURL(port)}, nil
}

// hydrateProjectCommand starts applications without selecting their workspace.
func hydrateProjectCommand(sid, id string) (string, bool) {
	applicationControlMu.Lock()
	defer applicationControlMu.Unlock()
	for _, workspace := range sm.GetAllRunningApps(sid) {
		for _, panel := range workspace.Apps {
			if panel.ID != id || panel.PluginID != "project-command" {
				continue
			}
			if panel.TerminalReady {
				return "", true
			}
			path := panel.ApplicationPath
			if path == "" {
				_, path = threadProjectContext(sm.Get(sid), workspace.Name)
			}
			var environment []string
			if panel.ApplicationPort != 0 {
				environment = []string{"PORT=" + strconv.Itoa(panel.ApplicationPort)}
			}
			_, err := tm.StartWithEnvironment(id, panel.Command, path, panel.Writable, environment)
			if err != nil {
				sm.RemoveAppByID(sid, id)
				return removeAppJS(id) + r.Notify("error", "Failed to start application: "+err.Error()), true
			}
			if !sm.HydrateTerminalAnywhere(sid, id) {
				tm.Stop(id)
				return "", true
			}
			panel.TerminalReady = true
			return renderAppContent(panel, sid, false, nil).ToJSReplace(appContentID(id)), true
		}
	}
	return "", false
}
