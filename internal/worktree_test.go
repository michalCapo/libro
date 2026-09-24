package libro

import (
	"os/exec"
	"path/filepath"
	"testing"
)

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
	if got := manager.GetProjectPath("test", "live/nisa/feature/test"); got != worktree {
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
	firstPath := manager.GetProjectPath("test", first)
	git(firstPath, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "worktree only")
	second, err := manager.createProjectWorktree("test", first, "second")
	if err != nil {
		t.Fatal(err)
	}
	if got := git(manager.GetProjectPath("test", second), "rev-parse", "HEAD"); got != base {
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
