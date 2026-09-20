package libro

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebsiteToolIcon(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	for _, linked := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/page":
				if linked {
					_, _ = w.Write([]byte(`<link rel="shortcut icon" href="/logo.png">`))
				}
			case "/logo.png", "/favicon.ico":
				_, _ = w.Write(png)
			default:
				http.NotFound(w, r)
			}
		}))
		if got := websiteToolIcon(server.URL + "/page?private=value"); got != iconData(png) {
			t.Errorf("linked=%v: icon mismatch", linked)
		}
		server.Close()
	}
}

func TestWebsiteToolIconRejectsNonImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>Not an icon</html>")) }))
	defer server.Close()
	if got := websiteToolIcon(server.URL); got != "" {
		t.Fatal("accepted HTML as an icon")
	}
	if got := iconData([]byte(strings.Repeat("x", 512*1024+1))); got != "" {
		t.Fatal("accepted oversized icon")
	}
}

func TestLocalToolIcon(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	dir := filepath.Join(root, "icons/hicolor/scalable/apps")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M0 0h16v16H0z"/></svg>`)
	if err := os.WriteFile(filepath.Join(dir, "libro-test-tool.svg"), svg, 0600); err != nil {
		t.Fatal(err)
	}
	if got := localToolIcon("libro-test-tool --help"); got != iconData(svg) {
		t.Fatal("installed CLI icon not found")
	}
}

func TestToolIconCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/favicon.ico" {
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
		}
	}))
	defer server.Close()
	p := Plugin{ID: "custom-tool-cache", Name: "Cache", Type: AppTypeURL, Dock: "right", URL: server.URL}
	first := toolIcon(p)
	count := requests
	if first == "" {
		t.Fatal("icon not found")
	}
	if toolIcon(p) != first || requests != count {
		t.Fatal("icon was not reused from cache")
	}
}
