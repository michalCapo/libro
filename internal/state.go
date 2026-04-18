package libro

import (
	"fmt"
	"os"
	"sort"
	"strings"
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
	ID            string
	Type          AppType
	URL           string // iframe source URL (for terminal apps, this is http://localhost:<port>)
	Command       string // original command (only for terminal apps)
	Width         Width
	PreviousWidth Width  // width before toggling to full (for ⌘+F maximize toggle)
	Port          int    // ttyd port (only for terminal apps)
	Writable      bool   // ttyd --writable flag (only for terminal apps)
	Name          string // optional display name
	IconURL       string // cached icon URL from DB (only for terminal apps)
}

// Project represents a named working directory
type Project struct {
	Name          string
	Path          string
	NavSlot       int
	IsGitRepo     bool   // detected at load time, not persisted
	Virtual       bool   // true for worktree-derived projects
	ParentProject string // name of parent project (for virtual projects)
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

func persistClosedProjectSnapshot(projectName string, snap *projectSnapshot) {
	if projectName == "" {
		return
	}
	if snap == nil || len(snap.Apps) == 0 {
		DBClearClosedProjectApps(projectName)
		return
	}
	closedApps := make([]ClosedProjectApp, 0, len(snap.Apps))
	for i, app := range snap.Apps {
		closedApps = append(closedApps, ClosedProjectApp{
			ProjectName:   projectName,
			Type:          string(app.Type),
			URL:           app.URL,
			Command:       app.Command,
			Width:         string(app.Width),
			PreviousWidth: string(app.PreviousWidth),
			Writable:      app.Writable,
			Name:          app.Name,
			IconURL:       app.IconURL,
			Position:      i,
			SelectedIndex: snap.SelectedIndex,
		})
	}
	DBSaveClosedProjectApps(projectName, closedApps)
}

func loadClosedSnapshots(projects []Project) map[string]*projectSnapshot {
	closed := make(map[string]*projectSnapshot)
	for _, p := range projects {
		rows := DBLoadClosedProjectApps(p.Name)
		if len(rows) == 0 {
			continue
		}
		apps := make([]Application, 0, len(rows))
		selectedIndex := 0
		for _, row := range rows {
			selectedIndex = row.SelectedIndex
			apps = append(apps, Application{
				Type:          AppType(row.Type),
				URL:           row.URL,
				Command:       row.Command,
				Width:         Width(row.Width),
				PreviousWidth: Width(row.PreviousWidth),
				Writable:      row.Writable,
				Name:          row.Name,
				IconURL:       row.IconURL,
			})
		}
		closed[p.Name] = &projectSnapshot{
			Apps:          apps,
			SelectedIndex: selectedIndex,
		}
	}
	return closed
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
	closedSnapshots   map[string]*projectSnapshot
	renderedProjects  map[string]bool // tracks which projects have DOM divs

	// LastAppCreatedProject tracks the project where the last app was created
	LastAppCreatedProject string

	// PreviousProject tracks the previously active project (for Ctrl+0)
	PreviousProject string

	// EditIndex tracks which saved app is being edited (-1 = new app)
	EditIndex int
	// EditDBID tracks the database row ID of the saved app being edited (-1 = new app)
	EditDBID int64

	// ManageOpen tracks whether the manage apps page is shown
	ManageOpen bool

	// WorktreeDialogOpen tracks whether the worktree popup is shown
	WorktreeDialogOpen bool

	// SidebarCollapsed tracks whether the project sidebar is collapsed
	SidebarCollapsed bool

	// ZenMode hides everything except running applications (top bar, sidebar, toolbars)
	ZenMode bool

	// NavSlots maps keyboard slot (2-9) → project name to switch to
	NavSlots map[int]string
	// NavProjectSlot maps root project name → assigned slot number
	NavProjectSlot map[string]int
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

func navSlotsFromProjects(projects []Project) (map[int]string, map[string]int) {
	slots := make(map[int]string)
	projectSlots := make(map[string]int)
	for _, p := range projects {
		if p.NavSlot < 2 || p.NavSlot > 9 {
			continue
		}
		if _, taken := slots[p.NavSlot]; taken {
			continue
		}
		slots[p.NavSlot] = p.Name
		projectSlots[p.Name] = p.NavSlot
	}
	return slots, projectSlots
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

	projects := DBLoadProjects()
	detectGitRepos(projects)
	rendered := make(map[string]bool)
	if len(projects) > 0 {
		rendered[projects[0].Name] = true
	}
	navSlots, navProjectSlots := navSlotsFromProjects(projects)
	closedSnapshots := loadClosedSnapshots(projects)

	zenMode := DBGetSetting("zen_mode", "0") == "1"
	sm.states[sid] = &AppState{
		Projects:         projects,
		ActiveProject:    projects[0].Name,
		snapshots:        make(map[string]*projectSnapshot),
		closedSnapshots:  closedSnapshots,
		renderedProjects: rendered,
		EditIndex:        -1,
		NavSlots:         navSlots,
		NavProjectSlot:   navProjectSlots,
		ZenMode:          zenMode,
		SidebarCollapsed: zenMode,
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
	projects := DBLoadProjects()
	detectGitRepos(projects)
	rendered := make(map[string]bool)
	if len(projects) > 0 {
		rendered[projects[0].Name] = true
	}
	navSlots, navProjectSlots := navSlotsFromProjects(projects)
	closedSnapshots := loadClosedSnapshots(projects)
	zenMode := DBGetSetting("zen_mode", "0") == "1"
	s := &AppState{
		Projects:         projects,
		ActiveProject:    projects[0].Name,
		snapshots:        make(map[string]*projectSnapshot),
		closedSnapshots:  closedSnapshots,
		renderedProjects: rendered,
		EditIndex:        -1,
		NavSlots:         navSlots,
		NavProjectSlot:   navProjectSlots,
		ZenMode:          zenMode,
		SidebarCollapsed: zenMode,
	}
	sm.states[sessionID] = s
	return s
}

// NextPort returns the next available port for ttyd, skipping any ports already in use.
func (sm *StateManager) NextPort() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for {
		sm.nextPort++
		if portFree(sm.nextPort) {
			return sm.nextPort
		}
	}
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
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
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
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
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
// appID must be pre-generated via NextAppID to avoid race conditions with TtydManager.
func (sm *StateManager) addTerminalApp(sessionID string, appID string, command string, port int, writable bool, width Width, name string, iconURL string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
		}
		sm.states[sessionID] = s
	}
	app := Application{
		ID:       appID,
		Type:     AppTypeTerminal,
		URL:      fmt.Sprintf("/ttyd/%d/", port),
		Command:  command,
		Width:    width,
		Port:     port,
		Writable: writable,
		Name:     name,
		IconURL:  iconURL,
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
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
		}
		sm.states[sessionID] = s
	}
	app := Application{
		ID:       appID,
		Type:     AppTypeTerminal,
		URL:      fmt.Sprintf("/ttyd/%d/", port),
		Command:  command,
		Width:    width,
		Port:     port,
		Writable: writable,
		Name:     name,
		IconURL:  iconURL,
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
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
		}
		sm.states[sessionID] = s
	}
	app := Application{
		ID:       appID,
		Type:     AppTypeTerminal,
		Width:    width,
		Command:  command,
		Port:     port,
		URL:      fmt.Sprintf("/ttyd/%d/", port),
		Writable: true,
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

// InsertPendingTerminal adds a terminal placeholder (no ttyd yet) at the given index.
func (sm *StateManager) InsertPendingTerminal(sessionID, appID string, width Width, index int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		s = &AppState{
			Projects:        []Project{{Name: "home", Path: defaultHomeDir()}},
			ActiveProject:   "home",
			snapshots:       make(map[string]*projectSnapshot),
			closedSnapshots: make(map[string]*projectSnapshot),
			EditIndex:       -1,
		}
		sm.states[sessionID] = s
	}
	app := Application{
		ID:    appID,
		Type:  AppTypeTerminal,
		Width: width,
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

// CloseActiveProjectApps clears the active project's running apps.
// It returns the project name and the removed apps for external cleanup.
func (sm *StateManager) CloseActiveProjectApps(sessionID string) (string, []Application) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return "", nil
	}
	projectName := s.ActiveProject
	if len(s.Apps) == 0 {
		return projectName, nil
	}
	appsCopy := cloneApplications(s.Apps)
	s.Apps = nil
	s.SelectedIndex = 0
	return projectName, appsCopy
}

// SaveActiveProjectApps remembers the active project's running apps without closing them.
func (sm *StateManager) SaveActiveProjectApps(sessionID string) (string, int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return "", 0
	}
	projectName := s.ActiveProject
	if len(s.Apps) == 0 {
		return projectName, 0
	}
	snap := &projectSnapshot{
		Apps:          cloneApplications(s.Apps),
		SelectedIndex: s.SelectedIndex,
	}
	s.closedSnapshots[projectName] = snap
	persistClosedProjectSnapshot(projectName, snap)
	return projectName, len(s.Apps)
}

// ClearClosedProjectApps removes the remembered reopen snapshot for the active project.
func (sm *StateManager) ClearClosedProjectApps(sessionID string) (string, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return "", false
	}
	projectName := s.ActiveProject
	snap, ok := s.closedSnapshots[projectName]
	if ok {
		delete(s.closedSnapshots, projectName)
	}
	DBClearClosedProjectApps(projectName)
	return projectName, ok && snap != nil && len(snap.Apps) > 0
}

// TakeClosedProjectApps returns and clears the last remembered closed apps for the active project.
func (sm *StateManager) TakeClosedProjectApps(sessionID string) (string, *projectSnapshot) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return "", nil
	}
	projectName := s.ActiveProject
	snap, ok := s.closedSnapshots[projectName]
	if !ok || snap == nil || len(snap.Apps) == 0 {
		return projectName, nil
	}
	delete(s.closedSnapshots, projectName)
	DBClearClosedProjectApps(projectName)
	return projectName, &projectSnapshot{
		Apps:          cloneApplications(snap.Apps),
		SelectedIndex: snap.SelectedIndex,
	}
}

// RememberClosedProjectApps overwrites the remembered closed apps for a project.
func (sm *StateManager) RememberClosedProjectApps(sessionID, projectName string, snap *projectSnapshot) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil || projectName == "" || snap == nil || len(snap.Apps) == 0 {
		return
	}
	s.closedSnapshots[projectName] = &projectSnapshot{
		Apps:          cloneApplications(snap.Apps),
		SelectedIndex: snap.SelectedIndex,
	}
	persistClosedProjectSnapshot(projectName, snap)
}

// RestoreActiveProjectApps replaces the active project's running apps.
func (sm *StateManager) RestoreActiveProjectApps(sessionID string, apps []Application, selectedIndex int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return
	}
	s.Apps = append([]Application(nil), apps...)
	if len(s.Apps) == 0 {
		s.SelectedIndex = 0
		return
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	if selectedIndex >= len(s.Apps) {
		selectedIndex = len(s.Apps) - 1
	}
	s.SelectedIndex = selectedIndex
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

// ToggleMaxWidth toggles the selected app between full width and its previous width.
// If the app is already full width, it restores the previous width (or LG if none saved).
// If not full width, it saves the current width and switches to full.
// Returns the new width and app ID, or empty strings if no app is selected.
func (sm *StateManager) ToggleMaxWidth(sessionID string) (Width, string) {
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
		app.Width = prev
		app.PreviousWidth = ""
		return prev, app.ID
	}
	// Save current width and go full
	app.PreviousWidth = app.Width
	app.Width = WidthFull
	return WidthFull, app.ID
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
	p := Project{Name: name, Path: path}
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
	delete(s.closedSnapshots, projectName)
	delete(s.renderedProjects, projectName)
	sm.clearNavSlot(s, projectName)

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

	// Track previous project for Ctrl+0
	s.PreviousProject = s.ActiveProject

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

// NextProject returns the name of the next project (clamped, no wrap-around)
func (sm *StateManager) NextProject(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil || len(s.Projects) <= 1 {
		return ""
	}
	for i, p := range s.Projects {
		if p.Name == s.ActiveProject {
			if i+1 >= len(s.Projects) {
				return ""
			}
			return s.Projects[i+1].Name
		}
	}
	return ""
}

// PrevProject returns the name of the previous project (clamped, no wrap-around)
func (sm *StateManager) PrevProject(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil || len(s.Projects) <= 1 {
		return ""
	}
	for i, p := range s.Projects {
		if p.Name == s.ActiveProject {
			if i == 0 {
				return ""
			}
			return s.Projects[i-1].Name
		}
	}
	return ""
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

// PreviousProject returns the previously active project, or ""
func (sm *StateManager) PreviousProject(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return ""
	}
	return s.PreviousProject
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
	delete(s.closedSnapshots, name)
	delete(s.renderedProjects, name)
	sm.clearNavSlot(s, name)

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

// ToggleSidebar flips the sidebar collapsed state
func (sm *StateManager) ToggleSidebar(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.SidebarCollapsed = !s.SidebarCollapsed
	}
}

// ToggleZenMode flips the zen mode state and persists it to the database.
func (sm *StateManager) ToggleZenMode(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s != nil {
		s.ZenMode = !s.ZenMode
		// In zen mode, always collapse sidebar
		if s.ZenMode {
			s.SidebarCollapsed = true
		}
		// Persist zen mode preference
		val := "0"
		if s.ZenMode {
			val = "1"
		}
		go DBSetSetting("zen_mode", val)
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

// AssignNavSlot assigns a keyboard nav slot (2-9) to a project or branch.
// Each branch gets its own independent slot. Returns the assigned slot, or 0.
func (sm *StateManager) AssignNavSlot(sessionID, projectName string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return 0
	}

	// If this project/branch already has a slot, keep it
	if slot, ok := s.NavProjectSlot[projectName]; ok {
		return slot
	}

	// Find next available slot (2-9)
	for i := 2; i <= 9; i++ {
		if _, taken := s.NavSlots[i]; !taken {
			s.NavSlots[i] = projectName
			s.NavProjectSlot[projectName] = i
			for idx := range s.Projects {
				if s.Projects[idx].Name == projectName {
					s.Projects[idx].NavSlot = i
					break
				}
			}
			return i
		}
	}
	// All slots taken — no assignment
	return 0
}

// NavSlotProject returns the project name for a given slot (2-9), or "".
func (sm *StateManager) NavSlotProject(sessionID string, slot int) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return ""
	}
	return s.NavSlots[slot]
}

// GetNavSlotForProject returns the slot number for a project or branch, or 0 if unassigned.
func (sm *StateManager) GetNavSlotForProject(sessionID, projectName string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s := sm.states[sessionID]
	if s == nil {
		return 0
	}
	return s.NavProjectSlot[projectName]
}

// RemoveNavSlot removes a project's nav slot assignment and returns the removed slot, or 0.
func (sm *StateManager) RemoveNavSlot(sessionID, projectName string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.states[sessionID]
	if s == nil {
		return 0
	}
	return sm.clearNavSlot(s, projectName)
}

// clearNavSlot removes a project's nav slot assignment (must hold lock).
func (sm *StateManager) clearNavSlot(s *AppState, projectName string) int {
	slot := 0
	if assigned, ok := s.NavProjectSlot[projectName]; ok {
		slot = assigned
		delete(s.NavSlots, assigned)
		delete(s.NavProjectSlot, projectName)
	}
	for idx := range s.Projects {
		if s.Projects[idx].Name == projectName {
			s.Projects[idx].NavSlot = 0
			break
		}
	}
	return slot
}
