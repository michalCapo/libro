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
// If editIndex >= 0, it updates the app at that index; otherwise it appends.
func dbSaveApp(projectName string, editIndex int, appType, urlOrCmd, width, name string, writable bool) {
	app := SavedApp{
		Type:     appType,
		Width:    width,
		Writable: writable,
		Name:     name,
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
	if editIndex >= 0 {
		DBUpdateSavedApp(projectName, editIndex, app)
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
		return r.Show(DialogID)
	})

	// Close add dialog
	app.Action("app.dialog.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.CloseDialog(sid)
		return r.Hide(DialogID)
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
		editIdx := state.EditIndex

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

			dbSaveApp(state.ActiveProject, editIdx, "terminal", command, string(width), name, writable)
		} else {
			url, _ := data["app-url"].(string)
			url = strings.TrimSpace(url)
			if url == "" {
				return r.Notify("error", "URL is required")
			}
			url = ensureScheme(url)

			dbSaveApp(state.ActiveProject, editIdx, "url", url, string(width), name, false)
		}

		sm.CloseDialog(sid)
		sm.Get(sid).EditIndex = -1

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(DialogID, renderAddDialog(false, sid)).
			Build()
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

		if hadApps > 0 {
			lastApp := state.Apps[len(state.Apps)-1]
			newIndex := len(state.Apps) - 1
			frame := renderAppFrame(lastApp, newIndex, true, sid)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid)
		}

		return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
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
		prepend := side == "left"

		if appType == "terminal" {
			command, _ := data["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = "bash"
			}

			writable := true
			if val, ok := data["writable"].(bool); ok {
				writable = val
			}

			iconURL, _ := data["iconUrl"].(string)

			// Check if strip already exists
			stateBefore := sm.Get(sid)
			hadApps := len(stateBefore.Apps)

			pwd := sm.GetActiveProjectPath(sid)
			appID := sm.NextAppID()
			port := sm.NextPort()
			if err := tm.Start(appID, port, command, writable, pwd); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			if prepend {
				sm.PrependTerminalApp(sid, appID, command, port, writable, width, name, iconURL)
			} else {
				sm.AddTerminalApp(sid, appID, command, port, writable, width, name, iconURL)
			}
			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]

			time.Sleep(500 * time.Millisecond)

			if hadApps > 0 {
				frame := renderAppFrame(*newApp, state.SelectedIndex, true, sid)
				return insertAppJS(frame, prepend, state.ActiveProject) + navigateJS(state, sid)
			}

			return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
		}

		// URL app
		url, _ := data["url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			return r.Notify("error", "URL is required")
		}
		url = ensureScheme(url)

		// Check if strip already exists
		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		if prepend {
			sm.PrependApp(sid, url, width, name)
		} else {
			sm.AddApp(sid, url, width, name)
		}
		state := sm.Get(sid)

		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid)
			return insertAppJS(frame, prepend, state.ActiveProject) + navigateJS(state, sid)
		}

		return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
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

		// If no apps left, full replace to show empty state
		if len(state.Apps) == 0 || hadApps <= 1 {
			return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
		}

		// Otherwise, remove just the app frame and update navigation
		return removeAppJS(appID) + navigateJS(state, sid)
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
		if len(state.Apps) == 0 || hadApps <= 1 {
			return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
		}
		return removeAppJS(appID) + navigateJS(state, sid)
	})

	// Navigate left - JS-only update to preserve iframes
	app.Action("app.navigate.left", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateLeft(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid)
	})

	// Navigate right - JS-only update to preserve iframes
	app.Action("app.navigate.right", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		sm.NavigateRight(sid)
		state := sm.Get(sid)
		return navigateJS(state, sid)
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
		return navigateJS(state, sid)
	})

	// Open a new empty browser panel
	app.Action("app.browse.new", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		side, _ := data["side"].(string)
		prepend := side == "left"

		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		if prepend {
			sm.PrependApp(sid, "", WidthLG, "New Tab")
		} else {
			sm.AddApp(sid, "", WidthLG, "New Tab")
		}
		state := sm.Get(sid)

		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid)
			return insertAppJS(frame, prepend, state.ActiveProject) + navigateJS(state, sid) +
				fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, newApp.ID)
		}

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Add(fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, state.Apps[state.SelectedIndex].ID)).
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

	// Clear browsing history
	app.Action("history.clear", func(ctx *r.Context) string {
		DBClearBrowsedURLs()
		return `window.__libroBrowsedURLs=[];if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}`
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
		sm.IsProjectRendered(sid, name) // mark as rendered
		state := sm.Get(sid)

		// New project always needs a new div appended, hide old project div
		jsSwitch := switchProjectJS(name, renderMainArea(state, sid))

		return r.NewResponse().
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Replace(ProjectDialogID, renderProjectDialog(false, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Build()
	})

	// Switch active project
	app.Action("project.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)

		if !sm.SwitchProject(sid, name) {
			return r.Notify("error", "Project not found")
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
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
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
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
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
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(focusSelectedAppJS(state.SelectedIndex)).
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
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Add(fmt.Sprintf(`(function(){var el=document.getElementById('project-main-%s');if(el)el.remove();})();`, name))

		if wasActive {
			resp.Replace(projectMainID("home"), renderMainArea(state, sid)).
				Add(updateHashJS("home"))
		}

		return resp.Build()
	})

	// Delete a saved app from DB
	app.Action("app.saved.delete", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		idx := -1
		if v, ok := data["index"].(float64); ok {
			idx = int(v)
		}
		if idx < 0 {
			return ""
		}
		state := sm.Get(sid)
		DBRemoveSavedApp(state.ActiveProject, idx)
		// Re-render the empty state / main area to refresh saved apps list
		return renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject))
	})

	// Set edit index for editing a saved app
	app.Action("app.saved.edit", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		idx := -1
		if v, ok := data["index"].(float64); ok {
			idx = int(v)
		}
		state := sm.Get(sid)
		state.EditIndex = idx
		return ""
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
