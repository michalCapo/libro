package libro

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	r "github.com/michalCapo/g-sui/ui"
)

func TestAgentUpdatePreferenceMigration(t *testing.T) {
	setupNoteControl(t)
	if agentAutoUpdate() != nil {
		t.Fatal("missing desktop preference was treated as enabled")
	}
	if err := setAgentAutoUpdate(false, true); err != nil {
		t.Fatal(err)
	}
	if enabled := agentAutoUpdate(); enabled == nil || *enabled {
		t.Fatal("disabled desktop preference was lost")
	}
	if err := setAgentAutoUpdate(true, true); err != nil {
		t.Fatal(err)
	}
	if *agentAutoUpdate() {
		t.Fatal("another frontend overwrote saved preference during migration")
	}
	if err := setAgentAutoUpdate(true, false); err != nil || !*agentAutoUpdate() {
		t.Fatal("setting could not be changed")
	}
}

func TestAgentUpdateVersions(t *testing.T) {
	for text, want := range map[string]string{"codex-cli 0.100.0": "0.100.0", "2.1.4 (Claude Code)": "2.1.4", "v1.2.3-beta.1": "1.2.3-beta.1", "unexpected": ""} {
		if got := agentVersion(text); got != want {
			t.Fatalf("version %q = %q", text, got)
		}
	}
	if !newerAgentVersion("0.100.0", "0.99.0") {
		t.Fatal("numeric release comparison failed")
	}
	for _, pair := range [][2]string{{"0.100.0", "0.100.0"}, {"0.100.0", "0.101.0"}, {"0.100.0", "0.100.0-beta.1"}, {"invalid", "1.0.0"}} {
		if newerAgentVersion(pair[0], pair[1]) {
			t.Fatal("updater selected downgrade or prerelease")
		}
	}
}

func TestAgentUpdatePlanUsesOwningPackage(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "@earendil-works", "pi-coding-agent", "cli.js")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	agent := agentUpdateDefinition{command: "pi", pkg: "@earendil-works/pi-coding-agent"}
	file, args := agentUpdatePlan(agent, executable, root, "npm", "1.2.3", func() string { return "" })
	if file != "npm" || !reflect.DeepEqual(args, []string{"install", "--global", agent.pkg + "@1.2.3"}) {
		t.Fatal("npm updater did not target owning package")
	}
	file, _ = agentUpdatePlan(agent, executable, filepath.Join(root, "other"), "npm", "1.2.3", func() string { return "" })
	if file != "" {
		t.Fatal("npm updater used unrelated package root")
	}
}

type updateRegistryTransport func(*http.Request) (*http.Response, error)

func (f updateRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func agentUpdateFixture(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake CLI fixtures use shell scripts")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	for _, agent := range []agentUpdateDefinition{updateAgents[0], updateAgents[2]} {
		dir := filepath.Join(root, filepath.FromSlash(agent.pkg))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(root, agent.command+".version")
		if err := os.WriteFile(state, []byte("1.0.0"), 0o600); err != nil {
			t.Fatal(err)
		}
		cli := filepath.Join(dir, "cli")
		script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n--version) /bin/cat %s;;\nupdate) if [ \"$2\" = --help ]; then printf '%%s' --extensions; else printf 'extensions\\n' >> %s; fi;;\nesac\n", quote(state), quote(filepath.Join(root, "updates.log")))
		if err := os.WriteFile(cli, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(cli, filepath.Join(bin, agent.command)); err != nil {
			t.Fatal(err)
		}
	}
	npm := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = root ]; then printf '%%s' %s; exit; fi\nprintf '%%s\\n' \"$3\" >> %s\ncase \"$3\" in\n@openai/*) ", quote(root), quote(filepath.Join(root, "updates.log")))
	switch mode {
	case "failed":
		npm += "exit 1"
	case "unchanged":
		npm += "true"
	default:
		npm += "printf 1.1.0 > " + quote(filepath.Join(root, "codex.version"))
	}
	npm += ";;\n*) printf 1.1.0 > " + quote(filepath.Join(root, "pi.version")) + ";;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(npm), 0o700); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: updateRegistryTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "registry.npmjs.org" || req.Header.Get("Authorization") != "" {
			return nil, fmt.Errorf("unexpected registry request")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"version":"1.1.0"}`)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previous })
	return root
}

func TestBackendAgentUpdatesVerifyAndContinue(t *testing.T) {
	for _, mode := range []string{"updated", "failed", "unchanged"} {
		t.Run(mode, func(t *testing.T) {
			root := agentUpdateFixture(t, mode)
			var mu sync.Mutex
			var notices [][2]string
			runAgentUpdates(context.Background(), func(title, _, variant string) {
				mu.Lock()
				notices = append(notices, [2]string{title, variant})
				mu.Unlock()
			})
			codexSuccess, codexFailure, piSuccess := false, false, false
			for _, notice := range notices {
				codexSuccess = codexSuccess || strings.HasPrefix(notice[0], "Codex updated") && notice[1] == "success"
				codexFailure = codexFailure || strings.HasPrefix(notice[0], "Codex could not") && notice[1] == "error"
				piSuccess = piSuccess || strings.HasPrefix(notice[0], "Pi updated") && notice[1] == "success"
			}
			if codexSuccess != (mode == "updated") || codexFailure == (mode == "updated") || !piSuccess {
				t.Fatal("update result was not verified or blocked another agent")
			}
			log, err := os.ReadFile(filepath.Join(root, "updates.log"))
			if err != nil || !strings.Contains(string(log), "extensions") {
				t.Fatal("Pi extensions did not update after Pi")
			}
		})
	}
}

func TestBackendAgentUpdatesCancelWhileAwaitingMigration(t *testing.T) {
	setupNoteControl(t)
	app := r.NewApp()
	t.Cleanup(func() { _ = app.Close() })
	stop := startAgentUpdates(app)
	stop()
	if err := setAgentAutoUpdate(false, true); err != nil {
		t.Fatal(err)
	}
	stop = startAgentUpdates(app)
	stop()
}
