package libro

import (
	"bytes"
	"encoding/base64"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"io"
	"mime"
	"net/http"
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
	Version   string      `json:"version,omitempty"`
	Unchanged bool        `json:"unchanged,omitempty"`
	ID        string      `json:"id"`
	Path      string      `json:"path"`
	Request   string      `json:"request"`
	Entries   []fileEntry `json:"entries,omitempty"`
	Directory bool        `json:"directory"`
	HTML      string      `json:"html,omitempty"`
	Text      string      `json:"text,omitempty"`
	MIME      string      `json:"mime,omitempty"`
	Data      string      `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Matches   []fileMatch `json:"matches,omitempty"`
	Truncated bool        `json:"truncated,omitempty"`
}

func readProjectFile(rootPath, path string) (fileResult, error) {
	return readProjectFileVersion(rootPath, path, "")
}

func readProjectFileVersion(rootPath, path, version string) (fileResult, error) {
	result := fileResult{Path: path}
	if path == "" {
		path = "."
	}
	if !filepath.IsLocal(path) {
		return result, fmt.Errorf("path must stay inside the files root")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return result, err
	}
	defer func() { _ = root.Close() }()
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
	defer func() { _ = f.Close() }()
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
	result.Version = fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
	if version != "" && version == result.Version {
		result.Unchanged = true
		return result, nil
	}
	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return result, err
	}
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mediaType == "" {
		mediaType = http.DetectContentType(header[:n])
	}
	mediaType, _, _ = mime.ParseMediaType(mediaType)
	preview := strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") || mediaType == "application/pdf"
	limit := 1024 * 1024
	if preview {
		limit = 32 * 1024 * 1024
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	content, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil {
		return result, err
	}
	if len(content) > limit {
		return result, fmt.Errorf("file is too large to preview (limit %d MB); use Open externally", limit/(1024*1024))
	}
	if preview {
		result.MIME = mediaType
		result.Data = base64.StdEncoding.EncodeToString(content)
		return result, nil
	}
	if !utf8.Valid(content) || strings.ContainsRune(string(content), 0) {
		return result, fmt.Errorf("unsupported file format; use Open externally")
	}
	result.Text = string(content)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		result.HTML = result.Text
	case ".md", ".markdown":
		var rendered bytes.Buffer
		if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert([]byte(result.Text), &rendered); err != nil {
			return result, err
		}
		result.HTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><style>:root{color-scheme:light dark}body{font:16px/1.6 system-ui,sans-serif;max-width:75ch;margin:0 auto;padding:24px;overflow-wrap:anywhere}pre{overflow:auto;padding:12px;background:light-dark(#f7f7f8,#222225)}table{border-collapse:collapse}th,td{border:1px solid #888;padding:6px 12px}img{max-width:100%}blockquote{margin-left:0;padding-left:12px;border-left:1px solid #888}</style></head><body>` + rendered.String() + "</body></html>"
	}
	return result, nil
}

func projectFileToOpen(rootPath, path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("path must stay inside the files root")
	}
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, path))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("path must stay inside the files root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("only regular files can be opened")
	}
	return resolved, nil
}

// filesRoot keeps navigation local to each files panel, independent of the project.
func filesRoot(projectPath string, parents int) string {
	root := filepath.Clean(projectPath)
	for range parents {
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return root
}

func registerFilesActions(app *r.App) {
	for _, action := range []string{"files.read", "files.open", "files.index", "files.search", "files.navigate"} {
		r.RegisterAction(app, action, func(_ *r.Context, in actionFilesInput) (r.Result, error) {
			sid := inputSID(in.SID)
			id := in.ID
			path := in.Path
			request := in.Request
			parents := in.Parents
			if parents < 0 || parents > 1024 {
				parents = 0
			}
			root := filesRoot(sm.GetActiveProjectPath(sid), int(parents))
			allowed := false
			for _, a := range sm.Get(sid).Apps {
				if a.ID == id && a.PluginID == "files" {
					allowed = true
					break
				}
			}
			result := fileResult{ID: id, Path: path, Request: request}
			if allowed && action == "files.open" {
				file, err := projectFileToOpen(root, path)
				if err == nil {
					err = openBrowser(file)
				}
				if err != nil {
					result.Error = err.Error()
				}
			} else if allowed {
				var value fileResult
				var err error
				switch action {
				case "files.navigate":
					kind := in.Kind

					version := in.Version

					line := in.Line

					column := in.Column

					value, err = navigateProjectFile(sm.GetActiveProjectPath(sid), path, kind, version, int(parents), int(line), int(column))
				case "files.index", "files.search":
					query := in.Query

					ignored := in.Ignored

					// Search always targets the active workspace, even after browsing a parent.
					value, err = searchProjectFiles(sm.GetActiveProjectPath(sid), query, ignored, action == "files.index")
				default:
					version := in.Version

					value, err = readProjectFileVersion(root, path, version)
				}
				result = value
				result.ID = id
				result.Request = request
				if err != nil {
					result.Error = err.Error()
				}
			} else {
				result.Error = "Switch to this project to browse its files"
			}
			return clientScript("window.libroFiles.receive(props[0]);", result), nil
		})
	}
}

func renderFiles(app Application) *r.Node {
	return r.Widget("files", struct{}{}, "ws-files").Attr("data-files", app.ID).Render(
		r.Div("ws-file-preview").Render(
			r.Div("ws-file-toolbar").Render(r.Div("ws-file-path").Text("Open file"),
				r.Div("ws-file-image-tools").Attr("hidden", "hidden").Attr("role", "group").Attr("aria-label", "Image zoom").Render(
					r.Button("ws-file-image-zoom").Attr("type", "button").Attr("data-image-zoom", "out").Attr("aria-label", "Zoom out").Text("−"),
					r.Button("ws-file-image-zoom ws-file-image-reset").Attr("type", "button").Attr("data-image-zoom", "reset").Attr("aria-label", "Reset zoom").Text("100%"),
					r.Button("ws-file-image-zoom").Attr("type", "button").Attr("data-image-zoom", "in").Attr("aria-label", "Zoom in").Text("+")),
				r.El("label", "ws-file-render").Attr("title", "Preview HTML and Markdown").Render(r.Input("").Attr("type", "checkbox"), r.Span("").Text("Preview")),
				r.El("label", "ws-file-wrap").Render(r.Input("").Attr("type", "checkbox").Attr("checked", "checked"), r.Span("").Text("Word wrap")),
				r.El("label", "ws-file-hidden").Render(r.Input("").Attr("type", "checkbox").Attr("checked", "checked"), r.Span("").Text("Hidden files")),
				r.Button("ws-button ws-file-help").Attr("type", "button").Attr("title", "Keyboard shortcuts").Attr("aria-label", "Keyboard shortcuts").Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("keyboard"))),
			r.Div("ws-file-context").Attr("aria-label", "Source location"),
			r.Div("ws-file-text is-wrapped").Attr("tabindex", "0").Text("Select a file from the project tree."),
			r.Div("ws-file-media").Attr("hidden", "hidden"),
		),
		r.Div("ws-file-sidebar").Render(
			r.Input("ws-file-filter").Attr("placeholder", "Find project files…").Attr("aria-label", "Filter files"),
			r.Div("ws-file-tree").Attr("role", "tree").Attr("aria-label", "Project files").Attr("tabindex", "0"),
			r.Div("ws-file-status").Attr("role", "status"),
		),
	)
}
