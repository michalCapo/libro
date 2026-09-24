package libro

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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
	sm.states["test"] = &AppState{ActiveProject: "project", Projects: []Project{{Name: "project", Path: path}, {Name: "other", Path: t.TempDir()}}, Apps: []Application{{ID: "agent", PluginID: "codex"}}}
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
	switchToProjectName("test", "other")
	js, result, err := controlApplication("test", path, "status")
	if err != nil || js != "" || result["status"] != "stopped" || sm.Get("test").ActiveProject != "other" {
		t.Fatalf("background status: %v %v", result, err)
	}
	js, result, err = controlApplication("test", path, "start")
	if err != nil || js == "" || result["status"] != "starting" {
		t.Fatalf("start: %v %v", result, err)
	}
	if sm.Get("test").ActiveProject != "other" || strings.Contains(js, "libroWorkspace.select(") {
		t.Fatal("start changed the visible workspace or selection")
	}
	first := sm.Get("test").snapshots["project"].Apps[1].ID
	switchToProjectName("test", "other")
	js, result, err = controlApplication("test", path, "start")
	if err != nil || js != "" || result["status"] != "starting" || sm.Get("test").ActiveProject != "other" {
		t.Fatalf("background idempotent start: %v %v", result, err)
	}
	switchToProjectName("test", "project")
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
	switchToProjectName("test", "other")
	_, result, err = controlApplication("test", path, "restart")
	if err != nil || result["status"] != "starting" || sm.Get("test").ActiveProject != "other" || sm.Get("test").snapshots["project"].Apps[1].ID == first {
		t.Fatal("restart did not replace terminal")
	}
	switchToProjectName("test", "other")
	_, result, err = controlApplication("test", path, "stop")
	if err != nil || result["status"] != "stopped" || sm.Get("test").ActiveProject != "other" || len(sm.Get("test").snapshots["project"].Apps) != 1 || sm.Get("test").snapshots["project"].Apps[0].ID != "agent" {
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
	if err := saveApplicationSettings(path, applicationSettings{Mode: "shared"}); err != nil {
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

func TestApplicationProjectResolution(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(filepath.Join(nested, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	state := &AppState{ActiveProject: "thread:one", Projects: []Project{{Name: "main", Path: root}, {Name: "nested", Path: nested}}, Threads: []Thread{{ID: "thread:one", Project: "main", Path: root}}}
	for _, test := range []struct{ path, workspace, root string }{
		{root, "thread:one", root},
		{filepath.Join(root, "src"), "thread:one", root},
		{filepath.Join(nested, "src"), "nested", nested},
	} {
		name, path, err := applicationProject(state, test.path)
		if err != nil || name != test.workspace || path != test.root {
			t.Fatalf("resolve %q: %q %q %v", test.path, name, path, err)
		}
	}
	for _, path := range []string{"", root + "-other", filepath.Dir(root)} {
		if _, _, err := applicationProject(state, path); err == nil {
			t.Fatalf("accepted unrelated path %q", path)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	name, path, err := applicationProject(state, filepath.Join(alias, "nested", "src"))
	if err != nil || name != "nested" || path != nested {
		t.Fatalf("resolve symlink: %q %q %v", name, path, err)
	}
}

func TestApplicationBackgroundHydrationPreservesWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX test command")
	}
	oldSM, oldTM := sm, tm
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); sm, tm = oldSM, oldTM })
	path := t.TempDir()
	state := &AppState{
		ActiveProject: "visible", SelectedIndex: 1,
		Projects: []Project{{Name: "visible", Path: t.TempDir()}, {Name: "background", Path: path}},
		Apps:     []Application{{ID: "agent"}, {ID: "browser"}},
	}
	sm.states["test"] = state
	panel, _ := sm.insertProjectCommand("test", "background", "sleep 60", 0, false, path)
	js, handled := hydrateProjectCommand("test", panel.ID)
	if !handled || js == "" || !tm.IsRunning(panel.ID) || !state.snapshots["background"].Apps[0].TerminalReady {
		t.Fatal("background application was marked ready without starting its process")
	}
	if state.ActiveProject != "visible" || state.SelectedIndex != 1 || len(state.Apps) != 2 {
		t.Fatal("background hydration changed the user's workspace")
	}
	if strings.Contains(js, "libroWorkspace.select(") || strings.Contains(js, "__libroSelectedApp=") {
		t.Fatal("background hydration emits selection changes")
	}
	if js, handled := hydrateProjectCommand("test", panel.ID); !handled || js != "" {
		t.Fatal("repeated hydration must preserve the running process")
	}
	if sm.RemoveAppByID("test", panel.ID) == nil || len(state.snapshots["background"].Apps) != 0 || state.SelectedIndex != 1 {
		t.Fatal("background removal changed visible selection")
	}
}

func TestPerThreadApplicationIsolation(t *testing.T) {
	oldDB, oldSM, oldTM := db, sm, tm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); _ = db.Close(); db, sm, tm = oldDB, oldSM, oldTM })
	createTables()
	root, branch := t.TempDir(), t.TempDir()
	state := &AppState{ActiveProject: "project", Projects: []Project{{Name: "project", Path: root}, {Name: "project/branch", Path: branch, Virtual: true, ParentProject: "project"}}, Apps: []Application{{ID: "agent", PluginID: "codex"}}}
	sm.states["test"] = state
	if err := setProjectCommand(root, `printf '%s' "$PORT" > assigned-port; sleep 60`); err != nil {
		t.Fatal(err)
	}
	if err := saveApplicationSettings(root, applicationSettings{Mode: "thread"}); err != nil {
		t.Fatal(err)
	}
	_, first, err := controlApplication("test", root, "start")
	if err != nil {
		t.Fatal(err)
	}
	firstID := state.Apps[1].ID
	_, second, err := controlApplication("test", branch, "start")
	if err != nil {
		t.Fatal(err)
	}
	if first["port"] == 0 || second["port"] == 0 || first["port"] == second["port"] {
		t.Fatalf("ports must be distinct: %v %v", first, second)
	}
	secondPanel := state.snapshots["project/branch"].Apps[0]
	if secondPanel.Command != projectCommand(root) || secondPanel.ApplicationPath != branch || !secondPanel.ApplicationPerThread {
		t.Fatal("thread must inherit command and use its own worktree")
	}
	if state.ActiveProject != "project" {
		t.Fatal("background launch changed selection")
	}
	js, again, err := controlApplication("test", branch, "start")
	if err != nil || js != "" || again["port"] != second["port"] {
		t.Fatal("start must be idempotent")
	}
	switchToProjectName("test", "project/branch")
	if len(state.Apps) != 1 || state.Apps[0].ID != secondPanel.ID {
		t.Fatal("switching moved another thread's application")
	}
	if runtime.GOOS != "windows" {
		if _, handled := hydrateProjectCommand("test", secondPanel.ID); !handled {
			t.Fatal("application not hydrated")
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			data, readErr := os.ReadFile(filepath.Join(branch, "assigned-port"))
			if readErr == nil && string(data) == strconv.Itoa(secondPanel.ApplicationPort) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("application did not receive PORT in its own worktree")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	_, restarted, err := controlApplication("test", branch, "restart")
	if err != nil || restarted["port"] != second["port"] || state.Apps[0].ID == secondPanel.ID {
		t.Fatalf("restart should retain its port: %v %v", restarted, err)
	}
	if state.snapshots["project"].Apps[1].ID != firstID {
		t.Fatal("restart affected another thread")
	}
	if _, _, err := controlApplication("test", branch, "stop"); err != nil {
		t.Fatal(err)
	}
	_, stillRunning, err := controlApplication("test", root, "status")
	if err != nil || stillRunning["status"] != "starting" || state.snapshots["project"].Apps[1].ID != firstID {
		t.Fatal("stop affected another thread")
	}
	if err := saveApplicationSettings(branch, applicationSettings{Mode: "thread", Port: first["port"].(int)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := controlApplication("test", branch, "start"); err == nil {
		t.Fatal("accepted another application's reserved port")
	}
	if len(state.Apps) != 0 {
		t.Fatal("port conflict created a panel")
	}
	if _, _, err := controlApplication("test", root, "stop"); err != nil {
		t.Fatal(err)
	}
	_, overridden, err := controlApplication("test", branch, "start")
	if err != nil || overridden["port"] != first["port"] {
		t.Fatalf("port override was not used: %v %v", overridden, err)
	}

	if _, _, err := controlApplication("test", branch, "stop"); err != nil {
		t.Fatal(err)
	}
	if err := saveApplicationSettings(root, applicationSettings{Mode: "shared"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := controlApplication("test", branch, "start"); err != nil {
		t.Fatal(err)
	}
	shared := state.Apps[0]
	if shared.ApplicationPath != root || shared.ApplicationPerThread {
		t.Fatal("shared app must run from project root")
	}
	switchToProjectName("test", "project")
	if len(state.Apps) != 2 || state.Apps[1].ID != shared.ID {
		t.Fatal("shared application did not follow the selected worktree")
	}
	if js, _, err := controlApplication("test", branch, "start"); err != nil || js != "" {
		t.Fatal("shared start duplicated application across worktrees")
	}
	if _, _, err := controlApplication("test", branch, "stop"); err != nil || len(state.Apps) != 1 {
		t.Fatal("shared stop did not remove the assigned application")
	}
}

func TestApplicationAgentScope(t *testing.T) {
	scope := t.TempDir()
	t.Setenv("LIBRO_APPLICATION_PATH", scope)
	for _, action := range []string{"status", "start", "stop", "restart"} {
		payload := json.RawMessage(fmt.Sprintf(`{"action":%q,"project":"/another-worktree"}`, action))
		if _, err := scopedApplicationCommand(payload, scope); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("MCP accepted a target override: %v", err)
		}
		if _, err := ApplicationCommand(payload); err == nil || !strings.Contains(err.Error(), "restricted") {
			t.Fatalf("agent CLI accepted another workspace: %v", err)
		}
	}
	for _, extra := range []string{`"pid":123`, `"command":"echo test"`, `"port":1234`} {
		if _, err := scopedApplicationCommand(json.RawMessage(`{"action":"stop",`+extra+`}`), scope); err == nil {
			t.Fatal("MCP accepted process selection")
		}
	}
}
