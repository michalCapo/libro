package libro

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareThreadTaskKeepsImagesWithoutScripts(t *testing.T) {
	pixel, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/shared/img/logo.png" {
			_, _ = w.Write(pixel)
			return
		}
		http.NotFound(w, req)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "email.html")
	page := `<html><head><meta http-equiv="refresh" content="0;url=https://example.com"><script>alert(1)</script></head>` +
		`<body onload="alert(2)"><h1>Hotfix 384</h1><img src="img/logo.png" srcset="x 2x"><img src="/missing.png">` +
		`<img srcset="` + strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + `/shared/img/logo.png 2x">` +
		`<a href="javascript:alert(3)">x</a><a href="file:///etc/passwd">f</a><a href="other">o</a><iframe src="https://example.com"></iframe></body></html>`
	if err := os.WriteFile(file, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	task, err := prepareThreadTask(context.Background(), file, server.URL+"/shared/email?sig=1")
	if err != nil {
		t.Fatal(err)
	}
	out := string(task.page)
	if strings.Count(out, "<script") != 1 || !strings.Contains(out, "<script>"+taskViewer+"</script>") {
		t.Fatalf("page scripts kept or image viewer missing: %s", out)
	}
	for _, gone := range []string{"alert", "refresh", "srcset", "<iframe", "file:"} {
		if strings.Contains(out, gone) {
			t.Fatalf("kept %q: %s", gone, out)
		}
	}
	if !strings.Contains(out, `src="data:image/png;base64,`) || !strings.Contains(out, "Hotfix 384") || !strings.Contains(out, "Content-Security-Policy") || !strings.Contains(out, `href="`+server.URL+`/shared/other"`) {
		t.Fatalf("image, text, policy or link missing: %s", out)
	}
	// Another name for this machine is not the task's host, so it stays blocked.
	if task.images != 3 || task.failed != 2 {
		t.Fatalf("images %d failed %d", task.images, task.failed)
	}
}

func TestPrepareThreadTaskEscapesText(t *testing.T) {
	file := filepath.Join(t.TempDir(), "card.md")
	if err := os.WriteFile(file, []byte("# Card\n<b>comment</b>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareThreadTask(context.Background(), "card.md", ""); err == nil {
		t.Fatal("accepted a relative path")
	}
	if _, err := prepareThreadTask(context.Background(), file, "file:///etc/passwd"); err == nil {
		t.Fatal("accepted a local link")
	}
	task, err := prepareThreadTask(context.Background(), file, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(task.page), "# Card\n&lt;b&gt;comment&lt;/b&gt;") {
		t.Fatal(string(task.page))
	}
}

func TestSaveThreadTaskBesideAgentAndForgetWithThread(t *testing.T) {
	sid, _ := childFixture(t)
	desktopWindow = true
	t.Cleanup(func() { desktopWindow = false })
	state := sm.Get(sid)
	state.Apps = []Application{{ID: "app-1", Type: AppTypeTerminal, Command: "claude"}}
	sm.nextID = 10
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "thread", Panel: "app-1", Thread: threadCommand{Action: "task"}, task: &threadTask{page: []byte("<p>task</p>")}}); err != nil {
		t.Fatal(err)
	}
	key := threadTaskKey(sm.Get(sid), state.ActiveProject)
	path := savedThreadTask(key)
	if data, err := os.ReadFile(path); err != nil || string(data) != "<p>task</p>" {
		t.Fatalf("task not saved: %q %v", data, err)
	}
	state = sm.Get(sid)
	if len(state.Apps) != 2 || !strings.HasPrefix(state.Apps[1].URL, fileURL(path)+"?saved=") {
		t.Fatalf("task not shown beside the agent: %+v", state.Apps)
	}
	removeThreadTask(key)
	if savedThreadTask(key) != "" {
		t.Fatal("task kept after the thread")
	}
}
