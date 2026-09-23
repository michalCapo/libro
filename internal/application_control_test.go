package libro

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"libro/internal/components"
)

func TestApplicationControlLifecycle(t *testing.T) {
	oldDB, oldSM, oldTM := db, sm, tm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); _ = db.Close(); db, sm, tm = oldDB, oldSM, oldTM })
	createTables()
	path := t.TempDir()
	sm.states["test"] = &AppState{ActiveProject: "project", Projects: []Project{{Name: "project", Path: path}}, Apps: []Application{{ID: "agent", PluginID: "codex"}}}
	if _, _, err = controlApplication("test", path, "start"); err == nil {
		t.Fatal("started without configured command")
	}
	if err = setProjectCommand(path, "echo test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = controlApplication("test", "/other", "restart"); err == nil {
		t.Fatal("accepted another project")
	}
	if _, _, err = controlApplication("test", path, "shell"); err == nil {
		t.Fatal("accepted unknown action")
	}
	js, result, err := controlApplication("test", path, "start")
	if err != nil || js == "" || result["status"] != "starting" {
		t.Fatalf("start: %v %v", result, err)
	}
	first := sm.Get("test").Apps[1].ID
	js, result, err = controlApplication("test", path, "start")
	if err != nil || js != "" || result["status"] != "starting" || len(sm.Get("test").Apps) != 2 {
		t.Fatal("repeated start must preserve pending launch")
	}
	if runtime.GOOS != "windows" {
		session, startErr := tm.Start(first, "", path, true)
		if startErr != nil {
			t.Fatal(startErr)
		}
		sm.HydrateTerminalByID("test", first, session.ID)
		js, result, err = controlApplication("test", path, "start")
		if err != nil || js != "" || result["status"] != "running" {
			t.Fatal("start must preserve a running application")
		}
		tm.Stop(first)
		_, result, err = controlApplication("test", path, "status")
		if err != nil || result["status"] != "stopped" {
			t.Fatal("exited process must report stopped")
		}
	}
	_, result, err = controlApplication("test", path, "restart")
	if err != nil || result["status"] != "starting" || sm.Get("test").Apps[1].ID == first {
		t.Fatal("restart did not replace terminal")
	}
	_, result, err = controlApplication("test", path, "stop")
	if err != nil || result["status"] != "stopped" || len(sm.Get("test").Apps) != 1 || sm.Get("test").Apps[0].ID != "agent" {
		t.Fatal("stop must preserve agent")
	}
	_, result, err = controlApplication("test", path, "status")
	if err != nil || result["status"] != "stopped" || result["configured"] != true {
		t.Fatal("incorrect stopped status")
	}
}

func TestApplicationCLIValidation(t *testing.T) {
	var output bytes.Buffer
	if err := RunApplicationCLI([]string{"--help"}, &output); err != nil || !strings.Contains(output.String(), "project settings") {
		t.Fatal("missing help")
	}
	for _, args := range [][]string{{"shell"}, {"start", "/project", "command"}} {
		if err := RunApplicationCLI(args, &output); err == nil {
			t.Fatal("invalid command accepted")
		}
	}
}

func TestProjectThreadsShareApplicationProcessButNotBrowsers(t *testing.T) {
	oldDB, oldSM, oldTM := db, sm, tm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); _ = db.Close(); db, sm, tm = oldDB, oldSM, oldTM })
	createTables()
	path := t.TempDir()
	if err := setProjectCommand(path, "echo test"); err != nil {
		t.Fatal(err)
	}
	state := &AppState{
		ActiveProject: "thread:one",
		Projects:      []Project{{Name: "project", Path: path}},
		Threads:       []Thread{{ID: "thread:one", Project: "project", Path: path}, {ID: "thread:two", Project: "project", Path: path}},
		Apps:          []Application{{ID: "browser-one", PluginID: "browser", Type: AppTypeURL, Dock: "right", URL: "about:blank"}},
		snapshots:     map[string]*projectSnapshot{"thread:two": {Apps: []Application{{ID: "browser-two", PluginID: "browser", Type: AppTypeURL, Dock: "right", URL: "about:blank"}}}},
	}
	sm.states["test"] = state
	firstScope, secondScope := browserScope("test", "browser-one"), browserScope("test", "browser-two")
	if firstScope == secondScope {
		t.Fatal("threads share browser profiles")
	}
	_, _, err = controlApplication("test", path, "start")
	if err != nil {
		t.Fatal(err)
	}
	first := state.Apps[1].ID
	switchThread := func(thread, browser string) {
		t.Helper()
		switchToProjectName("test", thread)
		if len(state.Apps) != 2 || state.Apps[0].ID != browser || state.Apps[1].PluginID != "project-command" {
			t.Fatalf("thread must contain its own browser and the shared application: %+v", state.Apps)
		}
		count := 0
		for _, workspace := range sm.GetAllRunningApps("test") {
			for _, panel := range workspace.Apps {
				if panel.PluginID == "project-command" {
					count++
				}
			}
		}
		if count != 1 {
			t.Fatalf("got %d application panels, want one", count)
		}
		if browserScope("test", "browser-one") != firstScope || browserScope("test", "browser-two") != secondScope {
			t.Fatal("switching threads changed browser ownership")
		}
	}
	switchThread("thread:two", "browser-two")
	js, result, err := controlApplication("test", path, "start")
	if err != nil || js != "" || result["status"] != "starting" || state.Apps[1].ID != first {
		t.Fatalf("second thread duplicated pending application: %v %v", result, err)
	}
	if runtime.GOOS != "windows" {
		session, err := tm.Start(first, "", path, true)
		if err != nil {
			t.Fatal(err)
		}
		sm.HydrateTerminalByID("test", first, session.ID)
		for _, thread := range []string{"thread:one", "thread:two"} {
			browser := "browser-one"
			if thread == "thread:two" {
				browser = "browser-two"
			}
			switchThread(thread, browser)
			js, result, err = controlApplication("test", path, "start")
			if err != nil || js != "" || result["status"] != "running" || state.Apps[1].ID != first || !tm.IsRunning(first) {
				t.Fatalf("thread failed to reuse live application: %v %v", result, err)
			}
		}
	}
	_, _, err = controlApplication("test", path, "restart")
	if err != nil || state.Apps[1].ID == first || tm.IsRunning(first) {
		t.Fatal("restart failed to replace shared process")
	}
	replacement := state.Apps[1].ID
	switchThread("thread:one", "browser-one")
	if state.Apps[1].ID != replacement {
		t.Fatal("restart was not shared")
	}
	_, result, err = controlApplication("test", path, "stop")
	if err != nil || result["status"] != "stopped" || len(state.Apps) != 1 {
		t.Fatal("stop failed")
	}
	switchToProjectName("test", "thread:two")
	_, result, err = controlApplication("test", path, "status")
	if err != nil || result["status"] != "stopped" || len(state.Apps) != 1 || state.Apps[0].ID != "browser-two" {
		t.Fatal("stop was not shared or removed the thread browser")
	}
}
