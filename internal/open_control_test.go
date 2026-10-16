package libro

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

func TestOpenFilePanelBesideCaller(t *testing.T) {
	sid, _ := childFixture(t)
	desktopWindow = true
	t.Cleanup(func() { desktopWindow = false })
	state := sm.Get(sid)
	state.Apps = []Application{{ID: "app-1", Type: AppTypeTerminal, Command: "vime"}, {ID: "app-2", Type: AppTypeTerminal, Command: "bash"}}
	state.snapshots["other"] = &projectSnapshot{Apps: []Application{{ID: "app-3", Type: AppTypeTerminal, Command: "vime"}}}
	sm.nextID = 10
	desktopWindow = false
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-1", URL: "file:///tmp/a.png"}); err == nil {
		t.Fatal("opened without the desktop window")
	}
	desktopWindow = true
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-1", URL: "https://example.com"}); err == nil {
		t.Fatal("opened a remote address")
	}
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-9", URL: "file:///tmp/a.png"}); err == nil {
		t.Fatal("opened for a closed panel")
	}
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-3", URL: "file:///tmp/a.png"}); err == nil {
		t.Fatal("opened in a hidden workspace")
	}
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-1", URL: "file:///tmp/a.png"}); err != nil {
		t.Fatal(err)
	}
	state = sm.Get(sid)
	if len(state.Apps) != 3 || state.SelectedIndex != 1 || state.Apps[1].URL != "file:///tmp/a.png" || state.Apps[1].PluginID != "browser" {
		t.Fatalf("file panel not beside caller: %+v", state.Apps)
	}
	state.SelectedIndex = 0
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "open", Panel: "app-2", URL: "file:///tmp/b.pdf"}); err != nil {
		t.Fatal(err)
	}
	state = sm.Get(sid)
	if len(state.Apps) != 3 || state.SelectedIndex != 1 || state.Apps[1].URL != "file:///tmp/b.pdf" {
		t.Fatalf("file panel not reused: %+v", state.Apps)
	}
}

func TestFileURLEscapesPath(t *testing.T) {
	if got := fileURL("/tmp/a b#1.html"); got != "file:///tmp/a%20b%231.html" {
		t.Fatal(got)
	}
}

func TestOpenCLIThroughBackendControl(t *testing.T) {
	sid, _ := childFixture(t)
	desktopWindow = true
	t.Cleanup(func() { desktopWindow = false })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LIBRO_INSTANCE", "")
	sm.backendSID = sid
	sm.Get(sid).Apps = []Application{{ID: "app-1", Type: AppTypeTerminal, Command: "vime"}}
	app := r.NewApp()
	t.Cleanup(func() { _ = app.Close() })
	cleanup, err := startBackendControl(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	file := filepath.Join(t.TempDir(), "page one.html")
	if err = os.WriteFile(file, []byte("<h1>Hi</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIBRO_PANEL_ID", "")
	if err = RunOpenCLI([]string{file}, io.Discard); err == nil {
		t.Fatal("opened outside a Libro terminal")
	}
	t.Setenv("LIBRO_PANEL_ID", "app-1")
	if err = RunOpenCLI([]string{file + ".missing"}, io.Discard); err == nil {
		t.Fatal("opened a missing file")
	}
	if err = RunOpenCLI([]string{file}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if apps := sm.Get(sid).Apps; len(apps) != 2 || apps[1].URL != fileURL(file) {
		t.Fatalf("file panel missing: %+v", apps)
	}
}

func TestTerminalKnowsItsPanel(t *testing.T) {
	_, _ = childFixture(t)
	if _, err := tm.StartWithEnvironment("app-5", `printf 'panel=%s bin=%s' "$LIBRO_PANEL_ID" "${LIBRO_BIN:+set}"; sleep 5`, t.TempDir(), true, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if output, _ := tm.Logs("app-5"); strings.Contains(output, "panel=app-5 bin=set") {
			return
		}
		if time.Now().After(deadline) {
			output, _ := tm.Logs("app-5")
			t.Fatalf("terminal environment: %q", output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
