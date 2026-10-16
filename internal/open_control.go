package libro

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

const openHelp = `Show a local file, such as an image, PDF, or HTML page, in a Libro Browser panel.
Usage: libro open FILE
Run it inside a Libro terminal. The file opens beside that terminal's panel and
reuses a Browser panel that already shows a local file.
`

// RunOpenCLI shows a local file beside the calling Libro terminal panel.
func RunOpenCLI(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" {
		_, err := io.WriteString(out, openHelp)
		return err
	}
	if len(args) != 1 {
		return errors.New("expected one file; see libro open --help")
	}
	panel := os.Getenv("LIBRO_PANEL_ID")
	if panel == "" {
		return errors.New("run libro open inside a Libro terminal")
	}
	path, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err = os.Stat(path); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"action": "open", "panel": panel, "url": fileURL(path)})
	if err != nil {
		return err
	}
	result, err := agentControlCommand(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}

func fileURL(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// desktopWindow is set when Libro runs its desktop window. Plain browser tabs
// block file:// pages, so open then fails and callers use their own viewer.
var desktopWindow bool

// openFilePanel shows a file URL in the calling panel's workspace without
// switching workspaces. A Browser panel that already shows a file is reused.
func openFilePanel(sid, panelID, address string) (r.Result, any, error) {
	if !strings.HasPrefix(address, "file:///") {
		return r.Result{}, nil, errors.New("open expects a local file")
	}
	if !desktopWindow {
		return r.Result{}, nil, errors.New("local files show only in the Libro desktop window")
	}
	_, workspace, _, found := sm.workspaceApp(sid, panelID)
	if !found {
		return r.Result{}, nil, errors.New("the calling Libro panel is closed")
	}
	source, reuse, reuseID, active := sm.fileBrowserTarget(sid, panelID, workspace)
	if !active {
		return r.Result{}, nil, errors.New("switch to this project in Libro first")
	}
	if reuseID != "" {
		sm.SetAppURLByID(sid, reuseID, address)
		sm.SelectApp(sid, reuse)
		state := sm.Get(sid)
		js := r.Merge(clientScript("window.__libroWvNavigate(props[0],props[1]);", reuseID, address), r.Result{}.Run(r.SetValue("urlinput-"+reuseID, address)), navigateJS(state, sid), updateAppPreviewJS(state))
		return js, map[string]any{"panel": reuseID, "url": address}, nil
	}
	width := DBDefaultPanelWidth()
	if w, ok := sm.ToolWidth(sid); ok {
		width = w
	}
	js := insertURLPanel(sid, address, "Browser", "browser", "right", width, source+1)
	state := sm.Get(sid)
	return js, map[string]any{"panel": state.Apps[state.SelectedIndex].ID, "url": address}, nil
}

// fileBrowserTarget reads the active workspace under the state lock. It returns
// the caller's index and any Browser panel that already shows a local file.
func (sm *StateManager) fileBrowserTarget(sid, panelID, workspace string) (source, reuse int, reuseID string, active bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	state := sm.states[sid]
	if state == nil || state.ActiveProject != workspace {
		return -1, -1, "", false
	}
	source, reuse = -1, -1
	for i, app := range state.Apps {
		if app.ID == panelID {
			source = i
		}
		if reuseID == "" && app.Type == AppTypeURL && (app.PluginID == "browser" || app.PluginID == "") && strings.HasPrefix(app.URL, "file://") {
			reuse, reuseID = i, app.ID
		}
	}
	return source, reuse, reuseID, true
}
