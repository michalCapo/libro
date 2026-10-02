package libro

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDoesNotInheritParentRepository(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	if _, err := worktreeGit(home, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(home, "new-project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	manager := NewStateManager()
	manager.states["test"] = &AppState{}
	if !manager.AddProjectWithOptions("test", "new-project", project, false) {
		t.Fatal("could not add project")
	}
	state := manager.Get("test")
	if state.Projects[0].IsGitRepo || strings.Contains(projectsJS(state), `"kind":"worktree"`) {
		t.Fatal("new project inherited the home repository")
	}
	if trees, err := GitListWorktrees(project); err != nil || len(trees) != 0 {
		t.Fatalf("parent worktrees leaked: %+v, %v", trees, err)
	}
	restoreWorktreeProject(manager, "test", "new-project/main")
	if len(state.Projects) != 1 {
		t.Fatal("restored a worktree pointing to the home folder")
	}
	if _, err := manager.createProjectWorktree("test", "new-project", "feature"); err == nil {
		t.Fatal("allowed a worktree in the parent repository")
	}
	if _, err := worktreeGit(project, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	if !GitIsRepo(project) {
		t.Fatal("nested repository root was not recognized")
	}
}

func TestProjectRepositoryAliases(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	_, root, branch := finishFixture(t)
	for _, path := range []string{root, branch} {
		if !GitIsRepo(path) {
			t.Fatalf("worktree root not recognized: %s", path)
		}
	}
	alias := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if !GitIsRepo(alias) {
		t.Fatal("repository symlink not recognized")
	}
	state := &AppState{Projects: []Project{{Name: "repo", Path: alias, IsGitRepo: true}}}
	if got := strings.Count(projectsJS(state), `"kind":"worktree"`); got != 1 {
		t.Fatalf("worktree rows = %d, want only the linked worktree", got)
	}
}

func TestNewProjectThreadsStayLast(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	_, repo, _ := finishFixture(t)
	manager := NewStateManager()
	state := &AppState{Projects: []Project{{Name: "repo", Path: repo, IsGitRepo: true}}}
	manager.states["test"] = state
	// Record the existing thread before adding branches whose paths sort earlier.
	projectsJS(state)
	for _, branch := range []string{"aaa-first", "aaa-second"} {
		if _, err := manager.createProjectWorktree("test", "repo", branch); err != nil {
			t.Fatal(err)
		}
		js := projectsJS(state)
		last := strings.LastIndex(js, `"kind":"worktree"`)
		if last < 0 || !strings.Contains(js[last:], `"branch":"`+branch+`"`) {
			t.Fatalf("new thread %s is not the last worktree", branch)
		}
		if got := projectsJS(state); got != js {
			t.Fatal("refresh changed the thread order")
		}
	}
}

func TestRestoreWorktreeWithSlashesInProjectAndBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	worktree := filepath.Join(root, "feature")
	run := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, output)
		}
	}
	run("init", repo)
	run("-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "initial")
	run("-C", repo, "worktree", "add", "-b", "feature/test", worktree)
	manager := NewStateManager()
	manager.states["test"] = &AppState{Projects: []Project{{Name: "live/nisa", Path: repo, IsGitRepo: true}}}
	restoreWorktreeProject(manager, "test", "live/nisa/feature/test")
	if got := projectPath(manager, "live/nisa/feature/test"); got != worktree {
		t.Fatalf("worktree path = %q, want %q", got, worktree)
	}
}

func TestProjectThreadsBranchFromBase(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	git := func(path string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	git(root, "init", repo)
	git(repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "base")
	base := git(repo, "rev-parse", "HEAD")
	manager := NewStateManager()
	manager.states["test"] = &AppState{Projects: []Project{{Name: "repo", Path: repo, IsGitRepo: true}}, snapshots: make(map[string]*projectSnapshot)}
	first, err := manager.createProjectWorktree("test", "repo", "first")
	if err != nil {
		t.Fatal(err)
	}
	firstPath := projectPath(manager, first)
	git(firstPath, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "worktree only")
	second, err := manager.createProjectWorktree("test", first, "second")
	if err != nil {
		t.Fatal(err)
	}
	if got := git(projectPath(manager, second), "rev-parse", "HEAD"); got != base {
		t.Fatal("new thread branched from worktree instead of base")
	}
	if _, err := manager.createProjectWorktree("test", "repo", "first"); err == nil {
		t.Fatal("duplicate branch accepted")
	}
	if !manager.SwitchProject("test", "repo") || manager.GetActiveProjectPath("test") != repo {
		t.Fatal("cannot return to base")
	}
	manager.states["test"].Apps = []Application{{ID: "base-tool", Dock: "right"}}
	if !manager.SwitchProject("test", first) || manager.GetActiveProjectPath("test") != firstPath {
		t.Fatal("cannot reopen worktree")
	}
	manager.states["test"].Apps = []Application{{ID: "worktree-tool", Dock: "right"}}
	manager.SwitchProject("test", "repo")
	if apps := manager.states["test"].Apps; len(apps) != 1 || apps[0].ID != "base-tool" {
		t.Fatal("base tools were not preserved")
	}
	manager.SwitchProject("test", first)
	if apps := manager.states["test"].Apps; len(apps) != 1 || apps[0].ID != "worktree-tool" {
		t.Fatal("worktree tools were not preserved")
	}
}

func TestProjectThreadRequiresCommittedBranch(t *testing.T) {
	root := t.TempDir()
	manager := NewStateManager()
	manager.states["test"] = &AppState{Projects: []Project{{Name: "repo", Path: root}}}
	if _, err := manager.createProjectWorktree("test", "repo", "thread"); err == nil {
		t.Fatal("non-Git directory accepted")
	}
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if _, err := manager.createProjectWorktree("test", "repo", "thread"); err == nil {
		t.Fatal("unborn branch accepted")
	}
	if len(manager.states["test"].Projects) != 1 {
		t.Fatal("failed creation registered a workspace")
	}
}

func projectPath(manager *StateManager, name string) string {
	for _, project := range manager.Get("test").Projects {
		if project.Name == name {
			return project.Path
		}
	}
	return ""
}
