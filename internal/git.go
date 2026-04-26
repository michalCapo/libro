package libro

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// gitAvailable caches whether the git binary is in $PATH.
var gitAvailable *bool

// GitAvailable returns true if the git command is available on this system.
func GitAvailable() bool {
	if gitAvailable != nil {
		return *gitAvailable
	}
	_, err := exec.LookPath("git")
	result := err == nil
	gitAvailable = &result
	return result
}

// Worktree represents a single git worktree.
type Worktree struct {
	Path   string // absolute path to worktree directory
	Branch string // branch name (e.g. "main", "feature-x")
	IsBare bool   // true for bare repos
}

// GitIsRepo returns true if the given path is inside a git repository.
func GitIsRepo(path string) bool {
	if !GitAvailable() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// GitListWorktrees returns all worktrees for the repository at repoPath.
func GitListWorktrees(repoPath string) ([]Worktree, error) {
	if !GitAvailable() {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			branch := strings.TrimPrefix(line, "branch ")
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
		case line == "bare":
			current.IsBare = true
		case line == "detached":
			if current.Branch == "" {
				current.Branch = "(detached)"
			}
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

// GitCurrentBranch returns the short name of the branch currently checked out
// at repoPath, or an empty string for detached HEAD or on error.
func GitCurrentBranch(repoPath string) string {
	if !GitAvailable() {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// GitCreateWorktree creates a new git worktree at wtPath, checking out a new
// branch named branch based on the current HEAD of the repository at repoPath.
func GitCreateWorktree(repoPath, branch, wtPath string) error {
	if !GitAvailable() {
		return fmt.Errorf("git is not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "add", "-b", branch, wtPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// GitListBranches returns all local branch names for the repository at repoPath.
func GitListBranches(repoPath string) ([]string, error) {
	if !GitAvailable() {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "branch", "--list", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		b := strings.TrimSpace(scanner.Text())
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}
