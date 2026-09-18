package libro

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// Thread is a single agent session, independent of registered projects.
type Thread struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
}

func loadThreads() []Thread {
	rows, err := db.Query("SELECT id, name, archived FROM threads ORDER BY rowid")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var threads []Thread
	for rows.Next() {
		var thread Thread
		if rows.Scan(&thread.ID, &thread.Name, &thread.Archived) == nil {
			threads = append(threads, thread)
		}
	}
	return threads
}

func (s *AppState) thread(id string) *Thread {
	for i := range s.Threads {
		if s.Threads[i].ID == id {
			return &s.Threads[i]
		}
	}
	return nil
}

func threadsJS(state *AppState) string {
	data, _ := json.Marshal(state.Threads)
	return "window.__libroThreads=" + string(data) + ";"
}

func registerThreadActions(app *r.App, switchWorkspace func(string, string) string) {
	registerAction(app, "thread.create", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		name, _ := ctx.WsData()["name"].(string)
		name = strings.TrimSpace(name)
		if len(name) > 200 {
			return r.Notify("error", "Enter a thread name (up to 200 characters)")
		}
		if name == "" {
			name = "New thread"
		}
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return r.Notify("error", "Could not create thread")
		}
		thread := Thread{ID: "thread:" + hex.EncodeToString(random[:]), Name: name}
		if _, err := db.Exec("INSERT INTO threads (id, name) VALUES (?, ?)", thread.ID, thread.Name); err != nil {
			return r.Notify("error", "Could not save thread")
		}
		sm.Get(sid)
		sm.mu.Lock()
		sm.states[sid].Threads = append(sm.states[sid].Threads, thread)
		sm.mu.Unlock()
		return switchWorkspace(sid, thread.ID)
	})
	registerAction(app, "thread.rename", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		data := ctx.WsData()
		id, _ := data["id"].(string)
		name, _ := data["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		if len(name) > 200 {
			name = name[:200]
		}
		state := sm.Get(sid)
		thread := state.thread(id)
		// Agent titles arrive on every status change; skip unchanged names.
		if thread == nil || thread.Name == name {
			return ""
		}
		if _, err := db.Exec("UPDATE threads SET name = ? WHERE id = ?", name, id); err != nil {
			return ""
		}
		sm.mu.Lock()
		thread.Name = name
		sm.mu.Unlock()
		return projectsJS(state)
	})
	registerAction(app, "thread.archive", func(ctx *r.Context) string {
		sid := extractSID(ctx)
		id, _ := ctx.WsData()["id"].(string)
		archived, _ := ctx.WsData()["archived"].(bool)
		state := sm.Get(sid)
		if state.thread(id) == nil {
			return r.Notify("error", "Thread not found")
		}
		if _, err := db.Exec("UPDATE threads SET archived = ? WHERE id = ?", archived, id); err != nil {
			return r.Notify("error", "Could not save thread")
		}
		sm.mu.Lock()
		state.thread(id).Archived = archived
		sm.mu.Unlock()
		// Archiving keeps the workspace alive and recoverable without interrupting commands.
		return projectsJS(state) + fmt.Sprintf("if(window.libroWorkspace)libroWorkspace.threadArchived(%t);", archived)
	})
}

func (s *AppState) canStartThreadApp(app Application) bool {
	if appDock(app) != "center" {
		return true
	}
	if !isAgentApp(app) {
		return false
	}
	for _, existing := range s.Apps {
		if appDock(existing) == "center" {
			return false
		}
	}
	return true
}
