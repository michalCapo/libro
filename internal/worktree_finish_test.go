package libro

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"libro/internal/components"
)

func finishFixture(t *testing.T) (*AppState, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		if _, err := worktreeGit(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	git("config", "commit.gpgsign", "false")
	writeFinishFile(t, root, "file.txt", "original\n")
	git("add", ".")
	git("commit", "-m", "initial")
	branch := root + "-feature"
	if err := GitCreateWorktree(root, "feature", branch); err != nil {
		t.Fatal(err)
	}
	state := &AppState{ActiveProject: "repo/feature", Projects: []Project{{Name: "repo", Path: root, IsGitRepo: true}, {Name: "repo/feature", Path: branch, Virtual: true, ParentProject: "repo"}}}
	return state, root, branch
}

func writeFinishFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func commitFinishFile(t *testing.T, root, name, content string) {
	t.Helper()
	writeFinishFile(t, root, name, content)
	if _, err := worktreeGit(root, "add", "--", name); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreeGit(root, "commit", "-m", name); err != nil {
		t.Fatal(err)
	}
}

func TestFinishThreadMergeAndSquash(t *testing.T) {
	for _, method := range []string{"merge", "squash"} {
		t.Run(method, func(t *testing.T) {
			state, root, branch := finishFixture(t)
			commitFinishFile(t, branch, "one.txt", "one\n")
			commitFinishFile(t, branch, "two.txt", "two\n")
			info, err := inspectFinishThread(state, "repo/feature", "")
			if err != nil || info.Base != "main" || info.TargetPath != root || info.Blocker != "" || !strings.Contains(info.Summary, "one.txt") {
				t.Fatalf("preview: %+v %v", info, err)
			}
			if err := integrateFinishThread(info, method, "Combined change"); err != nil {
				t.Fatal(err)
			}
			if method == "squash" {
				count, err := worktreeGit(root, "rev-list", "--count", info.TargetHead+"..HEAD")
				if err != nil || count != "1" {
					t.Fatalf("squash created %q commits: %v", count, err)
				}
			}
			warning, err := removeFinishedWorktree(info, false)
			if err != nil || warning != "" {
				t.Fatalf("cleanup: %s %v", warning, err)
			}
			if _, err := os.Stat(branch); !os.IsNotExist(err) {
				t.Fatal("worktree remains")
			}
			if _, err := worktreeGit(root, "rev-parse", "--verify", "refs/heads/feature"); err == nil {
				t.Fatal("thread branch remains")
			}
			for _, name := range []string{"one.txt", "two.txt"} {
				if _, err := os.Stat(filepath.Join(root, name)); err != nil {
					t.Fatal("merged file missing", err)
				}
			}
		})
	}
}

func TestFinishThreadDirtyAndConflictPreserveWorktree(t *testing.T) {
	for _, kind := range []string{"source-dirty", "target-dirty", "conflict", "stale"} {
		t.Run(kind, func(t *testing.T) {
			state, root, branch := finishFixture(t)
			commitFinishFile(t, branch, "file.txt", "feature\n")
			if kind == "source-dirty" {
				writeFinishFile(t, branch, "untracked.txt", "unsaved")
			}
			if kind == "target-dirty" {
				writeFinishFile(t, root, "untracked.txt", "unsaved")
			}
			if kind == "conflict" {
				commitFinishFile(t, root, "file.txt", "main\n")
			}
			info, err := inspectFinishThread(state, "repo/feature", "")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "stale" {
				commitFinishFile(t, branch, "later.txt", "later\n")
			}
			if err := integrateFinishThread(info, "merge", ""); err == nil {
				t.Fatal("unsafe merge accepted")
			}
			if _, err := os.Stat(branch); err != nil {
				t.Fatal("thread removed on failure")
			}
			if kind == "conflict" {
				if _, err := worktreeGit(root, "rev-parse", "--verify", "MERGE_HEAD"); err != nil {
					t.Fatal("conflict was silently reset")
				}
			}
		})
	}
}

func TestFinishThreadDefaultsExistingWorktreeToBaseBranch(t *testing.T) {
	state, root, _ := finishFixture(t)
	if _, err := worktreeGit(root, "config", "--unset", "branch.feature.libro-base"); err != nil {
		t.Fatal(err)
	}
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil || info.Base != "main" || info.Blocker != "" {
		t.Fatal("existing thread did not default to Base branch")
	}
	info, err = inspectFinishThread(state, "repo/feature", "main")
	if err != nil || info.Blocker != "" {
		t.Fatal("explicit destination rejected")
	}
	if _, err := inspectFinishThread(state, "repo", "main"); err == nil {
		t.Fatal("base project could be removed")
	}
}

func TestFinishThreadCleanupRejectsNewChangesAndDiscardRemovesThem(t *testing.T) {
	state, _, branch := finishFixture(t)
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil {
		t.Fatal(err)
	}
	writeFinishFile(t, branch, "untracked.txt", "unsaved")
	if _, err := removeFinishedWorktree(info, false); err == nil {
		t.Fatal("cleanup removed new uncommitted files")
	}
	if _, err := removeFinishedWorktree(info, true); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreeGit(info.Root, "rev-parse", "--verify", "refs/heads/feature"); err == nil {
		t.Fatal("discard kept the thread branch")
	}
}

func TestFinishThreadCleanupPreservesSharedApplication(t *testing.T) {
	state, root, branch := finishFixture(t)
	oldSM, oldTM := sm, tm
	sm, tm = NewStateManager(), components.NewTerminalManager()
	t.Cleanup(func() { tm.StopAll(); sm, tm = oldSM, oldTM })
	sm.states["finish"] = state
	state.Apps = []Application{{ID: "shared", PluginID: "project-command", Dock: "bottom", ApplicationPath: root}, {ID: "local", PluginID: "codex", Dock: "center"}}
	commitFinishFile(t, branch, "done.txt", "done\n")
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := integrateFinishThread(info, "merge", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cleanupFinishedThread("finish", info, false); err != nil {
		t.Fatal(err)
	}
	if state.ActiveProject != "repo" || len(state.Projects) != 1 || len(state.Apps) != 1 || state.Apps[0].ID != "shared" {
		t.Fatalf("cleanup lost shared process or kept thread: %+v", state)
	}
}

func TestFinishPRRepositoryUsesOriginHostAndOwner(t *testing.T) {
	for _, remote := range []string{"git@github.com:owner/repo.git", "ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo.git"} {
		got, err := finishPRRepository(remote)
		if err != nil || got != "github.com/owner/repo" {
			t.Fatalf("origin %q: %q %v", remote, got, err)
		}
	}
	for _, remote := range []string{"/local/repo", "file:///local/repo", "https://github.com/"} {
		if _, err := finishPRRepository(remote); err == nil {
			t.Fatal("accepted unsupported origin")
		}
	}
}

func TestFinishThreadFailedSquashCommitKeepsStagedChanges(t *testing.T) {
	state, root, branch := finishFixture(t)
	commitFinishFile(t, branch, "change.txt", "change\n")
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := integrateFinishThread(info, "squash", "Combined change"); err == nil {
		t.Fatal("failed commit reported success")
	}
	head, err := worktreeGit(root, "rev-parse", "HEAD")
	if err != nil || head != info.TargetHead {
		t.Fatal("failed commit changed destination HEAD")
	}
	staged, err := worktreeGit(root, "diff", "--cached", "--name-only")
	if err != nil || !strings.Contains(staged, "change.txt") {
		t.Fatal("failed commit lost staged changes")
	}
	if _, err := os.Stat(branch); err != nil {
		t.Fatal("failed commit removed worktree")
	}
}

func TestFinishThreadLockedWorktreeSurvivesCleanupFailure(t *testing.T) {
	state, root, branch := finishFixture(t)
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktreeGit(root, "worktree", "lock", branch); err != nil {
		t.Fatal(err)
	}
	if _, err := removeFinishedWorktree(info, false); err == nil {
		t.Fatal("locked worktree was removed")
	}
	if _, err := os.Stat(branch); err != nil {
		t.Fatal("cleanup failure removed worktree")
	}
	if _, err := worktreeGit(root, "rev-parse", "--verify", "refs/heads/feature"); err != nil {
		t.Fatal("cleanup failure deleted branch")
	}
}

func TestFinishPRPushesReviewedCommitAndKeepsThread(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX command stubs")
	}
	dir := t.TempDir()
	gitLog, ghLog := filepath.Join(dir, "git-args"), filepath.Join(dir, "gh-args")
	t.Setenv("LIBRO_TEST_GIT_ARGS", gitLog)
	t.Setenv("LIBRO_TEST_GH_ARGS", ghLog)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	gitStub := `#!/bin/sh
case "$3" in
 status) exit 0;;
 rev-parse) printf '%s\n' '/nonexistent-libro-test-operation';;
 remote) printf '%s\n' 'git@github.com:owner/repo.git';;
 push) printf '%s\n' "$@" > "$LIBRO_TEST_GIT_ARGS";;
 *) exit 1;;
esac
`
	ghStub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LIBRO_TEST_GH_ARGS\"\nprintf '%s\\n' 'https://github.com/owner/repo/pull/1'\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(gitStub), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(ghStub), 0755); err != nil {
		t.Fatal(err)
	}
	writeFinishFile(t, dir, "keep.txt", "keep")
	info := worktreeFinishPreview{Path: dir, Base: "main", Branch: "feature", SourceHead: "reviewed-commit", TargetHead: "target"}
	url, err := createFinishPR(info, "Review this", "Description")
	if err != nil || url != "https://github.com/owner/repo/pull/1" {
		t.Fatalf("PR result: %q %v", url, err)
	}
	args, err := os.ReadFile(gitLog)
	if err != nil || !strings.Contains(string(args), "origin\nreviewed-commit:refs/heads/feature") {
		t.Fatal("push did not use the reviewed commit")
	}
	args, err = os.ReadFile(ghLog)
	if err != nil || !strings.Contains(string(args), "--repo\ngithub.com/owner/repo\n--draft\n--base\nmain\n--head\nfeature") {
		t.Fatal("PR did not use the same origin and reviewed branches")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatal("PR removed thread files")
	}
}

func TestFinishThreadRecordedSourceTakesPrecedence(t *testing.T) {
	state, root, _ := finishFixture(t)
	if _, err := worktreeGit(root, "checkout", "-b", "other"); err != nil {
		t.Fatal(err)
	}
	info, err := inspectFinishThread(state, "repo/feature", "")
	if err != nil || info.Base != "main" {
		t.Fatal("original branch was replaced by Base's current branch")
	}
	info, err = inspectFinishThread(state, "repo/feature", "other")
	if err != nil || info.Base != "other" {
		t.Fatal("explicit destination was ignored")
	}
}
