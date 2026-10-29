package libro

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

// repoFileLimit keeps a huge untracked folder from flooding the panel.
const repoFileLimit = 200

type repoChange struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

type repoWorktree struct {
	Path    string       `json:"path"`
	Branch  string       `json:"branch"`
	Changes []repoChange `json:"changes"`
	More    int          `json:"more"`
	Error   string       `json:"error,omitempty"`
}

type repoBranch struct {
	Name     string `json:"name"`
	Upstream string `json:"upstream"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Gone     bool   `json:"gone"`
}

type repoStatus struct {
	Name       string         `json:"name"`
	Path       string         `json:"path"`
	Remote     bool           `json:"remote"`
	Fetched    string         `json:"fetched"`
	FetchError string         `json:"fetchError,omitempty"`
	Worktrees  []repoWorktree `json:"worktrees"`
	Branches   []repoBranch   `json:"branches"`
	Error      string         `json:"error,omitempty"`
}

type repoRequest struct {
	SID     string `json:"sid"`
	ID      string `json:"id"`
	Request string `json:"request"`
	Fetch   bool   `json:"fetch"`
}

// Overlapping fetches of one repository fail on ref locks.
var repoFetchMu sync.Mutex

// repoGit returns stdout untrimmed, because porcelain lines start with spaces.
func repoGit(timeout time.Duration, path string, args ...string) (string, error) {
	return repoGitEnv(timeout, path, nil, args...)
}

func repoGitEnv(timeout time.Duration, path string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"), env...)
	// SSH or credential helpers may keep the pipes open after git is killed.
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(exit.Stderr)))
		}
		if ctx.Err() != nil {
			return "", errors.New("git " + args[0] + " timed out")
		}
		return "", err
	}
	return string(out), nil
}

// readRepoStatus reports uncommitted files per worktree and each local
// branch's commits ahead of or behind its upstream.
func readRepoStatus(name, path string, fetch bool) repoStatus {
	status := repoStatus{Name: name, Path: path, Worktrees: []repoWorktree{}, Branches: []repoBranch{}}
	remotes, err := repoGit(5*time.Second, path, "remote")
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Remote = strings.TrimSpace(remotes) != ""
	if fetch && status.Remote {
		// A background fetch must fail instead of waiting for an SSH passphrase,
		// unless the user configured their own SSH command.
		var env []string
		if custom, _ := repoGit(5*time.Second, path, "config", "--get", "core.sshCommand"); os.Getenv("GIT_SSH_COMMAND") == "" && strings.TrimSpace(custom) == "" {
			env = []string{"GIT_SSH_COMMAND=ssh -o BatchMode=yes"}
		}
		if _, err := repoGitEnv(90*time.Second, path, env, "fetch", "--all", "--prune", "--quiet"); err != nil {
			status.FetchError = err.Error()
		}
	}
	if common, err := repoGit(5*time.Second, path, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		if info, err := os.Stat(filepath.Join(strings.TrimSpace(common), "FETCH_HEAD")); err == nil {
			status.Fetched = info.ModTime().UTC().Format(time.RFC3339)
		}
	}
	worktrees, err := GitListWorktrees(path)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	for _, worktree := range worktrees {
		if !worktree.IsBare {
			status.Worktrees = append(status.Worktrees, readWorktreeChanges(worktree))
		}
	}
	refs, err := repoGit(10*time.Second, path, "for-each-ref", "--format=%(refname)%00%(upstream:short)%00%(upstream:track,nobracket)", "refs/heads")
	if err != nil {
		status.Error = err.Error()
		return status
	}
	for line := range strings.SplitSeq(strings.TrimSpace(refs), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 {
			continue
		}
		// Full names stay unambiguous when a tag shares the branch name.
		branch := repoBranch{Name: strings.TrimPrefix(fields[0], "refs/heads/"), Upstream: fields[1]}
		for part := range strings.SplitSeq(fields[2], ", ") {
			if count, ok := strings.CutPrefix(part, "ahead "); ok {
				branch.Ahead, _ = strconv.Atoi(count)
			} else if count, ok := strings.CutPrefix(part, "behind "); ok {
				branch.Behind, _ = strconv.Atoi(count)
			} else if part == "gone" {
				branch.Gone = true
			}
		}
		// Without an upstream, count the commits no remote has yet.
		if branch.Upstream == "" && status.Remote {
			count, err := repoGit(10*time.Second, path, "rev-list", "--count", fields[0], "--not", "--remotes")
			if err != nil {
				status.Error = "could not count unpushed commits on " + branch.Name + ": " + err.Error()
			}
			branch.Ahead, _ = strconv.Atoi(strings.TrimSpace(count))
		}
		status.Branches = append(status.Branches, branch)
	}
	return status
}

func readWorktreeChanges(worktree Worktree) repoWorktree {
	result := repoWorktree{Path: worktree.Path, Branch: worktree.Branch, Changes: []repoChange{}}
	out, err := repoGit(20*time.Second, worktree.Path, "status", "--porcelain=v1", "-z")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		// A rename or copy is followed by its original path.
		if strings.ContainsAny(entry[:2], "RC") {
			i++
		}
		if len(result.Changes) == repoFileLimit {
			result.More++
			continue
		}
		result.Changes = append(result.Changes, repoChange{Code: strings.TrimSpace(entry[:2]), Path: entry[3:]})
	}
	return result
}

// readRepoStatuses runs a few repositories at a time so fetches overlap.
func readRepoStatuses(projects []Project, fetch bool) []repoStatus {
	result := make([]repoStatus, len(projects))
	limit := make(chan struct{}, 6)
	var wait sync.WaitGroup
	for i, project := range projects {
		wait.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()
			result[i] = readRepoStatus(project.Name, project.Path, fetch)
		})
	}
	wait.Wait()
	return result
}

// HTTP keeps slow fetches out of the serialized workspace actions.
func registerReposActions(app *r.App) {
	app.POST("/repos/action", func(w http.ResponseWriter, req *http.Request) {
		if origin := req.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != req.Host {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
		}
		var data repoRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 64*1024)).Decode(&data); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result := map[string]any{"id": data.ID, "request": data.Request}
		projects, allowed := repoProjects(data.SID, data.ID, "repos")
		if allowed {
			if data.Fetch {
				repoFetchMu.Lock()
				defer repoFetchMu.Unlock()
			}
			result["repos"] = readRepoStatuses(projects, data.Fetch)
		} else {
			result["error"] = "Open Repositories in this workspace first"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}

// repoProjects lists the session's Git projects once each, when the panel is
// an open plugin tool; worktree threads are read through their parent repository.
func repoProjects(sid, panel, plugin string) ([]Project, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	if state == nil {
		return nil, false
	}
	allowed := false
	for _, app := range state.Apps {
		allowed = allowed || app.ID == panel && app.PluginID == plugin
	}
	if !allowed {
		return nil, false
	}
	projects := []Project{}
	seen := map[string]bool{}
	for _, project := range state.Projects {
		if project.Virtual || !project.IsGitRepo || project.Path == "" || seen[project.Path] {
			continue
		}
		seen[project.Path] = true
		projects = append(projects, project)
	}
	return projects, true
}

func renderRepos(app Application) *r.Node {
	return r.Widget("repos", struct{}{}, "ws-repos").Key("repos-"+app.ID).Attr("data-repos", app.ID).Render(r.Div("ws-repos-status").Attr("role", "status").Text("Loading repositories…"))
}
