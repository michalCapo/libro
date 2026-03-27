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

var (
	sm = NewStateManager()
	tm = NewTtydManager()
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
					Add(insertAppJS(frame, prepend)).
					Add(navigateJS(state, sid)).
					Add(saveToLocalStorageJS("terminal", command, string(width), name, writable)).
					Build()
			}

			return r.NewResponse().
				Replace(MainAreaID, renderMainArea(state, sid)).
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

		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}

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
				Add(insertAppJS(frame, prepend)).
				Add(navigateJS(state, sid)).
				Add(saveToLocalStorageJS("url", url, string(width), name, false)).
				Build()
		}

		return r.NewResponse().
			Replace(MainAreaID, renderMainArea(state, sid)).
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
			if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				target = "https://" + target
			}
		} else {
			// Google search - igu=1 enables iframe-friendly mode
			target = "https://www.google.com/search?igu=1&q=" + url.QueryEscape(query)
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
			return insertAppJS(frame, false) + navigateJS(state, sid)
		}

		return renderMainArea(state, sid).ToJSReplace(MainAreaID)
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
				return insertAppJS(frame, prepend) + navigateJS(state, sid)
			}

			return renderMainArea(state, sid).ToJSReplace(MainAreaID)
		}

		// URL app
		url, _ := data["url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			return r.Notify("error", "URL is required")
		}
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}

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
			return insertAppJS(frame, prepend) + navigateJS(state, sid)
		}

		return renderMainArea(state, sid).ToJSReplace(MainAreaID)
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

		// If it was a terminal app, stop the ttyd process
		if removed != nil && removed.Type == AppTypeTerminal {
			tm.Stop(removed.ID)
		}

		state = sm.Get(sid)

		// If no apps left, full replace to show empty state
		if len(state.Apps) == 0 || hadApps <= 1 {
			return renderMainArea(state, sid).ToJSReplace(MainAreaID)
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
		var data struct {
			Index float64 `json:"index"`
		}
		ctx.Body(&data)
		sm.SelectApp(sid, int(data.Index))
		state := sm.Get(sid)
		return navigateJS(state, sid)
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
		state := sm.Get(sid)

		return r.NewResponse().
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Replace(MainAreaID, renderMainArea(state, sid)).
			Replace(ProjectDialogID, renderProjectDialog(false, sid)).
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
		return r.NewResponse().
			Replace(ProjectBarID, renderProjectBar(state, sid)).
			Replace(MainAreaID, renderMainArea(state, sid)).
			Add(updateHashJS(name)).
			Build()
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

	registerProxy(app)
	registerTtydProxy(app)
	app.Listen(":1439")
}

// extractSID gets the session ID from the action data payload
func extractSID(ctx *r.Context) string {
	data := ctx.WsData()
	if sid, ok := data["sid"].(string); ok {
		return sid
	}
	return "default"
}
