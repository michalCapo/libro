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
