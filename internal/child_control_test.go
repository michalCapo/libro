package libro

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"libro/internal/components"
)

func childFixture(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY fixture requires POSIX")
	}
	oldDB, oldSM, oldTM := db, sm, tm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "libro.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); _ = db.Close(); db, sm, tm = oldDB, oldSM, oldTM })
	createTables()
	state, root, _ := finishFixture(t)
	state.ActiveProject = "repo"
	state.snapshots = map[string]*projectSnapshot{}
	sm.states["qa"] = state
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// Avoid touching real browser sessions or the user's Bash startup files.
	bin := t.TempDir()
	browser := filepath.Join(bin, "agent-browser")
	t.Setenv("TEST_BROWSER_LOG", filepath.Join(bin, "browser.log"))
	writeFinishFile(t, bin, "agent-browser", "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$TEST_BROWSER_LOG\"\nexit 0\n")
	if err = os.Chmod(browser, 0o700); err != nil {
		t.Fatal(err)
	}
	bash := "#!/bin/sh\nif [ \"$1\" = '-lic' ]; then exec /bin/bash --noprofile --norc -c \"$2\"; fi\nexec /bin/bash \"$@\"\n"
	writeFinishFile(t, bin, "bash", bash)
	if err = os.Chmod(filepath.Join(bin, "bash"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	return "qa", root
}
func createTestChild(t *testing.T, sid, root, name string) childRecord {
	t.Helper()
	_, result, err := controlChildren(sid, root, childCommand{Action: "create", Name: name, Base: "main", Prompt: "QA the assigned app"})
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]any)["id"].(string)
	children, err := loadChildren(applicationPath(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		if child.ID == id {
			return child
		}
	}
	t.Fatal("saved child missing")
	return childRecord{}
}
func childAction(t *testing.T, sid, root string, args childCommand) map[string]any {
	t.Helper()
	_, result, err := controlChildren(sid, root, args)
	if err != nil {
		t.Fatal(err)
	}
	return result.(map[string]any)
}

func TestChildrenApplicationAndProcessIsolation(t *testing.T) {
	sid, root := childFixture(t)
	if err := setProjectCommand(root, "sleep 30"); err != nil {
		t.Fatal(err)
	}
	// A selected base branch differs from the source checkout's HEAD.
	if _, err := worktreeGit(root, "branch", "selected"); err != nil {
		t.Fatal(err)
	}
	writeFinishFile(t, root, "file.txt", "parent changed\n")
	if _, err := worktreeGit(root, "commit", "-am", "parent change"); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	pi := filepath.Join(bin, "pi")
	writeFinishFile(t, bin, "pi", "#!/bin/sh\nprintf 'binding=%s browser=%s\\n' \"$LIBRO_APPLICATION_PATH\" \"$AGENT_BROWSER_SESSION\"\nsleep 30\n")
	if err := os.Chmod(pi, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIBRO_APPLICATION_PATH", root)
	children := make([]childRecord, 3)
	apps := make([]string, 3)
	ports := map[int]bool{}
	for i := range children {
		_, result, err := controlChildren(sid, root, childCommand{Action: "create", Name: "QA " + string(rune('A'+i)), Base: "selected", Prompt: "Check app"})
		if err != nil {
			t.Fatal(err)
		}
		id := result.(map[string]any)["id"].(string)
		saved, err := loadChildren(applicationPath(root))
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range saved {
			if child.ID == id {
				children[i] = child
			}
		}
		child := &children[i]
		content, err := os.ReadFile(filepath.Join(child.Path, "file.txt"))
		if err != nil || string(content) != "original\n" {
			t.Fatal("child ignored selected base branch")
		}
		if sm.Get(sid).thread(child.ID) == nil {
			t.Fatal("child is not a visible thread")
		}
		if js := projectAutolaunchJS(&AppState{ActiveProject: child.ID, Threads: []Thread{{ID: child.ID, Managed: true}}}, sid); js != "" {
			t.Fatal("managed child autolaunched an unrelated agent")
		}
		childAction(t, sid, root, childCommand{Action: "launch", ID: id, Command: []string{pi, "-p"}, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash", Thinking: "high"})
		_, status, err := controlApplication(sid, child.Path, "start")
		if err != nil {
			t.Fatal(err)
		}
		port := status["port"].(int)
		if port == 0 || ports[port] {
			t.Fatal("children share an application port")
		}
		ports[port] = true
		for _, app := range sm.Get(sid).snapshots[id].Apps {
			if app.PluginID == "project-command" {
				apps[i] = app.ID
			}
		}
		if _, ok := hydrateProjectCommand(sid, apps[i]); !ok {
			t.Fatal("app not hydrated")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for i := range children {
		for {
			result := childAction(t, sid, root, childCommand{Action: "status", ID: children[i].ID})
			output := result["output"].(string)
			if strings.Contains(output, children[i].Path) && strings.Contains(output, children[i].Browser) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("child did not inherit its own Libro binding")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if _, _, err := controlApplication(sid, children[0].Path, "restart"); err != nil {
		t.Fatal(err)
	}
	if tm.IsRunning(apps[0]) || !tm.IsRunning(apps[1]) || !tm.IsRunning(apps[2]) {
		t.Fatal("application restart crossed ownership boundary")
	}
	first := childAction(t, sid, root, childCommand{Action: "interrupt", ID: children[0].ID})
	if first["status"] != "failed" {
		t.Fatal("interrupted agent not failed")
	}
	restarted := childAction(t, sid, root, childCommand{Action: "restart", ID: children[0].ID})
	if restarted["attempt"] != 2 {
		t.Fatal("restart did not create a fresh attempt")
	}
	childAction(t, sid, root, childCommand{Action: "cleanup", ID: children[0].ID})
	if !tm.IsRunning(apps[1]) || !tm.IsRunning(apps[2]) {
		t.Fatal("cleanup stopped sibling apps")
	}
	browserLog, err := os.ReadFile(os.Getenv("TEST_BROWSER_LOG"))
	if err != nil || !strings.Contains(string(browserLog), "--session\n"+children[0].Browser+"\nclose") || strings.Contains(string(browserLog), children[1].Browser) || strings.Contains(string(browserLog), children[2].Browser) {
		t.Fatal("cleanup closed a sibling browser session")
	}
	for _, child := range children[1:] {
		result := childAction(t, sid, root, childCommand{Action: "status", ID: child.ID})
		if result["status"] != "running" && result["status"] != "waiting" {
			t.Fatal("cleanup stopped sibling agent")
		}
	}
	if _, err := os.Stat(children[0].Path); !os.IsNotExist(err) {
		t.Fatal("owned worktree survived cleanup")
	}
	branches, err := GitListBranches(root)
	if err != nil || slices.Contains(branches, children[0].Branch) || !slices.Contains(branches, children[1].Branch) {
		t.Fatal("cleanup removed the wrong branches")
	}
	if sm.Get(sid).ActiveProject != "repo" {
		t.Fatal("background commands changed selected thread")
	}
}

func TestChildCleanupRecoveryAndPreservation(t *testing.T) {
	sid, root := childFixture(t)
	child := createTestChild(t, sid, root, "Recovery QA")
	writeFinishFile(t, child.Path, "screenshot.png", "test screenshot")
	writeFinishFile(t, child.Path, "file.txt", "uncommitted result")
	// Force preservation to fail after stopping; Git resources must survive.
	target := filepath.Join(child.Results, "worktree.tar.gz")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := controlChildren(sid, root, childCommand{Action: "cleanup", ID: child.ID}); err == nil {
		t.Fatal("cleanup ignored failed preservation")
	}
	if _, err := os.Stat(child.Path); err != nil {
		t.Fatal("failed preservation removed results")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	// Model a desktop restart midway through cleanup.
	sm.states[sid].Threads = nil
	sm.states[sid].snapshots = map[string]*projectSnapshot{}
	if restored := loadChildThreads(sm.states[sid].Projects); len(restored) != 1 || restored[0].ID != child.ID {
		t.Fatal("cleanup journal did not restore visible child")
	}
	result := childAction(t, sid, root, childCommand{Action: "cleanup", ID: child.ID})
	if result["phase"] != "cleaned" {
		t.Fatal("cleanup not completed")
	}
	childAction(t, sid, root, childCommand{Action: "cleanup", ID: child.ID})
	file, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	archive := tar.NewReader(gz)
	found := map[string]string{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		found[header.Name] = string(data)
	}
	if found["screenshot.png"] != "test screenshot" || found["file.txt"] != "uncommitted result" {
		t.Fatal("archive lost results or screenshots")
	}
	if !fileExists(filepath.Join(child.Results, "commits.bundle")) {
		t.Fatal("Git history not preserved")
	}
	if len(sm.Get(sid).Threads) != 0 {
		t.Fatal("cleaned child still visible")
	}
}

func TestChildrenRejectOtherOwnerAndSharedMode(t *testing.T) {
	sid, root := childFixture(t)
	child := createTestChild(t, sid, root, "Owned")
	_, _, err := controlChildren(sid, sm.Get(sid).Projects[1].Path, childCommand{Action: "interrupt", ID: child.ID})
	if err == nil {
		t.Fatal("another workspace controlled the child")
	}
	if err = saveApplicationSettings(root, applicationSettings{Mode: "shared"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = controlChildren(sid, root, childCommand{Action: "create", Name: "invalid", Base: "main", Prompt: "QA"}); err == nil {
		t.Fatal("isolated child accepted shared application mode")
	}
	if _, err = scopedChildrenCommand(json.RawMessage(`{"action":"list","project":"/another"}`), root); err == nil {
		t.Fatal("MCP accepted target override")
	}
}

func TestChildCredentialsAndFollowup(t *testing.T) {
	sid, root := childFixture(t)
	child := createTestChild(t, sid, root, "Credential QA")
	const secret = "fake-provider-key-for-test"
	t.Setenv("OPENROUTER_API_KEY", secret)
	if err := setAgentEnvironment([]environmentInput{{Name: "CUSTOM_PROVIDER", Value: "fake-custom-credential"}}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	pi := filepath.Join(bin, "pi")
	writeFinishFile(t, bin, "pi", "#!/bin/sh\nfor arg do last=$arg; done\nprintf '%s' \"$last\" > task.txt\nprintf '%s\\n' \"$OPENROUTER_API_KEY\"\nprintf '%s\\n' \"$CUSTOM_PROVIDER\"\nexit 0\n")
	if err := os.Chmod(pi, 0o700); err != nil {
		t.Fatal(err)
	}
	childAction(t, sid, root, childCommand{Action: "launch", ID: child.ID, Command: []string{pi, "-p"}})
	wait := func(attempt int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			result := childAction(t, sid, root, childCommand{Action: "status", ID: child.ID})
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "fake-custom-credential") {
				t.Fatal("tool response leaked provider credentials")
			}
			if result["status"] == "completed" && result["attempt"] == attempt {
				if !strings.Contains(result["output"].(string), "[REDACTED]") {
					t.Fatal("redacted result missing")
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("attempt did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(1)
	childAction(t, sid, root, childCommand{Action: "followup", ID: child.ID, Prompt: "Also test errors"})
	wait(2)
	data, err := os.ReadFile(filepath.Join(child.Path, "task.txt"))
	if err != nil || !strings.Contains(string(data), "QA the assigned app") || !strings.Contains(string(data), "Also test errors") {
		t.Fatal("followup lost saved task")
	}
	for _, name := range []string{"attempt-1.log", "attempt-2.log"} {
		data, err := os.ReadFile(filepath.Join(child.Results, name))
		if err != nil || strings.Contains(string(data), secret) || strings.Contains(string(data), "fake-custom-credential") {
			t.Fatal("attempt log leaked credentials")
		}
	}
	// Exit and last activity are recovered even when the desktop loses its PTYs.
	oldTM := tm
	tm = components.NewTerminalManager()
	defer func() { tm = oldTM }()
	result := childAction(t, sid, root, childCommand{Action: "status", ID: child.ID})
	if result["status"] != "completed" || result["exitCode"] == nil {
		t.Fatal("saved exit state was lost after restart")
	}
}

func TestChildCleanupAfterGitRemovalAndSharedApp(t *testing.T) {
	sid, root := childFixture(t)
	child := createTestChild(t, sid, root, "Removal recovery")
	if err := archiveChild(&child); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreeGit(root, "bundle", "create", filepath.Join(child.Results, "commits.bundle"), "refs/heads/"+child.Branch); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreeGit(root, "worktree", "remove", "--force", child.Path); err != nil {
		t.Fatal(err)
	}
	child.Phase = "removing"
	if err := saveChild(&child); err != nil {
		t.Fatal(err)
	}
	// A shared app parked in the child must remain owned by Base.
	if err := saveApplicationSettings(root, applicationSettings{Mode: "shared"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.StartWithEnvironment("shared-app", "sleep 30", root, true, nil); err != nil {
		t.Fatal(err)
	}
	sm.Get(sid).snapshots[child.ID] = &projectSnapshot{Apps: []Application{{ID: "shared-app", PluginID: "project-command", Dock: "bottom", Type: AppTypeTerminal, TerminalReady: true, ApplicationPath: root}}}
	childAction(t, sid, root, childCommand{Action: "cleanup", ID: child.ID})
	if !tm.IsRunning("shared-app") || !slices.ContainsFunc(sm.Get(sid).Apps, func(app Application) bool { return app.ID == "shared-app" }) {
		t.Fatal("cleanup killed or orphaned shared app")
	}
	branches, err := GitListBranches(root)
	if err != nil || slices.Contains(branches, child.Branch) {
		t.Fatal("retry did not remove owned temporary branch")
	}
	childAction(t, sid, root, childCommand{Action: "cleanup", ID: child.ID})
}

func TestChildRejectsShellCommandInjection(t *testing.T) {
	sid, root := childFixture(t)
	child := createTestChild(t, sid, root, "Command validation")
	if _, _, err := controlChildren(sid, root, childCommand{Action: "launch", ID: child.ID, Command: []string{"pi>/tmp/libro-qa-injection"}}); err == nil {
		t.Fatal("shell operator accepted as executable")
	}
	result := childAction(t, sid, root, childCommand{Action: "status", ID: child.ID})
	if result["attempt"] != 0 {
		t.Fatal("invalid executable created an attempt")
	}
}
