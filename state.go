package main

import (
	"fmt"
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
	Port     int  // ttyd port (only for terminal apps)
	Writable bool // ttyd --writable flag (only for terminal apps)
}

// AppState holds the per-session state
type AppState struct {
	Apps          []Application
	SelectedIndex int
	DialogOpen    bool
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

// NewSession creates a new session and returns its ID
func (sm *StateManager) NewSession() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextID++
	sid := fmt.Sprintf("session-%d", sm.nextID)
	sm.states[sid] = &AppState{}
	return sid
}

// Get returns the state for a session, creating one if it doesn't exist
func (sm *StateManager) Get(sessionID string) *AppState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.states[sessionID]; ok {
		return s
	}
	s := &AppState{}
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

// AddApp adds a new URL application to the session's state
func (sm *StateManager) AddApp(sessionID, url string, width Width) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{}
		sm.states[sessionID] = s
	}
	sm.nextID++
	app := Application{
		ID:    fmt.Sprintf("app-%d", sm.nextID),
		Type:  AppTypeURL,
		URL:   url,
		Width: width,
	}
	s.Apps = append(s.Apps, app)
	s.SelectedIndex = len(s.Apps) - 1
}

// AddTerminalApp adds a new terminal (ttyd) application to the session's state
func (sm *StateManager) AddTerminalApp(sessionID string, command string, port int, writable bool, width Width) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{}
		sm.states[sessionID] = s
	}
	sm.nextID++
	app := Application{
		ID:       fmt.Sprintf("app-%d", sm.nextID),
		Type:     AppTypeTerminal,
		URL:      fmt.Sprintf("http://localhost:%d", port),
		Command:  command,
		Width:    width,
		Port:     port,
		Writable: writable,
	}
	s.Apps = append(s.Apps, app)
	s.SelectedIndex = len(s.Apps) - 1
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

// OpenDialog sets the dialog open flag
func (sm *StateManager) OpenDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.DialogOpen = true
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
