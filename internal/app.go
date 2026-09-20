// Package libro implements the Libro server, desktop bundling, and UI components.
package libro

import (
	"embed"
	"encoding/json"
	"fmt"
	"libro/internal/components"
	"log"
	"slices"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	r "github.com/michalCapo/g-sui/ui"
)

func hydrateAppAfterScrollJS(appID string, data map[string]any) string {
	payload, err := json.Marshal(data)
	if err != nil {
		return "/* hydrate payload error */"
	}
	return fmt.Sprintf(`
(function(){
	var appID=%s;
	var payload=%s;
	function hydrate(){
		if(typeof __ws!=='undefined'&&__ws.call)__ws.call('app.hydrate',payload);
	}
	requestAnimationFrame(function(){
		var app=document.querySelector('[data-app-id="'+String(appID).replace(/"/g,'\\"')+'"]');
		if(app&&window.__libroScrollToApp)window.__libroScrollToApp(app);
		requestAnimationFrame(hydrate);
	});
})();
`, components.JSString(appID), string(payload))
}

func settleHydratedAppContentJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var appID=%s;
	var app=document.querySelector('[data-app-id="'+String(appID).replace(/"/g,'\\"')+'"]');
	function settle(){
		if(app&&window.__libroScrollToApp)window.__libroScrollToApp(app);
		var termFrame=document.querySelector('[data-terminal-app="'+String(appID).replace(/"/g,'\\"')+'"]');
		if(termFrame&&window.__libroFitTerminalFrame)window.__libroFitTerminalFrame(termFrame);
		if((window.__libroSelectedApp||'')===appID&&window.__libroFocusAppByID)window.__libroFocusAppByID(appID);
	}
	requestAnimationFrame(function(){settle();requestAnimationFrame(settle);});
})();
`, components.JSString(appID))
}

// actionResult keeps Libro's client-side view orchestration behind g-sui's
// typed Result API. The temporary node is removed after its trusted script has
// run so repeated actions do not grow the DOM.
func actionResult(js string) r.Result {
	if strings.TrimSpace(js) == "" {
		return r.Result{}
	}
	script := "var effect=this;try{(function(){\n" + js + "\n}).call(effect);}finally{effect.remove();}"
	return r.Result{}.Append(ActionEffectsID, r.Span("hidden").Attr("aria-hidden", "true").JS(script))
}

func registerAction(app *r.App, name string, handler func(*r.Context) string) {
	r.RegisterAction(app, name, func(ctx *r.Context, _ map[string]any) (r.Result, error) {
		return actionResult(handler(ctx)), nil
	})
}

// responseBuilder preserves the compact composition used by Libro while the
// action boundary itself returns a typed g-sui Result.
type responseBuilder struct {
	parts []string
}

func newResponse() *responseBuilder { return &responseBuilder{} }

func (b *responseBuilder) Add(js string) *responseBuilder {
	b.parts = append(b.parts, js)
	return b
}

func (b *responseBuilder) Replace(id string, node *r.Node) *responseBuilder {
	b.parts = append(b.parts, node.ToJSReplace(id))
	return b
}

func (b *responseBuilder) Build() string { return strings.Join(b.parts, "") }

// Closing a thread's agent archives the thread and closes its tools as well.
func closeWorkspaceApp(sid, appID string) string {
	apps, err := sm.CloseThreadAgent(sid, appID)
	if err != nil {
		return r.Notify("error", "Could not archive thread")
	}
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
		js += removeAppJS(app.ID)
	}
	if len(apps) == 0 {
		js = removeAppJS(appID)
	}
	state := sm.Get(sid)
	return js + navigateJS(state, sid) + renderTopBar(state, sid).ToJSReplace(TopBarID) + projectsJS(state)
}

// Autolaunch uses app.start so command validation and terminal lifecycle stay shared.
func projectAutolaunchJS(state *AppState, sid string) string {
	if state.ActiveProject == "" {
		return ""
	}
	if slices.ContainsFunc(state.Apps, isAgentApp) {
		return ""
	}
	threadAgent := defaultThreadAgent()
	for _, plugin := range plugins() {
		autolaunch := plugin.Autolaunch
		if state.thread(state.ActiveProject) != nil {
			autolaunch = plugin.ID == threadAgent
		}
		if !autolaunch || plugin.Disabled || plugin.Removed || plugin.Dock != "center" || plugin.Type != AppTypeTerminal {
			continue
		}
		payload, _ := json.Marshal(sidData(sid, "type", string(plugin.Type), "plugin", plugin.ID, "name", plugin.Name, "dock", "center", "writable", true, "autolaunchProject", state.ActiveProject))
		return fmt.Sprintf("__ws.call('app.start',%s);", payload)
	}
	return ""
}

// finalizeProjectCreate registers the project, optionally persists it, and
// returns the JS that switches to it and dismisses the dialog.
func finalizeProjectCreate(sid, path, name string, transient bool) string {
	if !sm.AddProjectWithOptions(sid, name, path, transient) {
		return r.Notify("error", "Project '"+name+"' already exists")
	}

	if !transient {
		DBSaveProject(name, path)
	}

	sm.CloseProjectDialog(sid)
	sm.SwitchProject(sid, name)
	sm.IsProjectRendered(sid, name)
	state := sm.Get(sid)

	jsSwitch := switchProjectJS(name, renderMainArea(state, sid))

	resp := newResponse().
		Add(projectsJS(state)).
		Replace(TopBarID, renderTopBar(state, sid)).
		Replace(ProjectDialogID, renderProjectDialog(false, sid)).
		Add(`if(window.__libroProjectDialogBind)window.__libroProjectDialogBind();`).
		Add(jsSwitch).
		Add(updateHashJS(name)).
		Add(projectAutolaunchJS(state, sid)).
		Add(focusSelectedAppJS(state))
	if transient {
		resp.Add(showToastJS("Opened folder", path, 1600))
	}
	return resp.Build()
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

func projectDirLookup(query string) []projectDirLookupMatch {
	query = strings.TrimSpace(query)
	if query == "" {
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
		if info, err := os.Stat(expanded); err == nil && info.IsDir() {
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
			if err != nil || len(out) >= 80 {
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

// CleanupRuntime tears down terminal backends.
func CleanupRuntime() {
	shutdownCleanupOnce.Do(func() {
		tm.StopAll()
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
func Run(assets embed.FS) {
	installShutdownSignalHandler()
	InitDB()
	defer CloseDB()
	defer CleanupRuntime()
	app := r.NewApp()
	app.Title = "Libro"
	app.Description = "Application Manager"
	app.Assets(assets, "assets", "/assets/")
	app.Favicon = "/assets/logo.svg"

	// Each page starts with an empty workspace.
	app.Page("/", func(_ *r.Context) *r.Node {
		sid := sm.NewSession()
		state := sm.Get(sid)

		return renderPage(state, sid)
	})

	registerSettingsActions(app)
	registerFilesActions(app)
	registerNotesActions(app)

	// Open add dialog
	registerAction(app, "app.dialog.open", func(_ *r.Context) string {
		return `if(window.libroWorkspace)libroWorkspace.launcher();`
	})

	// Quick browse - open URL or Google search

	// Open Neovim if available, otherwise fall back to Vim; notify if neither exists.
	registerAction(app, "app.nvim.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
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
			return showToastJS("Editor not installed", "Install nvim or vim to use ⌘/Win+E", 2600)
		}
		return fmt.Sprintf(`__ws.call('app.start',{sid:%s,type:'terminal',url:'',command:%s,writable:true,name:%s,iconUrl:'',side:'right'});`, components.JSString(sid), components.JSString(cmd), components.JSString(name))
	})

	// Open the Pi coding agent if available.
	registerAction(app, "app.pi.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		if _, err := exec.LookPath("pi"); err != nil {
			return showToastJS("Pi agent not installed", "Install pi to use ⌘/Win+Y", 2600)
		}
		return fmt.Sprintf(`__ws.call('app.start',{sid:%s,type:'terminal',url:'',command:'pi',writable:true,name:'pi',iconUrl:'',side:'right'});`, components.JSString(sid))
	})

	registerAction(app, "plugin.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		id, _ := ctx.WsData()["plugin"].(string)
		dock, _ := ctx.WsData()["dock"].(string)
		for _, p := range plugins() {
			if p.ID != id {
				continue
			}
			p.Command = agentCommand(p)
			if p.Command != "" {
				if _, err := exec.LookPath(extractBaseCmd(p.Command)); err != nil {
					return showToastJS(p.Name+" is not installed", "Install "+extractBaseCmd(p.Command)+" and try again.", 3200)
				}
			}
			if !validDock(dock) {
				dock = p.Dock
			}
			payload, _ := json.Marshal(sidData(sid, "type", string(p.Type), "command", p.Command, "url", p.URL, "name", p.Name, "plugin", p.ID, "dock", dock, "writable", true))
			return fmt.Sprintf("__ws.call('app.start',%s);", payload)
		}
		return r.Notify("error", "Plugin not found")
	})
	registerAction(app, "app.dock", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		id, _ := ctx.WsData()["id"].(string)
		dock, _ := ctx.WsData()["dock"].(string)
		if !validDock(dock) || dock == "bottom" {
			return ""
		}
		state := sm.Get(sid)
		for _, a := range state.Apps {
			if a.ID == id {
				if dock == "center" && !isAgentApp(a) {
					return r.Notify("error", "The main area is only for agents")
				}
				sm.SetAppPlugin(sid, id, a.PluginID, dock)
				return fmt.Sprintf("var f=document.getElementById(%s);if(f)f.dataset.dock=%s;if(window.libroWorkspace)libroWorkspace.select(%s);", components.JSString("frame-"+id), components.JSString(dock), components.JSString(id))
			}
		}
		return ""
	})

	// Start an application instance.
	registerAction(app, "app.start", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		if project, ok := data["autolaunchProject"].(string); ok {
			state := sm.Get(sid)
			if state.ActiveProject != project || projectAutolaunchJS(state, sid) == "" {
				return ""
			}
		}

		appType, _ := data["type"].(string)
		name, _ := data["name"].(string)
		side, _ := data["side"].(string)
		pluginID, _ := data["plugin"].(string)
		if pluginID == "" && appType == "terminal" {
			command, _ := data["command"].(string)
			candidate := pluginForApp(Application{Type: AppTypeTerminal, Command: command})
			if candidate.Dock == "center" && strings.TrimSpace(command) == candidate.Command {
				pluginID = candidate.ID
			}
		}
		dock, _ := data["dock"].(string)
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
				return r.Notify("error", "Plugin not found")
			}
			if plugin.Disabled || plugin.Removed {
				return r.Notify("error", "This agent is disabled in Settings")
			}
			plugin.Command = agentCommand(*plugin)
			if plugin.Type == AppTypeTerminal {
				data["command"] = plugin.Command
			}
			if plugin.Command != "" {
				if _, err := exec.LookPath(extractBaseCmd(plugin.Command)); err != nil {
					return showToastJS(plugin.Name+" is not installed", "Install "+extractBaseCmd(plugin.Command)+" and try again.", 3200)
				}
			}
		}
		command, _ := data["command"].(string)
		threadState := sm.Get(sid)
		if threadState.thread(threadState.ActiveProject) != nil {
			if !threadState.canStartThreadApp(Application{Type: AppType(appType), Command: command, PluginID: pluginID, Dock: dock}) {
				return ""
			}
		}
		width := defaultAppWidth(Application{Type: AppType(appType), Command: command, PluginID: pluginID})
		if val, ok := data["width"].(string); ok && val != "" {
			width = Width(val)
		}
		if dock == "center" && !isAgentApp(Application{Type: AppType(appType), Command: command, PluginID: pluginID}) {
			return r.Notify("error", "The main area is only for agents. Add other agents as agent plugins.")
		}
		if dock == "bottom" {
			width = WidthFull
			if appType != "terminal" || (pluginID != "" && pluginID != "terminal") {
				return r.Notify("error", "The bottom panel only supports a shell terminal")
			}
			data["command"] = ""
			name = "Terminal"
			for _, existing := range sm.Get(sid).Apps {
				if appDock(existing) == "bottom" {
					return fmt.Sprintf("if(window.libroWorkspace)libroWorkspace.select(%s);", components.JSString(existing.ID))
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
			command, _ := data["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = components.UserShellBase()
			}
			command = strings.ReplaceAll(command, "__dir__", pwd)

			writable := true
			if val, ok := data["writable"].(bool); ok {
				writable = val
			}

			iconURL, _ := data["iconUrl"].(string)

			// Check if strip already exists
			stateBefore := sm.Get(sid)
			hadApps := len(stateBefore.Apps)

			appID := sm.NextAppID()
			sm.InsertTerminalPlaceholder(sid, appID, width, command, writable, name, iconURL, insertIdx)
			sm.SetAppPlugin(sid, appID, pluginID, dock)
			var resizeAgentsJS strings.Builder
			for _, resized := range sm.SizeNewAgent(sid, appID, DBDefaultPanelWidth()) {
				resizeAgentsJS.WriteString(resizeJS(nil, resized.Width, resized.ID))
			}

			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]

			topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
			projJS := projectsJS(state)
			hydrateJS := hydrateAppAfterScrollJS(newApp.ID, sidData(sid, "id", newApp.ID))
			if hadApps > 0 {
				frame := renderAppFramePlaceholder(*newApp, state.SelectedIndex, true, sid)
				return resizeAgentsJS.String() + insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS
			}

			return renderMainAreaWithPlaceholder(state, sid, newApp.ID).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + projJS + navigateJS(state, sid) + hydrateJS
		}

		// URL app
		url, _ := data["url"].(string)
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

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(state.Apps[state.SelectedIndex].ID, sidData(sid, "id", state.Apps[state.SelectedIndex].ID, "openURL", state.Apps[state.SelectedIndex].URL == ""))
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS
		}

		return renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + projJS + navigateJS(state, sid) + hydrateJS
	})

	registerAction(app, "app.hydrate", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return "/* noop */"
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
			return "/* noop */"
		}

		if state.Apps[idx].Type == AppTypeTerminal && !state.Apps[idx].TerminalReady {
			term := state.Apps[idx]
			pwd := sm.GetActiveProjectPath(sid)
			var environment []string
			if isAgentApp(term) {
				environment = agentEnvironmentList()
			}
			session, err := tm.StartWithEnvironment(term.ID, term.Command, pwd, term.Writable, environment)
			if err != nil {
				sm.RemoveAppByID(sid, term.ID)
				state = sm.Get(sid)
				return removeAppJS(term.ID) + navigateJS(state, sid) + renderTopBar(state, sid).ToJSReplace(TopBarID) + projectsJS(state) + r.Notify("error", "Failed to start terminal: "+err.Error())
			}
			if !sm.HydrateTerminalByID(sid, term.ID, session.ID) {
				tm.Stop(term.ID)
				return r.Notify("error", "Terminal placeholder disappeared")
			}
			state = sm.Get(sid)
			for i := range state.Apps {
				if state.Apps[i].ID == appID {
					idx = i
					break
				}
			}
		}

		openURL, _ := data["openURL"].(bool)
		contentJS := renderAppContent(state.Apps[idx], sid, false, nil).ToJSReplace(appContentID(appID))
		return fmt.Sprintf(`
(function(){
	var appID=%s;
	var content=document.getElementById(%s);
	if(!content||!content.querySelector('[data-app-placeholder]'))return;
	var urlPopup=document.getElementById(%s);
	var urlPopupInput=document.getElementById('url-popup-input');
	var reopenURLPopup=!!(content&&urlPopup&&content.contains(urlPopup)&&!urlPopup.classList.contains('hidden'));
	var reopenURLPopupValue=urlPopupInput?urlPopupInput.value:'';
	if(window.__libroParkFloatingPopups)window.__libroParkFloatingPopups();
	%s
	if(reopenURLPopup&&window.__libroOpenURLPopupFor){
		setTimeout(function(){window.__libroOpenURLPopupFor(appID,reopenURLPopupValue);},30);
	}
})();
`, components.JSString(appID), components.JSString(appContentID(appID)), components.JSString(URLPopupID), contentJS) + settleHydratedAppContentJS(appID) + fmt.Sprintf(`
requestAnimationFrame(function(){requestAnimationFrame(function(){if(%t && window.__libroSelectedApp===%s && window.__libroOpenURLPopupFor)window.__libroOpenURLPopupFor(%s,'');});});`, openURL, components.JSString(appID), components.JSString(appID))
	})

	// Close/remove application
	registerAction(app, "app.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return ""
		}

		return closeWorkspaceApp(sid, appID)
	})

	registerAction(app, "project.close.check", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		return showCloseDialogJS([]ProjectApps{{Name: workspaceProjectLabel(state), Apps: state.Apps}},
			"Close project?", "Close project", "project.close", sid)
	})

	// Close every panel and terminal in the active project.
	registerAction(app, "project.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		apps := sm.CloseProject(sid)
		for _, a := range apps {
			if a.Type == AppTypeTerminal {
				tm.Stop(a.ID)
			}
		}
		state := sm.Get(sid)
		return newResponse().
			Add(parkFloatingPopupsJS()).
			Add(closeDevtoolsForAppsJS(apps)).
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Build()
	})

	// Close current (selected) app — no app ID needed from client
	registerAction(app, "app.close.current", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return "/* noop */"
		}
		appID := state.Apps[state.SelectedIndex].ID

		return closeWorkspaceApp(sid, appID)
	})

	// Emergency restart for a terminal app's native PTY session.
	registerAction(app, "app.terminal.restart", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return r.Notify("error", "No terminal app selected")
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
			return r.Notify("error", "Selected app is not a running terminal")
		}

		pwd := sm.GetActiveProjectPath(sid)
		var environment []string
		if isAgentApp(*term) {
			environment = agentEnvironmentList()
		}
		if err := tm.RestartWithEnvironment(term.ID, term.Command, term.Writable, pwd, environment); err != nil {
			return r.Notify("error", "Failed to restart terminal: "+err.Error())
		}

		return fmt.Sprintf(`(function(){if(window.__libroRestartTerminal)window.__libroRestartTerminal(%s);})();`, components.JSString(term.ID)) + settleAppFrameJS(term.ID) + r.Notify("success", "Terminal restarted")
	})

	// Navigate left - JS-only update to preserve iframes
	registerAction(app, "app.navigate.left", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateLeft(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Navigate right - JS-only update to preserve iframes
	registerAction(app, "app.navigate.right", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateRight(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Move app left — swap with neighbor, JS-only DOM swap to preserve iframes
	registerAction(app, "app.move.left", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		if !sm.MoveAppLeft(sid) {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return moveAppJS(state, sid, "left") + renderTopBar(state, sid).ToJSReplace(TopBarID)
	})

	// Move app right — swap with neighbor, JS-only DOM swap to preserve iframes
	registerAction(app, "app.move.right", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		if !sm.MoveAppRight(sid) {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return moveAppJS(state, sid, "right") + renderTopBar(state, sid).ToJSReplace(TopBarID)
	})

	// Move selected app to another project, then activate that project.
	registerAction(app, "app.move.to.project", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		target, _ := data["target"].(string)
		kind, _ := data["kind"].(string)
		parentProject, _ := data["project"].(string)
		wtPath, _ := data["path"].(string)
		branch, _ := data["branch"].(string)
		if target == "" {
			return r.Notify("error", "Project not found")
		}
		if kind == "worktree" && parentProject != "" && wtPath != "" && branch != "" {
			sm.AddVirtualProject(sid, target, wtPath, parentProject)
		}

		prevState := sm.Get(sid)
		if len(prevState.Apps) == 0 || prevState.SelectedIndex < 0 || prevState.SelectedIndex >= len(prevState.Apps) {
			return r.Notify("error", "No selected app")
		}
		sourceProject := prevState.ActiveProject
		appID := prevState.Apps[prevState.SelectedIndex].ID
		targetHadApps := projectHasRunningApps(prevState, target)
		targetRenderedBefore := sm.IsProjectRendered(sid, target)
		if sourceProject == target {
			return focusSelectedAppJS(prevState)
		}

		moved, ok := sm.MoveSelectedAppToProject(sid, target)
		if !ok || moved == nil {
			return r.Notify("error", "Project not found")
		}
		state := sm.Get(sid)

		var js strings.Builder
		js.WriteString(closeDevtoolsForAppJS(appID))
		if sourceSnap, ok := state.snapshots[sourceProject]; ok && sourceSnap != nil && len(sourceSnap.Apps) > 0 {
			js.WriteString(removeAppJS(appID))
			js.WriteString(navigateProjectJS(sourceProject, sourceSnap.Apps, sourceSnap.SelectedIndex, sid))
		} else {
			js.WriteString(parkFloatingPopupsJS())
			js.WriteString(renderMainAreaForProject(state, sid, sourceProject).ToJSReplace(projectMainID(sourceProject)))
		}

		if targetRenderedBefore {
			if targetHadApps {
				frame := renderAppFrame(*moved, state.SelectedIndex, true, sid)
				js.WriteString(insertAppJS(frame, false, target))
				js.WriteString(switchProjectJS(target, nil))
				js.WriteString(navigateJS(state, sid))
			} else {
				js.WriteString(renderMainArea(state, sid).ToJSReplace(projectMainID(target)))
				js.WriteString(switchProjectJS(target, nil))
			}
		} else {
			js.WriteString(switchProjectJS(target, renderMainArea(state, sid)))
		}

		return newResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(js.String()).
			Add(updateHashJS(target)).
			Add(focusSelectedAppJS(state)).
			Build()
	})

	// Resize app to specific width — JS-only update to preserve iframes
	registerAction(app, "app.resize", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return "/* noop */"
		}
		width := WidthLG
		if v, ok := data["width"].(string); ok && v != "" {
			width = Width(v)
		}
		maxPixels := 0
		if v, ok := data["maxPixel"].(float64); ok {
			maxPixels = int(v)
		}
		width = width.ClampFixedPixel(maxPixels)
		if sm.SetAppWidthByID(sid, appID, width) < 0 {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return resizeJS(state, width, appID)
	})

	// Toggle full width while keeping the other panels visible.
	registerAction(app, "app.resize.max.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		maxPixels := 0
		if v, ok := ctx.WsData()["maxPixel"].(float64); ok {
			maxPixels = int(v)
		}
		width, appID := sm.ToggleMaxWidth(sid, maxPixels)
		if appID == "" {
			return "/* noop */"
		}
		return resizeJS(sm.Get(sid), width, appID)
	})

	// Toggle maximize — switch selected app between full width and previous width
	registerAction(app, "app.maximize.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return ""
		}
		return fmt.Sprintf("if(window.libroWorkspace)libroWorkspace.maximize(%s);", selectedAppID(state))
	})

	// Step selected app width by one tier up/down.
	registerAction(app, "app.resize.step", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		delta := 0
		if v, ok := data["delta"].(float64); ok {
			delta = int(v)
		}
		if delta == 0 {
			return "/* noop */"
		}
		maxPixels := 0
		if v, ok := data["maxPixel"].(float64); ok {
			maxPixels = int(v)
		}
		newWidth, appID := sm.StepSelectedAppWidth(sid, delta, maxPixels)
		if appID == "" {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return resizeJS(state, newWidth, appID)
	})

	// Select specific app - JS-only update to preserve iframes
	registerAction(app, "app.select", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		idx := 0
		if v, ok := data["index"].(float64); ok {
			idx = int(v)
		}
		sm.SelectApp(sid, idx)
		state := sm.Get(sid)
		if focus, ok := data["focus"].(bool); ok && !focus {
			return ""
		}
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Open an empty browser panel
	registerAction(app, "app.browse.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		side, _ := data["side"].(string)
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

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(state.Apps[state.SelectedIndex].ID, sidData(sid, "id", state.Apps[state.SelectedIndex].ID, "openURL", state.Apps[state.SelectedIndex].URL == ""))
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS
		}

		return newResponse().
			Replace(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(navigateJS(state, sid)).
			Add(hydrateJS).
			Build()
	})

	// Quick open - show the app search dialog
	registerAction(app, "plugin.launcher.open", func(_ *r.Context) string {
		return `if(window.libroWorkspace)libroWorkspace.launcher();`
	})

	// Execute a terminal command directly (called from search dialog)

	// Set URL for a running app — navigates the iframe and updates session state only.
	registerAction(app, "app.url.set", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		newURL, _ := data["url"].(string)
		newURL = strings.TrimSpace(newURL)
		if appID == "" || newURL == "" {
			return ""
		}
		// Ensure URL has a scheme
		newURL = ensureScheme(newURL)
		idx := sm.SetAppURLByID(sid, appID, newURL)
		if idx < 0 {
			return ""
		}
		if observed, _ := data["observed"].(bool); observed {
			return ""
		}
		// Navigate the running browser instance.
		return fmt.Sprintf(`(function(){window.__libroWvNavigate(%s,%s);var inp=document.getElementById('urlinput-'+%s);if(inp)inp.value=%s;})();`, components.JSString(appID), components.JSString(newURL), components.JSString(appID), components.JSString(newURL))
	})

	// Lookup directories for the unified project dialog. Bare terms search common
	// code roots recursively; absolute paths list matching child directories.
	registerAction(app, "project.lookup", func(ctx *r.Context) string {
		data := ctx.WsData()
		query, _ := data["query"].(string)
		seq, _ := data["seq"].(float64)
		matches := projectDirLookup(query)
		payload, _ := json.Marshal(map[string]any{
			"query":   strings.TrimSpace(query),
			"seq":     int(seq),
			"matches": matches,
		})
		return fmt.Sprintf(`if(window.__libroProjectDialogSetDirMatches)window.__libroProjectDialogSetDirMatches(%s);`, string(payload))
	})

	// Open the unified project dialog in folder-browse mode.
	registerAction(app, "project.dialog.open", func(_ *r.Context) string {
		return `if(window.__libroOpenProjectDialog)window.__libroOpenProjectDialog();`
	})

	// Close project dialog
	registerAction(app, "project.dialog.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.CloseProjectDialog(sid)
		return r.Hide(ProjectDialogID)
	})

	// Create a new project
	registerAction(app, "project.create", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()

		path, _ := data["project-path"].(string)
		path = strings.TrimSpace(path)

		if path == "" {
			return r.Notify("error", "Folder path is required")
		}

		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Notify("error", "Path must be absolute")
		}

		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Notify("error", "Invalid folder selected")
		}

		// If the folder doesn't exist, surface the inline confirm bar instead
		// of failing — the user can either confirm creation or cancel.
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			msg := "Folder does not exist: " + path
			return fmt.Sprintf(
				`(function(){var bar=document.getElementById('project-path-confirm');var msg=document.getElementById('project-path-confirm-msg');if(!bar||!msg)return;msg.textContent=%s;bar.classList.remove('hidden');bar.dataset.path=%s;})();`,
				components.JSString(msg+" — Create it?"),
				components.JSString(path),
			)
		}

		return finalizeProjectCreate(sid, path, name, false)
	})

	// Open a folder as a session-only project. This sets the active working
	// directory for newly opened apps without persisting it to the project list.
	registerAction(app, "project.open.folder", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		path, _ := ctx.WsData()["project-path"].(string)
		path = strings.TrimSpace(path)
		if path == "" {
			return r.Notify("error", "Folder path is required")
		}
		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Notify("error", "Path must be absolute")
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return r.Notify("error", "Folder does not exist")
		}
		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Notify("error", "Invalid folder selected")
		}
		return finalizeProjectCreate(sid, path, name, true)
	})

	// Confirm creating a missing project folder, then create the project.
	registerAction(app, "project.create.confirm", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		path, _ := ctx.WsData()["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" {
			return r.Notify("error", "Folder path is required")
		}
		path = filepath.Clean(expandUserPath(path))
		if !filepath.IsAbs(path) {
			return r.Notify("error", "Path must be absolute")
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return r.Notify("error", "Failed to create folder: "+err.Error())
		}
		name, ok := projectNameForPath(sid, path)
		if !ok {
			return r.Notify("error", "Invalid folder selected")
		}
		return finalizeProjectCreate(sid, path, name, false)
	})

	switchToProjectName := func(sid, name string) string {
		if name == "" {
			return "/* noop */"
		}

		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)

		// Worktrees can exist before their virtual project has been created
		// in the current session. Resolve the matching worktree lazily.
		restoreWorktreeProject(sm, sid, name)

		if !sm.SwitchProject(sid, name) {
			return "/* noop */"
		}

		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, name) {
			// Project div exists in DOM, just hide/show
			jsSwitch = switchProjectJS(name, nil)
		} else {
			// Project div doesn't exist yet, append new content and hide old
			jsSwitch = switchProjectJS(name, renderMainArea(state, sid))
		}

		resp := newResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectAutolaunchJS(state, sid)).
			Add(focusSelectedAppJS(state))
		return resp.Build()
	}

	registerThreadActions(app, switchToProjectName)

	// Switch active project
	registerAction(app, "project.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		resp := switchToProjectName(sid, name)
		if resp == "/* noop */" {
			return r.Notify("error", "Project not found")
		}
		if id, _ := data["appId"].(string); id != "" {
			for index, panel := range sm.Get(sid).Apps {
				if panel.ID == id {
					sm.SelectApp(sid, index)
					resp += navigateJS(sm.Get(sid), sid)
					break
				}
			}
		}
		return resp
	})

	// Remove a project
	registerAction(app, "project.remove", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		if name == "" {
			return ""
		}

		// Check if we're removing the active project
		stateBefore := sm.Get(sid)
		wasActive := stateBefore.ActiveProject == name

		apps, ok := sm.RemoveProject(sid, name)
		if !ok {
			return r.Notify("error", "Cannot remove project")
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

		resp := newResponse().
			Add(parkFloatingPopupsJS()).
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(fmt.Sprintf(`(function(){var el=document.getElementById(%s);if(el)el.remove();})();`, components.JSString(projectMainID(name))))

		if wasActive {
			var content *r.Node
			if !sm.IsProjectRendered(sid, state.ActiveProject) {
				content = renderMainArea(state, sid)
			}
			resp.Add(switchProjectJS(state.ActiveProject, content)).
				Add(updateHashJS(state.ActiveProject)).
				Add(focusSelectedAppJS(state))
		}

		return resp.Build()
	})

	// Check if there are running apps before closing — returns JS to show dialog or force close
	registerAction(app, "app.close.check", func(ctx *r.Context) string {
		sid := extractSID(ctx)
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
			return fmt.Sprintf(`__ws.call('app.close.all',{sid:%s});`, components.JSString(sid))
		}

		return showCloseDialogJS(projectApps, "Quit Libro?", "Quit", "app.close.all", sid)
	})

	// Finish cleanup before allowing the renderer to close the desktop window.
	registerAction(app, "app.close.all", func(_ *r.Context) string {
		tm.StopAll()
		sm.mu.Lock()
		sm.states = make(map[string]*AppState)
		sm.mu.Unlock()
		return `if(window.libroElectron)window.libroElectron.forceClose();else window.close();`
	})

	// Switch to a worktree (creates virtual project if needed)
	registerAction(app, "worktree.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		parentProject, _ := data["project"].(string)
		wtPath, _ := data["path"].(string)
		branch, _ := data["branch"].(string)
		if parentProject == "" || wtPath == "" || branch == "" {
			return ""
		}
		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)

		vtName := parentProject + "/" + branch

		// Add virtual project if it doesn't exist
		sm.AddVirtualProject(sid, vtName, wtPath, parentProject)

		if !sm.SwitchProject(sid, vtName) {
			return r.Notify("error", "Failed to switch to worktree")
		}

		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}

		resp := newResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(projectAutolaunchJS(state, sid)).
			Add(focusSelectedAppJS(state))
		return resp.Build()
	})

	// Create a new worktree from the active project's current branch and switch to it.
	registerAction(app, "worktree.create", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		branch, _ := data["branch"].(string)
		branch = strings.TrimSpace(branch)
		if branch == "" {
			return r.Notify("error", "Branch name cannot be empty")
		}
		if strings.ContainsAny(branch, " \t\n\r~^:?*[\\") {
			return r.Notify("error", "Branch name contains invalid characters")
		}

		state := sm.Get(sid)
		if state == nil {
			return ""
		}

		var parentName, repoPath string
		for _, p := range state.Projects {
			if p.Name != state.ActiveProject {
				continue
			}
			if p.Virtual {
				parentName = p.ParentProject
				for _, pp := range state.Projects {
					if pp.Name == parentName {
						repoPath = pp.Path
						break
					}
				}
			} else {
				parentName = p.Name
				repoPath = p.Path
			}
			break
		}
		if repoPath == "" || !GitIsRepo(repoPath) {
			return r.Notify("error", "Current project is not a git repository")
		}

		vtName := parentName + "/" + branch
		for _, p := range state.Projects {
			if p.Name == vtName {
				return r.Notify("error", "Worktree for this branch already exists")
			}
		}

		safeBranch := strings.ReplaceAll(branch, "/", "-")
		wtPath := filepath.Join(filepath.Dir(repoPath), filepath.Base(repoPath)+"-"+safeBranch)

		if err := GitCreateWorktree(repoPath, branch, wtPath); err != nil {
			return r.Notify("error", "Failed to create worktree: "+err.Error())
		}

		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)

		sm.AddVirtualProject(sid, vtName, wtPath, parentName)

		if !sm.SwitchProject(sid, vtName) {
			return r.Notify("error", "Worktree created but failed to switch")
		}

		state = sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}

		return newResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(projectAutolaunchJS(state, sid)).
			Add(focusSelectedAppJS(state)).
			Build()
	})

	components.RegisterTerminalRoutes(app, tm, func(sid, terminalID string) bool {
		return sm.TerminalBelongsToSession(sid, terminalID)
	})

	// Live-switch native xterm themes when GNOME's color-scheme flips.
	var themeMu sync.Mutex
	components.WatchGnomeTheme(func() {
		themeMu.Lock()
		defer themeMu.Unlock()
		if err := app.Broadcast(actionResult(`(function(){if(window.__libroRefreshTerminalThemes)window.__libroRefreshTerminalThemes();})();`)); err != nil {
			log.Printf("libro: broadcast terminal theme: %v", err)
		}
	})

	if err := app.Listen(":" + Port()); err != nil {
		log.Printf("libro: app.Listen on :%s failed: %v", Port(), err)
	}
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

// extractSID gets the session ID from the action data payload
func extractSID(ctx *r.Context) string {
	data := ctx.WsData()
	if sid, ok := data["sid"].(string); ok {
		return sid
	}
	return "default"
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
