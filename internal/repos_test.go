package libro

import (
	"path/filepath"
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
	if len(changes["main"]) != 0 || len(changes["feature"]) != 1 || changes["feature"][0] != (repoChange{Code: "??", Path: "draft.txt"}) {
		t.Fatalf("changes: %+v", changes)
	}
}
