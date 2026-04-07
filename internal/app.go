package libro

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

// jsString returns a JSON-encoded string safe for embedding in JavaScript.
// It includes the surrounding quotes.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// dbSaveApp persists an app definition to the database for the given project.
// If editDBID > 0, it updates the app with that DB ID; otherwise it appends.
func dbSaveApp(projectName string, editDBID int64, appType, urlOrCmd, width, name string, writable, projectSpecific bool) {
	app := SavedApp{
		Type:            appType,
		Width:           width,
		Writable:        writable,
		Name:            name,
		ProjectSpecific: projectSpecific,
	}
	if appType == "terminal" {
		app.Command = urlOrCmd
		// Discover icon for commands not in the hardcoded map
		if info := lookupTermIcon(urlOrCmd); info == nil {
			app.IconURL = discoverTermIconURL(urlOrCmd)
		}
	} else {
		app.URL = urlOrCmd
	}
	if editDBID > 0 {
		DBUpdateSavedAppByID(editDBID, app)
	} else {
		DBAddSavedApp(projectName, app)
	}
}

// dbUpdateAppURL updates the URL of a saved app at the given index.
func dbUpdateAppURL(projectName string, index int, newURL string) {
	DBUpdateSavedAppURL(projectName, index, newURL)
}

var (
	sm = NewStateManager()
	tm = NewTtydManager()
)

// Run initializes and starts the Libro application server.
func Run(assets embed.FS) {
	KillStaleTtyd()
	InitDB()
	defer CloseDB()
	app := r.NewApp()
	app.Title = "Libro"
	app.Description = "Application Manager"
	app.Assets(assets, "assets", "/assets/")
	app.Favicon = "/assets/logo.svg"

	// Main page - generates a unique session ID per page load
	app.Page("/", func(ctx *r.Context) *r.Node {
		sid := sm.NewSession()
		state := sm.Get(sid)
		return renderPage(state, sid)
	})

	// Open add dialog
	app.Action("app.dialog.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		side := "right"
		if s, ok := data["side"].(string); ok && s != "" {
			side = s
		}
		sm.OpenDialog(sid, side)
		state := sm.Get(sid)
		js := r.Show(DialogID)
		if state.ManageOpen {
			js = removeManageOverlayJS() + js
		}
		return js
	})

	// Close add dialog
	app.Action("app.dialog.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.CloseDialog(sid)
		state := sm.Get(sid)
		js := r.Hide(DialogID)
		if state.ManageOpen {
			js += showManageOverlayJS(renderManageAppsPage(state, sid))
		}
		return js
	})

	// Add application (URL or Terminal)
	app.Action("app.save", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()

		appType, _ := data["app-type"].(string)

		// Determine selected width from radio button (name="app-width")
		width := WidthLG
		if val, ok := data["app-width"].(string); ok && val != "" {
			width = Width(val)
		}

		name := ""
		if val, ok := data["app-name"].(string); ok {
			name = strings.TrimSpace(val)
		}

		state := sm.Get(sid)
		editDBID := state.EditDBID

		projectSpecific := false
		if val, ok := data["app-project-specific"].(bool); ok {
			projectSpecific = val
		}

		if appType == "terminal" {
			command, _ := data["app-command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = "bash"
			}

			writable := true
			if val, ok := data["app-writable"].(bool); ok {
				writable = val
			}

			dbSaveApp(state.ActiveProject, editDBID, "terminal", command, string(width), name, writable, projectSpecific)
		} else {
			url, _ := data["app-url"].(string)
			url = strings.TrimSpace(url)
			if url == "" {
				return r.Notify("error", "URL is required")
			}
			url = ensureScheme(url)

			dbSaveApp(state.ActiveProject, editDBID, "url", url, string(width), name, false, projectSpecific)
		}

		sm.CloseDialog(sid)
		sm.Get(sid).EditDBID = -1

		resp := r.NewResponse().
			Replace(DialogID, renderAddDialog(false, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(savedAppsJS(state.ActiveProject))

		// Only replace main area when no apps are running (empty state),
		// to avoid destroying live iframes/webviews.
		if len(state.Apps) == 0 {
			resp.Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid))
		}

		// Re-show manage overlay if it was open before editing
		if state.ManageOpen {
			resp.Add(showManageOverlayJS(renderManageAppsPage(state, sid)))
		}

		return resp.Build()
	})

	// Quick browse - open URL or Google search
	app.Action("app.browse", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		query, _ := data["query"].(string)
		query = strings.TrimSpace(query)
		if query == "" {
			return ""
		}

		// Determine if input looks like a URL or a search query
		isURL := strings.Contains(query, ".") && !strings.Contains(query, " ")
		var target string
		if isURL {
			target = query
			target = ensureScheme(target)
		} else {
			// Google search
			target = "https://www.google.com/search?q=" + url.QueryEscape(query)
		}

		// Check if strip already exists
		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		// Save to browsed URL history (user-typed URLs only)
		go DBSaveBrowsedURL(target)
		sm.AddApp(sid, target, WidthLG, query)
		state := sm.Get(sid)

		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
		if hadApps > 0 {
			lastApp := state.Apps[len(state.Apps)-1]
			newIndex := len(state.Apps) - 1
			frame := renderAppFrame(lastApp, newIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + sidebarJS
		}

		return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + sidebarJS
	})

	// Start a saved/predefined application
	app.Action("app.start", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()

		appType, _ := data["type"].(string)
		width := WidthLG
		if val, ok := data["width"].(string); ok && val != "" {
			width = Width(val)
		}
		name, _ := data["name"].(string)
		side, _ := data["side"].(string)
		// Compute insertion index relative to currently selected app
		insertIdx := -1 // default: append
		if side == "left" {
			insertIdx = sm.SelectedIndex(sid)
		} else if side == "right" {
			insertIdx = sm.SelectedIndex(sid) + 1
		}

		pwd := sm.GetActiveProjectPath(sid)

		if appType == "terminal" {
			command, _ := data["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = "bash"
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
			port := sm.NextPort()
			if err := tm.Start(appID, port, command, writable, pwd); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			sm.InsertTerminalApp(sid, appID, command, port, writable, width, name, iconURL, insertIdx)
			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]

			time.Sleep(500 * time.Millisecond)

			topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
			sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
			if hadApps > 0 {
				frame := renderAppFrame(*newApp, state.SelectedIndex, true, sid, state.ZenMode)
				return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + sidebarJS
			}

			return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + sidebarJS
		}

		// URL app
		url, _ := data["url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			return r.Notify("error", "URL is required")
		}
		url = strings.ReplaceAll(url, "__dir__", pwd)
		url = ensureScheme(url)

		// Check if strip already exists
		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		sm.InsertApp(sid, url, width, name, insertIdx)
		state := sm.Get(sid)

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + sidebarJS
		}

		return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + sidebarJS
	})

	// Close/remove application
	app.Action("app.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return ""
		}

		// Check how many apps before removing
		state := sm.Get(sid)
		hadApps := len(state.Apps)

		removed := sm.RemoveAppByID(sid, appID)

		// Clean up associated processes
		if removed != nil {
			if removed.Type == AppTypeTerminal {
				tm.Stop(removed.ID)
			}
		}

		state = sm.Get(sid)
		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)

		// If no apps left, full replace to show empty state
		// Pool the webview first so it survives the DOM replace
		if len(state.Apps) == 0 || hadApps <= 1 {
			return poolWebviewJS(appID) + renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + sidebarJS
		}

		// Otherwise, remove just the app frame and update navigation
		return removeAppJS(appID) + navigateJS(state, sid) + topBarJS + sidebarJS
	})

	// Close current (selected) app — no app ID needed from client
	app.Action("app.close.current", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return ""
		}
		appID := state.Apps[state.SelectedIndex].ID
		hadApps := len(state.Apps)

		removed := sm.RemoveAppByID(sid, appID)
		if removed != nil {
			if removed.Type == AppTypeTerminal {
				tm.Stop(removed.ID)
			}
		}

		state = sm.Get(sid)
		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
		if len(state.Apps) == 0 || hadApps <= 1 {
			return poolWebviewJS(appID) + renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + sidebarJS
		}
		return removeAppJS(appID) + navigateJS(state, sid) + topBarJS + sidebarJS
	})

	// Navigate left - JS-only update to preserve iframes
	app.Action("app.navigate.left", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateLeft(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Navigate right - JS-only update to preserve iframes
	app.Action("app.navigate.right", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateRight(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Move app left — swap with neighbor, JS-only DOM swap to preserve iframes
	app.Action("app.move.left", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		if !sm.MoveAppLeft(sid) {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return moveAppJS(state, sid, "left") + renderTopBar(state, sid).ToJSReplace(TopBarID)
	})

	// Move app right — swap with neighbor, JS-only DOM swap to preserve iframes
	app.Action("app.move.right", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		if !sm.MoveAppRight(sid) {
			return "/* noop */"
		}
		state := sm.Get(sid)
		return moveAppJS(state, sid, "right") + renderTopBar(state, sid).ToJSReplace(TopBarID)
	})

	// Resize app to specific width — JS-only update to preserve iframes
	app.Action("app.resize", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		appID, _ := data["id"].(string)
		if appID == "" {
			return ""
		}
		width := WidthLG
		if v, ok := data["width"].(string); ok && v != "" {
			width = Width(v)
		}
		if sm.SetAppWidthByID(sid, appID, width) < 0 {
			return ""
		}
		state := sm.Get(sid)
		return resizeJS(state, width, appID)
	})

	// Select specific app - JS-only update to preserve iframes
	app.Action("app.select", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		idx := 0
		if v, ok := data["index"].(float64); ok {
			idx = int(v)
		}
		sm.SelectApp(sid, idx)
		state := sm.Get(sid)
		return navigateJS(state, sid) + updateAppPreviewJS(state)
	})

	// Open a new empty browser panel
	app.Action("app.browse.new", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		side, _ := data["side"].(string)
		// Compute insertion index relative to currently selected app
		insertIdx := -1 // default: append
		if side == "left" {
			insertIdx = sm.SelectedIndex(sid)
		} else if side == "right" {
			insertIdx = sm.SelectedIndex(sid) + 1
		}

		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		sm.InsertApp(sid, "", WidthLG, "New Tab", insertIdx)
		state := sm.Get(sid)

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + sidebarJS +
				fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, newApp.ID)
		}

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, state.Apps[state.SelectedIndex].ID)).
			Build()
	})

	// Quick run - open search dialog for command input
	app.Action("app.run.new", func(ctx *r.Context) string {
		data := ctx.WsData()
		side, _ := data["side"].(string)
		if side == "" {
			side = "right"
		}
		return fmt.Sprintf(`if(window.__libroOpenSearch)window.__libroOpenSearch('%s');`, side)
	})

	// Execute a terminal command directly (called from search dialog)
	app.Action("app.run.execute", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		command, _ := data["command"].(string)
		command = strings.TrimSpace(command)
		side, _ := data["side"].(string)
		if command == "" {
			return ""
		}

		insertIdx := -1
		if side == "left" {
			insertIdx = sm.SelectedIndex(sid)
		} else if side == "right" {
			insertIdx = sm.SelectedIndex(sid) + 1
		}

		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		appID := sm.NextAppID()
		port := sm.NextPort()
		pwd := sm.GetActiveProjectPath(sid)

		if err := tm.Start(appID, port, command, true, pwd); err != nil {
			return r.Notify("error", "Failed to start: "+err.Error())
		}

		// Save to run history
		go DBSaveRunCommand(command)

		time.Sleep(500 * time.Millisecond)

		// Insert a fully running terminal (not pending)
		sm.InsertTerminal(sid, appID, WidthLG, command, port, insertIdx)
		state := sm.Get(sid)

		// Update run commands JS
		runCmdsJS := runCommandsJS()

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		sidebarJS := renderProjectSidebar(state, sid).ToJSReplace(SidebarID)
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + sidebarJS + runCmdsJS
		}

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(runCmdsJS).
			Build()
	})

	// Set URL for an app — navigates the iframe to a new URL
	app.Action("app.url.set", func(ctx *r.Context) string {
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
		// Save to browsed URL history (user-typed URLs only)
		go DBSaveBrowsedURL(newURL)
		// Navigate webview and persist URL to DB
		state := sm.Get(sid)
		dbUpdateAppURL(state.ActiveProject, idx, newURL)
		return fmt.Sprintf(`(function(){window.__libroWvNavigate(%s,%s);var inp=document.getElementById('urlinput-'+%s);if(inp)inp.value=%s;})();`, jsString(appID), jsString(newURL), jsString(appID), jsString(newURL))
	})

	// Delete single history item
	app.Action("history.delete", func(ctx *r.Context) string {
		data := ctx.WsData()
		urlStr, _ := data["url"].(string)
		if urlStr == "" {
			return ""
		}
		DBDeleteBrowsedURL(urlStr)
		return fmt.Sprintf(`(function(){var u=%s;window.__libroBrowsedURLs=window.__libroBrowsedURLs.filter(function(x){return x!==u;});if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}})();`, jsString(urlStr))
	})

	// Clear browsing history
	app.Action("history.clear", func(ctx *r.Context) string {
		DBClearBrowsedURLs()
		return `window.__libroBrowsedURLs=[];if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}`
	})

	// Delete single run history item
	app.Action("run.history.delete", func(ctx *r.Context) string {
		data := ctx.WsData()
		command, _ := data["command"].(string)
		if command == "" {
			return ""
		}
		DBDeleteRunCommand(command)
		return fmt.Sprintf(`(function(){var c=%s;window.__libroRunCommands=(window.__libroRunCommands||[]).filter(function(x){return x!==c;});if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}})();`, jsString(command))
	})

	// Clear run history
	app.Action("run.history.clear", func(ctx *r.Context) string {
		DBClearRunCommands()
		return `window.__libroRunCommands=[];if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}`
	})

	// Browse directories for project picker
	app.Action("project.browse", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		path, _ := data["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" {
			return ""
		}
		// Resolve to absolute path and ensure it stays under home directory
		path = filepath.Clean(path)
		home, _ := os.UserHomeDir()
		if !filepath.IsAbs(path) || !strings.HasPrefix(path, home) {
			return ""
		}
		return r.NewResponse().
			Add(renderDirBrowser(path, sid).ToJSReplace(DirBrowserID)).
			Add(fmt.Sprintf("document.getElementById('project-path').value='%s';", path)).
			Build()
	})

	// Open project dialog
	app.Action("project.dialog.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.OpenProjectDialog(sid)
		return r.Show(ProjectDialogID)
	})

	// Close project dialog
	app.Action("project.dialog.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.CloseProjectDialog(sid)
		return r.Hide(ProjectDialogID)
	})

	// Create a new project
	app.Action("project.create", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()

		path, _ := data["project-path"].(string)
		path = strings.TrimSpace(path)

		if path == "" {
			return r.Notify("error", "Folder path is required")
		}

		name := filepath.Base(path)
		if name == "" || name == "." || name == "/" {
			return r.Notify("error", "Invalid folder selected")
		}

		if !sm.AddProject(sid, name, path) {
			return r.Notify("error", "Project '"+name+"' already exists")
		}

		// Persist project to DB
		DBSaveProject(name, path)

		sm.CloseProjectDialog(sid)
		sm.SwitchProject(sid, name)
		sm.AssignNavSlot(sid, name)
		sm.IsProjectRendered(sid, name) // mark as rendered
		state := sm.Get(sid)

		// New project always needs a new div appended, hide old project div
		jsSwitch := switchProjectJS(name, renderMainArea(state, sid))

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(ProjectDialogID, renderProjectDialog(false, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Build()
	})

	// Toggle sidebar collapsed/expanded
	app.Action("sidebar.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.ToggleSidebar(sid)
		state := sm.Get(sid)
		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Build()
	})

	// Toggle zen mode — hides top bar, sidebar, and app toolbars
	app.Action("zen.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.ToggleZenMode(sid)
		state := sm.Get(sid)
		resp := r.NewResponse().
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(SidebarID, renderProjectSidebar(state, sid))
		// Toggle toolbar visibility on all app frames and update zen borders
		if state.ZenMode {
			resp.Add(`document.querySelectorAll('[data-app-toolbar]').forEach(function(t){t.style.display='none';});`)
		} else {
			resp.Add(`document.querySelectorAll('[data-app-toolbar]').forEach(function(t){t.style.display='';});`)
		}
		// navigateJS handles zen border classes on selected/unselected apps
		resp.Add(navigateJS(state, sid))
		return resp.Build()
	})

	// Switch active project
	app.Action("project.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)

		if !sm.SwitchProject(sid, name) {
			return r.Notify("error", "Project not found")
		}

		// Assign nav slot when switching via sidebar click (not for home)
		if name != "home" {
			sm.AssignNavSlot(sid, name)
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

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Navigate to next project
	app.Action("project.navigate.next", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.NextProject(sid)
		if name == "" {
			return "/* noop */"
		}
		sm.SwitchProject(sid, name)
		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, name) {
			jsSwitch = switchProjectJS(name, nil)
		} else {
			jsSwitch = switchProjectJS(name, renderMainArea(state, sid))
		}

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Navigate to previous project
	app.Action("project.navigate.prev", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.PrevProject(sid)
		if name == "" {
			return "/* noop */"
		}
		sm.SwitchProject(sid, name)
		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, name) {
			jsSwitch = switchProjectJS(name, nil)
		} else {
			jsSwitch = switchProjectJS(name, renderMainArea(state, sid))
		}

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Select project by nav slot (Ctrl+1 = home, Ctrl+2-9 = assigned slots)
	app.Action("project.select", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		slot := 0
		if v, ok := data["index"].(float64); ok {
			slot = int(v) + 1 // JS sends 0-based, convert to 1-based slot
		}

		var name string
		if slot == 1 {
			// Ctrl+1 always goes to home
			name = "home"
		} else {
			// Ctrl+2-9: look up from nav slots
			name = sm.NavSlotProject(sid, slot)
		}
		if name == "" {
			return "/* noop */"
		}
		if !sm.SwitchProject(sid, name) {
			return "/* noop */"
		}
		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, name) {
			jsSwitch = switchProjectJS(name, nil)
		} else {
			jsSwitch = switchProjectJS(name, renderMainArea(state, sid))
		}

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Select previous project (Ctrl+0)
	app.Action("project.select.last", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.PreviousProject(sid)
		if name == "" {
			return "/* noop */"
		}
		if !sm.SwitchProject(sid, name) {
			return "/* noop */"
		}
		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, name) {
			jsSwitch = switchProjectJS(name, nil)
		} else {
			jsSwitch = switchProjectJS(name, renderMainArea(state, sid))
		}

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Remove a project
	app.Action("project.remove", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		if name == "" || name == "home" {
			return ""
		}

		// Check if we're removing the active project
		stateBefore := sm.Get(sid)
		wasActive := stateBefore.ActiveProject == name

		// If removing active project, switch to home first
		if wasActive {
			sm.SwitchProject(sid, "home")
		}

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

		resp := r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(fmt.Sprintf(`(function(){var el=document.getElementById('project-main-%s');if(el)el.remove();})();`, name))

		if wasActive {
			resp.Replace(projectMainID("home"), renderMainArea(state, sid)).
				Add(updateHashJS("home"))
		}

		return resp.Build()
	})

	// Toggle manage apps overlay
	app.Action("app.manage.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if state.ManageOpen {
			state.ManageOpen = false
			return removeManageOverlayJS()
		}
		state.ManageOpen = true
		return showManageOverlayJS(renderManageAppsPage(state, sid))
	})

	// Close manage apps overlay
	app.Action("app.manage.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		state.ManageOpen = false
		return removeManageOverlayJS()
	})

	// Delete a saved app from DB
	app.Action("app.saved.delete", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		var dbid int64
		if v, ok := data["dbid"].(float64); ok {
			dbid = int64(v)
		}
		if dbid <= 0 {
			return ""
		}
		DBRemoveSavedAppByID(dbid)
		state := sm.Get(sid)
		// Re-render the manage overlay and top bar to reflect the deletion
		return r.NewResponse().
			Replace(ManageDialogID, renderManageAppsPage(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(savedAppsJS(state.ActiveProject)).
			Build()
	})

	// Set edit DB ID for editing a saved app
	app.Action("app.saved.edit", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		var dbid int64
		if v, ok := data["dbid"].(float64); ok {
			dbid = int64(v)
		}
		state := sm.Get(sid)
		state.EditDBID = dbid
		return ""
	})

	// Check if there are running apps before closing — returns JS to show dialog or force close
	app.Action("app.close.check", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectApps := sm.GetAllRunningApps(sid)

		// No running apps — close immediately
		if len(projectApps) == 0 {
			return `if(window.libroElectron)window.libroElectron.forceClose();else window.close();`
		}

		// Build tree HTML: project > apps
		var html strings.Builder
		for _, pa := range projectApps {
			html.WriteString(fmt.Sprintf(`<div class="mb-2"><div class="flex items-center gap-1.5 text-xs font-medium text-gray-700 dark:text-zinc-300 mb-1"><span class="material-icons-round text-sm">folder</span>%s</div>`, pa.Name))
			for _, a := range pa.Apps {
				icon := "language"
				label := a.Name
				if a.Type == AppTypeTerminal {
					icon = "terminal"
					if label == "" {
						label = a.Command
					}
				} else {
					if label == "" {
						label = a.URL
					}
				}
				html.WriteString(fmt.Sprintf(`<div class="flex items-center gap-1.5 ml-5 py-0.5 text-xs text-gray-500 dark:text-zinc-500"><span class="material-icons-round text-xs">%s</span><span class="truncate">%s</span></div>`, icon, label))
			}
			html.WriteString(`</div>`)
		}

		return fmt.Sprintf(`(function(){var el=document.getElementById('close-dialog-apps');if(el)el.innerHTML=%s;document.getElementById('%s').classList.remove('hidden');var cb=document.getElementById('close-dialog-confirm');if(cb)cb.focus();})();`,
			jsString(html.String()), CloseDialogID)
	})

	// Close all running apps — the client handles window close separately
	app.Action("app.close.all", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectApps := sm.GetAllRunningApps(sid)

		// Stop all terminal processes across all projects
		for _, pa := range projectApps {
			for _, a := range pa.Apps {
				if a.Type == AppTypeTerminal {
					tm.Stop(a.ID)
				}
			}
		}

		return ""
	})

	// Switch to a worktree (creates virtual project if needed)
	app.Action("worktree.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		parentProject, _ := data["project"].(string)
		wtPath, _ := data["path"].(string)
		branch, _ := data["branch"].(string)
		if parentProject == "" || wtPath == "" || branch == "" {
			return ""
		}

		vtName := parentProject + "/" + branch

		// Add virtual project if it doesn't exist
		sm.AddVirtualProject(sid, vtName, wtPath, parentProject)

		if !sm.SwitchProject(sid, vtName) {
			return r.Notify("error", "Failed to switch to worktree")
		}

		// Assign nav slot for worktree switch (reuses parent project's slot)
		sm.AssignNavSlot(sid, vtName)

		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}

		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
			Build()
	})

	// Add a new worktree
	app.Action("worktree.add", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		parentProject, _ := data["project"].(string)
		branch, _ := data["branch"].(string)
		branch = strings.TrimSpace(branch)
		if parentProject == "" || branch == "" {
			return r.Notify("error", "Branch name is required")
		}

		projectPath := sm.GetProjectPath(sid, parentProject)
		if projectPath == "" {
			return r.Notify("error", "Project not found")
		}

		// Create worktree at sibling directory: ../projectName-branchName
		targetPath := filepath.Join(filepath.Dir(projectPath), filepath.Base(projectPath)+"-"+branch)

		if err := GitAddWorktree(projectPath, branch, targetPath); err != nil {
			return r.Notify("error", "Failed to add worktree: "+err.Error())
		}

		state := sm.Get(sid)
		return r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(worktreesJS(state)).
			Build()
	})

	// Remove a worktree
	app.Action("worktree.remove", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		parentProject, _ := data["project"].(string)
		wtPath, _ := data["path"].(string)
		if parentProject == "" || wtPath == "" {
			return ""
		}

		projectPath := sm.GetProjectPath(sid, parentProject)
		if projectPath == "" {
			return r.Notify("error", "Project not found")
		}

		// Find and remove virtual project for this worktree
		state := sm.Get(sid)
		for _, p := range state.Projects {
			if p.Virtual && p.Path == wtPath {
				wasActive := state.ActiveProject == p.Name
				apps, ok := sm.RemoveVirtualProject(sid, p.Name)
				if ok {
					for _, a := range apps {
						if a.Type == AppTypeTerminal {
							tm.Stop(a.ID)
						}
					}
				}
				if wasActive {
					sm.SwitchProject(sid, parentProject)
				}
				break
			}
		}

		if err := GitRemoveWorktree(projectPath, wtPath); err != nil {
			return r.Notify("error", "Failed to remove worktree: "+err.Error())
		}

		state = sm.Get(sid)
		resp := r.NewResponse().
			Replace(SidebarID, renderProjectSidebar(state, sid)).
			Add(worktreesJS(state))

		return resp.Build()
	})

	// Open worktree add dialog
	app.Action("worktree.dialog.open", func(ctx *r.Context) string {
		data := ctx.WsData()
		project, _ := data["project"].(string)
		if project == "" {
			return ""
		}
		// Return JS to open the worktree dialog with project context
		return fmt.Sprintf(`(function(){if(window.__libroOpenWorktreeDialog)window.__libroOpenWorktreeDialog(%s);})();`, jsString(project))
	})

	registerTtydProxy(app)
	app.Listen(":" + Port())
}

// ensureScheme adds http:// for local URLs and https:// for everything else.
func ensureScheme(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.0.0.1") || strings.HasPrefix(u, "0.0.0.0") ||
		strings.HasPrefix(u, "[::1]") || strings.HasPrefix(u, "[::0]") || strings.HasPrefix(u, "[::]") {
		return "http://" + u
	}
	// If it doesn't look like a URL, treat it as a Google search query
	if looksLikeSearchQuery(u) {
		return "https://www.google.com/search?q=" + url.QueryEscape(u)
	}
	return "https://" + u
}

// looksLikeSearchQuery returns true if the input doesn't look like a valid URL
// (e.g. contains spaces, has no dot, or no valid TLD-like segment).
func looksLikeSearchQuery(u string) bool {
	// Contains spaces → almost certainly a search query
	if strings.Contains(u, " ") {
		return true
	}
	// No dot and no colon (port) → not a domain
	if !strings.Contains(u, ".") && !strings.Contains(u, ":") {
		return true
	}
	return false
}

// extractSID gets the session ID from the action data payload
func extractSID(ctx *r.Context) string {
	data := ctx.WsData()
	if sid, ok := data["sid"].(string); ok {
		return sid
	}
	return "default"
}
