package libro

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestChatFileLinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	html := `<p><code>main.go:12</code> <code>missing.go</code> <code>os.Stat</code></p><pre><code>main.go</code></pre>`
	want := `<p><code data-chat-path="main.go:12" role="link" tabindex="0">main.go:12</code> <code>missing.go</code> <code>os.Stat</code></p><pre><code>main.go</code></pre>`
	if got := chatFileLinks(html, dir); got != want {
		t.Fatalf("links:\n got %s\nwant %s", got, want)
	}
}

func TestServeChatViewStaysInFolder(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "report")
	if err := os.Mkdir(report, 0700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{"report/index.html": "<h1>Report</h1>", "secret.txt": "secret"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	key := putChatView(chatViewEntry{dir: report})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat-view/{key}/{name...}", serveChatView)
	for path, status := range map[string]int{
		"/chat-view/" + key + "/index.html":        http.StatusOK,
		"/chat-view/" + key + "/..%2fsecret.txt":   http.StatusNotFound,
		"/chat-view/" + key + "/":                  http.StatusNotFound,
		"/chat-view/wrong-key/index.html":          http.StatusNotFound,
		"/chat-view/" + key + "/%2e%2e/secret.txt": http.StatusNotFound,
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Errorf("%s: status %d, want %d", path, w.Code, status)
		}
		if status == http.StatusOK && w.Header().Get("Content-Security-Policy") != "sandbox allow-scripts" {
			t.Errorf("%s: not sandboxed", path)
		}
	}
}
