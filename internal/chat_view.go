package libro

import (
	"bytes"
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	r "github.com/michalCapo/g-sui/ui"
)

// The chat viewer is a sheet from the right, as in Kodri, that shows a file
// named in Claude's answer or the images of a message. Files are served under
// a random key, so pages in browser panels cannot guess them, and as a
// sandbox, so an HTML report cannot act as Libro.

const (
	chatViewTTL       = time.Hour
	chatViewTextLimit = 2 << 20
)

type chatViewEntry struct {
	dir    string   // a file's folder, so an HTML report finds its assets
	images [][]byte // or the images of a message
	at     time.Time
}

var chatViews = struct {
	sync.Mutex
	m map[string]chatViewEntry
}{m: map[string]chatViewEntry{}}

func putChatView(entry chatViewEntry) string {
	key := rand.Text()
	entry.at = time.Now()
	chatViews.Lock()
	defer chatViews.Unlock()
	for k, old := range chatViews.m {
		if time.Since(old.at) > chatViewTTL {
			delete(chatViews.m, k)
		}
	}
	chatViews.m[key] = entry
	return key
}

type actionChatViewInput struct {
	SID   string `json:"sid"`
	AppID string `json:"appId"`
	Path  string `json:"path"` // a file, absolute or in the panel's folder
	Item  string `json:"item"` // or a message whose images to show
}

// chatViewSheet is what the sheet shows.
type chatViewSheet struct {
	Title string   `json:"title"`
	Kind  string   `json:"kind"` // image, html, pdf, markdown, text or none
	URLs  []string `json:"urls"`
	HTML  string   `json:"html,omitempty"` // a Markdown file, drawn
	Class string   `json:"class,omitempty"`
	Note  string   `json:"note,omitempty"`
	App   string   `json:"app"`
	Path  string   `json:"path,omitempty"` // a Markdown file's links start from its folder
}

func registerChatView(app *r.App) {
	registerWorkspaceAction(app, "chat.view", func(_ *r.Context, in actionChatViewInput) (r.Result, error) {
		sheet, err := chatView(inputSID(in.SID), in)
		if err != nil {
			return chatError(err), nil
		}
		sheet.App = in.AppID
		return clientScript("if(window.libroChat)libroChat.view(props[0]);", sheet), nil
	})
	app.GET("/chat-view/{key}/{name...}", serveChatView)
}

func chatView(sid string, in actionChatViewInput) (chatViewSheet, error) {
	panel, _, dir, found := sm.workspaceApp(sid, in.AppID)
	if !found || !panel.Chat {
		return chatViewSheet{}, errors.New("this chat panel is closed")
	}
	if in.Item != "" {
		snapshot, _ := tm.ChatSnapshot(in.AppID)
		for _, item := range snapshot.Items {
			if item.ID != in.Item || len(item.Images) == 0 {
				continue
			}
			key := putChatView(chatViewEntry{images: item.Images})
			sheet := chatViewSheet{Title: "1 image", Kind: "image"}
			if len(item.Images) > 1 {
				sheet.Title = strconv.Itoa(len(item.Images)) + " images"
			}
			for i := range item.Images {
				sheet.URLs = append(sheet.URLs, "/chat-view/"+key+"/"+strconv.Itoa(i))
			}
			return sheet, nil
		}
		return chatViewSheet{}, errors.New("these images are gone")
	}
	path := chatViewPath(in.Path, dir)
	// The served folder is the real file's, so a linked file stays inside it.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	info, err := os.Stat(path)
	if err != nil {
		return chatViewSheet{}, errors.New("file not found: " + in.Path)
	}
	if !info.Mode().IsRegular() {
		return chatViewSheet{}, errors.New("not a file: " + in.Path)
	}
	sheet := chatViewSheet{Title: in.Path, Kind: chatViewKind(path, info.Size())}
	if sheet.Kind == "none" {
		sheet.Note = "No preview for this file."
		if info.Size() > chatViewTextLimit {
			sheet.Note = "This file is too large to show."
		}
		return sheet, nil
	}
	if sheet.Kind == "markdown" {
		data, err := os.ReadFile(path)
		if err != nil {
			return chatViewSheet{}, err
		}
		var html bytes.Buffer
		if chatMarkdownRenderer.Convert([]byte(markdownImage.ReplaceAllString(string(data), "[")), &html) == nil {
			sheet.HTML, sheet.Class, sheet.Path = html.String(), chatMarkdownClass, path
			return sheet, nil
		}
		sheet.Kind = "text"
	}
	key := putChatView(chatViewEntry{dir: filepath.Dir(path)})
	sheet.URLs = []string{"/chat-view/" + key + "/" + url.PathEscape(filepath.Base(path))}
	return sheet, nil
}

// A path Claude names may end with a line and column: main.go:12:5.
var chatViewLine = regexp.MustCompile(`(:\d+){1,2}$`)

func chatViewPath(path, dir string) string {
	path = strings.TrimPrefix(strings.TrimSpace(path), "file://")
	path = chatViewLine.ReplaceAllString(path, "")
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return filepath.Clean(path)
}

func chatViewKind(path string, size int64) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico", ".avif":
		return "image"
	case ".html", ".htm":
		return "html"
	case ".pdf":
		return "pdf"
	}
	if size > chatViewTextLimit {
		return "none"
	}
	head := make([]byte, 8192)
	file, err := os.Open(path)
	if err != nil {
		return "none"
	}
	n, _ := file.Read(head)
	_ = file.Close()
	text := head[:n]
	// The cut may split the last character.
	for i := 0; n == len(head) && i < 3 && !utf8.Valid(text); i++ {
		text = text[:len(text)-1]
	}
	if bytes.IndexByte(text, 0) >= 0 || !utf8.Valid(text) {
		return "none"
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".md" || ext == ".markdown" {
		return "markdown"
	}
	return "text"
}

func serveChatView(w http.ResponseWriter, req *http.Request) {
	chatViews.Lock()
	entry, ok := chatViews.m[req.PathValue("key")]
	chatViews.Unlock()
	if !ok || time.Since(entry.at) > chatViewTTL {
		http.NotFound(w, req)
		return
	}
	name := req.PathValue("name")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if entry.images != nil {
		index, err := strconv.Atoi(name)
		if err != nil || index < 0 || index >= len(entry.images) {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(entry.images[index]))
		http.ServeContent(w, req, "", entry.at, bytes.NewReader(entry.images[index]))
		return
	}
	// The root keeps the report and its assets inside the file's folder.
	root, err := os.OpenRoot(entry.dir)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, req)
		return
	}
	// PDFs need Chromium's viewer, which a sandbox blocks.
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts")
	}
	// A sandboxed report has no origin; it may still load its own data and modules.
	if req.Header.Get("Origin") == "null" {
		w.Header().Set("Access-Control-Allow-Origin", "null")
	}
	http.ServeContent(w, req, info.Name(), info.ModTime(), file)
}
