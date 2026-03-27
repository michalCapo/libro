package main

import (
	"fmt"
	"os"
	"sync"
)

// AppType distinguishes between web URL apps and terminal (ttyd) apps
type AppType string

const (
	AppTypeURL      AppType = "url"
	AppTypeTerminal AppType = "terminal"
)

// Application represents a single web application displayed in an iframe
type Application struct {
	ID       string
	Type     AppType
	URL      string // iframe source URL (for terminal apps, this is http://localhost:<port>)
	Command  string // original command (only for terminal apps)
	Width    Width
	Port     int    // ttyd port (only for terminal apps)
	Writable bool   // ttyd --writable flag (only for terminal apps)
	Name     string // optional display name
}

// Project represents a named working directory
type Project struct {
	Name string
	Path string
}

// projectSnapshot stores a project's apps while it is not active
type projectSnapshot struct {
	Apps          []Application
	SelectedIndex int
}

// AppState holds the per-session state
type AppState struct {
	Apps          []Application
	SelectedIndex int
	DialogOpen    bool
	DialogSide    string // "left" or "right" — which side the dialog was opened from

	// Project state
	Projects          []Project
	ActiveProject     string
	ProjectDialogOpen bool
	snapshots         map[string]*projectSnapshot
}

// StateManager manages per-session app states
type StateManager struct {
	mu       sync.RWMutex
	states   map[string]*AppState
	nextID   int
	nextPort int
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	return &StateManager{
		states:   make(map[string]*AppState),
		nextPort: 7680, // start ttyd ports from 7681
	}
}

func defaultHomeDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	return home
}

// NewSession creates a new session and returns its ID
func (sm *StateManager) NewSession() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextID++
	sid := fmt.Sprintf("session-%d", sm.nextID)
	sm.states[sid] = &AppState{
		Projects: []Project{
			{Name: "home", Path: defaultHomeDir()},
		},
		ActiveProject: "home",
		snapshots:     make(map[string]*projectSnapshot),
	}
	return sid
}

// Get returns the state for a session, creating one if it doesn't exist
func (sm *StateManager) Get(sessionID string) *AppState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.states[sessionID]; ok {
		return s
	}
	s := &AppState{
		Projects: []Project{
			{Name: "home", Path: defaultHomeDir()},
		},
		ActiveProject: "home",
		snapshots:     make(map[string]*projectSnapshot),
	}
	sm.states[sessionID] = s
	return s
}

// NextPort returns the next available port for ttyd
func (sm *StateManager) NextPort() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextPort++
	return sm.nextPort
}

// addApp is the internal helper that inserts a URL app at the given position
func (sm *StateManager) addApp(sessionID, url string, width Width, name string, prepend bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{
			Projects:      []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject: "home",
			snapshots:     make(map[string]*projectSnapshot),
		}
		sm.states[sessionID] = s
	}
	sm.nextID++
	app := Application{
		ID:    fmt.Sprintf("app-%d", sm.nextID),
		Type:  AppTypeURL,
		URL:   url,
		Width: width,
		Name:  name,
	}
	if prepend {
		s.Apps = append([]Application{app}, s.Apps...)
		s.SelectedIndex = 0
	} else {
		s.Apps = append(s.Apps, app)
		s.SelectedIndex = len(s.Apps) - 1
	}
}

// AddApp adds a new URL application to the end of the session's app list
func (sm *StateManager) AddApp(sessionID, url string, width Width, name string) {
	sm.addApp(sessionID, url, width, name, false)
}

// PrependApp adds a new URL application to the beginning of the session's app list
func (sm *StateManager) PrependApp(sessionID, url string, width Width, name string) {
	sm.addApp(sessionID, url, width, name, true)
}

// addTerminalApp is the internal helper that inserts a terminal app at the given position
func (sm *StateManager) addTerminalApp(sessionID string, command string, port int, writable bool, width Width, name string, prepend bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{
			Projects:      []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject: "home",
			snapshots:     make(map[string]*projectSnapshot),
		}
		sm.states[sessionID] = s
	}
	sm.nextID++
	app := Application{
		ID:       fmt.Sprintf("app-%d", sm.nextID),
		Type:     AppTypeTerminal,
		URL:      fmt.Sprintf("/ttyd/%d/", port),
		Command:  command,
		Width:    width,
		Port:     port,
		Writable: writable,
		Name:     name,
	}
	if prepend {
		s.Apps = append([]Application{app}, s.Apps...)
		s.SelectedIndex = 0
	} else {
		s.Apps = append(s.Apps, app)
		s.SelectedIndex = len(s.Apps) - 1
	}
}

// AddTerminalApp adds a new terminal application to the end of the session's app list
func (sm *StateManager) AddTerminalApp(sessionID string, command string, port int, writable bool, width Width, name string) {
	sm.addTerminalApp(sessionID, command, port, writable, width, name, false)
}

// PrependTerminalApp adds a new terminal application to the beginning of the session's app list
func (sm *StateManager) PrependTerminalApp(sessionID string, command string, port int, writable bool, width Width, name string) {
	sm.addTerminalApp(sessionID, command, port, writable, width, name, true)
}

// RemoveApp removes an application by index and returns it (for cleanup)
func (sm *StateManager) RemoveApp(sessionID string, index int) *Application {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || index < 0 || index >= len(s.Apps) {
		return nil
	}
	removed := s.Apps[index]
	s.Apps = append(s.Apps[:index], s.Apps[index+1:]...)
	// Adjust selected index
	if len(s.Apps) == 0 {
		s.SelectedIndex = 0
	} else if s.SelectedIndex >= len(s.Apps) {
		s.SelectedIndex = len(s.Apps) - 1
	} else if index < s.SelectedIndex {
		s.SelectedIndex--
	}
	return &removed
}

// RemoveAppByID removes an application by its ID and returns it (for cleanup)
func (sm *StateManager) RemoveAppByID(sessionID, appID string) *Application {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return nil
	}
	for i, app := range s.Apps {
		if app.ID == appID {
			removed := s.Apps[i]
			s.Apps = append(s.Apps[:i], s.Apps[i+1:]...)
			if len(s.Apps) == 0 {
				s.SelectedIndex = 0
			} else if s.SelectedIndex >= len(s.Apps) {
				s.SelectedIndex = len(s.Apps) - 1
			} else if i < s.SelectedIndex {
				s.SelectedIndex--
			}
			return &removed
		}
	}
	return nil
}

// SetAppWidthByID sets the width of an app by its ID and returns the app's current index
func (sm *StateManager) SetAppWidthByID(sessionID, appID string, width Width) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return -1
	}
	for i, app := range s.Apps {
		if app.ID == appID {
			s.Apps[i].Width = width
			return i
		}
	}
	return -1
}

// SetAppURLByID changes the URL of an app by its ID. Returns the app index or -1.
func (sm *StateManager) SetAppURLByID(sessionID, appID, newURL string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return -1
	}
	for i, app := range s.Apps {
		if app.ID == appID {
			s.Apps[i].URL = newURL
			return i
		}
	}
	return -1
}

// NavigateLeft shifts focus to the previous app
func (sm *StateManager) NavigateLeft(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil && s.SelectedIndex > 0 {
		s.SelectedIndex--
	}
}

// NavigateRight shifts focus to the next app
func (sm *StateManager) NavigateRight(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil && s.SelectedIndex < len(s.Apps)-1 {
		s.SelectedIndex++
	}
}

// SelectApp sets the selected app index
func (sm *StateManager) SelectApp(sessionID string, index int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil && index >= 0 && index < len(s.Apps) {
		s.SelectedIndex = index
	}
}

// SetAppWidth sets the width of an app by index
func (sm *StateManager) SetAppWidth(sessionID string, index int, width Width) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || index < 0 || index >= len(s.Apps) {
		return
	}
	s.Apps[index].Width = width
}

// OpenDialog sets the dialog open flag and records which side it was opened from
func (sm *StateManager) OpenDialog(sessionID, side string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.DialogOpen = true
		s.DialogSide = side
	}
}

// CloseDialog clears the dialog open flag
func (sm *StateManager) CloseDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.DialogOpen = false
	}
}

// AddProject adds a new project to the session. Returns false if name already exists.
func (sm *StateManager) AddProject(sessionID, name, path string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return false
	}
	for _, p := range s.Projects {
		if p.Name == name {
			return false
		}
	}
	s.Projects = append(s.Projects, Project{Name: name, Path: path})
	return true
}

// SwitchProject switches the active project, saving and restoring app state
func (sm *StateManager) SwitchProject(sessionID, projectName string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return false
	}

	// Verify project exists
	found := false
	for _, p := range s.Projects {
		if p.Name == projectName {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	if s.ActiveProject == projectName {
		return true
	}

	// Save current project's apps
	s.snapshots[s.ActiveProject] = &projectSnapshot{
		Apps:          s.Apps,
		SelectedIndex: s.SelectedIndex,
	}

	// Load target project's apps
	if snap, ok := s.snapshots[projectName]; ok {
		s.Apps = snap.Apps
		s.SelectedIndex = snap.SelectedIndex
		delete(s.snapshots, projectName)
	} else {
		s.Apps = nil
		s.SelectedIndex = 0
	}

	s.ActiveProject = projectName
	return true
}

// GetActiveProjectPath returns the folder path for the active project
func (sm *StateManager) GetActiveProjectPath(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return defaultHomeDir()
	}
	for _, p := range s.Projects {
		if p.Name == s.ActiveProject {
			return p.Path
		}
	}
	return defaultHomeDir()
}

// OpenProjectDialog sets the project dialog open flag
func (sm *StateManager) OpenProjectDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.ProjectDialogOpen = true
	}
}

// CloseProjectDialog clears the project dialog open flag
func (sm *StateManager) CloseProjectDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.ProjectDialogOpen = false
	}
}
