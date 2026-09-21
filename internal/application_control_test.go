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
