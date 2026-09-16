package libro

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// AppType distinguishes between web URL apps and terminal apps
type AppType string

const (
	AppTypeURL      AppType = "url"
	AppTypeTerminal AppType = "terminal"
)

// Application represents a single web application displayed in an iframe
type Application struct {
	PluginID      string
	Dock          string
	ID            string
	Type          AppType
	URL           string // iframe source URL (for terminal apps, this is http://localhost:<port>)
	Command       string // original command (only for terminal apps)
	Width         Width
	PreviousWidth Width  // width before toggling to full (for ⌘+F maximize toggle)
	Writable      bool   // whether terminal input is accepted
	Name          string // optional display name
	IconURL       string // cached icon URL from DB (only for terminal apps)
	TerminalID    string // native PTY terminal ID (usually same as ID)
	TerminalReady bool   // native PTY session is running
}

// Project represents a named working directory
type Project struct {
	Name          string
	Path          string
	IsGitRepo     bool   // detected at load time, not persisted
	Virtual       bool   // true for worktree-derived projects
	ParentProject string // name of parent project (for virtual projects)
	Transient     bool   // true for session-only folder projects
}

// projectSnapshot stores a project's apps while it is not active
type projectSnapshot struct {
	Apps          []Application
	SelectedIndex int
}

func cloneApplications(apps []Application) []Application {
	if len(apps) == 0 {
		return nil
	}
	return append([]Application(nil), apps...)
}

// AppState holds the per-session state
type AppState struct {
	Apps          []Application
	SelectedIndex int

	// Project state
	Projects          []Project
	ActiveProject     string
	ProjectDialogOpen bool
	snapshots         map[string]*projectSnapshot

	renderedProjects map[string]bool // tracks which projects have DOM divs

	// LastAppCreatedProject tracks the project where the last app was created
	LastAppCreatedProject string

	// WorktreeDialogOpen tracks whether the worktree popup is shown
	WorktreeDialogOpen bool
}

// StateManager manages per-session app states
type StateManager struct {
	mu     sync.RWMutex
	states map[string]*AppState
	nextID int
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	return &StateManager{
		states: make(map[string]*AppState),
	}
}

func defaultHomeDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	return home
}

// NewSession creates a new session and returns its ID.
// Projects are loaded from the database.
func (sm *StateManager) NewSession() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextID++
	sid := fmt.Sprintf("session-%d", sm.nextID)
	sm.states[sid] = newAppStateFromDB()
	return sid
}

func newAppStateFromDB() *AppState {
	projects := DBLoadProjects()
	detectGitRepos(projects)
	rendered := make(map[string]bool)
	if len(projects) > 0 {
		rendered[projects[0].Name] = true
	}

	return &AppState{
		Projects:      projects,
		ActiveProject: projects[0].Name,
		snapshots:     make(map[string]*projectSnapshot),

		renderedProjects: rendered,
	}
}

// EnsureSession returns an existing session when possible, or creates one with
// the requested ID. This lets a renderer reload keep the same in-memory strip.
func (sm *StateManager) EnsureSession(sessionID string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sessionID != "" {
		if _, ok := sm.states[sessionID]; ok {
			return sessionID
		}
		sm.states[sessionID] = newAppStateFromDB()
		return sessionID
	}
	sm.nextID++
	sid := fmt.Sprintf("session-%d", sm.nextID)
	sm.states[sid] = newAppStateFromDB()
	return sid
}

// Get returns the state for a session, creating one if it doesn't exist
// IterTerminalApps invokes fn for every terminal Application across all
// sessions and project snapshots. Read lock is held for the duration, so fn
// must not call back into StateManager mutating methods.
func (sm *StateManager) IterTerminalApps(fn func(app Application)) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, s := range sm.states {
		if s == nil {
			continue
		}
		for _, a := range s.Apps {
			if a.Type == AppTypeTerminal && a.TerminalReady {
				fn(a)
			}
		}
		for _, snap := range s.snapshots {
			if snap == nil {
				continue
			}
			for _, a := range snap.Apps {
				if a.Type == AppTypeTerminal && a.TerminalReady {
					fn(a)
				}
			}
		}
	}
}

func (sm *StateManager) Get(sessionID string) *AppState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.states[sessionID]; ok {
		return s
	}
	s := newAppStateFromDB()
	sm.states[sessionID] = s
	return s
}

// NextAppID returns a unique app ID without adding any app to state.
func (sm *StateManager) NextAppID() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.nextID++
	return fmt.Sprintf("app-%d", sm.nextID)
}

// sortAppsByName sorts apps alphabetically by name (case-insensitive) and
// updates SelectedIndex to point to the app with the given ID.
func sortAppsByName(s *AppState, selectedAppID string) {
	sort.SliceStable(s.Apps, func(i, j int) bool {
		return strings.ToLower(s.Apps[i].Name) < strings.ToLower(s.Apps[j].Name)
	})
	for i, app := range s.Apps {
		if app.ID == selectedAppID {
			s.SelectedIndex = i
			break
		}
	}
}

// addApp is the internal helper that adds a URL app and sorts by name.
func (sm *StateManager) addApp(sessionID, url string, width Width, name string) {
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
	s.Apps = append(s.Apps, app)
	sortAppsByName(s, app.ID)
	s.LastAppCreatedProject = s.ActiveProject
}

// AddApp adds a new URL application, sorted by name.
func (sm *StateManager) AddApp(sessionID, url string, width Width, name string) {
	sm.addApp(sessionID, url, width, name)
}

// InsertApp adds a new URL application at the given index position.
// If index is out of range, it falls back to append + sort by name.
func (sm *StateManager) InsertApp(sessionID, url string, width Width, name string, index int) {
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
	if index < 0 || index > len(s.Apps) {
		s.Apps = append(s.Apps, app)
		sortAppsByName(s, app.ID)
	} else {
		s.Apps = append(s.Apps, Application{})
		copy(s.Apps[index+1:], s.Apps[index:])
		s.Apps[index] = app
		s.SelectedIndex = index
	}
	s.LastAppCreatedProject = s.ActiveProject
}

// addTerminalApp is the internal helper that adds a terminal app and sorts by name.
// appID must be pre-generated via NextAppID to avoid race conditions with terminal startup.
func (sm *StateManager) addTerminalApp(sessionID string, appID string, command string, port int, writable bool, width Width, name string, iconURL string) {
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
	app := Application{
		ID:            appID,
		Type:          AppTypeTerminal,
		Command:       command,
		Width:         width,
		Writable:      writable,
		Name:          name,
		IconURL:       iconURL,
		TerminalID:    appID,
		TerminalReady: port > 0,
	}
	s.Apps = append(s.Apps, app)
	sortAppsByName(s, app.ID)
	s.LastAppCreatedProject = s.ActiveProject
}

// AddTerminalApp adds a new terminal application, sorted by name.
func (sm *StateManager) AddTerminalApp(sessionID string, appID string, command string, port int, writable bool, width Width, name string, iconURL string) {
	sm.addTerminalApp(sessionID, appID, command, port, writable, width, name, iconURL)
}

// InsertTerminalApp adds a new terminal application at the given index position.
// If index is out of range, it falls back to append + sort by name.
func (sm *StateManager) InsertTerminalApp(sessionID string, appID string, command string, port int, writable bool, width Width, name string, iconURL string, index int) {
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
	app := Application{
		ID:            appID,
		Type:          AppTypeTerminal,
		Command:       command,
		Width:         width,
		Writable:      writable,
		Name:          name,
		IconURL:       iconURL,
		TerminalID:    appID,
		TerminalReady: port > 0,
	}
	if index < 0 || index > len(s.Apps) {
		s.Apps = append(s.Apps, app)
		sortAppsByName(s, app.ID)
	} else {
		s.Apps = append(s.Apps, Application{})
		copy(s.Apps[index+1:], s.Apps[index:])
		s.Apps[index] = app
		s.SelectedIndex = index
	}
	s.LastAppCreatedProject = s.ActiveProject
}

// InsertTerminal adds a running terminal with command and port at the given index.
func (sm *StateManager) InsertTerminal(sessionID, appID string, width Width, command string, port int, index int) {
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
	app := Application{
		ID:            appID,
		Type:          AppTypeTerminal,
		Width:         width,
		Command:       command,
		Writable:      true,
		TerminalID:    appID,
		TerminalReady: port > 0,
	}
	if index < 0 || index > len(s.Apps) {
		s.Apps = append(s.Apps, app)
		s.SelectedIndex = len(s.Apps) - 1
	} else {
		s.Apps = append(s.Apps, Application{})
		copy(s.Apps[index+1:], s.Apps[index:])
		s.Apps[index] = app
		s.SelectedIndex = index
	}
	s.LastAppCreatedProject = s.ActiveProject
}

// InsertPendingTerminal adds a terminal placeholder at the given index.
func (sm *StateManager) InsertPendingTerminal(sessionID, appID string, width Width, index int) {
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
	app := Application{
		ID:         appID,
		Type:       AppTypeTerminal,
		Width:      width,
		TerminalID: appID,
	}
	if index < 0 || index > len(s.Apps) {
		s.Apps = append(s.Apps, app)
		s.SelectedIndex = len(s.Apps) - 1
	} else {
		s.Apps = append(s.Apps, Application{})
		copy(s.Apps[index+1:], s.Apps[index:])
		s.Apps[index] = app
		s.SelectedIndex = index
	}
	s.LastAppCreatedProject = s.ActiveProject
}

// InsertTerminalPlaceholder adds a terminal shell before its PTY has been started.
func (sm *StateManager) InsertTerminalPlaceholder(sessionID, appID string, width Width, command string, writable bool, name string, iconURL string, index int) {
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
	app := Application{
		ID:         appID,
		Type:       AppTypeTerminal,
		Width:      width,
		Command:    command,
		Writable:   writable,
		Name:       name,
		IconURL:    iconURL,
		TerminalID: appID,
	}
	if index < 0 || index > len(s.Apps) {
		s.Apps = append(s.Apps, app)
		s.SelectedIndex = len(s.Apps) - 1
	} else {
		s.Apps = append(s.Apps, Application{})
		copy(s.Apps[index+1:], s.Apps[index:])
		s.Apps[index] = app
		s.SelectedIndex = index
	}
	s.LastAppCreatedProject = s.ActiveProject
}

// HydrateTerminalByID attaches native PTY runtime details to an existing
// terminal placeholder.
func (sm *StateManager) HydrateTerminalByID(sessionID, appID string, terminalID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return false
	}
	for i := range s.Apps {
		if s.Apps[i].ID != appID {
			continue
		}
		if s.Apps[i].Type != AppTypeTerminal || s.Apps[i].Command == "" {
			return false
		}
		s.Apps[i].URL = ""
		s.Apps[i].TerminalID = terminalID
		s.Apps[i].TerminalReady = true
		return true
	}
	return false
}

// HydrateTerminalAnywhere attaches native PTY runtime details to a terminal
// placeholder that may live in the active project or in any non-active
// project's snapshot. Returns true on success. The terminalID is set to the
// app's own ID, matching how tm.Start keys its sessions.
func (sm *StateManager) HydrateTerminalAnywhere(sessionID, appID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return false
	}
	hydrate := func(apps []Application) bool {
		for i := range apps {
			if apps[i].ID != appID {
				continue
			}
			if apps[i].Type != AppTypeTerminal || apps[i].Command == "" {
				return false
			}
			apps[i].URL = ""
			apps[i].TerminalID = appID
			apps[i].TerminalReady = true
			return true
		}
		return false
	}
	if hydrate(s.Apps) {
		return true
	}
	for _, snap := range s.snapshots {
		if snap == nil {
			continue
		}
		if hydrate(snap.Apps) {
			return true
		}
	}
	return false
}

// TerminalBelongsToSession reports whether a terminal ID belongs to the active session.
func (sm *StateManager) TerminalBelongsToSession(sessionID, terminalID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil || terminalID == "" {
		return false
	}
	matches := func(apps []Application) bool {
		for _, app := range apps {
			if app.Type != AppTypeTerminal {
				continue
			}
			if app.ID == terminalID || app.TerminalID == terminalID {
				return true
			}
		}
		return false
	}
	if matches(s.Apps) {
		return true
	}
	for _, snap := range s.snapshots {
		if snap != nil && matches(snap.Apps) {
			return true
		}
	}
	return false
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

func applyAppWidth(app *Application, width Width) {
	if app == nil {
		return
	}
	if width == "" {
		width = WidthLG
	}
	if width == WidthFull {
		if app.Width != WidthFull {
			app.PreviousWidth = app.Width
			if app.PreviousWidth == "" {
				app.PreviousWidth = WidthLG
			}
		}
		app.Width = WidthFull
		return
	}
	app.Width = width
	app.PreviousWidth = ""
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
			applyAppWidth(&s.Apps[i], width)
			return i
		}
	}
	return -1
}

// ToggleMaxWidth toggles the selected app between full width and its previous width.
// If the app is already full width, it restores the previous width (or LG if none saved).
// If not full width, it saves the current width and switches to full.
// Returns the new width and app ID, or empty strings if no app is selected.
func (sm *StateManager) ToggleMaxWidth(sessionID string, maxPixels int) (Width, string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || len(s.Apps) == 0 || s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Apps) {
		return "", ""
	}
	app := &s.Apps[s.SelectedIndex]
	if app.Width == WidthFull {
		// Restore previous width
		prev := app.PreviousWidth
		if prev == "" || prev == WidthFull {
			prev = WidthLG
		}
		prev = prev.ClampFixedPixel(maxPixels)
		app.Width = prev
		app.PreviousWidth = ""
		return prev, app.ID
	}
	// Save current width and go full
	applyAppWidth(app, WidthFull)
	return WidthFull, app.ID
}

// StepSelectedAppWidth moves the selected app width by one tier and returns the new width and app ID.
func (sm *StateManager) StepSelectedAppWidth(sessionID string, delta int, maxPixels int) (Width, string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || len(s.Apps) == 0 || s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Apps) {
		return "", ""
	}
	app := &s.Apps[s.SelectedIndex]
	current := app.Width
	if current == "" {
		current = WidthLG
	}
	next := current.Step(delta).ClampFixedPixel(maxPixels)
	applyAppWidth(app, next)
	return next, app.ID
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
	for _, snapshot := range s.snapshots {
		for i := range snapshot.Apps {
			if snapshot.Apps[i].ID == appID {
				snapshot.Apps[i].URL = newURL
				return i
			}
		}
	}
	return -1
}

// SelectedIndex returns the currently selected app index for the session
func (sm *StateManager) SelectedIndex(sessionID string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return 0
	}
	return s.SelectedIndex
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

// MoveAppLeft swaps the selected app with the one to its left
func (sm *StateManager) MoveAppLeft(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || s.SelectedIndex <= 0 {
		return false
	}
	i := s.SelectedIndex
	s.Apps[i], s.Apps[i-1] = s.Apps[i-1], s.Apps[i]
	s.SelectedIndex--
	return true
}

// MoveAppRight swaps the selected app with the one to its right
func (sm *StateManager) MoveAppRight(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || s.SelectedIndex >= len(s.Apps)-1 {
		return false
	}
	i := s.SelectedIndex
	s.Apps[i], s.Apps[i+1] = s.Apps[i+1], s.Apps[i]
	s.SelectedIndex++
	return true
}

// MoveSelectedAppToProject moves the selected app to another project and makes
// that project active.
func (sm *StateManager) MoveSelectedAppToProject(sessionID, projectName string) (*Application, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || projectName == "" || len(s.Apps) == 0 || s.SelectedIndex < 0 || s.SelectedIndex >= len(s.Apps) {
		return nil, false
	}

	found := false
	for _, p := range s.Projects {
		if p.Name == projectName {
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}

	moved := s.Apps[s.SelectedIndex]
	if s.ActiveProject == projectName {
		return &moved, true
	}

	sourceProject := s.ActiveProject
	sourceApps := append([]Application(nil), s.Apps[:s.SelectedIndex]...)
	sourceApps = append(sourceApps, s.Apps[s.SelectedIndex+1:]...)
	sourceSelected := s.SelectedIndex
	if len(sourceApps) == 0 {
		sourceSelected = 0
	} else if sourceSelected >= len(sourceApps) {
		sourceSelected = len(sourceApps) - 1
	}

	s.snapshots[sourceProject] = &projectSnapshot{
		Apps:          sourceApps,
		SelectedIndex: sourceSelected,
	}

	targetApps := []Application(nil)
	if snap, ok := s.snapshots[projectName]; ok && snap != nil {
		targetApps = append(targetApps, snap.Apps...)
		delete(s.snapshots, projectName)
	}
	targetApps = append(targetApps, moved)
	s.Apps = targetApps
	s.SelectedIndex = len(targetApps) - 1
	s.ActiveProject = projectName
	return &moved, true
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
	applyAppWidth(&s.Apps[index], width)
}

// AddProject adds a new persisted project to the session. Returns false if name already exists.
func (sm *StateManager) AddProject(sessionID, name, path string) bool {
	return sm.AddProjectWithOptions(sessionID, name, path, false)
}

// AddProjectWithOptions adds a project to the session. Transient projects are
// session-only and are not saved to the database by callers.
func (sm *StateManager) AddProjectWithOptions(sessionID, name, path string, transient bool) bool {
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
	p := Project{Name: name, Path: path, Transient: transient}
	if GitAvailable() {
		p.IsGitRepo = GitIsRepo(path)
	}
	s.Projects = append(s.Projects, p)
	return true
}

// RemoveProject removes a project from the session. Returns the project's snapshotted apps (for cleanup) and success.
// The "home" project cannot be removed.
func (sm *StateManager) RemoveProject(sessionID, projectName string) ([]Application, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || projectName == "home" {
		return nil, false
	}

	idx := -1
	for i, p := range s.Projects {
		if p.Name == projectName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, false
	}

	s.Projects = append(s.Projects[:idx], s.Projects[idx+1:]...)

	var apps []Application
	if snap, ok := s.snapshots[projectName]; ok {
		apps = snap.Apps
		delete(s.snapshots, projectName)
	}

	delete(s.renderedProjects, projectName)

	if s.ActiveProject == projectName {
		s.ActiveProject = "home"
	}

	return apps, true
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

// ProjectByIndex returns the project name at the given index (0-based), or "" if out of range
func (sm *StateManager) ProjectByIndex(sessionID string, index int) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil || index < 0 || index >= len(s.Projects) {
		return ""
	}
	return s.Projects[index].Name
}

// LastAppProject returns the project where the last app was created, or ""
func (sm *StateManager) LastAppProject(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return ""
	}
	return s.LastAppCreatedProject
}

// IsProjectRendered checks if a project's DOM div has been created.
// If not yet rendered, it marks it as rendered and returns false.
func (sm *StateManager) IsProjectRendered(sessionID, projectName string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return false
	}
	if s.renderedProjects[projectName] {
		return true
	}
	if s.renderedProjects == nil {
		s.renderedProjects = make(map[string]bool)
	}
	s.renderedProjects[projectName] = true
	return false
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

// ProjectApps holds the running apps for a single project
type ProjectApps struct {
	Name string
	Apps []Application
}

// GetAllRunningApps returns all running apps across all projects (active + snapshots),
// ordered by project list order.
func (sm *StateManager) GetAllRunningApps(sessionID string) []ProjectApps {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return nil
	}
	var result []ProjectApps
	for _, p := range s.Projects {
		var apps []Application
		if p.Name == s.ActiveProject {
			apps = s.Apps
		} else if snap, ok := s.snapshots[p.Name]; ok {
			apps = snap.Apps
		}
		if len(apps) > 0 {
			result = append(result, ProjectApps{Name: p.Name, Apps: apps})
		}
	}
	return result
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

// detectGitRepos sets IsGitRepo on each project by checking with git.
func detectGitRepos(projects []Project) {
	if !GitAvailable() {
		return
	}
	for i := range projects {
		projects[i].IsGitRepo = GitIsRepo(projects[i].Path)
	}
}

// AddVirtualProject adds a worktree-derived virtual project to the session.
// Virtual projects are not persisted to the database.
func (sm *StateManager) AddVirtualProject(sessionID, name, path, parentProject string) bool {
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
	s.Projects = append(s.Projects, Project{
		Name:          name,
		Path:          path,
		IsGitRepo:     true,
		Virtual:       true,
		ParentProject: parentProject,
	})
	return true
}

// RemoveVirtualProject removes a virtual project and cleans up its snapshot/apps.
func (sm *StateManager) RemoveVirtualProject(sessionID, name string) ([]Application, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return nil, false
	}

	idx := -1
	for i, p := range s.Projects {
		if p.Name == name && p.Virtual {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, false
	}

	s.Projects = append(s.Projects[:idx], s.Projects[idx+1:]...)

	var apps []Application
	if snap, ok := s.snapshots[name]; ok {
		apps = snap.Apps
		delete(s.snapshots, name)
	}

	delete(s.renderedProjects, name)

	if s.ActiveProject == name {
		s.ActiveProject = "home"
	}

	return apps, true
}

// OpenWorktreeDialog sets the worktree dialog open flag
func (sm *StateManager) OpenWorktreeDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.WorktreeDialogOpen = true
	}
}

// CloseWorktreeDialog clears the worktree dialog open flag
func (sm *StateManager) CloseWorktreeDialog(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.WorktreeDialogOpen = false
	}
}

// GetProjectPath returns the path for a named project
func (sm *StateManager) GetProjectPath(sessionID, projectName string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return ""
	}
	for _, p := range s.Projects {
		if p.Name == projectName {
			return p.Path
		}
	}
	return ""
}
