package libro

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/yuin/goldmark"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

type noteImage struct {
	ID   string `json:"id,omitempty"`
	Data string `json:"data"`
}

type projectNote struct {
	Images  []noteImage `json:"images"`
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Body    string      `json:"body"`
	State   string      `json:"state"`
	Updated string      `json:"updated"`
}

func loadNotes(project string) ([]projectNote, error) {
	rows, err := db.Query(`SELECT id, title, body, state, updated, images FROM notes WHERE project = ? ORDER BY updated DESC, id`, project)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	notes := []projectNote{}
	for rows.Next() {
		var note projectNote
		var images string
		if err := rows.Scan(&note.ID, &note.Title, &note.Body, &note.State, &note.Updated, &images); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(images), &note.Images); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func saveNote(project string, note projectNote) (projectNote, error) {
	note.Title = strings.TrimSpace(note.Title)
	if note.Title == "" || len(note.Title) > 200 || len(note.Body) > 1024*1024 {
		return note, fmt.Errorf("enter a title up to 200 bytes and a note up to 1 MB")
	}
	if note.State != "new" && note.State != "archived" {
		return note, fmt.Errorf("note state must be new or archived")
	}
	total := 0
	for _, attachment := range note.Images {
		data, _, err := decodeNoteImage(attachment.Data)
		if err != nil {
			return note, err
		}
		total += len(data)
	}
	if total > 4*1024*1024 {
		return note, fmt.Errorf("images must total 4 MB or less")
	}
	images, err := json.Marshal(note.Images)
	if err != nil {
		return note, err
	}
	note.Updated = time.Now().UTC().Format(time.RFC3339Nano)
	if note.ID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return note, err
		}
		note.ID = hex.EncodeToString(id[:])
		_, err := db.Exec(`INSERT INTO notes (id, project, title, body, state, updated, images) VALUES (?, ?, ?, ?, ?, ?, ?)`, note.ID, project, note.Title, note.Body, note.State, note.Updated, string(images))
		return note, err
	}
	result, err := db.Exec(`UPDATE notes SET title = ?, body = ?, state = ?, updated = ?, images = ? WHERE id = ? AND project = ?`, note.Title, note.Body, note.State, note.Updated, string(images), note.ID, project)
	if err != nil {
		return note, err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		err = fmt.Errorf("note not found in this project")
	}
	return note, err
}

func moveNote(project, target, id string) error {
	if target == "" || target == project {
		return fmt.Errorf("choose another project")
	}
	result, err := db.Exec(`UPDATE notes SET project = ?, updated = ? WHERE id = ? AND project = ?`, target, time.Now().UTC().Format(time.RFC3339Nano), id, project)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		err = fmt.Errorf("issue not found in this project")
	}
	return err
}

type noteRequest struct {
	Target  string      `json:"target"`
	SID     string      `json:"sid"`
	ID      string      `json:"id"`
	Project string      `json:"project"`
	Request string      `json:"request"`
	Action  string      `json:"action"`
	Note    projectNote `json:"note"`
	NoteID  string      `json:"noteID"`
	Body    string      `json:"body"`
}

// HTTP allows clipboard images larger than the UI websocket's 1 MB limit.
func registerNotesActions(app *r.App) {
	registerIssueControl(app)
	app.POST("/notes/action", func(w http.ResponseWriter, req *http.Request) {
		if origin := req.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != req.Host {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
		}
		var data noteRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 10*1024*1024)).Decode(&data); err != nil {
			http.Error(w, "note is too large or invalid", http.StatusBadRequest)
			return
		}
		result := handleNoteRequest(data)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}

func handleNoteRequest(data noteRequest) map[string]any {
	result := map[string]any{"id": data.ID, "request": data.Request}
	allowed := false
	projects := []string{}
	targetAllowed := false
	sm.mu.Lock()
	if state := sm.states[data.SID]; state != nil && data.Project == state.ActiveProject {
		for _, project := range state.Projects {
			if project.Name != data.Project {
				projects = append(projects, project.Name)
				targetAllowed = targetAllowed || project.Name == data.Target
			}
		}
		for _, panel := range state.Apps {
			if panel.ID == data.ID && panel.PluginID == "notes" {
				allowed = true
				break
			}
		}
	}
	sm.mu.Unlock()
	var err error
	if !allowed {
		err = fmt.Errorf("switch to this project to use its notes")
	} else {
		switch data.Action {
		case "preview":
			var html bytes.Buffer
			if len(data.Body) > 1024*1024 {
				err = fmt.Errorf("note is too long")
			} else {
				err = goldmark.Convert([]byte(data.Body), &html)
				result["html"] = html.String()
			}
		case "send":
			var notes []projectNote
			notes, err = loadNotes(data.Project)
			if err == nil {
				err = fmt.Errorf("note not found")
				for _, note := range notes {
					if note.ID == data.NoteID {
						result["prompt"], err = notePrompt(note)
						break
					}
				}
			}
		case "move":
			if !targetAllowed {
				err = fmt.Errorf("choose an existing project")
			} else {
				err = moveNote(data.Project, data.Target, data.NoteID)
				result["noteID"] = data.NoteID
			}
		case "save":
			result["note"], err = saveNote(data.Project, data.Note)
		case "list":
			result["notes"], err = loadNotes(data.Project)
			result["projects"] = projects
		default:
			err = fmt.Errorf("unknown notes action")
		}
	}
	if err != nil {
		result["error"] = err.Error()
	}
	return result
}

func renderNotes(app Application) *r.Node {
	return r.Div("ws-notes").Attr("data-notes", app.ID).Render(r.Div("ws-notes-status").Attr("role", "status").Text("Loading notes…"))
}

func decodeNoteImage(value string) ([]byte, string, error) {
	_, encoded, ok := strings.Cut(value, ",")
	if !ok || len(encoded) > 6*1024*1024 {
		return nil, "", fmt.Errorf("paste an image up to 4 MB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("invalid clipboard image")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width > 20000 || config.Height > 20000 {
		return nil, "", fmt.Errorf("use a PNG, JPEG, or GIF image up to 20000 pixels per side")
	}
	if value[:len(value)-len(encoded)] != "data:image/"+format+";base64," {
		return nil, "", fmt.Errorf("invalid image type")
	}
	return data, format, nil
}

func notePrompt(note projectNote) (string, error) {
	prompt := "Execute this note:\n\n" + note.Title + "\n\n" + note.Body
	if len(note.Images) == 0 {
		return prompt, nil
	}
	dir, err := libroDataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "note-images", note.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for i, attachment := range note.Images {
		data, format, err := decodeNoteImage(attachment.Data)
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, fmt.Sprintf("image-%d.%s", i+1, format))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", err
		}
		if attachment.ID != "" {
			prompt = strings.ReplaceAll(prompt, "note-image:"+attachment.ID, filepath.ToSlash(path))
		}
		prompt += "\n\nAttached image: " + path
	}
	return prompt, nil
}
