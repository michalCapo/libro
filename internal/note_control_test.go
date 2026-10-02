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

func setupNoteControl(t *testing.T) string {
	t.Helper()
	oldDB, oldSM := db, sm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "notes.db"))
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

func TestNoteControlLifecycle(t *testing.T) {
	path := setupNoteControl(t)
	call := func(command noteCommand) any {
		t.Helper()
		command.Project = path
		result, err := controlNotes("test", command)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := call(noteCommand{Action: "create", Title: " Fix login ", Body: "**Steps**\nTry signing in"}).(projectNote)
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
	call(noteCommand{Action: "set_status", ID: created.ID, Status: "archived"})
	read := call(noteCommand{Action: "read", ID: created.ID}).(projectNote)
	if read.State != "archived" || read.Title != created.Title || read.Body != created.Body || !reflect.DeepEqual(read.Images, created.Images) {
		t.Fatal("status update changed note content")
	}
	call(noteCommand{Action: "set_status", ID: created.ID, Status: "new"})
	other, err := saveNote("two", projectNote{Title: "Other project", State: "new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"read", "set_status", "delete"} {
		if _, err := controlNotes("test", noteCommand{Project: path, Action: action, ID: other.ID, Status: "archived"}); err == nil {
			t.Fatalf("%s accessed another project", action)
		}
	}
	call(noteCommand{Action: "delete", ID: created.ID})
	for _, action := range []string{"read", "set_status", "delete"} {
		if _, err := controlNotes("test", noteCommand{Project: path, Action: action, ID: created.ID, Status: "new"}); err == nil {
			t.Fatalf("%s accepted deleted note", action)
		}
	}
	// A stale UI editor cannot recreate a deleted note by saving it.
	if _, err := saveNote("one", created); err == nil {
		t.Fatal("stale save resurrected deleted note")
	}
	if _, err := readNote("two", other.ID); err != nil {
		t.Fatal("delete affected another note")
	}
}

func TestNoteControlUsesProjectFromThread(t *testing.T) {
	path := setupNoteControl(t)
	state := sm.states["test"]
	state.Threads = []Thread{{ID: "thread:test", Project: "one", Path: path}}
	state.ActiveProject = "thread:test"

	result, err := controlNotes("test", noteCommand{Project: path, Action: "create", Title: "From thread"})
	if err != nil || result.(projectNote).Title != "From thread" {
		t.Fatalf("thread note control failed: %v, %+v", err, result)
	}
	notes, err := loadNotes("one")
	if err != nil || len(notes) != 1 {
		t.Fatalf("note was not stored under project: %+v, %v", notes, err)
	}
}

func TestNotesSharedAcrossWorktrees(t *testing.T) {
	path := setupNoteControl(t)
	state := sm.states["test"]
	state.Apps = []Application{{ID: "notes", PluginID: "notes", Dock: "right"}}
	worktree := filepath.Join(t.TempDir(), "worktree")
	legacy, err := saveNote("one/feature", projectNote{Title: "Existing worktree note", State: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if !sm.AddVirtualProject("test", "one/feature", worktree, "one") {
		t.Fatal("could not register worktree")
	}
	if note, err := readNote("one", legacy.ID); err != nil || note.Title != legacy.Title {
		t.Fatalf("worktree note was not migrated: %+v, %v", note, err)
	}
	state.Threads = []Thread{{ID: "thread:feature", Project: "one/feature", Path: worktree}}
	for _, workspace := range []string{"one", "one/feature", "thread:feature"} {
		state.ActiveProject = workspace
		for _, requested := range []string{path, worktree} {
			result, err := controlNotes("test", noteCommand{Project: requested, Action: "read", ID: legacy.ID})
			if err != nil || result.(projectNote).ID != legacy.ID {
				t.Fatalf("%s cannot read shared note from %s: %v", workspace, requested, err)
			}
		}
		result := handleNoteRequest(noteRequest{SID: "test", ID: "notes", Project: "one", Action: "list"})
		if result["error"] != nil || len(result["notes"].([]projectNote)) != 1 {
			t.Fatalf("%s panel cannot read shared notes: %v", workspace, result)
		}
		if targets := result["projects"].([]string); !reflect.DeepEqual(targets, []string{"two"}) {
			t.Fatalf("worktrees offered as move targets: %v", targets)
		}
	}
	created, err := controlNotes("test", noteCommand{Project: worktree, Action: "create", Title: "From worktree"})
	if err != nil {
		t.Fatal(err)
	}
	state.ActiveProject = "one"
	if _, err := controlNotes("test", noteCommand{Project: path, Action: "set_status", ID: created.(projectNote).ID, Status: "archived"}); err != nil {
		t.Fatal(err)
	}
	state.ActiveProject = "one/feature"
	if _, err := controlNotes("test", noteCommand{Project: state.Projects[1].Path, Action: "list"}); err == nil {
		t.Fatal("worktree could access another project")
	}
}

func TestNotesPanelMovesAcrossWorktrees(t *testing.T) {
	setupNoteControl(t)
	state := sm.states["test"]
	state.Projects = append(state.Projects, Project{Name: "one/feature", Virtual: true, ParentProject: "one"})
	state.Apps = []Application{{ID: "notes", PluginID: "notes", Dock: "right"}, {ID: "agent", PluginID: "codex", Dock: "center"}}
	moved := sm.MoveSharedProjectApps("test", "one/feature")
	if len(moved) != 1 || moved[0].ID != "notes" || len(state.Apps) != 1 || state.Apps[0].ID != "agent" {
		t.Fatalf("notes panel was not shared with worktree: %+v", moved)
	}
	if !sm.SwitchProject("test", "one/feature") || len(state.Apps) != 1 || state.Apps[0].ID != "notes" {
		t.Fatal("worktree did not reuse notes panel")
	}
}

func TestNoteControlListAndValidation(t *testing.T) {
	path := setupNoteControl(t)
	for _, status := range []string{"new", "archived", "new"} {
		if _, err := controlNotes("test", noteCommand{Project: path, Action: "create", Title: "Task", Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(command noteCommand) map[string]any {
		t.Helper()
		command.Project, command.Action = path, "list"
		result, err := controlNotes("test", command)
		if err != nil {
			t.Fatal(err)
		}
		return result.(map[string]any)
	}
	first := list(noteCommand{Limit: 1})
	second := list(noteCommand{Limit: 1, Offset: 1})
	if first["hasMore"] != true || first["notes"].([]map[string]string)[0]["id"] == second["notes"].([]map[string]string)[0]["id"] {
		t.Fatal("pagination repeated note")
	}
	archived := list(noteCommand{Status: "archived"})
	if len(archived["notes"].([]map[string]string)) != 1 || archived["hasMore"] != false {
		t.Fatal("status filter failed")
	}
	empty := list(noteCommand{Offset: 100})
	if len(empty["notes"].([]map[string]string)) != 0 {
		t.Fatal("empty page contains notes")
	}
	for _, command := range []noteCommand{
		{Action: "create", Title: " "}, {Action: "create", Title: "Task", Status: "done"}, {Action: "create", Title: "Task", ID: "existing"},
		{Action: "set_status", ID: "missing"}, {Action: "read"}, {Action: "delete"}, {Action: "unknown"},
		{Action: "list", Limit: -1}, {Action: "list", Limit: 201}, {Action: "list", Offset: -1},
	} {
		command.Project = path
		if _, err := controlNotes("test", command); err == nil {
			t.Fatalf("accepted invalid command: %+v", command)
		}
	}
	for _, sid := range []string{"test", "missing"} {
		if _, err := controlNotes(sid, noteCommand{Project: "/other", Action: "list"}); err == nil {
			t.Fatal("accepted wrong project/session")
		}
	}
	sm.states["test"].ActiveProject = ""
	if _, err := controlNotes("test", noteCommand{Project: path, Action: "list"}); err == nil {
		t.Fatal("accepted no active project")
	}
}

func TestNoteControlHTTP(t *testing.T) {
	path := setupNoteControl(t)
	app := r.NewApp()
	registerNoteControl(app)
	handler := app.Handler()
	// Exercise the same endpoint called from Electron, with no Notes panel open.
	payload, err := json.Marshal(map[string]any{"sid": "test", "command": noteCommand{Action: "create", Project: path, Title: "Via bridge"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/notes/agent", bytes.NewReader(payload))
	reply := httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), "Via bridge") {
		t.Fatalf("create: %d %s", reply.Code, reply.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/notes/agent", bytes.NewReader(payload))
	req.Header.Set("Origin", "https://untrusted.example")
	reply = httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	if reply.Code != http.StatusForbidden {
		t.Fatal("accepted foreign origin")
	}
}
