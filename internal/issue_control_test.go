package libro

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	r "github.com/michalCapo/g-sui/ui"
)

func setupIssueControl(t *testing.T) string {
	t.Helper()
	oldDB, oldSM := db, sm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "issues.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm = NewStateManager()
	t.Cleanup(func() { _ = db.Close(); db, sm = oldDB, oldSM })
	createTables()
	path := t.TempDir()
	sm.states["test"] = &AppState{ActiveProject: "one", Projects: []Project{{Name: "one", Path: path}, {Name: "two", Path: filepath.Join(path, "two")}}}
	return path
}

func TestIssueControlLifecycle(t *testing.T) {
	path := setupIssueControl(t)
	call := func(command issueCommand) any {
		t.Helper()
		command.Project = path
		result, err := controlIssues("test", command)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := call(issueCommand{Action: "create", Title: " Fix login ", Body: "**Steps**\nTry signing in"}).(projectNote)
	if created.State != "new" || created.Title != "Fix login" || created.ID == "" {
		t.Fatalf("create: %+v", created)
	}
	// Include an existing attachment to ensure a status-only update preserves it.
	png, err := os.ReadFile("../winres/icon16.png")
	if err != nil {
		t.Fatal(err)
	}
	created.Images = []noteImage{{ID: "screenshot", Data: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}}
	created, err = saveNote("one", created)
	if err != nil {
		t.Fatal(err)
	}
	call(issueCommand{Action: "set_status", ID: created.ID, Status: "archived"})
	read := call(issueCommand{Action: "read", ID: created.ID}).(projectNote)
	if read.State != "archived" || read.Title != created.Title || read.Body != created.Body || !reflect.DeepEqual(read.Images, created.Images) {
		t.Fatal("status update changed issue content")
	}
	call(issueCommand{Action: "set_status", ID: created.ID, Status: "new"})
	other, err := saveNote("two", projectNote{Title: "Other project", State: "new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"read", "set_status", "delete"} {
		if _, err := controlIssues("test", issueCommand{Project: path, Action: action, ID: other.ID, Status: "archived"}); err == nil {
			t.Fatalf("%s accessed another project", action)
		}
	}
	call(issueCommand{Action: "delete", ID: created.ID})
	for _, action := range []string{"read", "set_status", "delete"} {
		if _, err := controlIssues("test", issueCommand{Project: path, Action: action, ID: created.ID, Status: "new"}); err == nil {
			t.Fatalf("%s accepted deleted issue", action)
		}
	}
	// A stale UI editor cannot recreate a deleted issue by saving it.
	if _, err := saveNote("one", created); err == nil {
		t.Fatal("stale save resurrected deleted issue")
	}
	if _, err := readIssue("two", other.ID); err != nil {
		t.Fatal("delete affected another issue")
	}
}

func TestIssueControlUsesProjectFromThread(t *testing.T) {
	path := setupIssueControl(t)
	state := sm.states["test"]
	state.Threads = []Thread{{ID: "thread:test", Project: "one", Path: path}}
	state.ActiveProject = "thread:test"

	result, err := controlIssues("test", issueCommand{Project: path, Action: "create", Title: "From thread"})
	if err != nil || result.(projectNote).Title != "From thread" {
		t.Fatalf("thread issue control failed: %v, %+v", err, result)
	}
	issues, err := loadNotes("one")
	if err != nil || len(issues) != 1 {
		t.Fatalf("issue was not stored under project: %+v, %v", issues, err)
	}
}

func TestIssueControlListAndValidation(t *testing.T) {
	path := setupIssueControl(t)
	for _, status := range []string{"new", "archived", "new"} {
		if _, err := controlIssues("test", issueCommand{Project: path, Action: "create", Title: "Task", Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(command issueCommand) map[string]any {
		t.Helper()
		command.Project, command.Action = path, "list"
		result, err := controlIssues("test", command)
		if err != nil {
			t.Fatal(err)
		}
		return result.(map[string]any)
	}
	first := list(issueCommand{Limit: 1})
	second := list(issueCommand{Limit: 1, Offset: 1})
	if first["hasMore"] != true || first["issues"].([]map[string]string)[0]["id"] == second["issues"].([]map[string]string)[0]["id"] {
		t.Fatal("pagination repeated issue")
	}
	archived := list(issueCommand{Status: "archived"})
	if len(archived["issues"].([]map[string]string)) != 1 || archived["hasMore"] != false {
		t.Fatal("status filter failed")
	}
	empty := list(issueCommand{Offset: 100})
	if len(empty["issues"].([]map[string]string)) != 0 {
		t.Fatal("empty page contains issues")
	}
	for _, command := range []issueCommand{
		{Action: "create", Title: " "}, {Action: "create", Title: "Task", Status: "done"}, {Action: "create", Title: "Task", ID: "existing"},
		{Action: "set_status", ID: "missing"}, {Action: "read"}, {Action: "delete"}, {Action: "unknown"},
		{Action: "list", Limit: -1}, {Action: "list", Limit: 201}, {Action: "list", Offset: -1},
	} {
		command.Project = path
		if _, err := controlIssues("test", command); err == nil {
			t.Fatalf("accepted invalid command: %+v", command)
		}
	}
	for _, sid := range []string{"test", "missing"} {
		if _, err := controlIssues(sid, issueCommand{Project: "/other", Action: "list"}); err == nil {
			t.Fatal("accepted wrong project/session")
		}
	}
	sm.states["test"].ActiveProject = ""
	if _, err := controlIssues("test", issueCommand{Project: path, Action: "list"}); err == nil {
		t.Fatal("accepted no active project")
	}
}

func TestIssueControlHTTP(t *testing.T) {
	path := setupIssueControl(t)
	app := r.NewApp()
	registerIssueControl(app)
	handler := app.Handler()
	// Exercise the same endpoint called from Electron, with no Issues panel open.
	payload, err := json.Marshal(map[string]any{"sid": "test", "command": issueCommand{Action: "create", Project: path, Title: "Via bridge"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/issues/agent", bytes.NewReader(payload))
	reply := httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), "Via bridge") {
		t.Fatalf("create: %d %s", reply.Code, reply.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/issues/agent", bytes.NewReader(payload))
	req.Header.Set("Origin", "https://untrusted.example")
	reply = httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusForbidden {
		t.Fatal("accepted foreign origin")
	}
}

func TestIssuesCLIHelpAndValidation(t *testing.T) {
	var output bytes.Buffer
	if err := RunIssuesCLI([]string{"--help"}, &output); err != nil || !strings.Contains(output.String(), "set_status") {
		t.Fatal("missing CLI help")
	}
	for _, args := range [][]string{{"not-json"}, {`{}`, "extra"}} {
		if err := RunIssuesCLI(args, &output); err == nil {
			t.Fatal("accepted invalid CLI arguments")
		}
	}
}
