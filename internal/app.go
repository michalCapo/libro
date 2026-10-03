// Package libro implements the Libro server, desktop bundling, and UI components.
package libro

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"libro/internal/components"
	"log"
	"net"
	"net/http"
	"slices"
	"time"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	r "github.com/michalCapo/g-sui/ui"
)

func hydrateAppAfterScrollJS(in actionAppHydrateInput) r.Result {
	return clientScript(`
(function(){
	var appID=props[0];
	var payload=props[1];
	function hydrate(){
		if(typeof __ws!=='undefined'&&__ws.call)__ws.call('app.hydrate',payload);
	}
	requestAnimationFrame(function(){
		var app=document.querySelector('[data-app-id="'+String(appID).replace(/"/g,'\\"')+'"]');
		if(app&&window.__libroScrollToApp)window.__libroScrollToApp(app);
		requestAnimationFrame(hydrate);
	});
})();
`, in.ID, in)
}

func settleHydratedAppContentJS(appID string) r.Result {
	return clientScript(`
(function(){
	var appID=props[0];
	var app=document.querySelector('[data-app-id="'+String(appID).replace(/"/g,'\\"')+'"]');
	function settle(){
		if(app&&window.__libroScrollToApp)window.__libroScrollToApp(app);
		var termFrame=document.querySelector('[data-terminal-app="'+String(appID).replace(/"/g,'\\"')+'"]');
		if(termFrame&&window.__libroFitTerminalFrame)window.__libroFitTerminalFrame(termFrame);
		if((window.__libroSelectedApp||'')===appID&&window.__libroFocusAppByID)window.__libroFocusAppByID(appID);
	}
	requestAnimationFrame(function(){settle();requestAnimationFrame(settle);});
})();
`, appID)
}

func reparentProjectAppsJS(apps []Application, project string) r.Result {
	if len(apps) == 0 {
		return r.Result{}
	}
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		ids = append(ids, app.ID)
	}
	return clientScript(`
(function(){
	var grid=document.getElementById(props[0]);
	if(!grid)return;
	props[1].forEach(function(id){
		var frame=document.getElementById('frame-'+id);
		if(frame)grid.appendChild(frame);
	});
})();`, stripID(project), ids)
}

func stateWithoutApps(state *AppState, removed []Application) *AppState {
	copy := *state
	ids := make(map[string]bool, len(removed))
	for _, app := range removed {
		ids[app.ID] = true
	}
	copy.Apps = nil
	for _, app := range state.Apps {
		if !ids[app.ID] {
			copy.Apps = append(copy.Apps, app)
		}
	}
	if len(copy.Apps) == 0 {
		copy.SelectedIndex = 0
	} else if copy.SelectedIndex >= len(copy.Apps) {
		copy.SelectedIndex = len(copy.Apps) - 1
	}
	return &copy
}

func switchToProjectName(sid, name string) r.Result {
	result, _ := switchToProjectNameChecked(sid, name)
	return result
}

func switchToProjectNameChecked(sid, name string) (r.Result, bool) {
	if name == "" {
		return r.Result{}, false
	}

	prevState := sm.Get(sid)
	closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)
	targetRendered := prevState.renderedProjects[name]

	// Worktrees can exist before their virtual project has been created
	// in the current session. Resolve the matching worktree lazily.
	restoreWorktreeProject(sm, sid, name)

	movedProjectApps := sm.MoveSharedProjectApps(sid, name)
	if !sm.SwitchProject(sid, name) {
		return r.Result{}, false
	}

	state := sm.Get(sid)

	var jsSwitch r.Result
	if targetRendered {
		// Project div exists in DOM, just hide/show
		jsSwitch = r.Merge(switchProjectJS(name, nil), reparentProjectAppsJS(movedProjectApps, name))
	} else {
		// Project div doesn't exist yet, append new content and hide old
		contentState := state
		if len(movedProjectApps) > 0 {
			contentState = stateWithoutApps(state, movedProjectApps)
		}
		jsSwitch = r.Merge(switchProjectJS(name, renderMainArea(contentState, sid)), reparentProjectAppsJS(movedProjectApps, name))
	}
	sm.IsProjectRendered(sid, name)

	resp := r.Result{}.
		Add(projectsJS(state)).
		Morph(TopBarID, renderTopBar(state, sid)).
		Add(closeDevtoolsJS).
		Add(jsSwitch).
		Add(navigateJS(state, sid)).
		Add(updateHashJS(name)).
		Add(projectAutolaunchJS(state, sid)).
		Add(focusSelectedAppJS(state))
	return resp, true
}

// Closing a thread's agent archives the thread and closes thread-local tools.
// Notes and the project command remain available to the other project threads.
func closeWorkspaceApp(sid, appID string) r.Result {
	target := sm.Get(sid).adjacentProjectThread()
	apps, err := sm.CloseThreadAgent(sid, appID)
	if err != nil {
		return r.Result{}.Run(r.Notify("error", "Could not archive thread"))
	}
	closedThread := apps != nil
	if apps == nil {
		if removed := sm.RemoveAppByID(sid, appID); removed != nil {
			apps = []Application{*removed}
		}
	}
	js := closeDevtoolsForAppsJS(apps)
	for _, app := range apps {
		if app.Type == AppTypeTerminal {
			tm.Stop(app.ID)
		}
		js = r.Merge(js, removeAppJS(app.ID))
	}
	if len(apps) == 0 {
		js = removeAppJS(appID)
	}
	if closedThread && target != "" {
		return r.Merge(js, switchToProjectName(sid, target))
	}
	state := sm.Get(sid)
	return r.Merge(js, trustedResponse("if(window.libroWorkspace)libroWorkspace.restorePanelFocus();"), r.Result{}.Morph(TopBarID, renderTopBar(state, sid)), projectsJS(state))
}

// closeOtherPanels closes every tool panel in the active workspace except the
// selected one. Thread agents and shared project panels stay open: closing an
// agent would archive its thread and shared panels belong to the whole project.
func closeOtherPanels(sid, keepID string) r.Result {
	state := sm.Get(sid)
	if len(state.Apps) == 0 {
		return r.Result{}
	}
	if keepID == "" {
		index := state.SelectedIndex
		if index < 0 || index >= len(state.Apps) {
			index = 0
		}
		keepID = state.Apps[index].ID
	}
	ids := make([]string, 0, len(state.Apps))
	for _, app := range state.Apps {
		if app.ID == keepID || appDock(app) == "center" || isSharedProjectApp(app) {
			continue
		}
		ids = append(ids, app.ID)
	}
	var closed []Application
	for _, id := range ids {
		if removed := sm.RemoveAppByID(sid, id); removed != nil {
			closed = append(closed, *removed)
		}
	}
	if len(closed) == 0 {
		return r.Result{}
	}
	var js r.Result

	js = js.Add(closeDevtoolsForAppsJS(closed))
	for _, app := range closed {
		if app.Type == AppTypeTerminal {
			tm.Stop(app.ID)
		}
		js = js.Add(removeAppJS(app.ID))
	}
	state = sm.Get(sid)
	return r.Merge(js, trustedResponse("if(window.libroWorkspace)libroWorkspace.restorePanelFocus();"), r.Result{}.Morph(TopBarID, renderTopBar(state, sid)), projectsJS(state))
}

// agentLaunch resumes a thread's saved agent session and keeps reporting new
// session IDs. Other terminals run their own command.
func agentLaunch(sid string, term Application) (string, func(string)) {
	if !isAgentApp(term) {
		return term.Command, nil
	}
	// Session reports write these fields under the same lock.
	sm.mu.RLock()
	state := sm.states[sid]
	var thread Thread
	found := false
	if state != nil {
		if t := state.thread(state.ActiveProject); t != nil {
			thread, found = *t, true
		}
	}
	sm.mu.RUnlock()
	if !found {
		return term.Command, nil
	}
	threadID := thread.ID
	command, baseCommand := term.Command, term.Command
	if thread.SessionID != "" && thread.AgentID == term.PluginID {
		baseCommand = thread.AgentCommand
		command = components.ResumeAgentCommand(baseCommand, thread.SessionID)
	}
	return command, func(id string) {
		sm.saveThreadSession(sid, threadID, term.ID, term.PluginID, baseCommand, id)
	}
}

// Autolaunch starts the thread's agent when a thread has no agent panel.
func projectAutolaunchJS(state *AppState, sid string) r.Result {
	plugin := projectAutolaunchPlugin(state)
	if plugin == nil {
		return r.Result{}
	}
	return r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Type: string(plugin.Type), Plugin: plugin.ID, Name: plugin.Name, Dock: "center", Writable: new(true), AutolaunchProject: new(state.ActiveProject)}))
}

func projectAutolaunchPlugin(state *AppState) *Plugin {
	thread := state.thread(state.ActiveProject)
	if thread == nil || thread.Managed || slices.ContainsFunc(state.Apps, isAgentApp) {
		return nil
	}
	agent := thread.AgentID
	if agent == "" {
		agent = defaultThreadAgent()
	}
	for _, plugin := range plugins() {
		if plugin.ID == agent && !plugin.Disabled && !plugin.Removed && plugin.Dock == "center" && plugin.Type == AppTypeTerminal {
			return &plugin
		}
	}
	return nil
}

// finalizeProjectCreate registers the project, optionally persists it, and
// returns the JS that switches to it and dismisses the dialog.
func finalizeProjectCreate(sid, path, name string, transient bool) r.Result {
	if !sm.AddProjectWithOptions(sid, name, path, transient) {
		return r.Result{}.Run(r.Notify("error", "Project '"+name+"' already exists"))
	}

	if !transient {
		DBSaveProject(name, path)
	}

	sm.CloseProjectDialog(sid)
	sm.SwitchProject(sid, name)
	sm.IsProjectRendered(sid, name)
	state := sm.Get(sid)

	jsSwitch := switchProjectJS(name, renderMainArea(state, sid))

	resp := r.Result{}.
		Add(projectsJS(state)).
		Morph(TopBarID, renderTopBar(state, sid)).
		Replace(ProjectDialogID, renderProjectDialog(false, sid)).
		Add(trustedResponse(`if(window.__libroProjectDialogBind)window.__libroProjectDialogBind();`)).
		Add(jsSwitch).
		Add(updateHashJS(name)).
		Add(projectAutolaunchJS(state, sid)).
		Add(focusSelectedAppJS(state))
	if transient {
		resp = resp.Add(showToastJS("Opened folder", path, "success"))
	}
	return resp
}

type projectDirLookupMatch struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Parent bool   `json:"parent,omitempty"`
}

func expandUserPath(path string) string {
	home, _ := os.UserHomeDir()
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") && home != "" {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	// In the project picker, ./ is a home-relative shorthand. It lets users
	// browse ~/code by typing ./code without leaving project-search mode for
	// ordinary terms like "nisa".
	if strings.HasPrefix(path, "./") && home != "" {
		return filepath.Join(home, strings.TrimPrefix(path, "./"))
	}
	return path
}

func projectDirLookup(ctx context.Context, query string) []projectDirLookupMatch {
	query = strings.TrimSpace(query)
	if query == "" || ctx.Err() != nil {
		return nil
	}

	home, _ := os.UserHomeDir()
	children := func(dir, prefix string) []projectDirLookupMatch {
		dir = filepath.Clean(expandUserPath(dir))
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var out []projectDirLookupMatch
		prefix = strings.ToLower(prefix)
		for _, e := range entries {
			if ctx.Err() != nil {
				break
			}
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			if prefix != "" && !strings.HasPrefix(strings.ToLower(name), prefix) {
				continue
			}
			out = append(out, projectDirLookupMatch{Name: name, Path: filepath.Join(dir, name)})
			if len(out) >= 80 {
				break
			}
		}
		if prefix == "" {
			if parent := filepath.Dir(dir); parent != dir {
				out = append([]projectDirLookupMatch{{Name: "..", Path: parent, Parent: true}}, out...)
			}
		}
		return out
	}

	expanded := expandUserPath(query)
	isExplicitPath := filepath.IsAbs(expanded) || strings.HasPrefix(query, "~/") || query == "~" || strings.HasPrefix(query, "./") || strings.HasPrefix(query, "../")
	if isExplicitPath {
		if strings.HasSuffix(query, string(os.PathSeparator)) {
			return children(expanded, "")
		}
		if expanded == home && (query == "~" || query == "./") {
			return children(expanded, "")
		}
		return children(filepath.Dir(expanded), filepath.Base(expanded))
	}

	target := strings.ToLower(query)
	roots := []string{}
	if home != "" {
		roots = append(roots, filepath.Join(home, "code"), home)
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	seenRoot := map[string]bool{}
	seen := map[string]bool{}
	var out []projectDirLookupMatch
	for _, root := range roots {
		if ctx.Err() != nil || len(out) >= 80 {
			break
		}
		root = filepath.Clean(root)
		if seenRoot[root] {
			continue
		}
		seenRoot[root] = true
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		rootDepth := len(strings.Split(strings.Trim(filepath.Clean(root), string(os.PathSeparator)), string(os.PathSeparator)))
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if ctx.Err() != nil || len(out) >= 80 {
				return filepath.SkipAll
			}
			if err != nil {
				return nil
			}
			name := d.Name()
			if path != root && d.IsDir() {
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
					return filepath.SkipDir
				}
				depth := len(strings.Split(strings.Trim(filepath.Clean(path), string(os.PathSeparator)), string(os.PathSeparator))) - rootDepth
				if depth > 5 {
					return filepath.SkipDir
				}
				if strings.Contains(strings.ToLower(name), target) || fuzzyMatchPath(name, target) {
					clean := filepath.Clean(path)
					if !seen[clean] {
						seen[clean] = true
						out = append(out, projectDirLookupMatch{Name: name, Path: clean})
					}
				}
			}
			return nil
		})
	}
	return out
}

func fuzzyMatchPath(text, query string) bool {
	text = strings.ToLower(text)
	query = strings.ToLower(query)
	if query == "" {
		return true
	}
	j := 0
	for i := 0; i < len(text) && j < len(query); i++ {
		if text[i] == query[j] {
			j++
		}
	}
	return j == len(query)
}

func projectNameForPath(sid, path string) (string, bool) {
	base := filepath.Base(path)
	if base == "" || base == "." || base == "/" {
		return "", false
	}
	parent := filepath.Base(filepath.Dir(path))
	name := base
	if parent != "" && parent != "." && parent != string(os.PathSeparator) {
		name = parent + "/" + base
	}
	state := sm.Get(sid)
	used := make(map[string]bool, len(state.Projects))
	for _, p := range state.Projects {
		used[p.Name] = true
	}
	if !used[name] {
		return name, true
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !used[candidate] {
			return candidate, true
		}
	}
	return "", false
}

var (
	sm                  = NewStateManager()
	tm                  = components.NewTerminalManager()
	shutdownCleanupOnce sync.Once
	signalHandlerOnce   sync.Once
)

// CleanupRuntime tears down terminal backends and language servers.
func CleanupRuntime() {
	shutdownCleanupOnce.Do(func() {
		tm.StopAll()
		closeNavigationServers()
	})
}

func installShutdownSignalHandler() {
	signalHandlerOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			sig := <-ch
			log.Printf("libro: received %s, cleaning up terminal sessions", sig)
			CleanupRuntime()
			CloseDB()
			signal.Stop(ch)
			os.Exit(0)
		}()
	})
}

// Run initializes and starts the Libro application server.
func Run(assets embed.FS, desktop bool) error {
	listener, err := net.Listen("tcp", ":"+Port())
	if err != nil {
		return fmt.Errorf("start Libro on port %s: %w", Port(), err)
	}
	defer func() { _ = listener.Close() }()

	installShutdownSignalHandler()
	InitDB()
	defer CloseDB()
	defer CleanupRuntime()
	app := r.NewApp()
	app.Title = "Libro"
	if name := os.Getenv("LIBRO_INSTANCE"); name != "" {
		app.Title += " (" + name + ")"
	}
	app.Description = "Application Manager"
	app.Assets(assets, "assets", "/assets/")
	app.Favicon = "/assets/logo.svg"
	registerWidgets(app)

	registerSettingsActions(app)
	registerFilesActions(app)
	registerNotesActions(app)
	registerVoiceRoutes(app)
	r.RegisterAction(app, "app.notify", func(_ *r.Context, in actionAppNotifyInput) (r.Result, error) {
		title := in.Title
		subtitle := in.Subtitle
		variant := in.Variant
		return showToastJS(title, subtitle, variant), nil
	})
	// Open add dialog
	r.RegisterAction(app, "app.dialog.open", func(_ *r.Context, in sessionInput) (r.Result, error) {
		return trustedResponse(`if(window.libroWorkspace)libroWorkspace.launcher();`), nil
	})
	// Quick browse - open URL or Google search

	// Open Neovim if available, otherwise fall back to Vim; notify if neither exists.
	r.RegisterAction(app, "app.nvim.open", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		cmd := ""
		name := ""
		if _, err := exec.LookPath("nvim"); err == nil {
			cmd = "nvim"
			name = "nvim"
		} else if _, err := exec.LookPath("vim"); err == nil {
			cmd = "vim"
			name = "vim"
		}
		if cmd == "" {
			return showToastJS("Editor not installed", "Install nvim or vim to use ⌘/Win+E", "error"), nil
		}
		return r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Type: "terminal", Command: cmd, Writable: new(true), Name: name, Side: "right"})), nil
	})
	// Open the Pi coding agent if available.
	r.RegisterAction(app, "app.pi.open", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		if _, err := exec.LookPath("pi"); err != nil {
			return showToastJS("Pi agent not installed", "Install pi to use ⌘/Win+Y", "error"), nil
		}
		return r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Type: "terminal", Command: "pi", Writable: new(true), Name: "pi", Side: "right"})), nil
	})
	r.RegisterAction(app, "plugin.open", func(_ *r.Context, in actionPluginOpenInput) (r.Result, error) {
		sid := inputSID(in.SID)
		id := in.Plugin
		dock := in.Dock
		for _, p := range plugins() {
			if p.ID != id {
				continue
			}
			p.Command = agentCommand(p)
			if p.Command != "" {
				if _, err := exec.LookPath(extractBaseCmd(p.Command)); err != nil {
					return showToastJS(p.Name+" is not installed", "Install "+extractBaseCmd(p.Command)+" and try again.", "error"), nil
				}
			}
			if !validDock(dock) {
				dock = p.Dock
			}
			return r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Type: string(p.Type), Command: p.Command, Url: p.URL, Name: p.Name, Plugin: p.ID, Dock: dock, Writable: new(true)})), nil
		}
		return r.Result{}.Run(r.Notify("error", "Plugin not found")), nil
	})
	r.RegisterAction(app, "app.dock", func(_ *r.Context, in actionAppDockInput) (r.Result, error) {
		sid := inputSID(in.SID)
		id := in.ID
		dock := in.Dock
		if !validDock(dock) || dock == "bottom" {
			return r.Result{}, nil
		}
		state := sm.Get(sid)
		for _, a := range state.Apps {
			if a.ID == id {
				if dock == "center" && !isAgentApp(a) {
					return r.Result{}.Run(r.Notify("error", "The main area is only for agents")), nil
				}
				sm.SetAppPlugin(sid, id, a.PluginID, dock)
				return r.Merge(r.Result{}.Run(r.SetAttr("frame-"+id, "data-dock", dock)), clientScript("if(window.libroWorkspace)libroWorkspace.select(props[0]);", id)), nil
			}
		}
		return r.Result{}, nil
	})
	// Start an application instance.
	actionAppStart = r.RegisterAction(app, "app.start", func(_ *r.Context, in actionAppStartInput) (r.Result, error) {
		sid := inputSID(in.SID)
		if project, ok := inputField(in.AutolaunchProject); ok {
			state := sm.Get(sid)
			if state.ActiveProject != project || projectAutolaunchPlugin(state) == nil {
				return r.Result{}, nil
			}
		}
		editorPath := ""
		if path, editing := inputField(in.EditorFile); editing {
			panel := in.FilesPanel

			if !slices.ContainsFunc(sm.Get(sid).Apps, func(a Application) bool { return a.ID == panel && a.PluginID == "files" }) {
				return r.Result{}.Run(r.Notify("error", "Open Files in this workspace first")), nil
			}
			var editor *Plugin
			editorID := editorToolID()
			for _, p := range plugins() {
				if p.ID == editorID && editorTool(p) {
					editor = &p
					break
				}
			}
			if editor == nil {
				return r.Result{}.Run(r.Notify("error", "Select an enabled file editor in Settings")), nil
			}
			parents := in.Parents

			if parents < 0 || parents > 1024 {
				parents = 0
			}
			var err error
			editorPath, err = projectFileToOpen(filesRoot(sm.GetActiveProjectPath(sid), int(parents)), path)
			if err != nil {
				return r.Result{}.Run(r.Notify("error", err.Error())), nil
			}
			in.Type, in.Plugin, in.Dock, in.Name = "terminal", editor.ID, "right", editor.Name
		}
		appType := in.Type
		name := in.Name
		side := in.Side
		pluginID := in.Plugin
		if pluginID == "" && appType == "terminal" {
			command := in.Command

			candidate := pluginForApp(Application{Type: AppTypeTerminal, Command: command})
			if candidate.Dock == "center" && strings.TrimSpace(command) == candidate.Command {
				pluginID = candidate.ID
			}
		}
		dock := in.Dock
		if pluginID != "" {
			var plugin *Plugin
			for _, candidate := range plugins() {
				if candidate.ID == pluginID {
					matched := candidate
					plugin = &matched
					break
				}
			}
			if plugin == nil {
				return r.Result{}.Run(r.Notify("error", "Plugin not found")), nil
			}
			if plugin.Disabled || plugin.Removed {
				return r.Result{}.Run(r.Notify("error", "This agent is disabled in Settings")), nil
			}
			plugin.Command = agentCommand(*plugin)
			if plugin.Type == AppTypeTerminal {
				in.Command = plugin.Command
			}
			if plugin.Command != "" {
				if _, err := exec.LookPath(extractBaseCmd(plugin.Command)); err != nil {
					return showToastJS(plugin.Name+" is not installed", "Install "+extractBaseCmd(plugin.Command)+" and try again.", "error"), nil
				}
			}
		}
		command := in.Command
		threadState := sm.Get(sid)
		candidate := Application{Type: AppType(appType), Command: command, PluginID: pluginID, Dock: dock}
		var replacedJS r.Result
		autolaunch := in.AutolaunchProject != nil
		replaceAgent := in.ReplaceAgent
		if !autolaunch && isAgentApp(candidate) && threadState.thread(threadState.ActiveProject) != nil && (replaceAgent || !slices.ContainsFunc(threadState.Apps, isAgentApp)) {
			removed, err := sm.ReplaceThreadAgent(sid, pluginID, command)
			if err != nil {
				return r.Result{}.Run(r.Notify("error", "Could not replace agent")), nil
			}
			for _, existing := range removed {
				tm.Stop(existing.ID)
				replacedJS = replacedJS.Add(removeAppJS(existing.ID))
			}
		}
		if threadState.needsProjectThread(candidate) {
			return r.Result{}.Run(actionThreadCreate.Call(actionThreadCreateInput{SID: sid, Agent: pluginID, Project: threadState.ActiveProject})), nil
		}
		// A new agent in a project checkout or worktree starts a fresh
		// conversation, so it drops the previous agent's description.
		if isAgentApp(candidate) && threadState.thread(threadState.ActiveProject) == nil {
			if _, path := threadProjectContext(threadState, threadState.ActiveProject); path != "" {
				_ = saveWorktreeTitle(path, "")
			}
		}
		if threadState.thread(threadState.ActiveProject) != nil {
			if !threadState.canStartThreadApp(candidate) {
				return r.Result{}, nil
			}
		}
		width := defaultAppWidth(Application{Type: AppType(appType), Command: command, PluginID: pluginID})
		if val, ok := inputField(in.Width); ok && val != "" {
			width = Width(val)
		}
		if dock == "center" && !isAgentApp(Application{Type: AppType(appType), Command: command, PluginID: pluginID}) {
			return r.Result{}.Run(r.Notify("error", "The main area is only for agents. Add other agents as agent plugins.")), nil
		}
		if dock == "bottom" {
			width = WidthFull
			if appType != "terminal" || (pluginID != "" && pluginID != "terminal") {
				return r.Result{}.Run(r.Notify("error", "The bottom panel only supports a shell terminal")), nil
			}
			in.Command = ""
			name = "Terminal"
			for _, existing := range sm.Get(sid).Apps {
				if appDock(existing) == "bottom" {
					return clientScript("if(window.libroWorkspace)libroWorkspace.select(props[0]);", existing.ID), nil
				}
			}
		}
		// Compute insertion index relative to currently selected app
		insertIdx := -1 // default: append
		switch side {
		case "left":
			insertIdx = sm.SelectedIndex(sid)
		case "right":
			insertIdx = sm.SelectedIndex(sid) + 1
		}
		pwd := sm.GetActiveProjectPath(sid)
		if appType == "terminal" {
			command := in.Command

			command = strings.TrimSpace(command)
			if command == "" {
				command = components.UserShellBase()
			}
			command = strings.ReplaceAll(command, "__dir__", pwd)
			if editorPath != "" {
				command = components.CommandWithFile(command, editorPath)
			}

			writable := true
			if val, ok := inputField(in.Writable); ok {
				writable = val
			}

			iconURL := in.IconUrl

			// Check if strip already exists
			stateBefore := sm.Get(sid)
			hadApps := len(stateBefore.Apps)

			appID := sm.NextAppID()
			sm.InsertTerminalPlaceholder(sid, appID, width, command, writable, name, iconURL, insertIdx)
			sm.SetAppPlugin(sid, appID, pluginID, dock)

			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]

			topBarJS := r.Result{}.Morph(TopBarID, renderTopBar(state, sid))
			projJS := projectsJS(state)
			hydrateJS := hydrateAppAfterScrollJS(actionAppHydrateInput{SID: sid, ID: newApp.ID})
			if hadApps > 0 {
				frame := renderAppFramePlaceholder(*newApp, state.SelectedIndex, true, sid)
				return r.Merge(replacedJS, insertAppJS(frame, false, state.ActiveProject), navigateJS(state, sid), topBarJS, projJS, hydrateJS), nil
			}

			return r.Merge(replacedJS, r.Result{}.Morph(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, newApp.ID)), topBarJS, projJS, navigateJS(state, sid), hydrateJS), nil
		}

		// URL app
		url := in.Url
		url = strings.TrimSpace(url)
		if url != "" {
			url = strings.ReplaceAll(url, "__dir__", pwd)
			url = ensureScheme(url)
		}

		// Check if strip already exists
		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)
		sm.InsertApp(sid, url, width, name, insertIdx)
		state := sm.Get(sid)
		sm.SetAppPlugin(sid, state.Apps[state.SelectedIndex].ID, pluginID, dock)
		state = sm.Get(sid)
		topBarJS := r.Result{}.Morph(TopBarID, renderTopBar(state, sid))
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(actionAppHydrateInput{SID: sid, ID: state.Apps[state.SelectedIndex].ID, OpenURL: state.Apps[state.SelectedIndex].URL == ""})
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid)
			return r.Merge(insertAppJS(frame, false, state.ActiveProject), navigateJS(state, sid), topBarJS, projJS, hydrateJS), nil
		}
		return r.Merge(r.Result{}.Morph(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID)), topBarJS, projJS, navigateJS(state, sid), hydrateJS), nil
	})
	r.RegisterAction(app, "app.hydrate", func(_ *r.Context, in actionAppHydrateInput) (r.Result, error) {
		sid := inputSID(in.SID)
		appID := in.ID
		if appID == "" {
			return r.Result{}, nil
		}
		if js, handled := hydrateProjectCommand(sid, appID); handled {
			return js, nil
		}
		state := sm.Get(sid)
		idx := -1
		for i := range state.Apps {
			if state.Apps[i].ID == appID {
				idx = i
				break
			}
		}
		// The app may live in a non-active project's snapshot (its DOM div is
		// hidden but still in the page). Use the broader lookup so we can
		// start the pty regardless of which project the user currently has open.
		if idx < 0 {
			if sm.HydrateTerminalAnywhere(sid, appID) {
				// HydrateTerminalAnywhere marks the app TerminalReady; the
				// subsequent render below needs state.Apps, so re-scan.
				state = sm.Get(sid)
				for i := range state.Apps {
					if state.Apps[i].ID == appID {
						idx = i
						break
					}
				}
			}
		}
		if idx < 0 {
			return r.Result{}, nil
		}
		if state.Apps[idx].Type == AppTypeTerminal && !state.Apps[idx].TerminalReady {
			term := state.Apps[idx]
			pwd := sm.GetActiveProjectPath(sid)
			var environment []string
			if isAgentApp(term) {
				environment = append(agentEnvironmentList(), "LIBRO_APPLICATION_PATH="+pwd)
			}
			command, reportSession := agentLaunch(sid, term)
			session, err := tm.StartWithSessionReporter(term.ID, command, pwd, term.Writable, environment, reportSession)
			if err != nil {
				sm.RemoveAppByID(sid, term.ID)
				state = sm.Get(sid)
				return r.Merge(removeAppJS(term.ID), navigateJS(state, sid), r.Result{}.Morph(TopBarID, renderTopBar(state, sid)), projectsJS(state), r.Result{}.Run(r.Notify("error", "Failed to start terminal: "+err.Error()))), nil
			}
			if !sm.HydrateTerminalByID(sid, term.ID, session.ID) {
				tm.Stop(term.ID)
				return r.Result{}.Run(r.Notify("error", "Terminal placeholder disappeared")), nil
			}
			state = sm.Get(sid)
			for i := range state.Apps {
				if state.Apps[i].ID == appID {
					idx = i
					break
				}
			}
		}
		openURL := in.OpenURL
		return r.Merge(
			// Preserve already hydrated content when a duplicate response arrives.
			clientScript(`var content=document.getElementById(props[0]);if(!content||!content.querySelector('[data-app-placeholder]')){if(content&&!content.hasAttribute('data-gsui-preserve')){content.__libroSkipHydrate=true;content.setAttribute('data-gsui-preserve','');}return;}var popup=document.getElementById(props[1]);var input=document.getElementById('url-popup-input');if(popup&&content.contains(popup)&&!popup.classList.contains('hidden')){popup.__libroHydrateValue=input?input.value:'';}if(window.__libroParkFloatingPopups)window.__libroParkFloatingPopups();`, appContentID(appID), URLPopupID),
			r.Result{}.Morph(appContentID(appID), renderAppContent(state.Apps[idx], sid, false, nil)),
			clientScript(`var content=document.getElementById(props[2]);if(content&&content.__libroSkipHydrate){delete content.__libroSkipHydrate;content.removeAttribute('data-gsui-preserve');}var popup=document.getElementById(props[1]);if(popup&&popup.__libroHydrateValue!==undefined){var value=popup.__libroHydrateValue;delete popup.__libroHydrateValue;setTimeout(function(){if(window.__libroOpenURLPopupFor)window.__libroOpenURLPopupFor(props[0],value);},30);}`, appID, URLPopupID, appContentID(appID)),
			settleHydratedAppContentJS(appID),
			clientScript(`requestAnimationFrame(function(){requestAnimationFrame(function(){if(props[0]&&window.__libroSelectedApp===props[1]&&window.__libroOpenURLPopupFor)window.__libroOpenURLPopupFor(props[1],'');});});`, openURL, appID)), nil
	})
	// Close/remove application
	actionAppClose = r.RegisterAction(app, "app.close", func(_ *r.Context, in actionAppCloseInput) (r.Result, error) {
		sid := inputSID(in.SID)
		appID := in.ID
		if appID == "" {
			return r.Result{}, nil
		}
		return closeWorkspaceApp(sid, appID), nil
	})
	// Close every panel except the selected one (thread agents and shared panels stay).
	r.RegisterAction(app, "app.close.others", func(_ *r.Context, in actionAppCloseOthersInput) (r.Result, error) {
		sid := inputSID(in.SID)
		keepID := in.ID
		return closeOtherPanels(sid, keepID), nil
	})
	r.RegisterAction(app, "project.close.check", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		state := sm.Get(sid)
		return showCloseDialogJS([]ProjectApps{{Name: workspaceProjectLabel(state), Apps: state.Apps}},
			"Close project?", "Close project", "project.close", sid), nil
	})
	// Close current (selected) app — no app ID needed from client
	r.RegisterAction(app, "app.close.current", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return r.Result{}, nil
		}
		appID := state.Apps[state.SelectedIndex].ID
		return closeWorkspaceApp(sid, appID), nil
	})
	// Emergency restart for a terminal app's native PTY session.
	r.RegisterAction(app, "app.terminal.restart", func(_ *r.Context, in actionAppTerminalRestartInput) (r.Result, error) {
		sid := inputSID(in.SID)
		appID := in.ID
		if appID == "" {
			return r.Result{}.Run(r.Notify("error", "No terminal app selected")), nil
		}
		state := sm.Get(sid)
		var term *Application
		for i := range state.Apps {
			if state.Apps[i].ID == appID {
				term = &state.Apps[i]
				break
			}
		}
		if term == nil || term.Type != AppTypeTerminal || term.Command == "" || !term.TerminalReady {
			return r.Result{}.Run(r.Notify("error", "Selected app is not a running terminal")), nil
		}
		if term.PluginID == "project-command" {
			path := term.ApplicationPath
			if path == "" {
				path = sm.GetActiveProjectPath(sid)
			}
			js, _, err := controlApplication(sid, path, "restart")
			if err != nil {
				return r.Merge(js, r.Result{}.Run(r.Notify("error", err.Error()))), nil
			}
			return js, nil
		}
		pwd := sm.GetActiveProjectPath(sid)
		var environment []string
		if isAgentApp(*term) {
			environment = append(agentEnvironmentList(), "LIBRO_APPLICATION_PATH="+pwd)
		}
		// Stopping saves the final session ID, which the resume command needs.
		tm.Stop(term.ID)
		command, reportSession := agentLaunch(sid, *term)
		if _, err := tm.StartWithSessionReporter(term.ID, command, pwd, term.Writable, environment, reportSession); err != nil {
			return r.Result{}.Run(r.Notify("error", "Failed to restart terminal: "+err.Error())), nil
		}
		// Without a thread record there is no session to resume, so the
		// restarted agent starts fresh and drops the old description.
		var js r.Result
		if isAgentApp(*term) && state.thread(state.ActiveProject) == nil && saveWorktreeTitle(pwd, "") == nil {
			js = projectsJS(state)
		}
		return r.Merge(js, clientScript("(function(){if(window.__libroRestartTerminal)window.__libroRestartTerminal(props[0]);})();", term.ID), settleAppFrameJS(term.ID), r.Result{}.Run(r.Notify("success", "Terminal restarted"))), nil
	})
	// Navigate left - JS-only update to preserve iframes
	r.RegisterAction(app, "app.navigate.left", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		sm.NavigateLeft(sid)
		state := sm.Get(sid)
		return r.Merge(navigateJS(state, sid), updateAppPreviewJS(state)), nil
	})
	// Navigate right - JS-only update to preserve iframes
	r.RegisterAction(app, "app.navigate.right", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		sm.NavigateRight(sid)
		state := sm.Get(sid)
		return r.Merge(navigateJS(state, sid), updateAppPreviewJS(state)), nil
	})
	// Move app left — swap with neighbor, JS-only DOM swap to preserve iframes
	r.RegisterAction(app, "app.move.left", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		if !sm.MoveAppLeft(sid) {
			return r.Result{}, nil
		}
		state := sm.Get(sid)
		return r.Merge(moveAppJS(state, sid, "left"), r.Result{}.Morph(TopBarID, renderTopBar(state, sid))), nil
	})
	// Move app right — swap with neighbor, JS-only DOM swap to preserve iframes
	r.RegisterAction(app, "app.move.right", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		if !sm.MoveAppRight(sid) {
			return r.Result{}, nil
		}
		state := sm.Get(sid)
		return r.Merge(moveAppJS(state, sid, "right"), r.Result{}.Morph(TopBarID, renderTopBar(state, sid))), nil
	})
	// Move selected app to another project, then activate that project.
	r.RegisterAction(app, "app.move.to.project", func(_ *r.Context, in actionAppMoveToProjectInput) (r.Result, error) {
		sid := inputSID(in.SID)
		target := in.Target
		kind := in.Kind
		parentProject := in.Project
		wtPath := in.Path
		branch := in.Branch
		if target == "" {
			return r.Result{}.Run(r.Notify("error", "Project not found")), nil
		}
		if kind == "worktree" && parentProject != "" && wtPath != "" && branch != "" {
			sm.AddVirtualProject(sid, target, wtPath, parentProject)
		}
		prevState := sm.Get(sid)
		if len(prevState.Apps) == 0 || prevState.SelectedIndex < 0 || prevState.SelectedIndex >= len(prevState.Apps) {
			return r.Result{}.Run(r.Notify("error", "No selected app")), nil
		}
		sourceProject := prevState.ActiveProject
		appID := prevState.Apps[prevState.SelectedIndex].ID
		targetHadApps := projectHasRunningApps(prevState, target)
		targetRenderedBefore := sm.IsProjectRendered(sid, target)
		if sourceProject == target {
			return focusSelectedAppJS(prevState), nil
		}
		moved, ok := sm.MoveSelectedAppToProject(sid, target)
		if !ok || moved == nil {
			return r.Result{}.Run(r.Notify("error", "Project not found")), nil
		}
		state := sm.Get(sid)
		var js r.Result
		js = js.Add(closeDevtoolsForAppJS(appID))
		if sourceSnap, ok := state.snapshots[sourceProject]; ok && sourceSnap != nil && len(sourceSnap.Apps) > 0 {
			js = js.Add(removeAppJS(appID))
			js = js.Add(navigateProjectJS(sourceProject, sourceSnap.Apps, sourceSnap.SelectedIndex, sid))
		} else {
			js = js.Add(trustedResponse(parkFloatingPopupsJS()))
			js = js.Add(r.Result{}.Morph(projectMainID(sourceProject), renderMainAreaForProject(state, sid, sourceProject)))
		}
		if targetRenderedBefore {
			if targetHadApps {
				frame := renderAppFrame(*moved, state.SelectedIndex, true, sid)
				js = js.Add(insertAppJS(frame, false, target))
				js = js.Add(switchProjectJS(target, nil))
				js = js.Add(navigateJS(state, sid))
			} else {
				js = js.Add(r.Result{}.Morph(projectMainID(target), renderMainArea(state, sid)))
				js = js.Add(switchProjectJS(target, nil))
			}
		} else {
			js = js.Add(switchProjectJS(target, renderMainArea(state, sid)))
		}
		return r.Result{}.
			Add(projectsJS(state)).
			Morph(TopBarID, renderTopBar(state, sid)).
			Add(js).
			Add(updateHashJS(target)).
			Add(focusSelectedAppJS(state)), nil
	})
	// Resize app to specific width — JS-only update to preserve iframes
	r.RegisterAction(app, "app.resize", func(_ *r.Context, in actionAppResizeInput) (r.Result, error) {
		sid := inputSID(in.SID)
		appID := in.ID
		if appID == "" {
			return r.Result{}, nil
		}
		width := WidthLG
		if v, ok := inputField(in.Width); ok && v != "" {
			width = Width(v)
		}
		maxPixels := 0
		if v, ok := inputField(in.MaxPixel); ok {
			maxPixels = int(v)
		}
		width = width.ClampFixedPixel(maxPixels)
		if sm.SetAppWidthByID(sid, appID, width) < 0 {
			return r.Result{}, nil
		}
		state := sm.Get(sid)
		return resizeJS(state, width, appID), nil
	})
	// Toggle full width while keeping the other panels visible.
	r.RegisterAction(app, "app.resize.max.toggle", func(_ *r.Context, in actionAppResizeMaxToggleInput) (r.Result, error) {
		sid := inputSID(in.SID)
		maxPixels := 0
		if v, ok := inputField(in.MaxPixel); ok {
			maxPixels = int(v)
		}
		width, appID := sm.ToggleMaxWidth(sid, maxPixels)
		if appID == "" {
			return r.Result{}, nil
		}
		return resizeJS(sm.Get(sid), width, appID), nil
	})
	// Toggle maximize — switch selected app between full width and previous width
	r.RegisterAction(app, "app.maximize.toggle", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return r.Result{}, nil
		}
		return clientScript("if(window.libroWorkspace)libroWorkspace.maximize(props[0]);", selectedAppID(state)), nil
	})
	// Step selected app width by one tier up/down.
	r.RegisterAction(app, "app.resize.step", func(_ *r.Context, in actionAppResizeStepInput) (r.Result, error) {
		sid := inputSID(in.SID)
		delta := 0
		if v, ok := inputField(in.Delta); ok {
			delta = int(v)
		}
		if delta == 0 {
			return r.Result{}, nil
		}
		maxPixels := 0
		if v, ok := inputField(in.MaxPixel); ok {
			maxPixels = int(v)
		}
		newWidth, appID := sm.StepSelectedAppWidth(sid, delta, maxPixels)
		if appID == "" {
			return r.Result{}, nil
		}
		state := sm.Get(sid)
		return resizeJS(state, newWidth, appID), nil
	})
	// Select specific app - JS-only update to preserve iframes
	actionAppSelect = r.RegisterAction(app, "app.select", func(_ *r.Context, in actionAppSelectInput) (r.Result, error) {
		sid := inputSID(in.SID)
		idx := 0
		if v, ok := inputField(in.Index); ok {
			idx = int(v)
		}
		sm.SelectApp(sid, idx)
		state := sm.Get(sid)
		if focus, ok := inputField(in.Focus); ok && !focus {
			return r.Result{}, nil
		}
		return r.Merge(navigateJS(state, sid), updateAppPreviewJS(state)), nil
	})
	// Open an empty browser panel
	r.RegisterAction(app, "app.browse.open", func(_ *r.Context, in actionAppBrowseOpenInput) (r.Result, error) {
		sid := inputSID(in.SID)
		side := in.Side

		// Compute insertion index relative to currently selected app
		insertIdx := -1 // default: append
		switch side {
		case "left":
			insertIdx = sm.SelectedIndex(sid)
		case "right":
			insertIdx = sm.SelectedIndex(sid) + 1
		}
		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)
		sm.InsertApp(sid, "", DBDefaultToolPanelWidth(), "New Tab", insertIdx)
		state := sm.Get(sid)
		topBarJS := r.Result{}.Morph(TopBarID, renderTopBar(state, sid))
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(actionAppHydrateInput{SID: sid, ID: state.Apps[state.SelectedIndex].ID, OpenURL: state.Apps[state.SelectedIndex].URL == ""})
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid)
			return r.Merge(insertAppJS(frame, false, state.ActiveProject), navigateJS(state, sid), topBarJS, projJS, hydrateJS), nil
		}
		return r.Result{}.
			Morph(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID)).
			Morph(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(navigateJS(state, sid)).
			Add(hydrateJS), nil
	})
	// Quick open - show the app search dialog
	r.RegisterAction(app, "plugin.launcher.open", func(_ *r.Context, in sessionInput) (r.Result, error) {
		return trustedResponse(`if(window.libroWorkspace)libroWorkspace.launcher();`), nil
	})
	// Execute a terminal command directly (called from search dialog)

	// Set URL for a running app — navigates the iframe and updates session state only.
	r.RegisterAction(app, "app.url.set", func(_ *r.Context, in actionAppUrlSetInput) (r.Result, error) {
		sid := inputSID(in.SID)
		appID := in.ID
		newURL := in.Url
		newURL = strings.TrimSpace(newURL)
		if appID == "" || newURL == "" {
			return r.Result{}, nil
		}
		// Ensure URL has a scheme
		newURL = ensureScheme(newURL)
		idx := sm.SetAppURLByID(sid, appID, newURL)
		if idx < 0 {
			return r.Result{}, nil
		}
		if observed := in.Observed; observed {
			return r.Result{}, nil
		}
		// Navigate the running browser instance.
		return r.Merge(clientScript("window.__libroWvNavigate(props[0],props[1]);", appID, newURL), r.Result{}.Run(r.SetValue("urlinput-"+appID, newURL))), nil
	})

	// Lookup directories for the unified project dialog. Bare terms search common
	// code roots recursively; absolute paths list matching child directories.
	app.GET("/project/lookup", func(w http.ResponseWriter, req *http.Request) {
		if !authorizeVoice(w, req) {
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		matches := projectDirLookup(ctx, req.URL.Query().Get("query"))
		if req.Context().Err() != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(matches)
	})
	// Open the unified project dialog in folder-browse mode.
	r.RegisterAction(app, "project.dialog.open", func(_ *r.Context, in sessionInput) (r.Result, error) {
		return trustedResponse(`if(window.__libroOpenProjectDialog)window.__libroOpenProjectDialog();`), nil
	})
	// Close project dialog
	r.RegisterAction(app, "project.dialog.close", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		sm.CloseProjectDialog(sid)
		return r.Result{}.Run(r.Hide(ProjectDialogID)), nil
	})
	// Create a new project
	r.RegisterAction(app, "project.create", func(_ *r.Context, in actionProjectCreateInput) (r.Result, error) {
		sid := inputSID(in.SID)
		path := in.ProjectPath
		path = strings.TrimSpace(path)
		if path == "" {
			return r.Result{}.Run(r.Notify("error", "Folder path is required")), nil
		}
		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Result{}.Run(r.Notify("error", "Path must be absolute")), nil
		}
		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Result{}.Run(r.Notify("error", "Invalid folder selected")), nil
		}

		// If the folder doesn't exist, surface the inline confirm bar instead
		// of failing — the user can either confirm creation or cancel.
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			msg := "Folder does not exist: " + path
			return r.Result{}.Run(r.Seq(r.SetText("project-path-confirm-msg", msg+" — Create it?"), r.SetAttr("project-path-confirm", "data-path", path), r.Show("project-path-confirm"))), nil

		}
		return finalizeProjectCreate(sid, path, name, false), nil
	})
	// Open a folder as a session-only project. This sets the active working
	// directory for newly opened apps without persisting it to the project list.
	r.RegisterAction(app, "project.open.folder", func(_ *r.Context, in actionProjectOpenFolderInput) (r.Result, error) {
		sid := inputSID(in.SID)
		path := in.ProjectPath
		path = strings.TrimSpace(path)
		if path == "" {
			return r.Result{}.Run(r.Notify("error", "Folder path is required")), nil
		}
		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Result{}.Run(r.Notify("error", "Path must be absolute")), nil
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return r.Result{}.Run(r.Notify("error", "Folder does not exist")), nil
		}
		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Result{}.Run(r.Notify("error", "Invalid folder selected")), nil
		}
		return finalizeProjectCreate(sid, path, name, true), nil
	})
	// Confirm creating a missing project folder, then create the project.
	r.RegisterAction(app, "project.create.confirm", func(_ *r.Context, in actionProjectCreateConfirmInput) (r.Result, error) {
		sid := inputSID(in.SID)
		path := in.Path
		path = strings.TrimSpace(path)
		if path == "" {
			return r.Result{}.Run(r.Notify("error", "Folder path is required")), nil
		}
		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Result{}.Run(r.Notify("error", "Path must be absolute")), nil
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return r.Result{}.Run(r.Notify("error", "Failed to create folder: "+err.Error())), nil
		}
		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Result{}.Run(r.Notify("error", "Invalid folder selected")), nil
		}
		return finalizeProjectCreate(sid, path, name, false), nil
	})
	// Close every panel and terminal in the requested workspace (active by default).
	actionProjectClose = r.RegisterAction(app, "project.close", func(_ *r.Context, in actionProjectCloseInput) (r.Result, error) {
		sid := inputSID(in.SID)
		state := sm.Get(sid)
		name := in.Name
		if name == "" {
			name = state.ActiveProject
		}
		active := name == state.ActiveProject
		target := ""
		if active {
			target = state.adjacentProjectThread()
		}
		apps, err := sm.CloseProject(sid, name)
		if err != nil {
			return r.Result{}.Run(r.Notify("error", "Could not close workspace: "+err.Error())), nil
		}
		for _, a := range apps {
			if a.Type == AppTypeTerminal {
				tm.Stop(a.ID)
			}
		}
		state = sm.Get(sid)
		resp := r.Result{}.Add(closeDevtoolsForAppsJS(apps))
		if active {
			resp = resp.Add(trustedResponse(parkFloatingPopupsJS())).
				Morph(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
				Morph(TopBarID, renderTopBar(state, sid))
		} else {
			for _, a := range apps {
				resp = resp.Add(removeAppJS(a.ID))
			}
		}
		response := resp.Add(projectsJS(state))
		if target != "" {
			response = r.Merge(response, switchToProjectName(sid, target))
		}
		return response, nil
	})

	registerThreadActions(app, switchToProjectName)
	registerFinishThreadActions(app)
	// Switch active project
	r.RegisterAction(app, "project.switch", func(_ *r.Context, in actionProjectSwitchInput) (r.Result, error) {
		sid := inputSID(in.SID)
		name := in.Name
		resp, ok := switchToProjectNameChecked(sid, name)
		if !ok {
			return r.Result{}.Run(r.Notify("error", "Project not found")), nil
		}
		id := in.AppId
		if focusAgent := in.FocusAgent; focusAgent && id == "" {
			for _, panel := range sm.Get(sid).Apps {
				if panel.Dock == "center" {
					id = panel.ID
					break
				}
			}
		}
		if id != "" {
			for index, panel := range sm.Get(sid).Apps {
				if panel.ID == id {
					sm.SelectApp(sid, index)
					resp = r.Merge(resp, navigateJS(sm.Get(sid), sid))
					resp = r.Merge(resp, clientScript("libroWorkspace.select(props[0], false);", id))
					break
				}
			}
		}
		return resp, nil
	})
	// Remove a project
	r.RegisterAction(app, "project.remove", func(_ *r.Context, in actionProjectRemoveInput) (r.Result, error) {
		sid := inputSID(in.SID)
		name := in.Name
		if name == "" {
			return r.Result{}, nil
		}

		// Check if we're removing the active project
		stateBefore := sm.Get(sid)
		wasActive := stateBefore.ActiveProject == name
		apps, ok := sm.RemoveProject(sid, name)
		if !ok {
			return r.Result{}.Run(r.Notify("error", "Cannot remove project")), nil
		}

		// Cleanup apps from the removed project's snapshot
		for _, a := range apps {
			if a.Type == AppTypeTerminal {
				tm.Stop(a.ID)
			}
		}
		state := sm.Get(sid)

		// Remove project from DB
		DBRemoveProject(name)
		resp := r.Result{}.
			Add(trustedResponse(parkFloatingPopupsJS())).
			Add(projectsJS(state)).
			Morph(TopBarID, renderTopBar(state, sid)).
			Add(r.Result{}.Run(r.Remove(projectMainID(name))))
		if wasActive {
			var content *r.Node
			if !sm.IsProjectRendered(sid, state.ActiveProject) {
				content = renderMainArea(state, sid)
			}
			resp = resp.Add(switchProjectJS(state.ActiveProject, content)).
				Add(updateHashJS(state.ActiveProject)).
				Add(focusSelectedAppJS(state))
		}
		return resp, nil
	})
	// Check if there are running apps before closing — returns JS to show dialog or force close
	r.RegisterAction(app, "app.close.check", func(_ *r.Context, in sessionInput) (r.Result, error) {
		sid := inputSID(in.SID)
		sm.mu.RLock()
		sessionIDs := make([]string, 0, len(sm.states))
		for id := range sm.states {
			sessionIDs = append(sessionIDs, id)
		}
		sm.mu.RUnlock()
		var projectApps []ProjectApps
		for _, id := range sessionIDs {
			projectApps = append(projectApps, sm.GetAllRunningApps(id)...)
		}

		// No running apps — close immediately
		if len(projectApps) == 0 {
			return r.Result{}.Run(actionAppCloseAll.Call(sessionInput{SID: sid})), nil
		}
		return showCloseDialogJS(projectApps, "Quit Libro?", "Quit", "app.close.all", sid), nil
	})
	// Finish cleanup before allowing the renderer to close the desktop window.
	actionAppCloseAll = r.RegisterAction(app, "app.close.all", func(_ *r.Context, in sessionInput) (r.Result, error) {
		tm.StopAll()
		sm.mu.Lock()
		sm.states = make(map[string]*AppState)
		sm.mu.Unlock()
		return trustedResponse(`if(window.libroElectron)window.libroElectron.forceClose();else window.close();`), nil
	})
	// Switch to a worktree (creates virtual project if needed)
	r.RegisterAction(app, "worktree.switch", func(_ *r.Context, in actionWorktreeSwitchInput) (r.Result, error) {
		sid := inputSID(in.SID)
		parentProject := in.Project
		wtPath := in.Path
		branch := in.Branch
		if parentProject == "" || wtPath == "" || branch == "" {
			return r.Result{}, nil
		}
		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)
		vtName := parentProject + "/" + branch

		// Add virtual project if it doesn't exist
		sm.AddVirtualProject(sid, vtName, wtPath, parentProject)
		if !sm.SwitchProject(sid, vtName) {
			return r.Result{}.Run(r.Notify("error", "Failed to switch to worktree")), nil
		}
		state := sm.Get(sid)
		var jsSwitch r.Result
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}
		resp := r.Result{}.
			Add(projectsJS(state)).
			Morph(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(projectAutolaunchJS(state, sid)).
			Add(focusSelectedAppJS(state))
		return resp, nil
	})
	// Create a new worktree from the active project's current branch and switch to it.
	r.RegisterAction(app, "worktree.create", func(_ *r.Context, in actionWorktreeCreateInput) (r.Result, error) {
		sid := inputSID(in.SID)
		branch := in.Branch
		branch = strings.TrimSpace(branch)
		if branch == "" {
			return r.Result{}.Run(r.Notify("error", "Branch name cannot be empty")), nil
		}
		if strings.ContainsAny(branch, " \t\n\r~^:?*[\\") {
			return r.Result{}.Run(r.Notify("error", "Branch name contains invalid characters")), nil
		}
		state := sm.Get(sid)
		if state == nil {
			return r.Result{}, nil
		}
		vtName, err := sm.createProjectWorktree(sid, state.ActiveProject, branch)
		if err != nil {
			return r.Result{}.Run(r.Notify("error", "Failed to create worktree: "+err.Error())), nil
		}
		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)
		if !sm.SwitchProject(sid, vtName) {
			return r.Result{}.Run(r.Notify("error", "Worktree created but failed to switch")), nil
		}
		state = sm.Get(sid)
		var jsSwitch r.Result
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}
		return r.Result{}.
			Add(projectsJS(state)).
			Morph(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(projectAutolaunchJS(state, sid)).
			Add(focusSelectedAppJS(state)), nil
	})

	components.RegisterTerminalRoutes(app, tm, func(sid, terminalID string) bool {
		return sm.TerminalBelongsToSession(sid, terminalID)
	})

	// Live-switch native xterm themes when GNOME's color-scheme flips.
	var themeMu sync.Mutex
	// Each page starts with an empty workspace.
	app.Page("/", func(_ *r.Context) *r.Node {
		sid := sm.NewSession()
		state := sm.Get(sid)

		return renderPage(state, sid)
	})

	components.WatchGnomeTheme(func() {
		themeMu.Lock()
		defer themeMu.Unlock()
		if err := app.Broadcast(trustedResponse(`(function(){if(window.__libroRefreshTerminalThemes)window.__libroRefreshTerminalThemes();})();`)); err != nil {
			log.Printf("libro: broadcast terminal theme: %v", err)
		}
	})

	if desktop {
		go func() {
			<-OpenDesktop("http://localhost:" + Port())
			CleanupRuntime()
			CloseDB()
			os.Exit(0)
		}()
	}
	log.Printf("Libro listening on http://localhost:%s", Port())
	return http.Serve(listener, app.Handler())
}

// ensureScheme adds http:// for local URLs and https:// for everything else.
func ensureScheme(u string) string {
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "file://") {
		return u
	}
	if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.0.0.1") || strings.HasPrefix(u, "0.0.0.0") ||
		strings.HasPrefix(u, "[::1]") || strings.HasPrefix(u, "[::0]") || strings.HasPrefix(u, "[::]") {
		return "http://" + u
	}
	return "https://" + u
}

func inputSID(sid string) string {
	if sid != "" {
		return sid
	}
	return "default"
}
func inputField[T any](value *T) (T, bool) {
	if value != nil {
		return *value, true
	}
	var zero T
	return zero, false
}

// restoreWorktreeProject resolves complete names; both project and branch names
// may contain slashes, so splitting on a slash loses part of the parent name.
func restoreWorktreeProject(manager *StateManager, sid, name string) {
	state := manager.Get(sid)
	for _, project := range state.Projects {
		if project.Name == name {
			return
		}
	}
	for _, project := range state.Projects {
		if project.Virtual || !strings.HasPrefix(name, project.Name+"/") {
			continue
		}
		worktrees, err := GitListWorktrees(project.Path)
		if err != nil {
			continue
		}
		for _, worktree := range worktrees {
			if !worktree.IsBare && project.Name+"/"+worktree.Branch == name {
				manager.AddVirtualProject(sid, name, worktree.Path, project.Name)
				return
			}
		}
	}
}
