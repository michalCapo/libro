package libro

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Plugin describes an app that runs in a Libro-managed terminal or browser.
// Plugins never need to implement PTY, webview, tab, or project lifecycle code.
type Plugin struct {
	Icon        string  `json:"icon,omitempty"`
	Autolaunch  bool    `json:"autolaunch,omitempty"`
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        AppType `json:"type"`
	Command     string  `json:"command,omitempty"`
	URL         string  `json:"url,omitempty"`
	Dock        string  `json:"dock"`
	Description string  `json:"description,omitempty"`
	Disabled    bool    `json:"disabled,omitempty"`
	Custom      bool    `json:"custom,omitempty"`
	Removed     bool    `json:"removed,omitempty"`
}

var builtinPlugins = []Plugin{
	{ID: "codex", Name: "Codex", Type: AppTypeTerminal, Command: "codex", Dock: "center", Description: "OpenAI coding agent"},
	{ID: "pi", Name: "Pi", Type: AppTypeTerminal, Command: "pi", Dock: "center", Description: "Pi coding agent"},
	{ID: "claude", Name: "Claude", Type: AppTypeTerminal, Command: "claude", Dock: "center", Description: "Claude Code"},
	{ID: "terminal", Name: "Terminal", Type: AppTypeTerminal, Dock: "right", Description: "Your default shell"},
	{ID: "browser", Name: "Browser", Type: AppTypeURL, Dock: "right", Description: "Browse websites and local previews"},
	{ID: "nvim", Name: "Nvim", Type: AppTypeTerminal, Command: "nvim", Dock: "right", Description: "Terminal text editor"},
	{ID: "lazyrepo", Name: "Git", Type: AppTypeTerminal, Command: "lazyrepo", Dock: "right", Description: "Repository tools"},
	{ID: "lazydata", Name: "Database", Type: AppTypeTerminal, Command: "lazydata", Dock: "right", Description: "Explore your data"},
	{ID: "files", Name: "Files", Type: AppTypeURL, Dock: "right", Description: "Browse project files with Vim navigation"},
	{ID: "notes", Name: "Notes", Type: AppTypeURL, Dock: "right", Description: "Track project notes and send them to an agent"},
	{ID: "opencode", Name: "OpenCode", Type: AppTypeTerminal, Command: "opencode", Dock: "center", Description: "OpenCode coding agent"},
}

var pluginOnce sync.Once
var installedPlugins []Plugin
var pluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func validDock(dock string) bool { return dock == "center" || dock == "right" || dock == "bottom" }

func validatePlugin(p Plugin) error {
	if !pluginIDPattern.MatchString(p.ID) || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("plugin needs a lowercase ID and a name")
	}
	if !validDock(p.Dock) {
		return fmt.Errorf("dock must be center, right, or bottom")
	}
	if p.Type != AppTypeURL && p.Type != AppTypeTerminal {
		return fmt.Errorf("type must be url or terminal")
	}
	if p.Dock == "center" && (p.Type != AppTypeTerminal || strings.TrimSpace(p.Command) == "") {
		return fmt.Errorf("center plugins must define an agent command")
	}
	if p.Type == AppTypeURL && p.Command != "" {
		return fmt.Errorf("browser plugins cannot declare a command")
	}
	return nil
}

func loadPlugins(dir string) []Plugin {
	result := append([]Plugin(nil), builtinPlugins...)
	files, _ := filepath.Glob(filepath.Join(dir, "*", "plugin.json"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			log.Printf("plugin %s: %v", file, err)
			continue
		}
		var p Plugin
		if err = json.Unmarshal(data, &p); err == nil {
			err = validatePlugin(p)
		}
		if err != nil {
			log.Printf("plugin %s: %v", file, err)
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing.ID == p.ID {
				duplicate = true
				break
			}
		}
		if duplicate {
			log.Printf("plugin %s: duplicate ID %s", file, p.ID)
			continue
		}
		result = append(result, p)
	}
	return result
}

func plugins() (result []Plugin) {
	defer func() {
		for i := range result {
			result[i].Icon = toolIcon(result[i])
		}
	}()
	pluginOnce.Do(func() {
		dir, err := libroDataDir()
		if err != nil {
			installedPlugins = append([]Plugin(nil), builtinPlugins...)
			return
		}
		installedPlugins = loadPlugins(filepath.Join(dir, "plugins"))
	})
	result = append([]Plugin(nil), installedPlugins...)
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		var raw string
		var custom []Plugin
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'custom_agents'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &custom) == nil {
			result = append(result, custom...)
		}
		var order []string
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'agent_order'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &order) == nil {
			rank := make(map[string]int, len(order))
			for i, id := range order {
				rank[id] = i + 1
			}
			// Unlisted plugins keep their existing order after the saved agents.
			sort.SliceStable(result, func(i, j int) bool {
				a, b := rank[result[i].ID], rank[result[j].ID]
				return a != 0 && (b == 0 || a < b)
			})
		}
		var autolaunch string
		_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'autolaunch_agent'`).Scan(&autolaunch)
		for i := range result {
			result[i].Autolaunch = result[i].ID == autolaunch
		}
		var names map[string]string
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'agent_names'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &names) == nil {
			for i := range result {
				if name := strings.TrimSpace(names[result[i].ID]); name != "" {
					result[i].Name = name
				}
			}
		}
		var removed map[string]bool
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'removed_agents'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &removed) == nil {
			for i := range result {
				result[i].Removed = removed[result[i].ID]
			}
		}
		var disabled map[string]bool
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'disabled_agents'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &disabled) == nil {
			for i := range result {
				result[i].Disabled = disabled[result[i].ID]
			}
		}
		var tools []Plugin
		if db.QueryRow(`SELECT value FROM settings WHERE key = 'tool_configs'`).Scan(&raw) == nil && json.Unmarshal([]byte(raw), &tools) == nil {
			for _, tool := range tools {
				found := false
				for i := range result {
					if result[i].ID == tool.ID {
						result[i] = tool
						found = true
						break
					}
				}
				if !found {
					result = append(result, tool)
				}
			}
		}
	}
	return result
}

func pluginForApp(app Application) Plugin {
	for _, p := range plugins() {
		if p.ID == app.PluginID && app.PluginID != "" {
			return p
		}
	}
	if app.Type == AppTypeURL {
		return builtinPlugins[4]
	}
	for _, p := range plugins() {
		if p.Command != "" && extractBaseCmd(app.Command) == extractBaseCmd(p.Command) {
			return p
		}
	}
	return builtinPlugins[3]
}

func isAgentApp(app Application) bool {
	plugin := pluginForApp(app)
	return app.Type == AppTypeTerminal && plugin.Type == AppTypeTerminal && plugin.Dock == "center"
}

func appDock(app Application) string {
	if app.Dock == "center" && !isAgentApp(app) {
		return "right"
	}
	if validDock(app.Dock) {
		return app.Dock
	}
	return pluginForApp(app).Dock
}

func (sm *StateManager) SetAppPlugin(sessionID, appID, pluginID, dock string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if state := sm.states[sessionID]; state != nil {
		for i := range state.Apps {
			if state.Apps[i].ID == appID {
				state.Apps[i].PluginID = pluginID
				if validDock(dock) {
					state.Apps[i].Dock = dock
				}
				return
			}
		}
	}
}
