package libro

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"
)

// Application mode belongs to the project; port overrides belong to a worktree.
type applicationSettings struct {
	Mode string `json:"mode"`
	Port int    `json:"port,omitempty"`
}

func loadApplicationSettings(path string) applicationSettings {
	settings := applicationSettings{Mode: "thread"}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		var value string
		if db.QueryRow(`SELECT value FROM settings WHERE key = ?`, "application:"+path).Scan(&value) == nil {
			_ = json.Unmarshal([]byte(value), &settings)
		}
	}
	return settings
}

func saveApplicationSettings(path string, settings applicationSettings) error {
	if settings.Mode != "shared" && settings.Mode != "thread" {
		return fmt.Errorf("choose shared or per-thread application mode")
	}
	if settings.Port < 0 || settings.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, or blank for automatic")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	_, err = db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "application:"+path, string(data))
	return err
}

func applicationRoot(state *AppState, workspace string) string {
	project, path := threadProjectContext(state, workspace)
	for _, p := range state.Projects {
		if p.Name == project && p.Virtual {
			project = p.ParentProject
			break
		}
	}
	for _, p := range state.Projects {
		if p.Name == project {
			return p.Path
		}
	}
	return path
}

func applicationConfiguration(state *AppState, workspace string) (string, string, applicationSettings) {
	_, path := threadProjectContext(state, workspace)
	root := applicationRoot(state, workspace)
	settings := loadApplicationSettings(root)
	if settings.Mode == "shared" {
		path = root
	} else {
		settings.Port = loadApplicationSettings(path).Port
	}
	command := projectCommand(path)
	if command == "" {
		command = projectCommand(root)
	}
	return path, command, settings
}

func applicationURL(port int) string {
	if port == 0 {
		return ""
	}
	return "http://localhost:" + strconv.Itoa(port)
}

// Pending panels reserve ports within Libro, before their processes bind them.
// The OS availability check is necessarily advisory for arbitrary start commands.
func allocateApplicationPort(requested int) (int, error) {
	sm.mu.RLock()
	used := map[int]bool{}
	collect := func(apps []Application) {
		for _, app := range apps {
			if app.ApplicationPort != 0 && (!app.TerminalReady || tm.IsRunning(app.ID)) {
				used[app.ApplicationPort] = true
			}
		}
	}
	for _, state := range sm.states {
		collect(state.Apps)
		for _, snapshot := range state.snapshots {
			if snapshot != nil {
				collect(snapshot.Apps)
			}
		}
	}
	sm.mu.RUnlock()
	if requested != 0 && used[requested] {
		return 0, fmt.Errorf("port %d is already assigned to another application", requested)
	}
	for range 100 {
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(requested))
		if requested != 0 && errors.Is(err, syscall.EADDRINUSE) {
			if err := killApplicationPort(requested); err != nil {
				return 0, fmt.Errorf("free application port %d: %w", requested, err)
			}
			// Killing a process is asynchronous; wait for its sockets to close.
			deadline := time.Now().Add(2 * time.Second)
			for errors.Is(err, syscall.EADDRINUSE) && time.Now().Before(deadline) {
				listener, err = net.Listen("tcp", ":"+strconv.Itoa(requested))
				if errors.Is(err, syscall.EADDRINUSE) {
					time.Sleep(time.Millisecond)
				}
			}
		}
		if err != nil {
			return 0, fmt.Errorf("application port unavailable: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("could not allocate an application port")
}

func applicationLiveURL(state *AppState, path string) string {
	find := func(apps []Application) string {
		for _, panel := range apps {
			if panel.PluginID == "project-command" && panel.ApplicationPath == path && (!panel.TerminalReady || tm.IsRunning(panel.ID)) {
				return applicationURL(panel.ApplicationPort)
			}
		}
		return ""
	}
	if url := find(state.Apps); url != "" {
		return url
	}
	for _, snapshot := range state.snapshots {
		if snapshot != nil {
			if url := find(snapshot.Apps); url != "" {
				return url
			}
		}
	}
	return ""
}
