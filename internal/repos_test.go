package libro

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRepoStatusReportsChangesAndBranchSync(t *testing.T) {
	_, root, branch := finishFixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	other := filepath.Join(t.TempDir(), "other")
	for _, args := range [][]string{{"init", "--bare", remote}, {"-C", root, "remote", "add", "origin", remote}, {"-C", root, "push", "-u", "origin", "main"}, {"clone", remote, other}} {
		if _, err := worktreeGit(".", args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "commit.gpgsign", "false"}} {
		if _, err := worktreeGit(other, args...); err != nil {
			t.Fatal(err)
		}
	}
	commitFinishFile(t, other, "remote.txt", "remote\n")
	if _, err := worktreeGit(other, "push"); err != nil {
		t.Fatal(err)
	}
	commitFinishFile(t, root, "local.txt", "local\n")
	commitFinishFile(t, branch, "thread.txt", "thread\n")
	writeFinishFile(t, branch, "draft.txt", "draft\n")
	// A tag with the branch's name must not hide its unpushed commits.
	if _, err := worktreeGit(root, "tag", "feature", "main"); err != nil {
		t.Fatal(err)
	}

	status := readRepoStatus("repo", root, true)
	if status.Error != "" || status.FetchError != "" || !status.Remote || status.Fetched == "" {
		t.Fatalf("status: %+v", status)
	}
	branches := map[string]repoBranch{}
	for _, b := range status.Branches {
		branches[b.Name] = b
	}
	if main := branches["main"]; main.Upstream != "origin/main" || main.Ahead != 1 || main.Behind != 1 {
		t.Fatalf("main: %+v", main)
	}
	if feature := branches["feature"]; feature.Upstream != "" || feature.Ahead != 1 {
		t.Fatalf("unpushed thread branch: %+v", feature)
	}
	changes := map[string][]repoChange{}
	for _, worktree := range status.Worktrees {
		changes[worktree.Branch] = worktree.Changes
	}
	if len(changes["main"]) != 0 || len(changes["feature"]) != 1 || changes["feature"][0] != (repoChange{Code: "??", Path: "draft.txt", Unstaged: true}) {
		t.Fatalf("changes: %+v", changes)
	}
}

func TestChangeRepoCommitsDiscardsAndMovesBranches(t *testing.T) {
	_, root, branch := finishFixture(t)
	projects := []Project{{Name: "repo", Path: root, IsGitRepo: true}}
	run := func(data repoRequest) {
		t.Helper()
		if err := changeRepo(projects, data); err != nil {
			t.Fatalf("%s: %v", data.Change, err)
		}
	}
	git := func(path string, args ...string) string {
		t.Helper()
		out, err := worktreeGit(path, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}

	// Only the checked files are committed, with both sides of a rename.
	writeFinishFile(t, root, "keep.txt", "keep\n")
	writeFinishFile(t, root, "new.txt", "new\n")
	git(root, "mv", "file.txt", "moved.txt")
	run(repoRequest{Change: "commit", Worktree: root, Paths: []string{"moved.txt", "new.txt"}, Message: "pick"})
	if status := git(root, "status", "--porcelain"); status != "?? keep.txt" {
		t.Fatalf("after commit: %q", status)
	}
	writeFinishFile(t, root, "moved.txt", "edited\n")
	run(repoRequest{Change: "discard", Worktree: root, Paths: []string{"moved.txt", "keep.txt"}})
	if status, content := git(root, "status", "--porcelain"), git(root, "show", "HEAD:moved.txt"); status != "" || content != "original" {
		t.Fatalf("after discard: %q %q", status, content)
	}

	// A conflicting merge stops, can be aborted, and continues once resolved.
	commitFinishFile(t, root, "conflict.txt", "main\n")
	commitFinishFile(t, branch, "conflict.txt", "feature\n")
	merge := repoRequest{Change: "merge", Worktree: branch, Onto: "main"}
	if err := changeRepo(projects, merge); err == nil || repoOperation(branch) != "merge" {
		t.Fatalf("merge: %v %q", err, repoOperation(branch))
	}
	run(repoRequest{Change: "abort", Worktree: branch})
	if err := changeRepo(projects, merge); err == nil {
		t.Fatal("merge did not conflict again")
	}
	writeFinishFile(t, branch, "conflict.txt", "both\n")
	run(repoRequest{Change: "continue", Worktree: branch})
	if parents := strings.Fields(git(branch, "log", "-1", "--format=%P")); repoOperation(branch) != "" || len(parents) != 2 {
		t.Fatalf("continue: %q %v", repoOperation(branch), parents)
	}

	// A first push sets the upstream.
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(".", "init", "--bare", remote)
	git(root, "remote", "add", "origin", remote)
	run(repoRequest{Change: "push", Worktree: branch, Branch: "feature"})
	if upstream := git(branch, "rev-parse", "--abbrev-ref", "feature@{upstream}"); upstream != "origin/feature" {
		t.Fatalf("upstream: %q", upstream)
	}

	for _, data := range []repoRequest{{Change: "merge", Worktree: branch, Onto: "--abort"}, {Change: "commit", Worktree: t.TempDir(), All: true, Message: "x"}} {
		if err := changeRepo(projects, data); err == nil {
			t.Fatalf("%+v was allowed", data)
		}
	}
}
