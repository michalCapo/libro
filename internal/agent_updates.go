package libro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

var agentVersionPattern = regexp.MustCompile(`(?:^|\s)v?(\d+\.\d+\.\d+(?:-[\w.-]+)?)(?:\+[\w.-]+)?(?:\s|$)`)

type agentUpdateDefinition struct{ command, name, pkg string }

var updateAgents = []agentUpdateDefinition{{"codex", "Codex", "@openai/codex"}, {"claude", "Claude", "@anthropic-ai/claude-code"}, {"pi", "Pi", "@earendil-works/pi-coding-agent"}, {"opencode", "OpenCode", "opencode-ai"}}

// A missing value is migrated from the desktop preference on its first connection.
func agentAutoUpdate() *bool {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return nil
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='agent_auto_update'`).Scan(&value); err != nil {
		return nil
	}
	enabled := value != "off"
	return &enabled
}
func setAgentAutoUpdate(enabled, initialize bool) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	value := "off"
	if enabled {
		value = "on"
	}
	conflict := "DO UPDATE SET value=excluded.value"
	if initialize {
		conflict = "DO NOTHING"
	}
	_, err := db.Exec(`INSERT INTO settings(key,value) VALUES ('agent_auto_update',?) ON CONFLICT(key) `+conflict, value)
	return err
}
func agentVersion(text string) string {
	match := agentVersionPattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
func newerAgentVersion(latest, current string) bool {
	if latest == "" || current == "" || strings.Contains(latest, "-") || strings.Contains(current, "-") {
		return false
	}
	left, right := strings.Split(latest, "."), strings.Split(current, ".")
	if len(left) != 3 || len(right) != 3 {
		return false
	}
	for i := range 3 {
		a, errA := strconv.Atoi(left[i])
		b, errB := strconv.Atoi(right[i])
		if errA != nil || errB != nil {
			return false
		}
		if a != b {
			return a > b
		}
	}
	return false
}
func runAgentUpdateCommand(ctx context.Context, dir, file string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, file, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	// Discard stderr: updater failures may include private installation details.
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}
func insideDir(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
func agentUpdatePlan(agent agentUpdateDefinition, executable, npmRoot, npm, latest string, help func() string) (string, []string) {
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", nil
	}
	if npmRoot != "" && npm != "" && insideDir(filepath.Join(npmRoot, filepath.FromSlash(agent.pkg)), resolved) {
		return npm, []string{"install", "--global", agent.pkg + "@" + latest}
	}
	home, _ := os.UserHomeDir()
	if agent.command == "claude" && strings.HasPrefix(resolved, filepath.Join(home, ".local", "share", "claude", "versions")+string(filepath.Separator)) {
		return executable, []string{"update"}
	}
	if agent.command == "opencode" && resolved == filepath.Join(home, ".opencode", "bin", "opencode") {
		return executable, []string{"upgrade", latest}
	}
	if agent.command == "codex" && !strings.Contains(filepath.ToSlash(resolved), "/node_modules/") && !strings.Contains(filepath.ToSlash(resolved), "/Cellar/") && !strings.Contains(filepath.ToSlash(resolved), "/Caskroom/") && regexp.MustCompile(`(?m)^\s+update\s`).MatchString(help()) {
		return executable, []string{"update"}
	}
	return "", nil
}

// Updates use public registry metadata and run outside any project directory.
func startAgentUpdates(app *r.App) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Only the first launch needs the frontend to migrate its old setting.
		// Later launches run entirely from the backend's saved preference.
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			if enabled := agentAutoUpdate(); enabled != nil {
				if !*enabled {
					return
				}
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		runAgentUpdates(ctx, func(title, detail, variant string) {
			if ctx.Err() == nil {
				_ = app.Broadcast(showToastJS(title, detail, variant))
			}
		})
	}()
	return func() { cancel(); <-done }
}

func runAgentUpdates(ctx context.Context, notify func(string, string, string)) {
	dir, err := os.MkdirTemp("", "libro-agent-update-")
	if err != nil {
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	run := func(timeout time.Duration, file string, args ...string) (string, error) {
		child, stop := context.WithTimeout(ctx, timeout)
		defer stop()
		return runAgentUpdateCommand(child, dir, file, args...)
	}
	npm, _ := exec.LookPath("npm")
	npmRoot := ""
	if npm != "" {
		root, err := run(15*time.Second, npm, "root", "--global")
		if err == nil {
			npmRoot, _ = filepath.EvalSymlinks(root)
		}
	}
	// Mise owns the tools it installs; its outdated list honors pinned versions and release-age rules.
	mise, _ := exec.LookPath("mise")
	miseBinary, _ := filepath.EvalSymlinks(mise)
	miseTools, miseLatest := map[string]string{}, map[string]string{}
	if mise != "" {
		if text, err := run(15*time.Second, mise, "ls", "--json", "--current"); err == nil {
			var tools map[string][]struct {
				InstallPath string `json:"install_path"`
			}
			_ = json.Unmarshal([]byte(text), &tools)
			for name, versions := range tools {
				for _, version := range versions {
					if path, err := filepath.EvalSymlinks(version.InstallPath); err == nil {
						miseTools[path] = name
					}
				}
			}
		}
		if text, err := run(time.Minute, mise, "outdated", "--json"); err == nil {
			var tools map[string]struct{ Latest string }
			_ = json.Unmarshal([]byte(text), &tools)
			for name, tool := range tools {
				miseLatest[name] = agentVersion(tool.Latest)
			}
		}
	}
	var wg sync.WaitGroup
	for _, definition := range updateAgents {
		wg.Go(func() {
			agent := definition
			executable, err := exec.LookPath(agent.command)
			if err != nil {
				return
			}
			if agent.command == "pi" {
				resolved, _ := filepath.EvalSymlinks(executable)
				if strings.Contains(resolved, "@mariozechner") {
					agent.pkg = "@mariozechner/pi-coding-agent"
				}
			}
			output, err := run(15*time.Second, executable, "--version")
			if err != nil {
				return
			}
			current := agentVersion(output)
			if current == "" || strings.Contains(current, "-") {
				return
			}
			resolved, _ := filepath.EvalSymlinks(executable)
			if mise != "" && resolved == miseBinary {
				// A mise shim runs the active version; ask mise for its real path.
				if path, err := run(15*time.Second, mise, "which", agent.command); err == nil {
					resolved, _ = filepath.EvalSymlinks(path)
				}
			}
			// An npm package wins over the mise-managed Node that hosts it.
			miseTool := ""
			if npmRoot == "" || !insideDir(filepath.Join(npmRoot, filepath.FromSlash(agent.pkg)), resolved) {
				for path, name := range miseTools {
					if insideDir(path, resolved) {
						miseTool = name
					}
				}
			}
			latest := ""
			if miseTool != "" {
				latest = miseLatest[miseTool]
			} else {
				requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
				request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://registry.npmjs.org/"+url.PathEscape(agent.pkg)+"/latest", nil)
				if err == nil {
					response, err := http.DefaultClient.Do(request)
					if err == nil {
						var metadata struct{ Version string }
						if response.StatusCode == 200 {
							_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&metadata)
							latest = agentVersion(metadata.Version)
						}
						_ = response.Body.Close()
					}
				}
				stop()
			}
			if newerAgentVersion(latest, current) && ctx.Err() == nil {
				file, args := mise, []string{"upgrade", miseTool}
				if miseTool == "" {
					file, args = agentUpdatePlan(agent, executable, npmRoot, npm, latest, func() string { text, _ := run(15*time.Second, executable, "--help"); return text })
				}
				if file == "" {
					notify(agent.name+" "+latest+" is available", "Update it with its package manager.", "info")
				} else {
					notify("Updating "+agent.name+"…", current+" → "+latest, "info")
					_, err := run(5*time.Minute, file, args...)
					if miseTool != "" {
						// The upgrade installs a new version directory, so look the command up again.
						if path, err := run(15*time.Second, mise, "which", agent.command); err == nil {
							executable = path
						}
					}
					installed, _ := run(15*time.Second, executable, "--version")
					if err == nil && newerAgentVersion(agentVersion(installed), current) {
						notify(agent.name+" updated to "+agentVersion(installed), "New sessions use the updated version.", "success")
					} else {
						notify(agent.name+" could not be updated", "Try its package manager in a terminal.", "error")
					}
				}
			}
			if agent.command == "pi" && ctx.Err() == nil {
				help, _ := run(15*time.Second, executable, "update", "--help")
				args := []string{"update"}
				if strings.Contains(help, "--extensions") {
					args = []string{"update", "--extensions", "--no-approve"}
				}
				if _, err := run(5*time.Minute, executable, args...); err != nil {
					notify("Pi extensions could not be updated", "Run Pi’s package update command in a terminal.", "error")
				}
			}
		})
	}
	wg.Wait()
}
