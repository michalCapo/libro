package libro

import (
	"context"
	"slices"
	"testing"
)

func TestProjectListKeepsSavedGitInfoUntilRefresh(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "first"}} {
		if _, err := worktreeGit(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	state := &AppState{Projects: []Project{{Name: "repo", Path: repo, IsGitRepo: true}}}
	branches := func() []string { return projectItemsWith(state, cachedProjectGitInfo)[0].Branches }

	if got := branches(); !slices.Equal(got, []string{"main"}) {
		t.Fatalf("first read = %v", got)
	}
	if _, err := worktreeGit(repo, "branch", "added"); err != nil {
		t.Fatal(err)
	}
	if got := branches(); !slices.Equal(got, []string{"main"}) {
		t.Fatalf("switch ran Git: %v", got)
	}
	if !refreshProjectGitInfo(context.Background(), gitProjectPaths(state)) {
		t.Fatal("refresh did not report the new branch")
	}
	if got := branches(); !slices.Equal(got, []string{"added", "main"}) {
		t.Fatalf("after refresh = %v", got)
	}
	if refreshProjectGitInfo(context.Background(), gitProjectPaths(state)) {
		t.Fatal("refresh reported a change when nothing changed")
	}
	if _, err := worktreeGit(repo, "branch", "other"); err != nil {
		t.Fatal(err)
	}
	forgetProjectGitInfo()
	if got := branches(); !slices.Equal(got, []string{"added", "main", "other"}) {
		t.Fatalf("Libro's own Git change was not seen: %v", got)
	}
}
