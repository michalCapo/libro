package libro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Orig is the old path of a rename or copy.
	Orig string `json:"-"`
	// Unstaged changes must be added before a commit.
	Unstaged bool `json:"-"`
}

type repoWorktree struct {
	Path    string       `json:"path"`
	Branch  string       `json:"branch"`
	Changes []repoChange `json:"changes"`
	More    int          `json:"more"`
	// Operation is a merge or rebase stopped on conflicts.
	Operation string `json:"operation,omitempty"`
	Error     string `json:"error,omitempty"`
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
	Label      string         `json:"label"`
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
	// Change runs one action in Worktree: commit or discard Paths (or All),
	// push or pull Branch, merge or rebase onto Onto, or continue or abort.
	Change   string   `json:"change"`
	Worktree string   `json:"worktree"`
	Paths    []string `json:"paths"`
	All      bool     `json:"all"`
	Message  string   `json:"message"`
	Branch   string   `json:"branch"`
	Onto     string   `json:"onto"`
}

// Overlapping fetches, pushes and pulls of one repository fail on ref locks.
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

// repoRemoteEnv makes remote commands fail instead of waiting for an SSH
// passphrase, unless the user configured their own SSH command.
func repoRemoteEnv(path string) []string {
	if custom, _ := repoGit(5*time.Second, path, "config", "--get", "core.sshCommand"); os.Getenv("GIT_SSH_COMMAND") == "" && strings.TrimSpace(custom) == "" {
		return []string{"GIT_SSH_COMMAND=ssh -o BatchMode=yes"}
	}
	return nil
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
		if _, err := repoGitEnv(90*time.Second, path, repoRemoteEnv(path), "fetch", "--all", "--prune", "--quiet"); err != nil {
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
	changes, err := worktreeEntries(worktree.Path)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(changes) > repoFileLimit {
		result.More = len(changes) - repoFileLimit
		changes = changes[:repoFileLimit]
	}
	result.Changes = append(result.Changes, changes...)
	result.Operation = repoOperation(worktree.Path)
	return result
}

// repoOperation names the merge or rebase in progress in a worktree.
func repoOperation(path string) string {
	dir, err := repoGit(5*time.Second, path, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return ""
	}
	for marker, operation := range map[string]string{"MERGE_HEAD": "merge", "rebase-merge": "rebase", "rebase-apply": "rebase"} {
		if _, err := os.Stat(filepath.Join(strings.TrimSpace(dir), marker)); err == nil {
			return operation
		}
	}
	return ""
}

func worktreeEntries(path string) ([]repoChange, error) {
	out, err := repoGit(20*time.Second, path, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	changes := []repoChange{}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		change := repoChange{Code: strings.TrimSpace(entry[:2]), Path: entry[3:], Unstaged: entry[1] != ' '}
		// A rename or copy is followed by its original path.
		if strings.ContainsAny(entry[:2], "RC") && i+1 < len(entries) {
			i++
			change.Orig = entries[i]
		}
		changes = append(changes, change)
	}
	return changes, nil
}

var (
	errRepoChangesGone = errors.New("the files changed meanwhile")
	errRepoNoOperation = errors.New("no merge or rebase is in progress")
)

// repoWorktreeBranch finds a worktree of a listed project and its branch.
func repoWorktreeBranch(projects []Project, path string) (string, bool) {
	for _, project := range projects {
		worktrees, _ := GitListWorktrees(project.Path)
		for _, worktree := range worktrees {
			if !worktree.IsBare && worktree.Path == path {
				return worktree.Branch, true
			}
		}
	}
	return "", false
}

// repoAgentRequest asks the agent running in the workspace of a stopped
// worktree to resolve its conflicts.
func repoAgentRequest(sid, path, operation string) map[string]string {
	name := ""
	sm.mu.RLock()
	if state := sm.states[sid]; state != nil {
		for _, project := range state.Projects {
			if project.Path == path {
				name = project.Name
			}
		}
		for _, thread := range state.Threads {
			if thread.Path == path {
				name = thread.ID
			}
		}
	}
	sm.mu.RUnlock()
	id := ""
	for _, workspace := range sm.GetAllRunningApps(sid) {
		for _, panel := range workspace.Apps {
			if workspace.Name == name && id == "" && isAgentApp(panel) && panel.TerminalReady && tm.IsRunning(panel.ID) {
				id = panel.ID
			}
		}
	}
	prompt := fmt.Sprintf("The user asked you to resolve the conflicts of the stopped %s in %q. Inspect its Git state and resolve every conflicted file, keeping the intent of both sides. Then git add the resolved files and finish it: git commit --no-edit for a merge, or GIT_EDITOR=true git rebase --continue for a rebase, resolving again if later commits conflict. Do not abort it, discard other changes or force push. Report what you resolved.", operation, path)
	return map[string]string{"appID": id, "workspace": name, "prompt": prompt}
}

// changeRepo runs one panel action in a worktree of a listed project.
func changeRepo(projects []Project, data repoRequest) error {
	checkedOut, listed := repoWorktreeBranch(projects, data.Worktree)
	if !listed {
		return errors.New("folder not found")
	}
	path := data.Worktree
	// Paths are file names, never patterns; editors must not wait for input.
	env := append(repoRemoteEnv(path), "GIT_LITERAL_PATHSPECS=1", "GIT_EDITOR=true")
	git := func(args ...string) error {
		_, err := repoGitEnv(2*time.Minute, path, env, args...)
		return err
	}
	read := func(args ...string) string {
		out, _ := repoGit(5*time.Second, path, args...)
		return strings.TrimSpace(out)
	}
	ref := data.Branch
	if data.Change == "merge" || data.Change == "rebase" {
		ref = data.Onto
	}
	if (data.Change == "push" || data.Change == "pull" || ref != "") && (ref == "" || strings.HasPrefix(ref, "-") || read("rev-parse", "--verify", "--quiet", ref+"^{commit}") == "") {
		return errors.New("branch not found")
	}
	switch data.Change {
	case "commit", "discard":
		return changeRepoFiles(git, path, data.Change, data.Paths, data.All, data.Message)
	case "push":
		remote, merge := read("config", "--get", "branch."+ref+".remote"), read("config", "--get", "branch."+ref+".merge")
		if remote != "" && merge != "" {
			return git("push", "--quiet", remote, "refs/heads/"+ref+":"+merge)
		}
		remotes := strings.Fields(read("remote"))
		if len(remotes) == 0 {
			return errors.New("no remote")
		}
		if slices.Contains(remotes, "origin") {
			remotes[0] = "origin"
		}
		return git("push", "--quiet", "-u", remotes[0], "refs/heads/"+ref)
	case "pull":
		if ref == checkedOut {
			return git("pull", "--quiet", "--no-edit")
		}
		// A branch no worktree has checked out can only move forward.
		remote, merge := read("config", "--get", "branch."+ref+".remote"), read("config", "--get", "branch."+ref+".merge")
		if remote == "" || merge == "" {
			return errors.New("branch has no upstream")
		}
		return git("fetch", "--quiet", remote, merge+":refs/heads/"+ref)
	case "merge":
		return git("merge", "--quiet", "--no-edit", ref)
	case "rebase":
		return git("rebase", "--quiet", ref)
	case "continue", "abort":
		operation := repoOperation(path)
		switch {
		case operation == "":
			return errRepoNoOperation
		case data.Change == "abort":
			return git(operation, "--abort")
		}
		// Resolved conflict files count as resolved; new files stay out.
		if err := git("add", "-u"); err != nil {
			return err
		}
		if operation == "merge" {
			return git("commit", "--quiet", "--no-edit")
		}
		return git("rebase", "--continue")
	}
	return errors.New("unknown action")
}

// changeRepoFiles commits or discards the listed changed files, or all of them.
func changeRepoFiles(git func(...string) error, path, change string, paths []string, all bool, message string) error {
	if all && change == "commit" {
		if err := git("add", "-A"); err != nil {
			return err
		}
		return git("commit", "--quiet", "-m", message)
	}
	if all {
		if err := git("reset", "--hard", "--quiet"); err != nil {
			return err
		}
		return git("clean", "-f", "-d", "--quiet")
	}
	entries, err := worktreeEntries(path)
	if err != nil {
		return err
	}
	var tracked, untracked, unstaged []string
	for _, entry := range entries {
		if entry.Unstaged && slices.Contains(paths, entry.Path) {
			unstaged = append(unstaged, entry.Path)
		}
		switch {
		case !slices.Contains(paths, entry.Path):
		case entry.Code == "??":
			untracked = append(untracked, entry.Path)
		case entry.Orig != "":
			tracked = append(tracked, entry.Path, entry.Orig)
		default:
			tracked = append(tracked, entry.Path)
		}
	}
	if len(tracked)+len(untracked) == 0 {
		return errRepoChangesGone
	}
	if change == "commit" {
		// Adding a path that is already staged as gone fails.
		if len(unstaged) > 0 {
			if err := git(append([]string{"add", "-A", "--"}, unstaged...)...); err != nil {
				return err
			}
		}
		return git(append([]string{"commit", "--quiet", "-m", message, "--"}, append(tracked, untracked...)...)...)
	}
	if len(tracked) > 0 {
		if err := git(append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, tracked...)...); err != nil {
			return err
		}
	}
	if len(untracked) > 0 {
		return git(append([]string{"clean", "-f", "-d", "--quiet", "--"}, untracked...)...)
	}
	return nil
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
			result[i].Label = project.displayName()
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
			if data.Fetch || data.Change == "push" || data.Change == "pull" {
				repoFetchMu.Lock()
				defer repoFetchMu.Unlock()
			}
			data.Message = strings.TrimSpace(data.Message)
			switch {
			case data.Change == "commit" && data.Message == "":
				result["changeError"] = "Write a commit message"
			case data.Change == "agent":
				_, listed := repoWorktreeBranch(projects, data.Worktree)
				if operation := repoOperation(data.Worktree); listed && operation != "" {
					result["agent"] = repoAgentRequest(data.SID, data.Worktree, operation)
				} else {
					result["changeError"] = "Could not ask the agent: " + errRepoNoOperation.Error()
				}
			case data.Change != "":
				if err := changeRepo(projects, data); err != nil {
					result["changeError"] = "Could not " + data.Change + ": " + err.Error()
				}
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

const reposHelp = `Refresh the Libro Repositories panel so it shows current Git status.
Call it after you change Git state: commit, push, pull, merge, rebase, or create, switch or delete a branch.
It reads local status only and does nothing when no Repositories panel is open.
CLI: libro repositories.
`

func reposTool() map[string]any {
	return map[string]any{"name": "repositories", "description": reposHelp, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}
}

// RunReposCLI refreshes Repositories for agents without MCP support.
func RunReposCLI(args []string, out io.Writer) error {
	if len(args) > 0 {
		_, err := io.WriteString(out, reposHelp)
		return err
	}
	result, err := agentControlCommand(json.RawMessage(`{"action":"repositories"}`))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}
