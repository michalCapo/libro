package libro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"libro/internal/components"

	r "github.com/michalCapo/g-sui/ui"
)

// releaseMissingLimit keeps a long-forgotten target from flooding the panel.
const releaseMissingLimit = 20

// A target names a place the project runs, like Installed or Production, and
// a command that prints the version running there.
type releaseTarget struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type releaseTargetStatus struct {
	releaseTarget
	Output  string   `json:"output"`
	Version string   `json:"version"`
	Known   bool     `json:"known"`
	Behind  int      `json:"behind"`
	Ahead   int      `json:"ahead"`
	Missing []string `json:"missing"`
	Error   string   `json:"error,omitempty"`
}

type releaseStatus struct {
	Name     string                `json:"name"`
	Path     string                `json:"path"`
	Branch   string                `json:"branch"`
	Head     string                `json:"head"`
	Tag      string                `json:"tag"`
	SinceTag int                   `json:"sinceTag"`
	Targets  []releaseTargetStatus `json:"targets"`
	Error    string                `json:"error,omitempty"`
}

type releaseRequest struct {
	SID     string          `json:"sid"`
	ID      string          `json:"id"`
	Request string          `json:"request"`
	Path    string          `json:"path"`
	Save    bool            `json:"save"`
	Targets []releaseTarget `json:"targets"`
}

var (
	releaseHash  = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	releaseToken = regexp.MustCompile(`[^\s"'(),;:=\[\]{}<>]+`)
	// Target commands may reach slow servers; a few run at a time.
	releaseCommandLimit = make(chan struct{}, 6)
)

func loadReleaseTargets(path string) []releaseTarget {
	targets := []releaseTarget{}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		var value string
		if db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "release-targets:"+path).Scan(&value) == nil {
			_ = json.Unmarshal([]byte(value), &targets)
		}
	}
	return targets
}

func saveReleaseTargets(path string, targets []releaseTarget) error {
	if len(targets) > 10 {
		return errors.New("add at most 10 targets")
	}
	for i, target := range targets {
		targets[i] = releaseTarget{Name: strings.TrimSpace(target.Name), Command: strings.TrimSpace(target.Command)}
		if targets[i].Name == "" || targets[i].Command == "" {
			return errors.New("give every target a name and a command")
		}
		if len(targets[i].Name) > 40 || strings.ContainsRune(targets[i].Name+targets[i].Command, 0) {
			return fmt.Errorf("target %q needs a shorter name without null characters", targets[i].Name)
		}
	}
	data, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return errors.New("settings database is unavailable")
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "release-targets:"+path, string(data))
	return err
}

// runReleaseTarget returns what the target's command printed to stdout.
func runReleaseTarget(path, command string) (string, error) {
	releaseCommandLimit <- struct{}{}
	defer func() { <-releaseCommandLimit }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := components.ShellCommand(ctx, command)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	// SSH may keep the pipes open after the shell is killed.
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", errors.New("the command took longer than 30 seconds")
	}
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			lines := strings.Split(strings.TrimSpace(string(exit.Stderr)), "\n")
			if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
				return "", errors.New(last)
			}
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveReleaseVersion finds the commit a target's output names: a commit
// hash or git describe output first, as they are exact, then a tag, with or
// without its "v" prefix, so "libro 0.0.88" matches tag v0.0.88.
func resolveReleaseVersion(path, output string, tags map[string]string) (version, commit string) {
	tokens := releaseToken.FindAllString(output, 50)
	for _, token := range tokens {
		if releaseHash.MatchString(token) || strings.Contains(token, "-g") {
			if out, err := repoGit(5*time.Second, path, "rev-parse", "--verify", "--quiet", "--end-of-options", token+"^{commit}"); err == nil {
				return token, strings.TrimSpace(out)
			}
		}
	}
	for _, token := range tokens {
		for _, tag := range []string{token, "v" + strings.TrimPrefix(token, "v"), strings.TrimPrefix(token, "v")} {
			if commit := tags[tag]; commit != "" {
				return tag, commit
			}
		}
	}
	return "", ""
}

func readReleaseStatus(name, path string) releaseStatus {
	status := releaseStatus{Name: name, Path: path, Targets: []releaseTargetStatus{}}
	head, err := repoGit(5*time.Second, path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		status.Error = "no commits yet"
		return status
	}
	head = strings.TrimSpace(head)
	status.Head = head[:min(len(head), 7)]
	if branch, err := repoGit(5*time.Second, path, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		status.Branch = strings.TrimSpace(branch)
	}
	if tag, err := repoGit(5*time.Second, path, "describe", "--tags", "--abbrev=0", "HEAD"); err == nil {
		status.Tag = strings.TrimSpace(tag)
		count, _ := repoGit(10*time.Second, path, "rev-list", "--count", "refs/tags/"+status.Tag+"..HEAD")
		status.SinceTag, _ = strconv.Atoi(strings.TrimSpace(count))
	}
	tags := map[string]string{}
	refs, err := repoGit(10*time.Second, path, "for-each-ref", "--format=%(refname:strip=2)%00%(objectname)%00%(*objectname)", "refs/tags")
	if err != nil {
		status.Error = err.Error()
	}
	for line := range strings.SplitSeq(strings.TrimSpace(refs), "\n") {
		// Annotated tags point at a tag object; the third field is its commit.
		if fields := strings.Split(line, "\x00"); len(fields) == 3 {
			tags[fields[0]] = fields[1]
			if fields[2] != "" {
				tags[fields[0]] = fields[2]
			}
		}
	}
	targets := loadReleaseTargets(path)
	status.Targets = make([]releaseTargetStatus, len(targets))
	var wait sync.WaitGroup
	for i, target := range targets {
		wait.Go(func() { status.Targets[i] = readReleaseTarget(path, head, target, tags) })
	}
	wait.Wait()
	return status
}

func readReleaseTarget(path, head string, target releaseTarget, tags map[string]string) releaseTargetStatus {
	result := releaseTargetStatus{releaseTarget: target, Missing: []string{}}
	output, err := runReleaseTarget(path, target.Command)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if output == "" {
		result.Error = "the command printed nothing"
		return result
	}
	result.Output = strings.SplitN(output, "\n", 2)[0]
	version, commit := resolveReleaseVersion(path, output, tags)
	if commit == "" {
		return result
	}
	result.Version, result.Known = version, true
	counts, err := repoGit(10*time.Second, path, "rev-list", "--left-right", "--count", commit+"..."+head)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if fields := strings.Fields(counts); len(fields) == 2 {
		result.Ahead, _ = strconv.Atoi(fields[0])
		result.Behind, _ = strconv.Atoi(fields[1])
	}
	if result.Behind > 0 {
		log, err := repoGit(10*time.Second, path, "log", "--format=%s", "-n", strconv.Itoa(releaseMissingLimit), commit+".."+head)
		if err == nil {
			result.Missing = strings.Split(strings.TrimSpace(log), "\n")
		}
	}
	return result
}

func readReleaseStatuses(projects []Project) []releaseStatus {
	result := make([]releaseStatus, len(projects))
	var wait sync.WaitGroup
	for i, project := range projects {
		wait.Go(func() { result[i] = readReleaseStatus(project.Name, project.Path) })
	}
	wait.Wait()
	return result
}

// HTTP keeps slow target commands out of the serialized workspace actions.
func registerReleasesActions(app *r.App) {
	app.POST("/releases/action", func(w http.ResponseWriter, req *http.Request) {
		if origin := req.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != req.Host {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
		}
		var data releaseRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 64*1024)).Decode(&data); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result := map[string]any{"id": data.ID, "request": data.Request, "path": data.Path}
		projects, allowed := repoProjects(data.SID, data.ID, "releases")
		// A save or a single-project reread only reaches a listed project.
		if data.Path != "" {
			projects = slices.DeleteFunc(projects, func(p Project) bool { return p.Path != data.Path })
		}
		switch {
		case !allowed:
			result["error"] = "Open Releases in this workspace first"
		case data.Path != "" && len(projects) == 0 || data.Save && data.Path == "":
			result["error"] = "Project not found"
		default:
			if data.Save {
				if err := saveReleaseTargets(data.Path, data.Targets); err != nil {
					result["error"] = "Could not save targets: " + err.Error()
					break
				}
			}
			result["releases"] = readReleaseStatuses(projects)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}

func renderReleases(app Application) *r.Node {
	return r.Widget("releases", struct{}{}, "ws-repos ws-releases").Key("releases-"+app.ID).Attr("data-releases", app.ID).Render(r.Div("ws-repos-status").Attr("role", "status").Text("Loading releases…"))
}
