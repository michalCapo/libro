package components

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// AgentSession is an earlier agent conversation that can be resumed.
type AgentSession struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated int64  `json:"updated"` // Unix milliseconds
}

const agentSessionLimit = 50

// Claude stores a folder's sessions under its path with every other character replaced.
var claudeProjectName = regexp.MustCompile(`[^a-zA-Z0-9]`)

// AgentSessions lists earlier Claude and Codex conversations started in dir,
// newest first. Sessions without a prompt are skipped.
func AgentSessions(dir string, environment map[string]string) []AgentSession {
	sessions := append(claudeSessions(dir, environment), codexSessions(dir, environment)...)
	slices.SortFunc(sessions, func(a, b AgentSession) int { return cmp.Compare(b.Updated, a.Updated) })
	return sessions[:min(len(sessions), agentSessionLimit)]
}

func claudeSessions(dir string, environment map[string]string) []AgentSession {
	paths, _ := filepath.Glob(filepath.Join(claudeConfigDir(environment), "projects", claudeProjectName.ReplaceAllString(dir, "-"), "*.jsonl"))
	type file struct {
		path    string
		updated time.Time
	}
	files := make([]file, 0, len(paths))
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files = append(files, file{path, info.ModTime()})
		}
	}
	slices.SortFunc(files, func(a, b file) int { return b.updated.Compare(a.updated) })
	var sessions []AgentSession
	for _, f := range files[:min(len(files), agentSessionLimit)] {
		id := strings.TrimSuffix(filepath.Base(f.path), ".jsonl")
		if !agentSessionPattern.MatchString(id) {
			continue
		}
		// Different folders can share an encoded name: a-b and a/b.
		if !claudeSessionIn(f.path, dir) {
			continue
		}
		if title := claudeSessionTitle(f.path); title != "" {
			sessions = append(sessions, AgentSession{Kind: "claude", ID: id, Title: title, Updated: f.updated.UnixMilli()})
		}
	}
	return sessions
}

// claudeSessionIn reports whether a transcript started in dir or below it.
// A transcript that names no folder is trusted by its encoded name.
func claudeSessionIn(path, dir string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(io.LimitReader(f, 4<<20))
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		if !bytes.Contains(scanner.Bytes(), []byte(`"cwd"`)) {
			continue
		}
		var entry struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.Cwd != "" {
			return entry.Cwd == dir || strings.HasPrefix(entry.Cwd, strings.TrimSuffix(dir, "/")+"/")
		}
	}
	return true
}

// claudeSessionTitle prefers a title set with /rename, then Claude's own
// title, then the first prompt. Titles are read from the end of the file only.
func claudeSessionTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	const tail = 1 << 20
	if info, err := f.Stat(); err == nil && info.Size() > tail {
		// The first line read after the seek is partial and fails to parse.
		_, _ = f.Seek(-tail, io.SeekEnd)
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var custom, generated string
	for scanner.Scan() {
		if !bytes.Contains(scanner.Bytes(), []byte(`-title"`)) {
			continue
		}
		var entry struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil {
			continue
		}
		switch entry.Type {
		case "custom-title":
			custom = entry.CustomTitle
		case "ai-title":
			generated = entry.AITitle
		}
	}
	if title := cmp.Or(cleanAgentTitle(custom), cleanAgentTitle(generated)); title != "" {
		return title
	}
	return claudeFirstPrompt(path)
}

func codexSessions(dir string, environment map[string]string) []AgentSession {
	db := openCodexStateDB(codexHome(environment))
	if db == nil {
		return nil
	}
	defer func() { _ = db.Close() }()
	// Exec runs and spawned agents are not conversations the user started.
	const query = ` FROM threads WHERE cwd = ? AND archived = 0 AND source IN ('cli', 'vscode') ORDER BY updated_at DESC LIMIT ?`
	rows, err := db.Query(`SELECT id, COALESCE(name, ''), title, first_user_message, updated_at`+query, dir, agentSessionLimit)
	if err != nil {
		// Older Codex databases do not have the user-assigned name column.
		rows, err = db.Query(`SELECT id, '', title, first_user_message, updated_at`+query, dir, agentSessionLimit)
	}
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var sessions []AgentSession
	for rows.Next() {
		var id, name, title, prompt string
		var updated int64
		if rows.Scan(&id, &name, &title, &prompt, &updated) != nil || !agentSessionPattern.MatchString(id) {
			continue
		}
		if title = cmp.Or(cleanAgentTitle(name), cleanAgentTitle(title), cleanAgentTitle(prompt)); title != "" {
			sessions = append(sessions, AgentSession{Kind: "codex", ID: id, Title: title, Updated: updated * 1000})
		}
	}
	return sessions
}

// claudeConfigDir is the agent's CLAUDE_CONFIG_DIR, then Libro's, then ~/.claude.
func claudeConfigDir(environment map[string]string) string {
	if dir := cmp.Or(environment["CLAUDE_CONFIG_DIR"], os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// codexHome is the agent's CODEX_HOME, then Libro's, then ~/.codex.
func codexHome(environment map[string]string) string {
	if dir := cmp.Or(environment["CODEX_HOME"], os.Getenv("CODEX_HOME")); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}
