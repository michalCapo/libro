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
	ApplicationPort      int
	ApplicationPerThread bool
	ApplicationPath      string
	PluginID             string
	Dock                 string
	ID                   string
	Type                 AppType
	URL                  string // iframe source URL (for terminal apps, this is http://localhost:<port>)
	Command              string // original command (only for terminal apps)
	Width                Width
	PreviousWidth        Width  // width before toggling to full (for ⌘+F maximize toggle)
	Writable             bool   // whether terminal input is accepted
	Name                 string // optional display name
	IconURL              string // cached icon URL from DB (only for terminal apps)
	TerminalID           string // native PTY terminal ID (usually same as ID)
	TerminalReady        bool   // native PTY session is running
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
	Threads           []Thread
	Projects          []Project
	ActiveProject     string
	ProjectDialogOpen bool
	snapshots         map[string]*projectSnapshot
	closedWorkspaces  map[string]bool

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
	activeProject := ""
	rendered[activeProject] = true

	return &AppState{
		Projects:      projects,
		Threads:       loadThreads(),
		ActiveProject: activeProject,
		snapshots:     make(map[string]*projectSnapshot),

		renderedProjects: rendered,
	}
}

// Get returns the state for a session, creating one if it doesn't exist.
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
			snapshots: make(map[string]*projectSnapshot),
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
			snapshots: make(map[string]*projectSnapshot),
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
			snapshots: make(map[string]*projectSnapshot),
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

// InsertTerminalPlaceholder adds a terminal shell before its PTY has been started.
func (sm *StateManager) InsertTerminalPlaceholder(sessionID, appID string, width Width, command string, writable bool, name string, iconURL string, index int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{
			snapshots: make(map[string]*projectSnapshot),
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
	return removeApp(s, index)
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
			return removeApp(s, i)
		}
	}
	for _, snapshot := range s.snapshots {
		if snapshot == nil {
			continue
		}
		for i, app := range snapshot.Apps {
			if app.ID == appID {
				state := &AppState{Apps: snapshot.Apps, SelectedIndex: snapshot.SelectedIndex}
				removed := removeApp(state, i)
				snapshot.Apps, snapshot.SelectedIndex = state.Apps, state.SelectedIndex
				return removed
			}
		}
	}
	return nil
}

// removeApp updates selection while the state manager lock is held.
func removeApp(s *AppState, index int) *Application {
	removed := s.Apps[index]
	preferAgent := index == s.SelectedIndex && isAgentApp(removed)
	s.Apps = append(s.Apps[:index], s.Apps[index+1:]...)
	if preferAgent {
		for i := index - 1; i >= 0; i-- {
			if isAgentApp(s.Apps[i]) {
				s.SelectedIndex = i
				return &removed
			}
		}
		for i := index; i < len(s.Apps); i++ {
			if isAgentApp(s.Apps[i]) {
				s.SelectedIndex = i
				return &removed
			}
		}
	}
	if len(s.Apps) == 0 {
		s.SelectedIndex = 0
	} else if index < s.SelectedIndex {
		s.SelectedIndex--
	} else if s.SelectedIndex >= len(s.Apps) {
		s.SelectedIndex = len(s.Apps) - 1
	}
	return &removed
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
func (sm *StateManager) RemoveProject(sessionID, projectName string) ([]Application, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return nil, false
	}

	return s.removeProject(projectName)
}

func (s *AppState) removeProject(projectName string) ([]Application, bool) {
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
		apps = s.Apps
		s.Apps = nil
		s.SelectedIndex = 0
		s.ActiveProject = ""
		if len(s.Projects) > 0 {
			s.ActiveProject = s.Projects[0].Name
			if snap, ok := s.snapshots[s.ActiveProject]; ok {
				s.Apps = snap.Apps
				s.SelectedIndex = snap.SelectedIndex
				delete(s.snapshots, s.ActiveProject)
			}
		}
	}

	return apps, true
}

// CloseProject archives the active thread and clears its panels for cleanup.
func (sm *StateManager) CloseProject(sessionID string) ([]Application, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return nil, nil
	}
	if thread := s.thread(s.ActiveProject); thread != nil {
		if _, err := db.Exec("UPDATE threads SET archived = 1 WHERE id = ?", thread.ID); err != nil {
			return nil, err
		}
		thread.Archived = true
	}
	if s.closedWorkspaces == nil {
		s.closedWorkspaces = make(map[string]bool)
	}
	s.closedWorkspaces[s.ActiveProject] = true
	apps := s.Apps
	s.Apps = nil
	s.SelectedIndex = 0
	return apps, nil
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
	found := s.thread(projectName) != nil
	for _, p := range s.Projects {
		if p.Name == projectName {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	delete(s.closedWorkspaces, projectName)
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

// MoveSharedProjectApps moves project-level panels into another workspace for
// the same project. Agent, browser, and other tool panels stay with the thread.
func (sm *StateManager) MoveSharedProjectApps(sessionID, target string) []Application {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || target == s.ActiveProject {
		return nil
	}
	targetProject := s.projectScope(target)
	if targetProject == "" {
		return nil
	}
	if s.snapshots == nil {
		s.snapshots = make(map[string]*projectSnapshot)
	}
	targetSnapshot := s.snapshots[target]
	if targetSnapshot == nil {
		targetSnapshot = &projectSnapshot{}
		s.snapshots[target] = targetSnapshot
	}
	sharedKey := func(app Application) string {
		if appDock(app) == "bottom" {
			return app.ID
		}
		return app.PluginID
	}
	existing := make(map[string]bool, len(targetSnapshot.Apps))
	for _, app := range targetSnapshot.Apps {
		existing[sharedKey(app)] = true
	}
	var moved []Application
	moveFrom := func(workspace string, apps *[]Application, selectedIndex *int) {
		selectedID := ""
		if *selectedIndex >= 0 && *selectedIndex < len(*apps) {
			selectedID = (*apps)[*selectedIndex].ID
		}
		kept := make([]Application, 0, len(*apps))
		for _, app := range *apps {
			sameProject := s.projectScope(workspace) == targetProject
			if app.PluginID == "project-command" {
				sameProject = applicationRoot(s, workspace) == applicationRoot(s, target)
			}
			if sameProject && isSharedProjectApp(app) && !existing[sharedKey(app)] {
				moved = append(moved, app)
				targetSnapshot.Apps = append(targetSnapshot.Apps, app)
				existing[sharedKey(app)] = true
				continue
			}
			kept = append(kept, app)
		}
		*apps = kept
		*selectedIndex = 0
		for i, app := range kept {
			if app.ID == selectedID {
				*selectedIndex = i
				return
			}
			if isAgentApp(app) {
				*selectedIndex = i
			}
		}
	}
	moveFrom(s.ActiveProject, &s.Apps, &s.SelectedIndex)
	for workspace, snapshot := range s.snapshots {
		if workspace == target || snapshot == nil {
			continue
		}
		moveFrom(workspace, &snapshot.Apps, &snapshot.SelectedIndex)
	}
	return moved
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
	if thread := s.thread(s.ActiveProject); thread != nil {
		if thread.Path != "" {
			return thread.Path
		}
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
	for _, thread := range s.Threads {
		var apps []Application
		if thread.ID == s.ActiveProject {
			apps = s.Apps
		} else if snap := s.snapshots[thread.ID]; snap != nil {
			apps = snap.Apps
		}
		if len(apps) > 0 {
			result = append(result, ProjectApps{Name: thread.ID, Apps: apps})
		}
	}
	return result
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

// insertProjectCommand keeps the user's workspace and selection unchanged.
func (sm *StateManager) insertProjectCommand(sid, workspace, command string, port int, perThread bool, path string) (Application, int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state := sm.states[sid]
	apps := &state.Apps
	if workspace != state.ActiveProject {
		if state.snapshots == nil {
			state.snapshots = make(map[string]*projectSnapshot)
		}
		if state.snapshots[workspace] == nil {
			state.snapshots[workspace] = &projectSnapshot{}
		}
		apps = &state.snapshots[workspace].Apps
	}
	sm.nextID++
	id := fmt.Sprintf("app-%d", sm.nextID)
	panel := Application{ApplicationPort: port, ApplicationPerThread: perThread, ApplicationPath: path, ID: id, TerminalID: id, Type: AppTypeTerminal, PluginID: "project-command", Dock: "bottom", Width: WidthFull, Command: command, Writable: true, Name: "Project command"}
	index := len(*apps)
	*apps = append(*apps, panel)
	return panel, index
}
