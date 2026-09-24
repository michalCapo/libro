package libro

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

var worktreeFinishMu sync.Mutex

func worktreeGit(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s (%w)", args[0], strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

type worktreeFinishPreview struct {
	Name       string   `json:"name"`
	Branch     string   `json:"branch"`
	Base       string   `json:"base"`
	Branches   []string `json:"branches"`
	SourceHead string   `json:"sourceHead"`
	TargetHead string   `json:"targetHead"`
	TargetPath string   `json:"targetPath"`
	Summary    string   `json:"summary"`
	Dirty      string   `json:"dirty"`
	Blocker    string   `json:"blocker"`
	Path       string   `json:"-"`
	Root       string   `json:"-"`
	Parent     string   `json:"-"`
}

func inspectFinishThread(state *AppState, name, base string) (worktreeFinishPreview, error) {
	info := worktreeFinishPreview{Name: name}
	// Resolve from Git's worktree list, never from a client-supplied disk path.
	for _, project := range state.Projects {
		if project.Virtual {
			continue
		}
		trees, err := GitListWorktrees(project.Path)
		if err != nil {
			continue
		}
		for _, tree := range trees {
			if tree.IsBare || tree.Branch == "(detached)" || applicationPath(tree.Path) == applicationPath(project.Path) || project.Name+"/"+tree.Branch != name {
				continue
			}
			info.Branch, info.Path, info.Root, info.Parent = tree.Branch, tree.Path, project.Path, project.Name
		}
	}
	if info.Path == "" {
		return info, fmt.Errorf("choose a project worktree thread; the base project cannot be removed")
	}
	var err error
	info.SourceHead, err = worktreeGit(info.Path, "rev-parse", "HEAD")
	if err != nil {
		return info, err
	}
	info.Dirty, err = worktreeGit(info.Path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return info, err
	}
	info.Branches, err = GitListBranches(info.Root)
	if err != nil {
		return info, err
	}
	info.Branches = slices.DeleteFunc(info.Branches, func(branch string) bool { return branch == info.Branch })
	if base == "" {
		base, _ = worktreeGit(info.Root, "config", "--get", "branch."+info.Branch+".libro-base")
		if base == "" {
			// Older threads predate source-branch tracking; default to Base's branch.
			base = GitCurrentBranch(info.Root)
		}
	}
	info.Base = base
	if base == "" {
		info.Blocker = "Choose a destination branch. This worktree has no recorded source branch."
		return info, nil
	}
	if !slices.Contains(info.Branches, base) {
		info.Blocker = "The destination branch no longer exists. Choose another branch."
		return info, nil
	}
	info.TargetHead, err = worktreeGit(info.Root, "rev-parse", "refs/heads/"+base)
	if err != nil {
		return info, err
	}
	trees, err := GitListWorktrees(info.Root)
	if err != nil {
		return info, err
	}
	for _, tree := range trees {
		if tree.Branch == base {
			info.TargetPath = tree.Path
			break
		}
	}
	stats, err := worktreeGit(info.Root, "diff", "--stat", info.TargetHead+"..."+info.SourceHead, "--")
	if err != nil {
		return info, err
	}
	commits, err := worktreeGit(info.Root, "log", "-20", "--oneline", info.TargetHead+".."+info.SourceHead, "--")
	if err != nil {
		return info, err
	}
	info.Summary = strings.TrimSpace(commits + "\n\n" + stats)
	if len(info.Summary) > 16000 {
		info.Summary = info.Summary[:16000] + "\n…"
	}
	if info.Dirty != "" {
		info.Blocker = "Commit or stash this thread's changes before finishing."
	} else if info.TargetPath == "" {
		info.Blocker = "Check out the destination branch in a separate worktree before a local merge."
	} else if err := cleanFinishTarget(info.TargetPath); err != nil {
		info.Blocker = err.Error()
	}
	return info, nil
}

func cleanFinishTarget(path string) error {
	status, err := worktreeGit(path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("the worktree at %s has uncommitted changes; commit or stash them first", path)
	}
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		location, err := worktreeGit(path, "rev-parse", "--git-path", marker)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(location) {
			location = filepath.Join(path, location)
		}
		if _, err := os.Stat(location); err == nil {
			return fmt.Errorf("finish or abort the Git operation in %s first", path)
		}
	}
	return nil
}

// Local integration never resets a conflicted checkout or forces worktree removal.
func integrateFinishThread(info worktreeFinishPreview, method, message string) error {
	if info.Blocker != "" {
		return fmt.Errorf("%s", info.Blocker)
	}
	if method != "merge" && method != "squash" {
		return fmt.Errorf("unknown merge method")
	}
	head, err := worktreeGit(info.TargetPath, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	source, err := worktreeGit(info.Path, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != info.TargetHead || source != info.SourceHead || GitCurrentBranch(info.TargetPath) != info.Base {
		return fmt.Errorf("the branches changed; refresh the preview")
	}
	if err := cleanFinishTarget(info.TargetPath); err != nil {
		return err
	}
	if err := cleanFinishTarget(info.Path); err != nil {
		return err
	}
	args := []string{"merge", "--no-edit", "--no-autostash"}
	if method == "squash" {
		args = append(args, "--squash")
	}
	args = append(args, "--", info.SourceHead)
	if _, err := worktreeGit(info.TargetPath, args...); err != nil {
		return fmt.Errorf("merge did not finish in %s. Resolve or abort it there; the thread was kept. %w", info.TargetPath, err)
	}
	if method == "squash" {
		changes, err := worktreeGit(info.TargetPath, "diff", "--cached", "--name-only")
		if err != nil {
			return err
		}
		if changes != "" {
			if strings.TrimSpace(message) == "" {
				message = "Squash " + info.Branch
			}
			if _, err := worktreeGit(info.TargetPath, "commit", "-m", message); err != nil {
				return fmt.Errorf("squash changes remain staged in %s; the thread was kept. %w", info.TargetPath, err)
			}
		}
	}
	return nil
}

func removeFinishedWorktree(info worktreeFinishPreview, discard bool) (string, error) {
	head, err := worktreeGit(info.Path, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if head != info.SourceHead {
		return "", fmt.Errorf("the thread changed while finishing; review it again before cleanup")
	}
	if !discard {
		status, err := worktreeGit(info.Path, "status", "--porcelain=v1", "--untracked-files=all")
		if err != nil {
			return "", err
		}
		if status != "" {
			return "", fmt.Errorf("the thread gained uncommitted changes; it was kept")
		}
	}
	args := []string{"worktree", "remove"}
	if discard {
		args = append(args, "--force")
	}
	args = append(args, "--", info.Path)
	if _, err := worktreeGit(info.Root, args...); err != nil {
		return "", fmt.Errorf("worktree cleanup failed; the thread was kept: %w", err)
	}
	trees, err := GitListWorktrees(info.Root)
	if err != nil {
		return "Worktree removed; branch kept because its worktree status could not be checked.", nil
	}
	for _, tree := range trees {
		if tree.Branch == info.Branch {
			return "Worktree removed; branch kept because another worktree uses it.", nil
		}
	}
	// Compare-and-delete only the reviewed commit, including after a squash.
	if _, err := worktreeGit(info.Root, "update-ref", "-d", "refs/heads/"+info.Branch, info.SourceHead); err != nil {
		return "Worktree removed, but branch deletion failed: " + err.Error(), nil
	}
	_, _ = worktreeGit(info.Root, "config", "--remove-section", "branch."+info.Branch)
	return "", nil
}

func createFinishPR(info worktreeFinishPreview, title, body string) (string, error) {
	if info.Dirty != "" || info.TargetHead == "" {
		return "", fmt.Errorf("commit this thread's changes and choose a destination branch first")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("install GitHub CLI (gh) and run gh auth login to create a draft PR")
	}
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("enter a pull request title")
	}
	if err := cleanFinishTarget(info.Path); err != nil {
		return "", err
	}
	remote, err := worktreeGit(info.Path, "remote", "get-url", "--push", "origin")
	if err != nil {
		return "", err
	}
	repository, err := finishPRRepository(remote)
	if err != nil {
		return "", err
	}
	if _, err := worktreeGit(info.Path, "push", "origin", info.SourceHead+":refs/heads/"+info.Branch); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--repo", repository, "--draft", "--base", info.Base, "--head", info.Branch, "--title", title, "--body", body)
	cmd.Dir = info.Path
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("branch pushed, but PR creation failed; the thread was kept: %s", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// Pin PR creation to the same origin that received the push, even in forks.
func finishPRRepository(remote string) (string, error) {
	if !strings.Contains(remote, "://") && strings.Contains(remote, "@") {
		remote = "ssh://" + strings.Replace(remote, ":", "/", 1)
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh") {
		return "", fmt.Errorf("origin must be a GitHub HTTPS or SSH remote to create a PR")
	}
	path := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	if len(strings.Split(path, "/")) != 2 {
		return "", fmt.Errorf("origin must identify a GitHub owner and repository")
	}
	return parsed.Host + "/" + path, nil
}

func registerFinishThreadActions(app *r.App) {
	registerAction(app, "thread.finish.preview", func(ctx *r.Context) string {
		name, _ := ctx.WsData()["name"].(string)
		base, _ := ctx.WsData()["base"].(string)
		request, _ := ctx.WsData()["request"].(string)
		info, err := inspectFinishThread(sm.Get(extractSID(ctx)), name, base)
		reply := map[string]any{"info": info}
		if err != nil {
			reply["error"] = err.Error()
		}
		encoded, _ := json.Marshal(reply)
		return fmt.Sprintf("libroWorkspace.finishThreadPreview(%s,%s);", components.JSString(request), encoded)
	})
	registerAction(app, "thread.finish", func(ctx *r.Context) string {
		worktreeFinishMu.Lock()
		defer worktreeFinishMu.Unlock()
		sid := extractSID(ctx)
		data := ctx.WsData()
		name, _ := data["name"].(string)
		base, _ := data["base"].(string)
		method, _ := data["method"].(string)
		sourceHead, _ := data["sourceHead"].(string)
		targetHead, _ := data["targetHead"].(string)
		message, _ := data["message"].(string)
		body, _ := data["body"].(string)
		finish := func(err error, warning, url string) string {
			reply := map[string]any{"warning": warning, "url": url}
			if err != nil {
				reply["error"] = err.Error()
			}
			encoded, _ := json.Marshal(reply)
			return fmt.Sprintf("libroWorkspace.finishThreadResult(%s,%s);", components.JSString(name), encoded)
		}
		info, err := inspectFinishThread(sm.Get(sid), name, base)
		if err != nil {
			return finish(err, "", "")
		}
		if sourceHead != info.SourceHead || method != "discard" && targetHead != info.TargetHead {
			return finish(fmt.Errorf("the branches changed; refresh the preview and review again"), "", "")
		}
		if method == "pr" {
			url, err := createFinishPR(info, message, body)
			return finish(err, "", url)
		}
		if method != "discard" {
			if err := integrateFinishThread(info, method, message); err != nil {
				return finish(err, "", "")
			}
		}
		js, warning, err := cleanupFinishedThread(sid, info, method == "discard")
		return js + finish(err, warning, "")
	})
}

func cleanupFinishedThread(sid string, info worktreeFinishPreview, discard bool) (string, string, error) {
	name := info.Name
	applicationControlMu.Lock()
	defer applicationControlMu.Unlock()
	// Move shared applications back to Base before stopping this thread's panels.
	restoreWorktreeProject(sm, sid, name)
	js := switchToProjectName(sid, name) + switchToProjectName(sid, info.Parent)
	for _, workspace := range sm.GetAllRunningApps(sid) {
		if workspace.Name != name {
			continue
		}
		for _, panel := range workspace.Apps {
			tm.Stop(panel.ID)
			sm.RemoveAppByID(sid, panel.ID)
			js += removeAppJS(panel.ID)
		}
	}
	warning, err := removeFinishedWorktree(info, discard)
	if err != nil {
		return js, "", err
	}
	sm.RemoveProject(sid, name)
	js += fmt.Sprintf("document.getElementById(%s)?.remove();", components.JSString(projectMainID(name)))
	return js + projectsJS(sm.Get(sid)), warning, nil
}
