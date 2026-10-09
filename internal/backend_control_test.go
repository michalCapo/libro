package libro

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

func TestBackendToolsWithoutFrontend(t *testing.T) {
	sid, root := childFixture(t)
	sm.Get(sid).ActiveProject = "other"
	sm.Get(sid).SelectedIndex = 3
	if err := setProjectCommand(root, "sleep 30"); err != nil {
		t.Fatal(err)
	}
	_, result, err := dispatchBackendCommand(sid, backendCommand{Action: "application", Project: root, Operation: "start"})
	if err != nil || result.(map[string]any)["status"] != "running" {
		t.Fatalf("backend start: %v", err)
	}
	if sm.Get(sid).ActiveProject != "other" || sm.Get(sid).SelectedIndex != 3 {
		t.Fatal("tool changed frontend selection")
	}
	if _, _, err := dispatchBackendCommand(sid, backendCommand{Action: "application", Project: t.TempDir(), Operation: "stop"}); err == nil {
		t.Fatal("accepted unregistered project")
	}
	_, result, err = dispatchBackendCommand(sid, backendCommand{Action: "application", Project: root, Operation: "stop"})
	if err != nil || result.(map[string]any)["status"] != "stopped" {
		t.Fatalf("backend stop: %v", err)
	}
	_, result, err = dispatchBackendCommand(sid, backendCommand{Action: "children", Project: root, Command: childCommand{Action: "create", Name: "Backend QA", Base: "main", Prompt: "Check the backend"}})
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]any)["id"].(string)
	if sm.Get(sid).thread(id) == nil || sm.Get(sid).ActiveProject != "other" {
		t.Fatal("child depended on frontend selection")
	}
	_, _, err = dispatchBackendCommand(sid, backendCommand{Action: "children", Project: sm.Get(sid).Projects[1].Path, Command: childCommand{Action: "interrupt", ID: id}})
	if err == nil {
		t.Fatal("another workspace controlled the child")
	}
}

func TestBackendToolTransportAndScope(t *testing.T) {
	sid, root := childFixture(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LIBRO_INSTANCE", "")
	t.Setenv("LIBRO_APPLICATION_PATH", root)
	sm.backendSID = sid
	app := r.NewApp()
	t.Cleanup(func() { _ = app.Close() })
	cleanup, err := startBackendControl(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	result, err := scopedApplicationCommand(json.RawMessage(`{"action":"status"}`), root)
	if err != nil || !strings.Contains(string(result), `"status":"stopped"`) {
		t.Fatalf("application transport: %v", err)
	}
	result, err = scopedChildrenCommand(json.RawMessage(`{"action":"list"}`), root)
	if err != nil || string(result) != "[]" {
		t.Fatalf("children transport: %v", err)
	}
	sm.Get(sid).ActiveProject = ""
	result, err = NotesCommand(json.RawMessage(`{"action":"create","title":"Backend note"}`))
	if err != nil || !strings.Contains(string(result), `"title":"Backend note"`) {
		t.Fatalf("notes without selection: %v", err)
	}
	for _, action := range []string{"list", "delete"} {
		if _, err := NotesCommand(json.RawMessage(`{"action":"` + action + `","project":"/another-workspace"}`)); err == nil {
			t.Fatal("notes escaped agent scope")
		}
	}
	path, err := controlConnectionPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("connection descriptor is not private")
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("descriptor survived shutdown")
	}
	if _, err := scopedApplicationCommand(json.RawMessage(`{"action":"status"}`), root); err == nil {
		t.Fatal("tool succeeded after shutdown")
	}
}

func TestBackendControlRejectsInvalidRequests(t *testing.T) {
	sid, _ := childFixture(t)
	token := strings.Repeat("a", 64) // Synthetic test token.
	app := r.NewApp()
	t.Cleanup(func() { _ = app.Close() })
	handler := backendControlHandler(app, token, sid)
	for _, test := range []struct {
		name, body, auth, origin string
		status                   int
	}{
		{"unauthenticated", `{}`, "", "", http.StatusUnauthorized},
		{"bad token", `{}`, "Bearer invalid", "", http.StatusUnauthorized},
		{"browser origin", `{}`, "Bearer " + token, "http://localhost", http.StatusUnauthorized},
		{"unknown field", `{"action":"application","target":"foreign"}`, "Bearer " + token, "", http.StatusOK},
		{"multiple commands", `{} {}`, "Bearer " + token, "", http.StatusOK},
		{"unknown action", `{"action":"foreign"}`, "Bearer " + token, "", http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(test.body))
			req.Header.Set("Authorization", test.auth)
			req.Header.Set("Origin", test.origin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != test.status {
				t.Fatalf("status %d", w.Code)
			}
			if w.Code == http.StatusOK && !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatal("invalid command accepted")
			}
		})
	}
}

func TestBackendDescriptorCleanupKeepsNewInstance(t *testing.T) {
	sid, _ := childFixture(t)
	sm.backendSID = sid
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	app := r.NewApp()
	t.Cleanup(func() { _ = app.Close() })
	cleanup, err := startBackendControl(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	path, _ := controlConnectionPath()
	replacement := []byte(`{"port":1234,"token":"synthetic-new-instance"}`)
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup()
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, replacement) {
		t.Fatal("old cleanup removed new descriptor")
	}
}

func TestBackendApplicationLogsStayBounded(t *testing.T) {
	sid, root := childFixture(t)
	if err := setProjectCommand(root, "printf '%81920sEND' ''; sleep 30"); err != nil {
		t.Fatal(err)
	}
	_, _, err := dispatchBackendCommand(sid, backendCommand{Action: "application", Project: root, Operation: "start"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, result, err := controlApplication(sid, root, "logs")
		if err != nil {
			t.Fatal(err)
		}
		reply := result
		output := reply["logs"].(string)
		if strings.HasSuffix(output, "END") {
			if len(output) != 64<<10 || reply["truncated"] != true {
				t.Fatal("application logs exceeded tool limit or lost truncation")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("application produced no output")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
