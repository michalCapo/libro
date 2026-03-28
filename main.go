package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

// saveToLocalStorageJS returns JS that saves or updates an app definition in localStorage
func saveToLocalStorageJS(appType, urlOrCmd, width, name string, writable bool) string {
	return fmt.Sprintf(`
(function(){
	var apps=JSON.parse(localStorage.getItem('libro-apps')||'[]');
	var entry={type:'%s',width:'%s',writable:%v,name:'%s'};
	if(entry.type==='terminal'){entry.command='%s';}else{entry.url='%s';}
	var editIdx=localStorage.getItem('libro-edit-idx');
	if(editIdx!==null){
		var idx=parseInt(editIdx);
		if(idx>=0&&idx<apps.length){apps[idx]=entry;}else{apps.push(entry);}
		localStorage.removeItem('libro-edit-idx');
	}else{
		apps.push(entry);
	}
	localStorage.setItem('libro-apps',JSON.stringify(apps));
})();
`, appType, width, writable, name, urlOrCmd, urlOrCmd)
}

// updateLocalStorageURLJS returns JS that updates the URL of an app at a given index in localStorage
func updateLocalStorageURLJS(index int, newURL string) string {
	return fmt.Sprintf(`
(function(){
	var apps=JSON.parse(localStorage.getItem('libro-apps')||'[]');
	if(%d>=0&&%d<apps.length){apps[%d].url='%s';localStorage.setItem('libro-apps',JSON.stringify(apps));}
})();
`, index, index, index, newURL)
}

var (
	sm = NewStateManager()
	tm = NewTtydManager()
	cm = NewChromeManager()
)

func main() {
	app := r.NewApp()
	app.Title = "Libro"
	app.Description = "Application Manager"

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
	app.Action("app.add", func(ctx *r.Context) string {
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

		// Determine insertion side from dialog state
		stateBefore := sm.Get(sid)
		prepend := stateBefore.DialogSide == "left"
		hadApps := len(stateBefore.Apps)

		if appType == "terminal" {
			// Terminal (ttyd) app
			command, _ := data["app-command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = "bash"
			}

			writable := true
			if val, ok := data["app-writable"].(bool); ok {
				writable = val
			}

			pwd := sm.GetActiveProjectPath(sid)
			port := sm.NextPort()
			if err := tm.Start("pending", port, command, writable, pwd); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			if prepend {
				sm.PrependTerminalApp(sid, command, port, writable, width, name)
			} else {
				sm.AddTerminalApp(sid, command, port, writable, width, name)
			}
			state := sm.Get(sid)
			// Update the ttyd app ID to match the actual app ID for process tracking
			newApp := &state.Apps[state.SelectedIndex]
			tm.mu.Lock()
			if cmd, ok := tm.processes["pending"]; ok {
				delete(tm.processes, "pending")
				tm.processes[newApp.ID] = cmd
			}
			tm.mu.Unlock()

			sm.CloseDialog(sid)

			// Small delay to let ttyd start
			time.Sleep(500 * time.Millisecond)

			if hadApps > 0 {
				frame := renderAppFrame(*newApp, state.SelectedIndex, true, sid)
				return r.NewResponse().
					Replace(DialogID, renderAddDialog(false, sid)).
					Add(insertAppJS(frame, prepend, state.ActiveProject)).
					Add(navigateJS(state, sid)).
					Add(saveToLocalStorageJS("terminal", command, string(width), name, writable)).
					Build()
			}

			return r.NewResponse().
				Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
				Replace(DialogID, renderAddDialog(false, sid)).
				Add(saveToLocalStorageJS("terminal", command, string(width), name, writable)).
				Build()
		}

		// URL app
		url, _ := data["app-url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			return r.Notify("error", "URL is required")
		}

		url = ensureScheme(url)

		if prepend {
			sm.PrependApp(sid, url, width, name)
		} else {
			sm.AddApp(sid, url, width, name)
		}
		sm.CloseDialog(sid)
		state := sm.Get(sid)

		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFrame(newApp, state.SelectedIndex, true, sid)
			return r.NewResponse().
				Replace(DialogID, renderAddDialog(false, sid)).
				Add(insertAppJS(frame, prepend, state.ActiveProject)).
				Add(navigateJS(state, sid)).
				Add(saveToLocalStorageJS("url", url, string(width), name, false)).
				Build()
		}

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(DialogID, renderAddDialog(false, sid)).
			Add(saveToLocalStorageJS("url", url, string(width), name, false)).
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

			// Check if strip already exists
			stateBefore := sm.Get(sid)
			hadApps := len(stateBefore.Apps)

			pwd := sm.GetActiveProjectPath(sid)
			port := sm.NextPort()
			if err := tm.Start("pending", port, command, writable, pwd); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			if prepend {
				sm.PrependTerminalApp(sid, command, port, writable, width, name)
			} else {
				sm.AddTerminalApp(sid, command, port, writable, width, name)
			}
			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]
			tm.mu.Lock()
			if cmd, ok := tm.processes["pending"]; ok {
				delete(tm.processes, "pending")
				tm.processes[newApp.ID] = cmd
			}
			tm.mu.Unlock()

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
			} else if removed.Type == AppTypeURL {
				cm.closeTab(removed.ID)
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
		// Navigate Chrome tab (if connected) and update localStorage
		return fmt.Sprintf(`(function(){var c=window.__chromeWS&&window.__chromeWS['%s'];if(c&&c.readyState===1)c.send(JSON.stringify({t:'nav',url:'%s'}));var inp=document.getElementById('urlinput-%s');if(inp)inp.value='%s';})();`, appID, newURL, appID, newURL) +
			updateLocalStorageURLJS(idx, newURL)
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
		return renderDirBrowser(path, sid).ToJSReplace(DirBrowserID)
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

		name, _ := data["project-name"].(string)
		name = strings.TrimSpace(name)
		path, _ := data["project-path"].(string)
		path = strings.TrimSpace(path)

		if name == "" {
			return r.Notify("error", "Project name is required")
		}
		if path == "" {
			return r.Notify("error", "Folder path is required")
		}

		if !sm.AddProject(sid, name, path) {
			return r.Notify("error", "Project '"+name+"' already exists")
		}

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
			Add(saveProjectToLocalStorageJS(name, path)).
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
			} else if a.Type == AppTypeURL {
				cm.closeTab(a.ID)
			}
		}

		state := sm.Get(sid)

		resp := r.NewResponse().
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Add(removeProjectFromLocalStorageJS(name)).
			Add(fmt.Sprintf(`(function(){var el=document.getElementById('project-main-%s');if(el)el.remove();})();`, name))

		if wasActive {
			resp.Replace(projectMainID("home"), renderMainArea(state, sid)).
				Add(updateHashJS("home"))
		}

		return resp.Build()
	})

	// Initialize projects from localStorage on page load
	app.Action("project.init", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		projects, ok := data["projects"].([]interface{})
		if !ok || len(projects) == 0 {
			return ""
		}
		for _, p := range projects {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := pm["name"].(string)
			path, _ := pm["path"].(string)
			if name != "" && path != "" && name != "home" {
				sm.AddProject(sid, name, path)
			}
		}
		state := sm.Get(sid)
		return renderProjectBar(state, sid).ToJSReplace(ProjectBarID)
	})

	registerTtydProxy(app)
	registerChrome(app)
	app.Listen(":1439")
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
