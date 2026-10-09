package libro

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
)

// startBackendControl exposes only local agent tools, never a public HTTP route.
// The descriptor is private to the server account and its child processes.
func startBackendControl(app *r.App) (func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		_ = listener.Close()
		return nil, err
	}
	token := hex.EncodeToString(secret[:])
	path, err := controlConnectionPath()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	connection, err := json.Marshal(struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}{listener.Addr().(*net.TCPAddr).Port, token})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		var file *os.File
		file, err = os.CreateTemp(filepath.Dir(path), ".backend-control-*")
		if err == nil {
			defer func() { _ = os.Remove(file.Name()) }()
			_, err = file.Write(connection)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(file.Name(), path)
			}
		}
	}
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &http.Server{Handler: backendControlHandler(app, token, backendSessionID()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("libro: backend control: %v", err)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = server.Close()
			// A newer instance must keep its own connection descriptor.
			if data, err := os.ReadFile(path); err == nil && string(data) == string(connection) {
				_ = os.Remove(path)
			}
		})
	}, nil
}

func backendControlHandler(app *r.App, token, sid string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(request.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/" {
			http.Error(w, "expected POST /", http.StatusMethodNotAllowed)
			return
		}
		var command backendCommand
		decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&command)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("expected one command")
			}
		}
		var result any
		if err == nil {
			backendActionMu.Lock()
			var patch r.Result
			patch, result, err = dispatchBackendCommand(sid, command)
			if broadcastErr := app.Broadcast(patch); broadcastErr != nil {
				log.Printf("libro: backend control update: %v", broadcastErr)
			}
			backendActionMu.Unlock()
		}
		reply := map[string]any{"result": result}
		if err != nil {
			reply["error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			log.Printf("libro: backend control reply: %v", err)
		}
	})
}

type backendCommand struct {
	Action    string       `json:"action"`
	Operation string       `json:"operation,omitempty"`
	Project   string       `json:"project"`
	Command   childCommand `json:"command"`
	Note      noteCommand  `json:"note"`
}

// dispatchBackendCommand operates on the persistent backend workspace. It does not
// need a browser, and lifecycle commands do not select a different workspace.
func dispatchBackendCommand(sid string, command backendCommand) (r.Result, any, error) {
	if sid == "" {
		return r.Result{}, nil, errors.New("libro backend workspace is unavailable")
	}
	switch command.Action {
	case "application":
		patch, result, err := controlApplication(sid, command.Project, command.Operation)
		if err != nil || command.Operation != "start" && command.Operation != "restart" {
			return patch, result, err
		}
		workspace, _, err := applicationProject(sm.Get(sid), command.Project)
		if err != nil {
			return patch, result, err
		}
		path, _, settings := applicationConfiguration(sm.Get(sid), workspace)
		perThread := settings.Mode == "thread"
		for _, running := range sm.GetAllRunningApps(sid) {
			root := applicationRoot(sm.Get(sid), running.Name)
			if perThread && running.Name != workspace || !perThread && (root == "" || applicationPath(root) != applicationPath(path)) {
				continue
			}
			for _, panel := range running.Apps {
				if panel.PluginID == "project-command" && panel.ApplicationPerThread == perThread && !panel.TerminalReady {
					hydrated, _ := hydrateProjectCommand(sid, panel.ID)
					patch = patch.Add(hydrated)
					if _, _, _, found := sm.workspaceApp(sid, panel.ID); !found {
						return patch, nil, errors.New("failed to start the configured application")
					}
				}
			}
		}
		_, result, err = controlApplication(sid, command.Project, "status")
		return patch, result, err
	case "notes":
		result, err := controlNotes(sid, command.Note)
		return clientScript("window.libroNotes?.refresh();"), result, err
	case "children":
		return controlChildren(sid, command.Project, command.Command)
	default:
		return r.Result{}, nil, errors.New("unknown backend control action")
	}
}
