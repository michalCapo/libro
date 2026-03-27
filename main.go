package main

import (
	"fmt"
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
		sm.OpenDialog(sid)
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

			port := sm.NextPort()
			if err := tm.Start("pending", port, command, writable); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			sm.AddTerminalApp(sid, command, port, writable, width, name)
			state := sm.Get(sid)
			// Update the ttyd app ID to match the actual app ID for process tracking
			lastApp := &state.Apps[len(state.Apps)-1]
			tm.mu.Lock()
			if cmd, ok := tm.processes["pending"]; ok {
				delete(tm.processes, "pending")
				tm.processes[lastApp.ID] = cmd
			}
			tm.mu.Unlock()

			sm.CloseDialog(sid)

			// Small delay to let ttyd start
			time.Sleep(500 * time.Millisecond)

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

		sm.AddApp(sid, url, width, name)
		sm.CloseDialog(sid)
		state := sm.Get(sid)

		return r.NewResponse().
			Replace(MainAreaID, renderMainArea(state, sid)).
			Replace(DialogID, renderAddDialog(false, sid)).
			Add(saveToLocalStorageJS("url", url, string(width), name, false)).
			Build()
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

			port := sm.NextPort()
			if err := tm.Start("pending", port, command, writable); err != nil {
				return r.Notify("error", "Failed to start ttyd: "+err.Error())
			}

			sm.PrependTerminalApp(sid, command, port, writable, width, name)
			state := sm.Get(sid)
			lastApp := &state.Apps[0]
			tm.mu.Lock()
			if cmd, ok := tm.processes["pending"]; ok {
				delete(tm.processes, "pending")
				tm.processes[lastApp.ID] = cmd
			}
			tm.mu.Unlock()

			time.Sleep(500 * time.Millisecond)

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

		sm.PrependApp(sid, url, width, name)
		state := sm.Get(sid)
		return renderMainArea(state, sid).ToJSReplace(MainAreaID)
	})

	// Close/remove application
	app.Action("app.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		var data struct {
			Index float64 `json:"index"`
		}
		ctx.Body(&data)
		removed := sm.RemoveApp(sid, int(data.Index))

		// If it was a terminal app, stop the ttyd process
		if removed != nil && removed.Type == AppTypeTerminal {
			tm.Stop(removed.ID)
		}

		state := sm.Get(sid)
		return renderMainArea(state, sid).ToJSReplace(MainAreaID)
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

	// Resize app to specific width
	app.Action("app.resize", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		index := 0
		if v, ok := data["index"].(float64); ok {
			index = int(v)
		}
		width := WidthLG
		if v, ok := data["width"].(string); ok && v != "" {
			width = Width(v)
		}
		sm.SetAppWidth(sid, index, width)
		state := sm.Get(sid)
		return renderMainArea(state, sid).ToJSReplace(MainAreaID)
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

	registerProxy(app)
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
