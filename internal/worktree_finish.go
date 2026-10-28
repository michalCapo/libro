package libro

import (
	"context"
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
	// Checkout marks a thread on the project's own checkout; it can only discard changes.
	Checkout bool `json:"checkout"`
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
		for _, project := range state.Projects {
			// Only a repository root, so a discard never reaches files outside the project.
			if !project.Virtual && project.Name == name && GitIsRepo(project.Path) {
				if branch := GitCurrentBranch(project.Path); branch != "" {
					info.Branch, info.Path, info.Root, info.Parent, info.Checkout = branch, project.Path, project.Path, project.Name, true
				}
			}
		}
	}
	if info.Path == "" {
		return info, fmt.Errorf("choose a project thread on a Git branch")
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
	if info.Checkout {
		info.Blocker = "Merge, squash and pull requests need a thread with its own worktree."
		return info, nil
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
	_ = saveWorktreeTitle(info.Path, "")
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
	registerWorkspaceAction(app, "thread.finish.preview", func(_ *r.Context, in actionThreadFinishPreviewInput) (r.Result, error) {
		name := in.Name
		base := in.Base
		request := in.Request
		info, err := inspectFinishThread(sm.Get(inputSID(in.SID)), name, base)
		reply := map[string]any{"info": info}
		if err != nil {
			reply["error"] = err.Error()
		}
		return clientScript("libroWorkspace.finishThreadPreview(props[0],props[1]);", request, reply), nil
	})
	registerWorkspaceAction(app, "thread.finish", func(_ *r.Context, in actionThreadFinishInput) (r.Result, error) {
		worktreeFinishMu.Lock()
		defer worktreeFinishMu.Unlock()
		sid := inputSID(in.SID)
		name := in.Name
		base := in.Base
		method := in.Method
		sourceHead := in.SourceHead
		targetHead := in.TargetHead
		message := in.Message
		body := in.Body
		var agentMerge map[string]string
		finish := func(err error, warning, url string) r.Result {
			reply := map[string]any{"warning": warning, "url": url, "agentMerge": agentMerge}
			if err != nil {
				reply["error"] = err.Error()
			}
			return clientScript("libroWorkspace.finishThreadResult(props[0],props[1]);", name, reply)
		}
		info, err := inspectFinishThread(sm.Get(sid), name, base)
		if err != nil {
			return finish(err, "", ""), nil
		}
		js, warning, url, conflict, err := finishThread(sid, info, method, sourceHead, targetHead, message, body)
		if conflict {
			agentMerge = mergeAgentRequest(sid, info, method)
		}
		return r.Merge(js, finish(err, warning, url)), nil
	})
}

// finishThread runs a reviewed finish. Conflict reports a failed local merge,
// which leaves the thread open. The caller holds worktreeFinishMu.
func finishThread(sid string, info worktreeFinishPreview, method, sourceHead, targetHead, message, body string) (js r.Result, warning, url string, conflict bool, err error) {
	if sourceHead != info.SourceHead || method != "discard" && targetHead != info.TargetHead {
		return js, "", "", false, fmt.Errorf("the branches changed; refresh the preview and review again")
	}
	if info.Checkout {
		if method != "discard" {
			return js, "", "", false, fmt.Errorf("%s", info.Blocker)
		}
		js, err = discardCheckoutThread(sid, info)
		return js, "", "", false, err
	}
	if method == "pr" {
		url, err = createFinishPR(info, message, body)
		return js, "", url, false, err
	}
	if method != "discard" {
		if err = integrateFinishThread(info, method, message); err != nil {
			return js, "", "", true, err
		}
	}
	js, warning, err = cleanupFinishedThread(sid, info, method == "discard")
	return js, warning, "", false, err
}

func cleanupFinishedThread(sid string, info worktreeFinishPreview, discard bool) (r.Result, string, error) {
	name := info.Name
	applicationControlMu.Lock()
	defer applicationControlMu.Unlock()
	// Move shared applications back to Base before stopping this thread's panels.
	restoreWorktreeProject(sm, sid, name)
	js := r.Merge(switchToProjectName(sid, name), switchToProjectName(sid, info.Parent))
	for _, workspace := range sm.GetAllRunningApps(sid) {
		if workspace.Name != name {
			continue
		}
		for _, panel := range workspace.Apps {
			tm.Stop(panel.ID)
			sm.RemoveAppByID(sid, panel.ID)
			js = r.Merge(js, removeAppJS(panel.ID))
		}
	}
	warning, err := removeFinishedWorktree(info, discard)
	if err != nil {
		return js, "", err
	}
	sm.RemoveProject(sid, name)
	js = r.Merge(js, r.Result{}.Run(r.Remove(projectMainID(name))))
	return r.Merge(js, projectsJS(sm.Get(sid))), warning, nil
}

// A checkout thread keeps its branch, commits and ignored files. Discarding
// closes it first so its agent stops writing, then drops uncommitted changes.
func discardCheckoutThread(sid string, info worktreeFinishPreview) (r.Result, error) {
	js, err := closeWorkspace(sid, info.Name)
	if err != nil {
		return js, err
	}
	head, err := worktreeGit(info.Path, "rev-parse", "HEAD")
	if err != nil {
		return js, err
	}
	if head != info.SourceHead || GitCurrentBranch(info.Path) != info.Branch {
		return js, fmt.Errorf("the thread changed while closing; nothing was discarded")
	}
	// Clean before reset, so ignore rules that are not committed yet still protect their files.
	if _, err := worktreeGit(info.Path, "clean", "-fd"); err != nil {
		return js, err
	}
	if _, err := worktreeGit(info.Path, "reset", "--hard", info.SourceHead); err != nil {
		return js, err
	}
	return js, nil
}

// Only the failed thread's agent may receive this request.
func mergeAgentRequest(sid string, info worktreeFinishPreview, method string) map[string]string {
	id := ""
	for _, workspace := range sm.GetAllRunningApps(sid) {
		if workspace.Name != info.Name {
			continue
		}
		for _, panel := range workspace.Apps {
			if isAgentApp(panel) && panel.TerminalReady && tm.IsRunning(panel.ID) {
				id = panel.ID
				break
			}
		}
	}
	// Conflicts are resolved in the thread's worktree so the destination checkout stays clean.
	prompt := fmt.Sprintf("The user confirmed that you should fix a failed %s. Source branch: %q in %q. Destination branch: %q in %q. Inspect the Git state of both checkouts first. If the failed %s left changes in the destination checkout, undo only that unfinished merge there (git merge --abort, or git reset --merge after a squash) and make no other changes there. Then, in the source worktree, merge %q into %q, resolve the conflicts, and commit. Do not discard other changes or force push. Do not delete either branch or worktree, and do not stop shared applications. Report the result so the user can finish the thread again in Libro.", method, info.Branch, info.Path, info.Base, info.TargetPath, method, info.Base, info.Branch)
	return map[string]string{"appID": id, "workspace": info.Name, "prompt": prompt}
}
