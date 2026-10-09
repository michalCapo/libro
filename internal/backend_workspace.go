package libro

import (
	"sync"

	r "github.com/michalCapo/g-sui/ui"
)

// Frontend actions and agent tools mutate the same backend workspace.
var backendActionMu sync.Mutex

var backendCleanupMu sync.Mutex
var backendCleanup func()

func stopBackend() {
	backendCleanupMu.Lock()
	cleanup := backendCleanup
	backendCleanup = nil
	backendCleanupMu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func registerWorkspaceAction[T any](app *r.App, name string, handler func(*r.Context, T) (r.Result, error)) r.ActionRef[T] {
	return r.RegisterAction(app, name, func(ctx *r.Context, in T) (r.Result, error) {
		backendActionMu.Lock()
		defer backendActionMu.Unlock()
		return handler(ctx, in)
	})
}

func backendSessionID() string {
	return sm.backendSessionID()
}

// The backend restores its workspace before a frontend or agent connects.
func (sm *StateManager) backendSessionID() string {
	sm.backendMu.Lock()
	defer sm.backendMu.Unlock()
	if sm.backendSID == "" {
		sm.backendSID = sm.NewSession()
		sm.restoreLayout(sm.backendSID)
	}
	return sm.backendSID
}

func resumeBackendTerminals(sid string) {
	sm.mu.RLock()
	state := sm.states[sid]
	var panels []Application
	if state != nil {
		panels = append(panels, state.Apps...)
		for _, snapshot := range state.snapshots {
			panels = append(panels, snapshot.Apps...)
		}
	}
	sm.mu.RUnlock()
	for _, panel := range panels {
		// Applications still start manually; restored agents and shells resume.
		if panel.Type == AppTypeTerminal && panel.PluginID != "project-command" && !panel.TerminalReady {
			hydrateApp(sid, panel.ID, false)
		}
	}
}
