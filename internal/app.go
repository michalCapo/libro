// Package libro implements the Libro server, desktop bundling, and UI components.
package libro

import (
	"embed"
	"encoding/json"
	"fmt"
	"libro/internal/components"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

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
	ensureProjectNavSlot(sid, name, !transient)
	sm.IsProjectRendered(sid, name)
	state := sm.Get(sid)

	jsSwitch := switchProjectJS(name, renderMainArea(state, sid))

	resp := r.NewResponse().
		Add(projectsJS(state)).
		Replace(TopBarID, renderTopBar(state, sid)).
		Replace(ProjectDialogID, renderProjectDialog(false, sid)).
		Add(`if(window.__libroProjectDialogBind)window.__libroProjectDialogBind();`).
		Add(jsSwitch).
		Add(updateHashJS(name)).
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

func copyPasswordFieldJS(text, label string) string {
	return fmt.Sprintf(`
(function(){
	var text=%s;
	if(window.libroElectron&&window.libroElectron.copyToClipboard){
		window.libroElectron.copyToClipboard(text);
	}else if(navigator.clipboard&&navigator.clipboard.writeText){
		navigator.clipboard.writeText(text);
	}else{
		var ta=document.createElement('textarea');
		ta.value=text;
		ta.style.position='fixed';
		ta.style.opacity='0';
		document.body.appendChild(ta);
		ta.focus();
		ta.select();
		try{document.execCommand('copy');}catch(e){}
		document.body.removeChild(ta);
	}
	if(window.__libroShowToast)window.__libroShowToast(%s,'',1400);
})();
`, components.JSString(text), components.JSString(label))
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

func ensureProjectNavSlot(sid, name string, persist bool) int {
	if name == "" || name == "home" {
		return 0
	}
	if slot := sm.GetNavSlotForProject(sid, name); slot >= 2 && slot <= 9 {
		return slot
	}
	slot := sm.AssignNavSlot(sid, name)
	if persist && slot >= 2 && slot <= 9 {
		DBSetProjectNavSlot(name, slot)
	}
	return slot
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

	restoreClosedApps := func(sid string, snap *projectSnapshot) (int, []string) {
		if snap == nil || len(snap.Apps) == 0 {
			sm.RestoreActiveProjectApps(sid, nil, 0)
			return 0, nil
		}
		pwd := sm.GetActiveProjectPath(sid)
		restored := make([]Application, 0, len(snap.Apps))
		skipped := make([]string, 0)
		restoredSelectedIndex := snap.SelectedIndex
		originalSelectedIndex := max(snap.SelectedIndex, 0)
		if originalSelectedIndex >= len(snap.Apps) {
			originalSelectedIndex = len(snap.Apps) - 1
		}
		for originalIndex, prev := range snap.Apps {
			switch prev.Type {
			case AppTypeTerminal:
				appID := sm.NextAppID()
				command := strings.ReplaceAll(prev.Command, "__dir__", pwd)
				if command == "" {
					command = components.UserShellBase()
				}
				session, err := tm.Start(appID, command, pwd, prev.Writable)
				if err != nil {
					name := strings.TrimSpace(prev.Name)
					if name == "" {
						name = strings.TrimSpace(prev.Command)
					}
					if name == "" {
						name = "terminal"
					}
					skipped = append(skipped, name)
					if originalIndex < originalSelectedIndex && restoredSelectedIndex > 0 {
						restoredSelectedIndex--
					}
					continue
				}
				restored = append(restored, Application{
					ID:            appID,
					Type:          AppTypeTerminal,
					Command:       command,
					Width:         prev.Width,
					PreviousWidth: prev.PreviousWidth,
					Writable:      prev.Writable,
					Name:          prev.Name,
					IconURL:       prev.IconURL,
					TerminalID:    session.ID,
					TerminalReady: true,
				})
			default:
				appID := sm.NextAppID()
				restored = append(restored, Application{
					ID:            appID,
					Type:          AppTypeURL,
					URL:           prev.URL,
					Width:         prev.Width,
					PreviousWidth: prev.PreviousWidth,
					Name:          prev.Name,
					IconURL:       prev.IconURL,
				})
			}
		}
		if restoredSelectedIndex < 0 {
			restoredSelectedIndex = 0
		}
		sm.RestoreActiveProjectApps(sid, restored, restoredSelectedIndex)
		return len(restored), skipped
	}

	// Main page - generates a unique session ID per page load
	app.Page("/", func(ctx *r.Context) *r.Node {
		sid := sm.NewSession()
		state := sm.Get(sid)
		return renderPage(state, sid)
	})

	app.Action("password.setup", func(ctx *r.Context) string {
		data := ctx.WsData()
		master, _ := data["master"].(string)
		confirm, _ := data["confirm"].(string)
		if master != confirm {
			return r.Notify("error", "Master passwords do not match")
		}
		if err := setupPasswordVault(master); err != nil {
			return r.Notify("error", err.Error())
		}
		return passwordVaultStatusJS() + passwordEntriesJS() + `document.getElementById('password-setup-master').value='';document.getElementById('password-setup-confirm').value='';if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();` + showToastJS("Password vault ready", "", 1500)
	})

	app.Action("password.unlock", func(ctx *r.Context) string {
		data := ctx.WsData()
		master, _ := data["master"].(string)
		if err := unlockPasswordVault(master); err != nil {
			return r.Notify("error", err.Error())
		}
		return passwordVaultStatusJS() + passwordEntriesJS() + `document.getElementById('password-unlock-master').value='';if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();` + showToastJS("Password vault unlocked", "", 1200)
	})

	app.Action("password.save", func(ctx *r.Context) string {
		data := ctx.WsData()
		name, _ := data["name"].(string)
		urlStr, _ := data["url"].(string)
		username, _ := data["username"].(string)
		password, _ := data["password"].(string)
		note, _ := data["note"].(string)
		id, _ := numericID(data["id"])
		name = strings.TrimSpace(name)
		urlStr = strings.TrimSpace(urlStr)
		if name == "" && urlStr == "" {
			return r.Notify("error", "Name or URL is required")
		}
		if password == "" {
			return r.Notify("error", "Password is required")
		}
		entry := PasswordEntry{ID: id, Name: name, URL: urlStr, Username: username, Password: password, Note: note}
		if id > 0 {
			if err := DBUpdatePasswordEntry(entry); err != nil {
				return r.Notify("error", err.Error())
			}
		} else if err := DBAddPasswordEntry(entry); err != nil {
			return r.Notify("error", err.Error())
		}
		return passwordEntriesJS() + `
document.getElementById('password-entry-name').value='';
document.getElementById('password-entry-url').value='';
document.getElementById('password-entry-username').value='';
document.getElementById('password-entry-password').value='';
document.getElementById('password-entry-note').value='';
if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();
` + showToastJS("Password saved", "", 1300)
	})

	app.Action("password.entry", func(ctx *r.Context) string {
		data := ctx.WsData()
		id, _ := numericID(data["id"])
		mode, _ := data["mode"].(string)
		entry, err := DBLoadPasswordEntry(id)
		if err != nil {
			return r.Notify("error", err.Error())
		}
		payload, _ := jsonMarshal(map[string]any{
			"id":       entry.ID,
			"name":     entry.Name,
			"url":      entry.URL,
			"username": entry.Username,
			"password": entry.Password,
			"note":     entry.Note,
		})
		return fmt.Sprintf("if(window.__libroPasswordShowEntry)window.__libroPasswordShowEntry(%s,%s);", string(payload), components.JSString(mode))
	})

	app.Action("password.copy", func(ctx *r.Context) string {
		data := ctx.WsData()
		id, _ := numericID(data["id"])
		field, _ := data["field"].(string)
		entry, err := DBLoadPasswordEntry(id)
		if err != nil {
			return r.Notify("error", err.Error())
		}
		DBTouchPasswordEntry(id)
		switch field {
		case "username":
			return passwordEntriesJS() + copyPasswordFieldJS(entry.Username, "Username copied")
		default:
			return passwordEntriesJS() + copyPasswordFieldJS(entry.Password, "Password copied")
		}
	})

	app.Action("password.delete", func(ctx *r.Context) string {
		data := ctx.WsData()
		id, _ := numericID(data["id"])
		if id <= 0 {
			return r.Notify("error", "Invalid password entry")
		}
		if err := DBDeletePasswordEntry(id); err != nil {
			return r.Notify("error", err.Error())
		}
		return passwordEntriesJS() + `if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();` + showToastJS("Password deleted", "", 1200)
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
		return fmt.Sprintf(`if(window.__libroCloseAllPopups)window.__libroCloseAllPopups('%s');`, DialogID) +
			r.Show(DialogID) +
			fmt.Sprintf(`if(window.__libroRefreshWidthAvailability)window.__libroRefreshWidthAvailability(document.getElementById('%s'));`, DialogID)
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
		editDBID := state.EditDBID

		projectSpecific := false
		if val, ok := data["app-project-specific"].(bool); ok {
			projectSpecific = val
		}
		if projectSpecific {
			for _, p := range state.Projects {
				if p.Name == state.ActiveProject && p.Transient {
					return r.Notify("error", "Temporary folders cannot save project-specific apps")
				}
			}
		}

		if appType == "terminal" {
			command, _ := data["app-command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				command = components.UserShellBase()
			}

			writable := true
			if val, ok := data["app-writable"].(bool); ok {
				writable = val
			}

			dbSaveApp(savedAppsProjectName(state, state.ActiveProject), editDBID, "terminal", command, string(width), name, writable, projectSpecific)
		} else {
			url, _ := data["app-url"].(string)
			url = strings.TrimSpace(url)
			if url == "" {
				return r.Notify("error", "URL is required")
			}
			url = ensureScheme(url)

			dbSaveApp(savedAppsProjectName(state, state.ActiveProject), editDBID, "url", url, string(width), name, false, projectSpecific)
		}

		sm.CloseDialog(sid)
		sm.Get(sid).EditDBID = -1

		// Re-fetch state to ensure we have latest state for rendering
		state = sm.Get(sid)

		resp := r.NewResponse().
			Replace(DialogID, renderAddDialog(false, sid)).
			Replace(ManageDialogID, renderManageAppsPage(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(savedAppsJS(state))

		// Refresh rendered empty-state project panes without touching live running app panes.
		for projectName := range state.renderedProjects {
			if projectHasRunningApps(state, projectName) {
				continue
			}
			resp.Replace(projectMainID(projectName), renderMainAreaForProject(state, sid, projectName))
			if projectName == state.ActiveProject {
				resp.Add(fmt.Sprintf(`(function(){var el=document.getElementById('%s');if(el)el.style.display='flex';})();`, projectMainID(projectName)))
			} else {
				resp.Add(fmt.Sprintf(`(function(){var el=document.getElementById('%s');if(el)el.style.display='none';})();`, projectMainID(projectName)))
			}
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

		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(state.Apps[state.SelectedIndex].ID, sidData(sid, "id", state.Apps[state.SelectedIndex].ID))
		if hadApps > 0 {
			newIndex := state.SelectedIndex
			newApp := state.Apps[newIndex]
			frame := renderAppFramePlaceholder(newApp, newIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + projJS + hydrateJS
		}

		return renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID).ToJSReplace(projectMainID(state.ActiveProject)) + projJS + navigateJS(state, sid) + hydrateJS
	})

	// Open Neovim if available, otherwise fall back to Vim; notify if neither exists.
	app.Action("app.nvim.open", func(ctx *r.Context) string {
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
		return fmt.Sprintf(`__ws.call('app.start',{sid:%s,type:'terminal',url:'',command:%s,width:'lg',writable:true,name:%s,iconUrl:'',side:'right'});`, components.JSString(sid), components.JSString(cmd), components.JSString(name))
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

			state := sm.Get(sid)
			newApp := &state.Apps[state.SelectedIndex]

			topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
			projJS := projectsJS(state)
			hydrateJS := hydrateAppAfterScrollJS(newApp.ID, sidData(sid, "id", newApp.ID))
			if hadApps > 0 {
				frame := renderAppFramePlaceholder(*newApp, state.SelectedIndex, true, sid, state.ZenMode)
				return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS
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

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(state.Apps[state.SelectedIndex].ID, sidData(sid, "id", state.Apps[state.SelectedIndex].ID))
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS
		}

		return renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + projJS + navigateJS(state, sid) + hydrateJS
	})

	app.Action("app.hydrate", func(ctx *r.Context) string {
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
		if idx < 0 {
			return "/* noop */"
		}

		if state.Apps[idx].Type == AppTypeTerminal && !state.Apps[idx].TerminalReady {
			term := state.Apps[idx]
			pwd := sm.GetActiveProjectPath(sid)
			session, err := tm.Start(term.ID, term.Command, pwd, term.Writable)
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

		contentJS := renderAppContent(state.Apps[idx], sid, false, nil).ToJSReplace(appContentID(appID))
		return fmt.Sprintf(`
(function(){
	%s
})();
`, contentJS) + settleHydratedAppContentJS(appID)
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
		projJS := projectsJS(state)

		// If no apps left, full replace to show empty state
		// Pool the webview first so it survives the DOM replace
		if len(state.Apps) == 0 || hadApps <= 1 {
			return poolWebviewJS(appID) + renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + projJS
		}

		// Otherwise, remove just the app frame and update navigation
		return removeAppJS(appID) + navigateJS(state, sid) + topBarJS + projJS
	})

	// Close current (selected) app — no app ID needed from client
	app.Action("app.close.current", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		if len(state.Apps) == 0 {
			return "/* noop */"
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
		projJS := projectsJS(state)
		if len(state.Apps) == 0 || hadApps <= 1 {
			return poolWebviewJS(appID) + renderMainArea(state, sid).ToJSReplace(projectMainID(state.ActiveProject)) + topBarJS + projJS
		}
		return removeAppJS(appID) + navigateJS(state, sid) + topBarJS + projJS
	})

	// Emergency restart for a terminal app's native PTY session.
	app.Action("app.terminal.restart", func(ctx *r.Context) string {
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
		if err := tm.Restart(term.ID, term.Command, term.Writable, pwd); err != nil {
			return r.Notify("error", "Failed to restart terminal: "+err.Error())
		}

		return fmt.Sprintf(`(function(){if(window.__libroRestartTerminal)window.__libroRestartTerminal(%s);})();`, components.JSString(term.ID)) + settleAppFrameJS(term.ID) + r.Notify("success", "Terminal restarted")
	})

	// Close all running apps in the active project.
	app.Action("project.apps.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectName, apps := sm.CloseActiveProjectApps(sid)
		targetProject := sm.ProjectToShowAfterClosingActive(sid, projectName)
		if projectName != "" && projectName != "home" {
			sm.RemoveNavSlot(sid, projectName)
			DBSetProjectNavSlot(projectName, 0)
		}
		for _, a := range apps {
			if a.Type == AppTypeTerminal {
				tm.Stop(a.ID)
			}
		}

		if targetProject != "" && targetProject != projectName {
			sm.SwitchProject(sid, targetProject)
		}

		state := sm.Get(sid)
		resp := r.NewResponse().
			Add(closeDevtoolsForAppsJS(apps)).
			Replace(projectMainID(projectName), renderMainAreaForProject(state, sid, projectName)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state))

		if state.ActiveProject != projectName {
			if sm.IsProjectRendered(sid, state.ActiveProject) {
				resp.Add(switchProjectJS(state.ActiveProject, nil))
			} else {
				resp.Add(switchProjectJS(state.ActiveProject, renderMainArea(state, sid)))
			}
			resp.Add(updateHashJS(state.ActiveProject)).
				Add(projectToastJS(state.ActiveProject)).
				Add(focusSelectedAppJS(state)).
				Add(savedAppsJS(state))
		} else {
			resp.Add(focusSelectedAppJS(state))
		}

		resp.Add(showToastJS("Closed project", projectName, 1300))
		return resp.Build()
	})

	// Save all running apps in the active project for reopen without closing them.
	app.Action("project.apps.save", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectName, count := sm.SaveActiveProjectApps(sid)
		if count == 0 {
			return r.Notify("error", "No open apps in "+projectName)
		}
		state := sm.Get(sid)
		return r.NewResponse().
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(showToastJS("Saved apps", projectName, 1300)).
			Build()
	})

	// Clear the saved reopen snapshot for the active project.
	app.Action("project.apps.clean", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectName, hadSaved := sm.ClearClosedProjectApps(sid)
		if !hadSaved {
			return r.Notify("error", "Nothing saved for "+projectName)
		}
		state := sm.Get(sid)
		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(showToastJS("Cleared saved apps", projectName, 1300)).
			Build()
	})

	// Reopen the saved apps for the active project.
	app.Action("project.apps.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		projectName, snap := sm.TakeClosedProjectApps(sid)
		if snap == nil || len(snap.Apps) == 0 {
			return r.Notify("error", "Nothing to open for "+projectName)
		}
		restoredCount, skipped := restoreClosedApps(sid, snap)
		if restoredCount == 0 {
			msg := "Failed to reopen apps"
			if len(skipped) > 0 {
				msg = "Failed to reopen: " + strings.Join(skipped, ", ")
			}
			return r.Notify("error", msg)
		}
		state := sm.Get(sid)
		time.Sleep(500 * time.Millisecond)
		toastTitle := "Reopened apps"
		toastBody := projectName
		if len(skipped) > 0 {
			toastTitle = "Reopened with skips"
			toastBody = strings.Join(skipped, ", ")
		}
		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainArea(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(focusSelectedAppJS(state)).
			Add(showToastJS(toastTitle, toastBody, 1800)).
			Build()
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

	// Move selected app to another project, then activate that project.
	app.Action("app.move.to.project", func(ctx *r.Context) string {
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
			js.WriteString(navigateProjectJS(sourceProject, sourceSnap.Apps, sourceSnap.SelectedIndex, state.ZenMode, sid))
		} else {
			js.WriteString(poolWebviewJS(appID))
			js.WriteString(renderMainAreaForProject(state, sid, sourceProject).ToJSReplace(projectMainID(sourceProject)))
		}

		if targetRenderedBefore {
			if targetHadApps {
				frame := renderAppFrame(*moved, state.SelectedIndex, true, sid, state.ZenMode)
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

		return r.NewResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(js.String()).
			Add(updateHashJS(target)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state)).
			Add(savedAppsJS(state)).
			Build()
	})

	// Resize app to specific width — JS-only update to preserve iframes
	app.Action("app.resize", func(ctx *r.Context) string {
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

	// Toggle maximize — switch selected app between full width and previous width
	app.Action("app.maximize.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		maxPixels := 0
		if v, ok := data["maxPixel"].(float64); ok {
			maxPixels = int(v)
		}
		newWidth, appID := sm.ToggleMaxWidth(sid, maxPixels)
		if appID == "" {
			return ""
		}
		state := sm.Get(sid)
		return resizeJS(state, newWidth, appID)
	})

	// Step selected app width by one tier up/down.
	app.Action("app.resize.step", func(ctx *r.Context) string {
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

	// Open an empty browser panel
	app.Action("app.browse.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		side, _ := data["side"].(string)
		popup, _ := data["popup"].(bool)
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

		sm.InsertApp(sid, "", WidthLG, "New Tab", insertIdx)
		state := sm.Get(sid)

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(state.Apps[state.SelectedIndex].ID, sidData(sid, "id", state.Apps[state.SelectedIndex].ID))
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			focusJS := fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, newApp.ID)
			if popup {
				focusJS = fmt.Sprintf(`setTimeout(function(){if(window.__libroOpenURLPopupFor)window.__libroOpenURLPopupFor(%s,'');},220);`, components.JSString(newApp.ID))
			}
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + hydrateJS + focusJS
		}

		focusJS := fmt.Sprintf(`setTimeout(function(){var inp=document.getElementById('urlinput-%s');if(inp){inp.value='';inp.focus();inp.select();}},200);`, state.Apps[state.SelectedIndex].ID)
		if popup {
			focusJS = fmt.Sprintf(`setTimeout(function(){if(window.__libroOpenURLPopupFor)window.__libroOpenURLPopupFor(%s,'');},220);`, components.JSString(state.Apps[state.SelectedIndex].ID))
		}

		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, state.Apps[state.SelectedIndex].ID)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(navigateJS(state, sid)).
			Add(hydrateJS).
			Add(focusJS).
			Build()
	})

	// Quick open - show the app search dialog
	app.Action("app.run.open", func(ctx *r.Context) string {
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
		switch side {
		case "left":
			insertIdx = sm.SelectedIndex(sid)
		case "right":
			insertIdx = sm.SelectedIndex(sid) + 1
		}

		stateBefore := sm.Get(sid)
		hadApps := len(stateBefore.Apps)

		appID := sm.NextAppID()
		sm.InsertTerminalPlaceholder(sid, appID, WidthLG, command, true, "", "", insertIdx)

		// Save to run history
		go DBSaveRunCommand(command)
		state := sm.Get(sid)

		// Update run commands JS
		runCmdsJS := runCommandsJS()

		topBarJS := renderTopBar(state, sid).ToJSReplace(TopBarID)
		projJS := projectsJS(state)
		hydrateJS := hydrateAppAfterScrollJS(appID, sidData(sid, "id", appID))
		if hadApps > 0 {
			newApp := state.Apps[state.SelectedIndex]
			frame := renderAppFramePlaceholder(newApp, state.SelectedIndex, true, sid, state.ZenMode)
			return insertAppJS(frame, false, state.ActiveProject) + navigateJS(state, sid) + topBarJS + projJS + runCmdsJS + hydrateJS
		}

		newApp := state.Apps[state.SelectedIndex]
		return r.NewResponse().
			Replace(projectMainID(state.ActiveProject), renderMainAreaWithPlaceholder(state, sid, newApp.ID)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(runCmdsJS).
			Add(navigateJS(state, sid)).
			Add(hydrateJS).
			Build()
	})

	// Set URL for a running app — navigates the iframe and updates session state only.
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
		// Navigate webview. Do not update saved_apps here: a running app's current
		// URL may diverge from the saved launcher URL (e.g. trello.com -> a board).
		return fmt.Sprintf(`(function(){window.__libroWvNavigate(%s,%s);var inp=document.getElementById('urlinput-'+%s);if(inp)inp.value=%s;})();`, components.JSString(appID), components.JSString(newURL), components.JSString(appID), components.JSString(newURL))
	})

	// Delete single history item
	app.Action("history.delete", func(ctx *r.Context) string {
		data := ctx.WsData()
		urlStr, _ := data["url"].(string)
		if urlStr == "" {
			return ""
		}
		DBDeleteBrowsedURL(urlStr)
		return fmt.Sprintf(`(function(){var u=%s;window.__libroBrowsedURLs=window.__libroBrowsedURLs.filter(function(x){return x!==u;});if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}})();`, components.JSString(urlStr))
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
		return fmt.Sprintf(`(function(){var c=%s;window.__libroRunCommands=(window.__libroRunCommands||[]).filter(function(x){return x!==c;});if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}})();`, components.JSString(command))
	})

	// Clear run history
	app.Action("run.history.clear", func(ctx *r.Context) string {
		DBClearRunCommands()
		return `window.__libroRunCommands=[];if(window.__libroSearchRegistered){var inp=document.getElementById('search-input');if(inp){var ev=new Event('input');inp.dispatchEvent(ev);}}`
	})

	// Lookup directories for the unified project dialog. Bare terms search common
	// code roots recursively; absolute paths list matching child directories.
	app.Action("project.lookup", func(ctx *r.Context) string {
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
	app.Action("project.dialog.open", func(ctx *r.Context) string {
		return `if(window.__libroOpenProjectDialog)window.__libroOpenProjectDialog();`
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
	app.Action("project.open.folder", func(ctx *r.Context) string {
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
	app.Action("project.create.confirm", func(ctx *r.Context) string {
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

	// Toggle zen mode — hides top bar, sidebar, and app toolbars
	app.Action("zen.toggle", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		source, _ := ctx.WsData()["source"].(string)
		sm.ToggleZenMode(sid)
		state := sm.Get(sid)
		resp := r.NewResponse().
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state))
		// Toggle toolbar visibility on all app frames and update zen borders
		if state.ZenMode {
			resp.Add(`document.querySelectorAll('[data-app-toolbar]').forEach(function(t){t.style.display='none';});`)
		} else {
			resp.Add(`document.querySelectorAll('[data-app-toolbar]').forEach(function(t){t.style.display='';});`)
		}
		// navigateJS handles zen border classes on selected/unselected apps
		resp.Add(navigateJS(state, sid))
		if source == "click" {
			resp.Add(showToastJS("Zen mode", "Toggle with ⌘ + Z", 1800))
		}
		return resp.Build()
	})

	switchToProjectName := func(sid, name string, assignShortcut bool) string {
		if name == "" {
			return "/* noop */"
		}

		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)

		// Branch shortcuts can exist before their virtual project has been created
		// in the current session. Resolve the matching worktree lazily.
		if strings.Contains(name, "/") {
			parts := strings.SplitN(name, "/", 2)
			parentProject, branch := parts[0], parts[1]
			projectPath := sm.GetProjectPath(sid, parentProject)
			if projectPath != "" && branch != "" {
				if worktrees, err := GitListWorktrees(projectPath); err == nil {
					for _, wt := range worktrees {
						if wt.IsBare || wt.Branch != branch {
							continue
						}
						sm.AddVirtualProject(sid, name, wt.Path, parentProject)
						break
					}
				}
			}
		}

		if !sm.SwitchProject(sid, name) {
			return "/* noop */"
		}

		assignedSlot := 0
		if assignShortcut && sm.GetNavSlotForProject(sid, name) == 0 {
			assignedSlot = ensureProjectNavSlot(sid, name, true)
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

		resp := r.NewResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(name)).
			Add(projectToastJS(state.ActiveProject)).
			Add(focusSelectedAppJS(state)).
			Add(savedAppsJS(state))
		if assignedSlot >= 2 && assignedSlot <= 9 {
			resp.Add(showToastJS(fmt.Sprintf("Ctrl+%d assigned", assignedSlot), name, 1500))
		}
		return resp.Build()
	}

	// Switch active project
	app.Action("project.switch", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		assignShortcut, _ := data["assignShortcut"].(bool)
		resp := switchToProjectName(sid, name, assignShortcut)
		if resp == "/* noop */" {
			return r.Notify("error", "Project not found")
		}
		return resp
	})

	// Navigate to next project
	app.Action("project.navigate.next", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.NextProject(sid)
		if name == "" {
			return "/* noop */"
		}
		return switchToProjectName(sid, name, false)
	})

	// Navigate to previous project
	app.Action("project.navigate.prev", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.PrevProject(sid)
		if name == "" {
			return "/* noop */"
		}
		return switchToProjectName(sid, name, false)
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
		return switchToProjectName(sid, name, false)
	})

	// Select previous project (Ctrl+0)
	app.Action("project.select.last", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name := sm.PreviousProject(sid)
		if name == "" {
			return "/* noop */"
		}
		return switchToProjectName(sid, name, false)
	})

	// Add a nav slot shortcut to a project/branch
	app.Action("nav.slot.add", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		if name == "" || name == "home" {
			return ""
		}
		if slot := sm.AssignNavSlot(sid, name); slot >= 2 && slot <= 9 {
			DBSetProjectNavSlot(name, slot)
		}
		state := sm.Get(sid)
		return r.NewResponse().
			Add(projectsJS(state)).
			Build()
	})

	// Remove a nav slot shortcut from a project/branch
	app.Action("nav.slot.remove", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		if name == "" || name == "home" {
			return ""
		}
		sm.RemoveNavSlot(sid, name)
		DBSetProjectNavSlot(name, 0)
		state := sm.Get(sid)
		return r.NewResponse().
			Add(projectsJS(state)).
			Build()
	})

	// Toggle a nav slot for the currently active project/worktree (Win+X).
	app.Action("nav.slot.toggle.active", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		stateBefore := sm.Get(sid)
		name := stateBefore.ActiveProject
		if name == "" {
			return "/* noop */"
		}
		if name == "home" {
			return showToastJS("Ctrl+1 is fixed", "Home is always assigned to Ctrl+1", 1400)
		}

		title := ""
		subtitle := ""
		if slot := sm.GetNavSlotForProject(sid, name); slot >= 2 && slot <= 9 {
			sm.RemoveNavSlot(sid, name)
			DBSetProjectNavSlot(name, 0)
			title = fmt.Sprintf("Ctrl+%d removed", slot)
			subtitle = name
		} else {
			slot := sm.AssignNavSlot(sid, name)
			if slot < 2 || slot > 9 {
				return showToastJS("No free shortcut", "Ctrl+2 through Ctrl+9 are already assigned", 1500)
			}
			DBSetProjectNavSlot(name, slot)
			title = fmt.Sprintf("Ctrl+%d assigned", slot)
			subtitle = name
		}

		state := sm.Get(sid)
		return r.NewResponse().
			Add(projectsJS(state)).
			Add(showToastJS(title, subtitle, 1300)).
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
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(fmt.Sprintf(`(function(){var el=document.getElementById('project-main-%s');if(el)el.remove();})();`, name))

		if wasActive {
			resp.Replace(projectMainID("home"), renderMainArea(state, sid)).
				Add(updateHashJS("home"))
		}

		return resp.Build()
	})

	// Open manage apps overlay
	app.Action("app.manage.open", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		state.ManageOpen = true
		return r.NewResponse().
			Replace(ManageDialogID, renderManageAppsPage(state, sid)).
			Add(r.Show(ManageDialogID)).
			Build()
	})

	// Close manage apps popup
	app.Action("app.manage.close", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		state := sm.Get(sid)
		state.ManageOpen = false
		return r.Hide(ManageDialogID)
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
		// Re-render the visible surfaces that consume saved app data.
		resp := r.NewResponse().
			Replace(ManageDialogID, renderManageAppsPage(state, sid)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(projectsJS(state)).
			Add(savedAppsJS(state))
		for projectName := range state.renderedProjects {
			if projectHasRunningApps(state, projectName) {
				continue
			}
			resp.Replace(projectMainID(projectName), renderMainAreaForProject(state, sid, projectName))
			if projectName == state.ActiveProject {
				resp.Add(fmt.Sprintf(`(function(){var el=document.getElementById('%s');if(el)el.style.display='flex';})();`, projectMainID(projectName)))
			} else {
				resp.Add(fmt.Sprintf(`(function(){var el=document.getElementById('%s');if(el)el.style.display='none';})();`, projectMainID(projectName)))
			}
		}
		return resp.Build()
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
			fmt.Fprintf(&html, `<div class="mb-2"><div class="flex items-center gap-1.5 text-xs font-medium text-gray-700 dark:text-zinc-300 mb-1"><span class="material-icons-round text-sm">folder</span>%s</div>`, pa.Name)
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
				fmt.Fprintf(&html, `<div class="flex items-center gap-1.5 ml-5 py-0.5 text-xs text-gray-500 dark:text-zinc-500"><span class="material-icons-round text-xs">%s</span><span class="truncate">%s</span></div>`, icon, label)
			}
			html.WriteString(`</div>`)
		}

		return fmt.Sprintf(`(function(){var el=document.getElementById('close-dialog-apps');if(el)el.innerHTML=%s;var dlg=document.getElementById('%s');if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);if(dlg)dlg.classList.remove('hidden');var cb=document.getElementById('close-dialog-confirm');if(cb)cb.focus();})();`,
			components.JSString(html.String()), CloseDialogID)
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
		prevState := sm.Get(sid)
		closeDevtoolsJS := closeDevtoolsForAppsJS(prevState.Apps)

		vtName := parentProject + "/" + branch

		// Add virtual project if it doesn't exist
		sm.AddVirtualProject(sid, vtName, wtPath, parentProject)

		if !sm.SwitchProject(sid, vtName) {
			return r.Notify("error", "Failed to switch to worktree")
		}

		assignedSlot := 0
		if sm.GetNavSlotForProject(sid, vtName) == 0 {
			assignedSlot = ensureProjectNavSlot(sid, vtName, false)
		}

		state := sm.Get(sid)

		var jsSwitch string
		if sm.IsProjectRendered(sid, vtName) {
			jsSwitch = switchProjectJS(vtName, nil)
		} else {
			jsSwitch = switchProjectJS(vtName, renderMainArea(state, sid))
		}

		resp := r.NewResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
			Add(focusSelectedAppJS(state))
		if assignedSlot >= 2 && assignedSlot <= 9 {
			resp.Add(showToastJS(fmt.Sprintf("Ctrl+%d assigned", assignedSlot), vtName, 1500))
		}
		return resp.Build()
	})

	// Create a new worktree from the active project's current branch and switch to it.
	app.Action("worktree.create", func(ctx *r.Context) string {
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

		return r.NewResponse().
			Add(projectsJS(state)).
			Replace(TopBarID, renderTopBar(state, sid)).
			Add(closeDevtoolsJS).
			Add(jsSwitch).
			Add(updateHashJS(vtName)).
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
		app.Broadcast(`(function(){if(window.__libroRefreshTerminalThemes)window.__libroRefreshTerminalThemes();})();`)
	})

	if err := app.Listen(":" + Port()); err != nil {
		log.Printf("libro: app.Listen on :%s failed: %v", Port(), err)
	}
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

func numericID(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}
