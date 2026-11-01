package libro

import (
	"path/filepath"

	"libro/internal/components"

	r "github.com/michalCapo/g-sui/ui"
)

// resumableSession is an earlier session with the enabled agent that resumes it.
type resumableSession struct {
	components.AgentSession
	Plugin string `json:"plugin"`
	Agent  string `json:"agent"`
}

func registerSessionActions(app *r.App) {
	registerWorkspaceAction(app, "session.list", func(_ *r.Context, in sessionInput) (r.Result, error) {
		return clientScript("if(window.libroWorkspace)libroWorkspace.sessionPicker(props[0]);", resumableSessions(inputSID(in.SID))), nil
	})
}

// resumableSessions lists the active folder's earlier agent sessions that no
// panel or open thread uses, in any window, so two agents never share one.
func resumableSessions(sid string) []resumableSession {
	sm.mu.RLock()
	open := map[string]bool{}
	for _, s := range sm.states {
		for _, app := range s.Apps {
			open[app.SessionID] = true
		}
		for _, snapshot := range s.snapshots {
			if snapshot != nil {
				for _, app := range snapshot.Apps {
					open[app.SessionID] = true
				}
			}
		}
		for _, thread := range s.Threads {
			if !thread.Archived {
				open[thread.SessionID] = true
			}
		}
	}
	sm.mu.RUnlock()
	agents := map[string]*Plugin{}
	sessions := []resumableSession{}
	for _, session := range components.AgentSessions(sm.GetActiveProjectPath(sid), agentEnvironment()) {
		agent, ok := agents[session.Kind]
		if !ok {
			agent = sessionAgent(session.Kind)
			agents[session.Kind] = agent
		}
		if agent != nil && !open[session.ID] {
			sessions = append(sessions, resumableSession{session, agent.ID, agent.Name})
		}
	}
	return sessions
}

// sessionAgent is the enabled agent that runs the kind's program, the
// built-in one first.
func sessionAgent(kind string) *Plugin {
	var match *Plugin
	for _, plugin := range plugins() {
		if plugin.Disabled || plugin.Removed || plugin.Dock != "center" || plugin.Type != AppTypeTerminal || filepath.Base(extractBaseCmd(agentCommand(plugin))) != kind {
			continue
		}
		if plugin.ID == kind {
			return &plugin
		}
		if match == nil {
			match = &plugin
		}
	}
	return match
}
