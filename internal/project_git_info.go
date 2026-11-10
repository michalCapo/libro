package libro

import (
	"context"
	"log"
	"reflect"
	"sync"

	r "github.com/michalCapo/g-sui/ui"
)

// projectGitInfo is what the sidebar shows from Git for one project.
type projectGitInfo struct {
	Branches    []string
	Current     string
	Worktrees   []Worktree
	WorktreesOK bool
}

var projectGitInfoCache = struct {
	sync.Mutex
	items map[string]projectGitInfo
}{items: map[string]projectGitInfo{}}

// A pending refresh already covers every later request.
var projectGitRefreshRequests = make(chan struct{}, 1)

func requestProjectGitRefresh() {
	select {
	case projectGitRefreshRequests <- struct{}{}:
	default:
	}
}

func readProjectGitInfo(path string) projectGitInfo {
	var info projectGitInfo
	if branches, err := GitListBranches(path); err == nil {
		info.Branches = branches
	}
	info.Current = GitCurrentBranch(path)
	worktrees, err := GitListWorktrees(path)
	info.Worktrees, info.WorktreesOK = worktrees, err == nil
	projectGitInfoCache.Lock()
	projectGitInfoCache.items[path] = info
	projectGitInfoCache.Unlock()
	return info
}

// forgetProjectGitInfo makes the next read see a change Libro just made in the
// project at path. Only that project waits for Git; the others keep their saved
// info. A background read that was already running is repaired by the refresh
// that the next projectsJS requests.
func forgetProjectGitInfo(path string) {
	projectGitInfoCache.Lock()
	delete(projectGitInfoCache.items, path)
	projectGitInfoCache.Unlock()
}

// Actions use this so that they never wait for Git.
func cachedProjectGitInfo(path string) projectGitInfo {
	projectGitInfoCache.Lock()
	info, ok := projectGitInfoCache.items[path]
	projectGitInfoCache.Unlock()
	if ok {
		return info
	}
	return readProjectGitInfo(path)
}

// refreshProjectGitInfo reads Git again and reports whether anything changed.
func refreshProjectGitInfo(ctx context.Context, paths []string) bool {
	changed := false
	for _, path := range paths {
		if ctx.Err() != nil {
			break
		}
		projectGitInfoCache.Lock()
		old, known := projectGitInfoCache.items[path]
		projectGitInfoCache.Unlock()
		if info := readProjectGitInfo(path); !known || !reflect.DeepEqual(old, info) {
			changed = true
		}
	}
	return changed
}

func gitProjectPaths(state *AppState) []string {
	var paths []string
	for _, project := range state.Projects {
		if !project.Virtual && project.IsGitRepo {
			paths = append(paths, project.Path)
		}
	}
	return paths
}

// startProjectGitRefresh keeps the sidebar current. Git runs outside
// backendActionMu, so it never delays the user's actions. Stopping does not wait
// for a running Git command: quitting must save the session first.
func startProjectGitRefresh(app *r.App) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-projectGitRefreshRequests:
			}
			backendActionMu.Lock()
			paths := gitProjectPaths(sm.Get(backendSessionID()))
			backendActionMu.Unlock()
			if !refreshProjectGitInfo(ctx, paths) {
				continue
			}
			backendActionMu.Lock()
			if ctx.Err() == nil {
				state := sm.Get(backendSessionID())
				if err := app.Broadcast(projectItemsJS(state, projectItemsWith(state, cachedProjectGitInfo))); err != nil {
					log.Printf("libro: project git refresh: %v", err)
				}
			}
			backendActionMu.Unlock()
		}
	}()
	return cancel
}
