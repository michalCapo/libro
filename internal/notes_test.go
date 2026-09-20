package libro

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	r "github.com/michalCapo/g-sui/ui"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotesPersistenceAndIsolation(t *testing.T) {
	original := db
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); db = original })
	createTables()
	note, err := saveNote("one", projectNote{Title: "Fix the bug", Body: "**Steps**\n- Reproduce", State: "new"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveNote("one", projectNote{Title: "Another task", State: "new"})
	if err != nil || second.ID == note.ID {
		t.Fatalf("multiple notes: %v", err)
	}
	note.Body = "Updated"
	note.State = "archived"
	if _, err := saveNote("two", note); err == nil {
		t.Fatal("updated another project's note")
	}
	if _, err := saveNote("one", note); err != nil {
		t.Fatal(err)
	}
	notes, err := loadNotes("one")
	if err != nil || len(notes) != 2 || notes[0].State != "archived" || notes[0].Body != "Updated" {
		t.Fatalf("persistence: %+v, %v", notes, err)
	}
	notes, err = loadNotes("two")
	if err != nil || len(notes) != 0 {
		t.Fatalf("project isolation: %+v, %v", notes, err)
	}
	for _, invalid := range []projectNote{{Title: " ", State: "new"}, {Title: "Invalid", State: "done"}, {Title: "Image", State: "new", Images: []noteImage{{Data: "data:image/png;base64,bad"}}}} {
		if _, err := saveNote("one", invalid); err == nil {
			t.Fatal("accepted invalid note")
		}
	}
}

func TestNoteImagesAndAgentPrompt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bytes, err := os.ReadFile("../winres/icon16.png")
	if err != nil {
		t.Fatal(err)
	}
	image := noteImage{Data: "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes)}
	note := projectNote{ID: "test", Title: "Fix layout", Body: "Use **this** image", Images: []noteImage{image}}
	prompt, err := notePrompt(note)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.Split(prompt, "Attached image: ")[1]
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != string(bytes) {
		t.Fatalf("image export: %v", err)
	}
	if !strings.Contains(prompt, note.Body) || !strings.Contains(prompt, note.Title) {
		t.Fatal("prompt lost note content")
	}
	if _, _, err := decodeNoteImage(strings.Replace(image.Data, "image/png", "image/jpeg", 1)); err == nil {
		t.Fatal("accepted mismatched image type")
	}
}

func TestNotesRequestAuthorizationAndMarkdown(t *testing.T) {
	original := sm
	sm = NewStateManager()
	t.Cleanup(func() { sm = original })
	sm.states["session"] = &AppState{ActiveProject: "one", Apps: []Application{{ID: "notes", PluginID: "notes"}}}
	req := noteRequest{SID: "session", ID: "notes", Project: "one", Action: "preview", Body: "# Heading\n\n**Bold**\n\n<script>alert(1)</script>"}
	result := handleNoteRequest(req)
	html, ok := result["html"].(string)
	if !ok || !strings.Contains(html, "<strong>Bold</strong>") || strings.Contains(html, "<script>") {
		t.Fatalf("unsafe or invalid Markdown: %v", result)
	}
	req.Project = "two"
	if handleNoteRequest(req)["error"] == nil {
		t.Fatal("allowed another project")
	}
	req.Project = "one"
	req.ID = "terminal"
	if handleNoteRequest(req)["error"] == nil {
		t.Fatal("allowed a non-notes panel")
	}
	req.ID = "notes"
	req.SID = "unknown"
	if handleNoteRequest(req)["error"] == nil {
		t.Fatal("allowed an unknown session")
	}
}

func TestNotesHTTPLargeImage(t *testing.T) {
	originalDB, originalSM := db, sm
	var err error
	db, err = sql.Open("sqlite", filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	sm = NewStateManager()
	t.Cleanup(func() { _ = db.Close(); db, sm = originalDB, originalSM })
	createTables()
	sm.states["session"] = &AppState{ActiveProject: "one", Apps: []Application{{ID: "notes", PluginID: "notes"}}}
	png, err := os.ReadFile("../winres/icon16.png")
	if err != nil {
		t.Fatal(err)
	}
	// Valid PNG with padding exceeds the UI websocket limit after encoding.
	png = append(png, make([]byte, 1024*1024)...)
	note := projectNote{Title: "Screenshot", State: "new", Images: []noteImage{{Data: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}}}
	data, err := json.Marshal(noteRequest{SID: "session", ID: "notes", Project: "one", Action: "save", Note: note})
	if err != nil {
		t.Fatal(err)
	}
	app := r.NewApp()
	registerNotesActions(app)
	handler := app.Handler()
	request := httptest.NewRequest(http.MethodPost, "/notes/action", bytes.NewReader(data))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("large image rejected: %d %s", response.Code, response.Body)
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Error != "" {
		t.Fatalf("save failed: %v %s", err, result.Error)
	}
	notes, err := loadNotes("one")
	if err != nil || len(notes) != 1 || len(notes[0].Images) != 1 || notes[0].Images[0].Data != note.Images[0].Data {
		t.Fatalf("image did not persist: %v", err)
	}
	request = httptest.NewRequest(http.MethodPost, "/notes/action", bytes.NewReader(data))
	request.Header.Set("Origin", "https://untrusted.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatal("allowed a cross-origin request")
	}
}
