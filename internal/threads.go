package libro

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

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
}

func loadThreads() []Thread {
	rows, err := db.Query("SELECT id, name, archived, project, path, session_id, agent_id, agent_command FROM threads WHERE project = '' ORDER BY rowid")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var threads []Thread
	for rows.Next() {
		var thread Thread
		if rows.Scan(&thread.ID, &thread.Name, &thread.Archived, &thread.Project, &thread.Path, &thread.SessionID, &thread.AgentID, &thread.AgentCommand) == nil {
			threads = append(threads, thread)
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

func registerThreadActions(app *r.App, switchWorkspace func(string, string) r.Result) {
	actionThreadCreate = r.RegisterAction(app, "thread.create", func(_ *r.Context, in actionThreadCreateInput) (r.Result, error) {
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
			response := switchWorkspace(sid, workspace)
			if agentID != "" {
				response = r.Merge(response, r.Result{}.Run(actionAppStart.Call(actionAppStartInput{SID: sid, Plugin: agentID, Type: "terminal", Dock: "center", Writable: new(true)})))
			}
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
		return switchWorkspace(sid, thread.ID), nil
	})
	r.RegisterAction(app, "thread.rename", func(_ *r.Context, in actionThreadRenameInput) (r.Result, error) {
		sid := inputSID(in.SID)
		id := in.ID
		name := in.Name
		name = strings.TrimSpace(name)
		if name == "" {
			return r.Result{}, nil
		}
		if len(name) > 200 {
			name = name[:200]
		}
		state := sm.Get(sid)
		thread := state.thread(id)
		// Agent titles arrive on every status change; skip unchanged names.
		if thread == nil || thread.Name == name {
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
	r.RegisterAction(app, "worktree.title", func(_ *r.Context, in actionWorktreeTitleInput) (r.Result, error) {
		sid := inputSID(in.SID)
		name := strings.TrimSpace(in.Name)
		if len(name) > 200 {
			name = name[:200]
		}
		state := sm.Get(sid)
		for _, p := range state.Projects {
			if p.Name != in.Project || !p.Virtual {
				continue
			}
			// Agent titles arrive on every status change; skip unchanged names.
			if name == "" || worktreeTitle(p.Path) == name || saveWorktreeTitle(p.Path, name) != nil {
				return r.Result{}, nil
			}
			return projectsJS(state), nil
		}
		return r.Result{}, nil
	})
	r.RegisterAction(app, "thread.archive", func(_ *r.Context, in actionThreadArchiveInput) (r.Result, error) {
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

func (s *AppState) canStartThreadApp(app Application) bool {
	if appDock(app) != "center" {
		return true
	}
	if !isAgentApp(app) {
		return false
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
	if _, err := db.Exec("UPDATE threads SET session_id = '', agent_id = ?, agent_command = ?, archived = 0 WHERE id = ?", agentID, command, thread.ID); err != nil {
		return nil, err
	}
	thread.SessionID, thread.AgentID, thread.AgentCommand = "", agentID, command
	thread.Archived = false
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
// A nil result leaves normal panel closing to the caller.
func (sm *StateManager) CloseThreadAgent(sid, appID string) ([]Application, error) {
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
	for _, app := range state.Apps {
		if app.ID != appID || appDock(app) != "center" {
			continue
		}
		if _, err := db.Exec("UPDATE threads SET archived = 1 WHERE id = ?", thread.ID); err != nil {
			return nil, err
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
		return apps, nil
	}
	return nil, nil
}

// Ignore late session reports from a panel that has been replaced.
func (sm *StateManager) saveThreadSession(sid, threadID, appID, agentID, command, sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
		return
	}
	apps := state.Apps
	if state.ActiveProject != threadID {
		snapshot := state.snapshots[threadID]
		if snapshot == nil {
			return
		}
		apps = snapshot.Apps
	}
	if !slices.ContainsFunc(apps, func(app Application) bool { return app.ID == appID }) {
		return
	}
	thread := state.thread(threadID)
	if thread == nil || (thread.SessionID == sessionID && thread.AgentID == agentID && thread.AgentCommand == command) {
		return
	}
	if _, err := db.Exec("UPDATE threads SET session_id = ?, agent_id = ?, agent_command = ? WHERE id = ?", sessionID, agentID, command, threadID); err != nil {
		return
	}
	thread.SessionID, thread.AgentID, thread.AgentCommand = sessionID, agentID, command
}
