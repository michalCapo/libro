package libro

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"libro/internal/components"

	r "github.com/michalCapo/g-sui/ui"
)

// Thread is a single agent session with its own tool state. Project and Path
// keep project-backed threads in the directory where they were created.
type Thread struct {
	Managed      bool   `json:"managed,omitempty"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Archived     bool   `json:"archived"`
	Project      string `json:"project,omitempty"`
	Path         string `json:"-"`
	SessionID    string `json:"-"`
	AgentID      string `json:"-"`
	AgentCommand string `json:"-"`
	AgentModel   string `json:"-"`
	AgentEffort  string `json:"-"`
}

func loadThreads() []Thread {
	rows, err := db.Query("SELECT id, name, archived, project, path, session_id, agent_id, agent_command, agent_model, agent_effort FROM threads WHERE project = '' ORDER BY rowid")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var threads []Thread
	for rows.Next() {
		var thread Thread
		if rows.Scan(&thread.ID, &thread.Name, &thread.Archived, &thread.Project, &thread.Path, &thread.SessionID, &thread.AgentID, &thread.AgentCommand, &thread.AgentModel, &thread.AgentEffort) == nil {
			threads = append(threads, thread)
		}
	}
	_ = rows.Close()
	codexHome := agentEnvironment()["CODEX_HOME"]
	for i := range threads {
		if title := components.RecoverCodexTitle(threads[i].Name, codexHome); title != threads[i].Name {
			threads[i].Name = title
			_, _ = db.Exec("UPDATE threads SET name = ? WHERE id = ?", title, threads[i].ID)
		}
	}
	return threads
}

func (s *AppState) thread(id string) *Thread {
	for i := range s.Threads {
		if s.Threads[i].ID == id {
			return &s.Threads[i]
		}
	}
	return nil
}

func (s *AppState) workspaceApps(workspace string) []Application {
	if workspace == s.ActiveProject {
		return s.Apps
	}
	if snapshot := s.snapshots[workspace]; snapshot != nil {
		return snapshot.Apps
	}
	return nil
}

// primaryAgentID returns the first agent panel. Only it names the workspace
// and owns the thread's saved session; other agent panels keep their own.
func primaryAgentID(apps []Application) string {
	for _, app := range apps {
		if isAgentApp(app) {
			return app.ID
		}
	}
	return ""
}

// workspaceAgent ignores panels removed or replaced before their title arrived.
func (s *AppState) workspaceAgent(workspace, appID string) bool {
	id := primaryAgentID(s.workspaceApps(workspace))
	return id != "" && (appID == "" || appID == id)
}

// Empty checkouts must not display a description left by an earlier agent.
func (s *AppState) workspaceTitle(workspace, path string) string {
	if !s.workspaceAgent(workspace, "") {
		return ""
	}
	return worktreeTitle(path)
}

func threadProjectContext(state *AppState, id string) (string, string) {
	if state == nil || id == "" {
		return "", ""
	}
	if thread := state.thread(id); thread != nil {
		return thread.Project, thread.Path
	}
	for _, project := range state.Projects {
		if project.Name == id {
			return project.Name, project.Path
		}
	}
	return "", ""
}

func (s *AppState) projectScope(id string) string {
	project, _ := threadProjectContext(s, id)
	if project == "" && id != "" && !strings.HasPrefix(id, "thread:") {
		return id
	}
	return project
}

// noteScope keeps notes with the parent project across all its workspaces.
func (s *AppState) noteScope(id string) string {
	name := s.projectScope(id)
	for _, project := range s.Projects {
		if project.Name == name && project.Virtual {
			return project.ParentProject
		}
	}
	return name
}

func isSharedProjectApp(app Application) bool {
	return !app.ApplicationPerThread && (app.PluginID == "notes" || app.PluginID == "project-command" || appDock(app) == "bottom")
}

func enabledAgentPlugin(id string) bool {
	for _, plugin := range plugins() {
		if plugin.ID == id {
			return !plugin.Disabled && !plugin.Removed && plugin.Dock == "center" && plugin.Type == AppTypeTerminal
		}
	}
	return false
}

// startAgentJS opens an agent in a new workspace. The start is skipped if the
// user has switched away or an agent is already open there.
func startAgentJS(sid, workspace, agentID string) r.Result {
	return r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Plugin: agentID, Type: "terminal", Dock: "center", Writable: new(true), AutolaunchProject: new(workspace)}))
}

// createProjectWorktree always branches from the original project, even when
// the request comes from one of its existing worktrees.
func (sm *StateManager) createProjectWorktree(sid, projectID, branch string) (string, error) {
	state := sm.Get(sid)
	project, _ := threadProjectContext(state, projectID)
	for _, p := range state.Projects {
		if p.Name == project && p.Virtual {
			project = p.ParentProject
			break
		}
	}
	for _, p := range state.Projects {
		if p.Name != project {
			continue
		}
		if !GitIsRepo(p.Path) || GitCurrentBranch(p.Path) == "" {
			return "", fmt.Errorf("project threads require a Git repository with a committed branch")
		}
		path := filepath.Join(filepath.Dir(p.Path), filepath.Base(p.Path)+"-"+strings.ReplaceAll(branch, "/", "-"))
		if err := GitCreateWorktree(p.Path, branch, path); err != nil {
			return "", err
		}
		name := p.Name + "/" + branch
		sm.AddVirtualProject(sid, name, path, p.Name)
		return name, nil
	}
	return "", fmt.Errorf("project not found")
}

func registerThreadActions(app *r.App, switchWorkspace func(string, string, string) (r.Result, bool)) {
	actionThreadCreate = registerWorkspaceAction(app, "thread.create", func(_ *r.Context, in actionThreadCreateInput) (r.Result, error) {
		sid := inputSID(in.SID)
		name := in.Name
		name = strings.TrimSpace(name)
		if len(name) > 200 {
			return r.Result{}.Run(r.Notify("error", "Enter a thread name (up to 200 characters)")), nil
		}
		if name == "" {
			name = "New thread"
		}
		agentID := in.Agent
		if agentID != "" && !enabledAgentPlugin(agentID) {
			return r.Result{}.Run(r.Notify("error", "This agent is not available")), nil
		}
		state := sm.Get(sid)
		projectID := in.Project
		restoreWorktreeProject(sm, sid, projectID)
		project, path := threadProjectContext(state, projectID)
		if projectID != "" && state.thread(projectID) == nil && project == "" {
			return r.Result{}.Run(r.Notify("error", "Project not found")), nil
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return r.Result{}.Run(r.Notify("error", "Could not create thread")), nil
		}
		if project != "" {
			workspace, err := sm.createProjectWorktree(sid, projectID, "thread-"+hex.EncodeToString(random[:4]))
			if err != nil {
				return r.Result{}.Run(r.Notify("error", "Could not create project thread: "+err.Error())), nil
			}
			response, _ := switchWorkspace(sid, workspace, agentID)
			return response, nil
		}
		thread := Thread{ID: "thread:" + hex.EncodeToString(random[:]), Name: name, Project: project, Path: path, AgentID: agentID}
		// Project threads live only in the current session.
		if thread.Project == "" {
			if _, err := db.Exec("INSERT INTO threads (id, name, agent_id) VALUES (?, ?, ?)", thread.ID, thread.Name, thread.AgentID); err != nil {
				return r.Result{}.Run(r.Notify("error", "Could not save thread")), nil
			}
		}
		sm.mu.Lock()
		sm.states[sid].Threads = append(sm.states[sid].Threads, thread)
		sm.mu.Unlock()
		response, _ := switchWorkspace(sid, thread.ID, agentID)
		return response, nil
	})
	registerWorkspaceAction(app, "thread.rename", func(_ *r.Context, in actionThreadRenameInput) (r.Result, error) {
		sid := inputSID(in.SID)
		id := in.ID
		name := strings.TrimSpace(in.Name)
		if runes := []rune(name); len(runes) > 200 {
			name = string(runes[:200])
		}
		state := sm.Get(sid)
		thread := state.thread(id)
		// Child threads keep the name their parent chose. A first prompt only
		// names a new thread, so a resumed session keeps its saved description.
		if in.AppID == "" || !state.workspaceAgent(id, in.AppID) || thread == nil || thread.Managed || in.Fallback && thread.Name != "New thread" {
			return r.Result{}, nil
		}
		// A new agent session has no description yet.
		if name == "" {
			name = "New thread"
		}
		// Agent titles arrive on every status change; skip unchanged names.
		if thread.Name == name {
			return r.Result{}, nil
		}
		if _, err := db.Exec("UPDATE threads SET name = ? WHERE id = ?", name, id); err != nil {
			return r.Result{}, nil
		}
		sm.mu.Lock()
		thread.Name = name
		sm.mu.Unlock()
		return projectsJS(state), nil
	})
	registerWorkspaceAction(app, "worktree.title", func(_ *r.Context, in actionWorktreeTitleInput) (r.Result, error) {
		sid := inputSID(in.SID)
		name := strings.TrimSpace(in.Name)
		if runes := []rune(name); len(runes) > 200 {
			name = string(runes[:200])
		}
		state := sm.Get(sid)
		if in.AppID == "" || !state.workspaceAgent(in.Project, in.AppID) {
			return r.Result{}, nil
		}
		// The project's base checkout is saved by path, the same as a worktree.
		for _, p := range state.Projects {
			if p.Name != in.Project {
				continue
			}
			// Agent titles arrive on every status change; skip unchanged names.
			// A first prompt never replaces a saved description, and an empty
			// name clears the description of a previous agent session.
			saved := worktreeTitle(p.Path)
			if saved == name || in.Fallback && saved != "" || saveWorktreeTitle(p.Path, name) != nil {
				return r.Result{}, nil
			}
			return projectsJS(state), nil
		}
		return r.Result{}, nil
	})
	registerWorkspaceAction(app, "thread.archive", func(_ *r.Context, in actionThreadArchiveInput) (r.Result, error) {
		sid := inputSID(in.SID)
		id := in.ID
		archived := in.Archived
		state := sm.Get(sid)
		if state.thread(id) == nil {
			return r.Result{}.Run(r.Notify("error", "Thread not found")), nil
		}
		if _, err := db.Exec("UPDATE threads SET archived = ? WHERE id = ?", archived, id); err != nil {
			return r.Result{}.Run(r.Notify("error", "Could not save thread")), nil
		}
		sm.mu.Lock()
		state.thread(id).Archived = archived
		sm.mu.Unlock()
		sm.scheduleLayoutSave(sid)
		// Archiving keeps the workspace alive and recoverable without interrupting commands.
		return r.Merge(projectsJS(state), clientScript("if(window.libroWorkspace)libroWorkspace.threadArchived(props[0]);", archived)), nil
	})
}

// adjacentProjectThread prefers the previous open thread in the same project.
func (s *AppState) adjacentProjectThread() string {
	active := s.thread(s.ActiveProject)
	if active == nil {
		return ""
	}
	previous := ""
	found := false
	for _, thread := range s.Threads {
		if thread.ID == active.ID {
			if previous != "" {
				return previous
			}
			found = true
			continue
		}
		if thread.Archived || thread.Project != active.Project {
			continue
		}
		if found {
			return thread.ID
		}
		previous = thread.ID
	}
	return ""
}

// An occupied project workspace starts a new worktree for another agent.
func (s *AppState) needsProjectThread(app Application) bool {
	if s.projectScope(s.ActiveProject) == "" || !isAgentApp(app) {
		return false
	}
	return slices.ContainsFunc(s.Apps, isAgentApp)
}

// An added agent panel joins the thread's other agents.
func (s *AppState) canStartThreadApp(app Application, add bool) bool {
	if appDock(app) != "center" {
		return true
	}
	if !isAgentApp(app) {
		return false
	}
	if add {
		thread := s.thread(s.ActiveProject)
		return thread == nil || !thread.Managed
	}
	for _, existing := range s.Apps {
		if appDock(existing) == "center" {
			return false
		}
	}
	return true
}

// ReplaceThreadAgent starts a fresh conversation while keeping the thread's tools.
func (sm *StateManager) ReplaceThreadAgent(sid, agentID, command string) ([]Application, error) {
	defer sm.scheduleLayoutSave(sid)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
		return nil, nil
	}
	thread := state.thread(state.ActiveProject)
	if thread == nil {
		return nil, nil
	}
	// A fresh conversation gets a new description. Child threads keep the name
	// their parent chose.
	name := thread.Name
	if !thread.Managed {
		name = "New thread"
	}
	if _, err := db.Exec("UPDATE threads SET name = ?, session_id = '', agent_id = ?, agent_command = ?, agent_model = '', agent_effort = '', archived = 0 WHERE id = ?", name, agentID, command, thread.ID); err != nil {
		return nil, err
	}
	thread.Name, thread.SessionID, thread.AgentID, thread.AgentCommand = name, "", agentID, command
	thread.Archived = false
	thread.AgentModel, thread.AgentEffort = "", ""
	var removed, kept []Application
	for _, app := range state.Apps {
		if isAgentApp(app) {
			removed = append(removed, app)
		} else {
			kept = append(kept, app)
		}
	}
	state.Apps = kept
	state.SelectedIndex = 0
	return removed, nil
}

// CloseThreadAgent persists the archive before clearing the active thread.
// It returns the closed panels and whether the thread was archived. Nil panels
// leave normal panel closing to the caller. Closing one of several agent panels
// keeps the thread; the next agent takes over its session in the same lock, so
// a late session report from the closed panel cannot win.
func (sm *StateManager) CloseThreadAgent(sid, appID string) ([]Application, bool, error) {
	defer sm.scheduleLayoutSave(sid)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
		return nil, false, nil
	}
	thread := state.thread(state.ActiveProject)
	if thread == nil {
		return nil, false, nil
	}
	for i, app := range state.Apps {
		if app.ID != appID || appDock(app) != "center" {
			continue
		}
		if next := slices.IndexFunc(state.Apps, func(other Application) bool { return other.ID != appID && isAgentApp(other) }); next >= 0 {
			if primaryAgentID(state.Apps) == appID {
				if err := thread.useAgent(state.Apps[next]); err != nil {
					return nil, false, err
				}
			}
			return []Application{*removeApp(state, i)}, false, nil
		}
		// A reordered panel may not have reported its session to the thread yet.
		if app.SessionID != "" && app.SessionID != thread.SessionID {
			if err := thread.useAgent(app); err != nil {
				return nil, false, err
			}
		}
		if _, err := db.Exec("UPDATE threads SET archived = 1 WHERE id = ?", thread.ID); err != nil {
			return nil, false, err
		}
		thread.Archived = true
		apps := make([]Application, 0, len(state.Apps))
		shared := make([]Application, 0, 2)
		for _, existing := range state.Apps {
			if thread.Project != "" && isSharedProjectApp(existing) {
				shared = append(shared, existing)
			} else {
				apps = append(apps, existing)
			}
		}
		state.Apps = shared
		state.SelectedIndex = 0
		return apps, true, nil
	}
	return nil, false, nil
}

// useAgent stores an agent panel's session as the thread's session.
func (thread *Thread) useAgent(agent Application) error {
	if _, err := db.Exec("UPDATE threads SET session_id = ?, agent_id = ?, agent_command = ?, agent_model = ?, agent_effort = ? WHERE id = ?", agent.SessionID, agent.PluginID, agent.Command, agent.AgentModel, agent.AgentEffort, thread.ID); err != nil {
		return err
	}
	thread.SessionID, thread.AgentID, thread.AgentCommand = agent.SessionID, agent.PluginID, agent.Command
	thread.AgentModel, thread.AgentEffort = agent.AgentModel, agent.AgentEffort
	return nil
}

// Ignore late session reports from a panel that has been replaced.
func (sm *StateManager) saveThreadSession(sid, _ string, appID, agentID, command, sessionID string) {
	// Empty Codex titles can be temporary; retain the last confirmed session.
	if sessionID == "" || sm.layoutClosed.Load() {
		return
	}
	if sm.recordAgentSession(sid, appID, agentID, command, sessionID, false) {
		sm.scheduleLayoutSave(sid)
	}
}

func (sm *StateManager) recordAgentSession(sid, appID, agentID, command, sessionID string, shutdown bool) bool {
	model, effort := components.RecoverAgentSettings(command, sessionID, agentEnvironment()["CODEX_HOME"])
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if !shutdown && sm.layoutClosed.Load() {
		return false
	}
	state := sm.states[sid]
	if state == nil {
		return false
	}
	workspace := state.ActiveProject
	apps := state.Apps
	contains := func(apps []Application) bool {
		return slices.ContainsFunc(apps, func(app Application) bool { return app.ID == appID })
	}
	if !contains(apps) {
		apps = nil
		for name, snapshot := range state.snapshots {
			if snapshot != nil && contains(snapshot.Apps) {
				workspace, apps = name, snapshot.Apps
				break
			}
		}
	}
	for i := range apps {
		panel := &apps[i]
		if panel.ID != appID {
			continue
		}
		thread := state.thread(workspace)
		if primaryAgentID(apps) != appID {
			thread = nil
		}
		if panel.SessionID == sessionID {
			if model == "" {
				model = panel.AgentModel
			}
			if effort == "" {
				effort = panel.AgentEffort
			}
		}
		if thread != nil && thread.SessionID == sessionID && thread.AgentID == agentID {
			if model == "" {
				model = thread.AgentModel
			}
			if effort == "" {
				effort = thread.AgentEffort
			}
		}
		changed := panel.SessionID != sessionID || panel.AgentModel != model || panel.AgentEffort != effort
		panel.SessionID, panel.AgentModel, panel.AgentEffort = sessionID, model, effort
		if thread == nil {
			return changed
		}
		if thread.SessionID == sessionID && thread.AgentID == agentID && thread.AgentCommand == command && thread.AgentModel == model && thread.AgentEffort == effort {
			return changed
		}
		if _, err := db.Exec("UPDATE threads SET session_id = ?, agent_id = ?, agent_command = ?, agent_model = ?, agent_effort = ? WHERE id = ?", sessionID, agentID, command, model, effort, thread.ID); err != nil {
			return changed
		}
		thread.SessionID, thread.AgentID, thread.AgentCommand = sessionID, agentID, command
		thread.AgentModel, thread.AgentEffort = model, effort
		return true
	}
	return false
}
