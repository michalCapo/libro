package libro

import (
	"testing"

	"libro/internal/components"
)

func TestBackendRestoresAndResumesBeforeFrontend(t *testing.T) {
	layoutTestDB(t)
	previousSM, previousTM := sm, tm
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); sm.flushLayout(); sm, tm = previousSM, previousTM })
	DBSaveProject("project", t.TempDir())
	DBSaveProject("other", t.TempDir())
	saved := NewStateManager()
	sid := saved.NewSession()
	saved.SwitchProject(sid, "project")
	saved.InsertTerminalPlaceholder(sid, "active-shell", WidthMD, "sleep 30", true, "Shell", "", 0)
	saved.SwitchProject(sid, "other")
	saved.InsertTerminalPlaceholder(sid, "other-shell", WidthMD, "sleep 30", true, "Shell", "", 0)
	saved.saveLayout(sid)
	sid = backendSessionID()
	if sm.Get(sid).ActiveProject != "other" || len(sm.states) != 1 {
		t.Fatal("workspace was not restored before frontend")
	}
	resumeBackendTerminals(sid)
	for _, workspace := range sm.GetAllRunningApps(sid) {
		for _, panel := range workspace.Apps {
			if !panel.TerminalReady || !tm.IsRunning(panel.ID) {
				t.Fatal("restored process required frontend hydration")
			}
		}
	}
	if backendSessionID() != sid || inputSID("foreign-frontend") != sid {
		t.Fatal("frontend replaced backend workspace")
	}
	if !sm.ReopenSession(sid) {
		t.Fatal("frontend could not reattach")
	}
	if len(sm.states) != 1 || sm.Get(sid).ActiveProject != "other" {
		t.Fatal("reattachment duplicated or lost workspace")
	}
}

func TestTerminalLaunchDoesNotNeedFrontend(t *testing.T) {
	sid, _ := childFixture(t)
	id := sm.NextAppID()
	sm.InsertTerminalPlaceholder(sid, id, WidthMD, "sleep 30", true, "Shell", "", 0)
	hydrateAppAfterScrollJS(actionAppHydrateInput{SID: sid, ID: id})
	if !tm.IsRunning(id) {
		t.Fatal("terminal start still needed a frontend callback")
	}
	panel, _, _, found := sm.workspaceApp(sid, id)
	if !found || !panel.TerminalReady {
		t.Fatal("backend did not record the running terminal")
	}
	hydrateAppAfterScrollJS(actionAppHydrateInput{SID: sid, ID: id})
	if !tm.IsRunning(id) {
		t.Fatal("repeat mounting stopped terminal")
	}
}

func TestBackendResumesHomeShellButLeavesApplicationsStopped(t *testing.T) {
	sid, root := childFixture(t)
	sm.Get(sid).ActiveProject = ""
	sm.InsertTerminalPlaceholder(sid, "home-shell", WidthMD, "sleep 30", true, "Shell", "", 0)
	sm.InsertTerminalPlaceholder(sid, "manual-application", WidthMD, "sleep 30", true, "Application", "", 1)
	sm.SetAppPlugin(sid, "manual-application", "project-command", "bottom")
	state := sm.Get(sid)
	state.Apps[1].ApplicationPath = root
	resumeBackendTerminals(sid)
	if !tm.IsRunning("home-shell") {
		t.Fatal("home workspace depended on frontend hydration")
	}
	if tm.IsRunning("manual-application") {
		t.Fatal("restoring a workspace started its application automatically")
	}
}
