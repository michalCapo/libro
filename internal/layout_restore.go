package libro

import (
	"encoding/json"
	r "github.com/michalCapo/g-sui/ui"
	"log"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Layout versions only gain fields. Runtime PTY and application port state is
// deliberately excluded so reopening a page cannot revive project commands.
type savedLayout struct {
	V             int
	ActiveProject string
	Open          []savedWorkspace
	Transient     []Project
	WorktreeOrder map[string]int
}

type savedWorkspace struct {
	Name          string
	Path          string
	SelectedIndex int
	Apps          []savedApp
}

type savedApp struct {
	ID, PluginID, Dock, Name, Command, URL, IconURL string
	Type                                            AppType
	Width, PreviousWidth                            Width
	Writable                                        bool
	ApplicationPath                                 string
	ApplicationPerThread                            bool
	SessionID, AgentModel, AgentEffort              string
}

func saveApp(app Application) savedApp {
	return savedApp{
		ID: app.ID, PluginID: app.PluginID, Dock: app.Dock, Name: app.Name,
		Command: app.Command, URL: app.URL, IconURL: app.IconURL, Type: app.Type,
		Width: app.Width, PreviousWidth: app.PreviousWidth, Writable: app.Writable,
		ApplicationPath: app.ApplicationPath, ApplicationPerThread: app.ApplicationPerThread,
		SessionID: app.SessionID, AgentModel: app.AgentModel, AgentEffort: app.AgentEffort,
	}
}

func (app savedApp) application() Application {
	return Application{
		ID: app.ID, PluginID: app.PluginID, Dock: app.Dock, Name: app.Name,
		Command: app.Command, URL: app.URL, IconURL: app.IconURL, Type: app.Type,
		Width: app.Width, PreviousWidth: app.PreviousWidth, Writable: app.Writable,
		ApplicationPath: app.ApplicationPath, ApplicationPerThread: app.ApplicationPerThread,
		SessionID: app.SessionID, AgentModel: app.AgentModel, AgentEffort: app.AgentEffort,
	}
}

func (sm *StateManager) saveLayout(sid string) {
	if db == nil {
		return
	}
	sm.mu.RLock()
	state := sm.states[sid]
	if state == nil {
		sm.mu.RUnlock()
		return
	}
	layout := savedLayout{V: 1, ActiveProject: state.ActiveProject, WorktreeOrder: maps.Clone(state.worktreeOrder)}
	seen := make(map[string]bool)
	add := func(name string) {
		if seen[name] || state.closedWorkspaces[name] {
			return
		}
		seen[name] = true
		apps, selected := state.Apps, state.SelectedIndex
		if name != state.ActiveProject {
			snapshot := state.snapshots[name]
			if snapshot == nil {
				return
			}
			apps, selected = snapshot.Apps, snapshot.SelectedIndex
		}
		_, path := threadProjectContext(state, name)
		workspace := savedWorkspace{Name: name, Path: path, SelectedIndex: selected}
		for _, app := range apps {
			saved := saveApp(app)
			if thread := state.thread(name); thread != nil && thread.AgentID == app.PluginID && isAgentApp(app) {
				saved.SessionID, saved.AgentModel, saved.AgentEffort = thread.SessionID, thread.AgentModel, thread.AgentEffort
			}
			workspace.Apps = append(workspace.Apps, saved)
		}
		layout.Open = append(layout.Open, workspace)
	}
	for _, project := range state.Projects {
		if project.Transient {
			layout.Transient = append(layout.Transient, project)
		}
		add(project.Name)
	}
	for _, thread := range state.Threads {
		add(thread.ID)
	}
	// Include the home workspace and any snapshots outside the sidebar list.
	add(state.ActiveProject)
	names := make([]string, 0, len(state.snapshots))
	for name := range state.snapshots {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add(name)
	}
	sm.mu.RUnlock()

	raw, err := json.Marshal(layout)
	if err == nil {
		_, err = db.Exec(`INSERT INTO settings (key,value) VALUES ('layout',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	}
	if err != nil {
		log.Printf("db: save layout: %v", err)
	}
}

// Call after releasing sm.mu. Only the page that restored the layout saves it,
// so a second window cannot replace it with its own empty workspace.
// Serializing saves prevents an older timer overwriting shutdown.
func (sm *StateManager) scheduleLayoutSave(sid string) {
	sm.layoutMu.Lock()
	defer sm.layoutMu.Unlock()
	if !sm.layoutEnabled || sm.layoutClosed.Load() || sid != sm.layoutSession {
		return
	}
	sm.layoutRevision++
	revision := sm.layoutRevision
	if sm.layoutTimer != nil {
		sm.layoutTimer.Stop()
	}
	sm.layoutTimer = time.AfterFunc(time.Second, func() {
		sm.layoutMu.Lock()
		defer sm.layoutMu.Unlock()
		if !sm.layoutClosed.Load() && sm.layoutRevision == revision {
			sm.saveLayout(sid)
		}
	})
}

func (sm *StateManager) flushLayout() {
	sm.layoutMu.Lock()
	defer sm.layoutMu.Unlock()
	sm.layoutClosed.Store(true)
	if sm.layoutTimer != nil {
		sm.layoutTimer.Stop()
	}
	sm.saveLayout(sm.layoutSession)
}

// reopenLayout lets the next page restore again after a quit that leaves the
// server running (a browser or an attached desktop window).
func (sm *StateManager) reopenLayout() {
	sm.layoutMu.Lock()
	defer sm.layoutMu.Unlock()
	sm.layoutClosed.Store(false)
	sm.layoutRestored, sm.layoutEnabled, sm.layoutSession = false, false, ""
}

func layoutPathExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Only the first page after backend start owns restored panel IDs. Later pages
// get empty workspaces, keeping native terminals isolated between windows.
// restoreMu makes them wait until the restored IDs are reserved.
func (sm *StateManager) restoreLayout(sid string) {
	sm.restoreMu.Lock()
	defer sm.restoreMu.Unlock()
	sm.layoutMu.Lock()
	if sm.layoutRestored {
		sm.layoutMu.Unlock()
		return
	}
	sm.layoutRestored = true
	sm.layoutSession = sid
	sm.layoutMu.Unlock()
	defer func() {
		sm.layoutMu.Lock()
		sm.layoutEnabled = true
		sm.layoutMu.Unlock()
	}()
	if db == nil {
		return
	}
	var raw string
	var layout savedLayout
	if db.QueryRow("SELECT value FROM settings WHERE key = 'layout'").Scan(&raw) != nil || json.Unmarshal([]byte(raw), &layout) != nil || layout.V != 1 {
		return
	}
	for _, project := range layout.Transient {
		if layoutPathExists(project.Path) {
			sm.AddProjectWithOptions(sid, project.Name, project.Path, true)
		}
	}
	for _, workspace := range layout.Open {
		if strings.Contains(workspace.Name, "/") {
			restoreWorktreeProject(sm, sid, workspace.Name)
		}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
		return
	}
	state.worktreeOrder = layout.WorktreeOrder
	valid := func(name string) bool {
		if name == "" {
			return true
		}
		if thread := state.thread(name); thread != nil {
			return !thread.Archived && (thread.Path == "" || layoutPathExists(thread.Path)) &&
				(thread.Project == "" || slices.ContainsFunc(state.Projects, func(p Project) bool { return p.Name == thread.Project && layoutPathExists(p.Path) }))
		}
		return slices.ContainsFunc(state.Projects, func(p Project) bool { return p.Name == name && layoutPathExists(p.Path) })
	}
	for _, workspace := range layout.Open {
		// A recreated worktree may have the same name at another path; its
		// agents belong to the old checkout.
		if _, path := threadProjectContext(state, workspace.Name); !valid(workspace.Name) || workspace.Path != "" && path != workspace.Path {
			continue
		}
		snapshot := &projectSnapshot{}
		for i, saved := range workspace.Apps {
			if n, err := strconv.Atoi(strings.TrimPrefix(saved.ID, "app-")); strings.HasPrefix(saved.ID, "app-") && err == nil && n > sm.nextID {
				sm.nextID = n
			}
			// Managed child agents have no command to start again.
			if saved.PluginID == "project-command" || saved.Type == AppTypeTerminal && saved.Command == "" {
				continue
			}
			if i <= workspace.SelectedIndex {
				snapshot.SelectedIndex = len(snapshot.Apps)
			}
			snapshot.Apps = append(snapshot.Apps, saved.application())
		}
		state.snapshots[workspace.Name] = snapshot
	}
	state.ActiveProject = ""
	if state.snapshots[layout.ActiveProject] != nil {
		state.ActiveProject = layout.ActiveProject
	} else if len(layout.Open) > 0 {
		for _, workspace := range layout.Open {
			if state.snapshots[workspace.Name] != nil {
				state.ActiveProject = workspace.Name
				break
			}
		}
	}
	if snapshot := state.snapshots[state.ActiveProject]; snapshot != nil {
		state.Apps, state.SelectedIndex = snapshot.Apps, snapshot.SelectedIndex
		delete(state.snapshots, state.ActiveProject)
	}
	state.renderedProjects = map[string]bool{state.ActiveProject: true}
}

// Use the existing app.hydrate action when a restored workspace is shown.
func pendingTerminalsNode(state *AppState, sid string) *r.Node {
	ids := []string{}
	for _, app := range state.Apps {
		if app.Type == AppTypeTerminal && !app.TerminalReady && app.PluginID != "project-command" {
			ids = append(ids, app.ID)
		}
	}
	return clientScriptNode(`
(function hydrate(){
	if(typeof __ws==='undefined'||!__ws.connected||!__ws.connected()){setTimeout(hydrate,50);return;}
	props[1].forEach(function(id){__ws.call('app.hydrate',{sid:props[0],id:id});});
})();`, sid, ids)
}

func pendingTerminalsJS(state *AppState, sid string) r.Result {
	return r.Result{}.Append(ActionEffectsID, pendingTerminalsNode(state, sid))
}
