package libro

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// Thread is a single agent session with its own tool state. Project and Path
// keep project-backed threads in the directory where they were created.
type Thread struct {
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
	rows, err := db.Query("SELECT id, name, archived, project, path, session_id, agent_id, agent_command FROM threads ORDER BY rowid")
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

func threadsJS(state *AppState) string {
	data, _ := json.Marshal(state.Threads)
	return "window.__libroThreads=" + string(data) + ";"
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

func isSharedProjectApp(app Application) bool {
	return app.PluginID == "notes" || app.PluginID == "project-command"
}

func enabledAgentPlugin(id string) bool {
	for _, plugin := range plugins() {
		if plugin.ID == id {
			return !plugin.Disabled && !plugin.Removed && plugin.Dock == "center" && plugin.Type == AppTypeTerminal
		}
	}
	return false
}

func defaultProjectThreadAgent() string {
	configured := defaultThreadAgent()
	fallback := ""
	for _, plugin := range plugins() {
		if plugin.Disabled || plugin.Removed || plugin.Dock != "center" || plugin.Type != AppTypeTerminal {
			continue
		}
		if _, err := exec.LookPath(extractBaseCmd(agentCommand(plugin))); err != nil {
			continue
		}
		if plugin.ID == configured {
			return plugin.ID
		}
		if fallback == "" || plugin.Autolaunch {
			fallback = plugin.ID
		}
	}
	return fallback
}

func registerThreadActions(app *r.App, switchWorkspace func(string, string) string) {
	registerAction(app, "thread.create", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		name = strings.TrimSpace(name)
		if len(name) > 200 {
			return r.Notify("error", "Enter a thread name (up to 200 characters)")
		}
		if name == "" {
			name = "New thread"
		}
		agentID, _ := data["agent"].(string)
		if agentID != "" && !enabledAgentPlugin(agentID) {
			return r.Notify("error", "This agent is not available")
		}
		state := sm.Get(sid)
		projectID, _ := data["project"].(string)
		restoreWorktreeProject(sm, sid, projectID)
		project, path := threadProjectContext(state, projectID)
		if projectID != "" && state.thread(projectID) == nil && project == "" {
			return r.Notify("error", "Project not found")
		}
		if project != "" && agentID == "" {
			agentID = defaultProjectThreadAgent()
			if agentID == "" {
				return r.Notify("error", "Install or enable an agent before creating a project thread")
			}
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return r.Notify("error", "Could not create thread")
		}
		thread := Thread{ID: "thread:" + hex.EncodeToString(random[:]), Name: name, Project: project, Path: path, AgentID: agentID}
		if _, err := db.Exec("INSERT INTO threads (id, name, project, path, agent_id) VALUES (?, ?, ?, ?, ?)", thread.ID, thread.Name, thread.Project, thread.Path, thread.AgentID); err != nil {
			return r.Notify("error", "Could not save thread")
		}
		sm.mu.Lock()
		sm.states[sid].Threads = append(sm.states[sid].Threads, thread)
		sm.mu.Unlock()
		return switchWorkspace(sid, thread.ID)
	})
	registerAction(app, "thread.rename", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		id, _ := data["id"].(string)
		name, _ := data["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		if len(name) > 200 {
			name = name[:200]
		}
		state := sm.Get(sid)
		thread := state.thread(id)
		// Agent titles arrive on every status change; skip unchanged names.
		if thread == nil || thread.Name == name {
			return ""
		}
		if _, err := db.Exec("UPDATE threads SET name = ? WHERE id = ?", name, id); err != nil {
			return ""
		}
		sm.mu.Lock()
		thread.Name = name
		sm.mu.Unlock()
		return projectsJS(state)
	})
	registerAction(app, "thread.archive", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		id, _ := ctx.WsData()["id"].(string)
		archived, _ := ctx.WsData()["archived"].(bool)
		state := sm.Get(sid)
		if state.thread(id) == nil {
			return r.Notify("error", "Thread not found")
		}
		if _, err := db.Exec("UPDATE threads SET archived = ? WHERE id = ?", archived, id); err != nil {
			return r.Notify("error", "Could not save thread")
		}
		sm.mu.Lock()
		state.thread(id).Archived = archived
		sm.mu.Unlock()
		// Archiving keeps the workspace alive and recoverable without interrupting commands.
		return projectsJS(state) + fmt.Sprintf("if(window.libroWorkspace)libroWorkspace.threadArchived(%t);", archived)
	})
}

// Selecting a project returns to an open thread instead of creating another.
func (s *AppState) projectWorkspace(project string) string {
	if s.thread(project) != nil {
		return project
	}
	if thread := s.thread(s.ActiveProject); thread != nil && thread.Project == project && !thread.Archived {
		return thread.ID
	}
	for i := len(s.Threads) - 1; i >= 0; i-- {
		if thread := s.Threads[i]; thread.Project == project && !thread.Archived {
			return thread.ID
		}
	}
	return project
}

// Project agents always belong to a thread. An occupied thread starts a sibling.
func (s *AppState) needsProjectThread(app Application) bool {
	if s.projectScope(s.ActiveProject) == "" || !isAgentApp(app) {
		return false
	}
	if s.thread(s.ActiveProject) == nil {
		return true
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
			if isSharedProjectApp(existing) {
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

// Record the session against the launching thread, even after workspace switches.
func (sm *StateManager) saveThreadSession(sid, threadID, agentID, command, sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
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
