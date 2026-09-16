package libro

import (
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type fileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}
type fileResult struct {
	ID        string      `json:"id"`
	Path      string      `json:"path"`
	Request   string      `json:"request"`
	Entries   []fileEntry `json:"entries,omitempty"`
	Directory bool        `json:"directory"`
	Text      string      `json:"text,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func readProjectFile(rootPath, path string) (fileResult, error) {
	result := fileResult{Path: path}
	if path == "" {
		path = "."
	}
	if !filepath.IsLocal(path) {
		return result, fmt.Errorf("path must stay inside the project")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return result, err
	}
	defer root.Close()
	infoBefore, err := root.Stat(path)
	if err != nil {
		return result, err
	}
	if !infoBefore.IsDir() && !infoBefore.Mode().IsRegular() {
		return result, fmt.Errorf("only regular files can be previewed")
	}
	f, err := root.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if info.IsDir() {
		result.Directory = true
		entries, err := f.ReadDir(-1)
		if err != nil {
			return result, err
		}
		for _, entry := range entries {
			result.Entries = append(result.Entries, fileEntry{Name: entry.Name(), Path: filepath.ToSlash(filepath.Join(path, entry.Name())), Dir: entry.IsDir()})
		}
		sort.Slice(result.Entries, func(i, j int) bool {
			a, b := result.Entries[i], result.Entries[j]
			if a.Dir != b.Dir {
				return a.Dir
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		return result, nil
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("only regular files can be previewed")
	}
	const limit = 1024 * 1024
	bytes, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return result, err
	}
	if len(bytes) > limit {
		return result, fmt.Errorf("file is too large to preview (limit 1 MB)")
	}
	if !utf8.Valid(bytes) || strings.ContainsRune(string(bytes), 0) {
		return result, fmt.Errorf("binary file — text preview unavailable")
	}
	result.Text = string(bytes)
	return result, nil
}

func registerFilesActions(app *r.App) {
	registerAction(app, "files.read", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		id, _ := data["id"].(string)
		path, _ := data["path"].(string)
		request, _ := data["request"].(string)
		allowed := false
		for _, a := range sm.Get(sid).Apps {
			if a.ID == id && a.PluginID == "files" {
				allowed = true
				break
			}
		}
		result := fileResult{ID: id, Path: path, Request: request}
		if allowed {
			value, err := readProjectFile(sm.GetActiveProjectPath(sid), path)
			result = value
			result.ID = id
			result.Request = request
			if err != nil {
				result.Error = err.Error()
			}
		} else {
			result.Error = "Switch to this project to browse its files"
		}
		raw, _ := json.Marshal(result)
		return fmt.Sprintf("window.libroFiles.receive(%s);", raw)
	})
}

func renderFiles(app Application) *r.Node {
	return r.Div("ws-files").Attr("data-files", app.ID).Render(
		r.Div("ws-file-preview").Render(r.Div("ws-file-path").Text("Open file"), r.El("pre", "ws-file-text").Attr("tabindex", "0").Text("Select a file from the project tree.")),
		r.Div("ws-file-sidebar").Render(
			r.Input("ws-file-filter").Attr("placeholder", "Filter files…").Attr("aria-label", "Filter files"),
			r.Div("ws-file-tree").Attr("role", "tree").Attr("aria-label", "Project files").Attr("tabindex", "0"),
			r.Div("ws-file-help").Text("j/k move · h/l fold/open · gg/G first/last · / filter · Enter preview"),
			r.Div("ws-file-status").Attr("role", "status"),
		),
	)
}
