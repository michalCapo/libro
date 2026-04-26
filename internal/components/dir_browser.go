package components

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// DirBrowser renders a single-level directory listing for currentPath.
// Each entry triggers a `project.browse` action with the new path.
func DirBrowser(currentPath string, sid string) *r.Node {
	dirCls := "dir-item flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-gray-700 dark:text-zinc-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded cursor-pointer transition-colors"

	entries, err := os.ReadDir(currentPath)
	var dirs []string
	if err != nil {
		log.Printf("components: DirBrowser os.ReadDir(%q) failed: %v", currentPath, err)
	} else {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
	}

	var items []*r.Node
	parentPath := filepath.Dir(currentPath)
	if parentPath != currentPath {
		items = append(items, r.Div(dirCls).
			Attr("data-name", "..").
			Attr("data-path", parentPath).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-base").Text("arrow_upward"),
				r.Span("").Text(".."),
			).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": parentPath},
		}))
	}

	for _, d := range dirs {
		fullPath := filepath.Join(currentPath, d)
		items = append(items, r.Div(dirCls).
			Attr("data-name", d).
			Attr("data-path", fullPath).
			Render(
				r.I("material-icons-round text-amber-500 dark:text-amber-400 text-base").Text("folder"),
				r.Span("truncate").Text(d),
			).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": fullPath},
		}))
	}

	dirList := r.Div("max-h-80 overflow-y-auto space-y-0.5 border border-gray-200 dark:border-zinc-700 rounded-md p-1.5 bg-gray-50/50 dark:bg-zinc-800/50").
		ID("dir-browser-list").
		Attr("data-current", currentPath)
	if len(items) == 0 {
		dirList.Render(
			r.Div("px-3 py-2 text-xs font-mono text-gray-400 dark:text-zinc-600 italic").Text("No subdirectories"),
		)
	} else {
		dirList.Render(items...)
	}

	return r.Div("").ID(DirBrowserID).Render(dirList)
}
