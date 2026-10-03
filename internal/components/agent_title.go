package components

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

var codexTitleState = regexp.MustCompile(`^(Working|Thinking|Waiting|Ready|Starting)(?:\s*[·|—-]\s*|$)`)

// Codex also uses an abbreviated UUID (ending in "...") as its unnamed topic.
var sessionUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f.-]+(?:…)?(?:\s|$)`)

func cleanAgentTitle(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if title == "" || sessionUUID.MatchString(title) || title == "renaming..." || title == "Claude Code" {
		return ""
	}
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:200])
	}
	return title
}

// Window titles are optional hints. Prompt/session metadata works even when a
// CLI does not generate a title. Shell titles must never rename an agent thread.
func agentWindowTitle(kind, title string) string {
	switch kind {
	case "codex":
		if !codexTitleState.MatchString(title) {
			return ""
		}
		title = codexTitleState.ReplaceAllString(title, "")
		if match := codexSessionPattern.FindString(title); match != "" {
			title = strings.TrimPrefix(title, match)
		}
		title = strings.TrimRightFunc(title, func(r rune) bool { return unicode.IsSpace(r) || r >= 0x2800 && r <= 0x28ff })
	case "pi":
		if !strings.HasPrefix(title, "π - ") {
			return ""
		}
		end := strings.LastIndex(title, " - ")
		if end <= 4 {
			return ""
		}
		title = title[4:end]
	case "claude":
		// Claude's generated title is prefixed by a status glyph.
		if title == "" || unicode.IsLetter([]rune(title)[0]) || unicode.IsNumber([]rune(title)[0]) {
			return ""
		}
		title = strings.TrimLeftFunc(title, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	case "opencode":
	default:
		return ""
	}
	return cleanAgentTitle(title)
}

// setAgentTitle requires agentMu. A first prompt is a fallback (1); agent names
// (2) and Codex's saved name (3) may replace it. Follow-up prompts may not.
func (s *TerminalSession) setAgentTitle(title string, priority int) {
	title = cleanAgentTitle(title)
	if title == "" {
		return
	}
	s.mu.Lock()
	if s.closed || s.agentEnded || priority < s.titlePriority || priority == 1 && s.agentTitle != "" {
		s.mu.Unlock()
		return
	}
	// The agent's own title may repeat the first prompt. It stays a fallback.
	if title == s.agentTitle {
		s.mu.Unlock()
		return
	}
	s.agentTitle, s.titlePriority, s.titleCleared = title, priority, false
	s.mu.Unlock()
	s.broadcast(terminalWSMessage{Type: "agent-title", Data: title, Fallback: priority == 1})
}

// clearAgentTitle requires agentMu. A new agent session drops the old
// description, including one saved before Libro restarted.
func (s *TerminalSession) clearAgentTitle() {
	s.mu.Lock()
	s.agentTitle, s.titlePriority, s.titleCleared = "", 0, true
	s.mu.Unlock()
	s.broadcast(terminalWSMessage{Type: "agent-title"})
}

func (s *TerminalSession) readCodexTitle() {
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	hint := s.agentSessionID
	if s.codexSessionPrefix != "" && !strings.HasPrefix(hint, s.codexSessionPrefix) {
		hint = s.codexSessionPrefix + "..."
	}
	id, title, priority := codexThreadTitle(s.activity.codexHome, hint)
	if id != "" {
		s.updateAgentSession(id)
		s.setAgentTitle(title, priority)
	}
}

// RecoverCodexTitle repairs UUID labels saved by older Libro versions. Human
// names and missing/ambiguous sessions are left alone.
func RecoverCodexTitle(title, codexHome string) string {
	match := codexSessionPattern.FindStringSubmatch(title)
	if match != nil && match[1] == title {
		if _, recovered, _ := codexThreadTitle(codexHome, title); recovered != "" {
			return recovered
		}
	}
	return title
}

func codexThreadTitle(home, hint string) (string, string, int) {
	if hint == "" {
		return "", "", 0
	}
	db := openCodexStateDB(home)
	if db == nil {
		return "", "", 0
	}
	defer func() { _ = db.Close() }()
	id := hint
	if prefix, shortened := strings.CutSuffix(hint, "..."); shortened {
		// Resolve only a unique ID, never the newest session in a directory.
		rows, err := db.Query(`SELECT id FROM threads WHERE id LIKE ? LIMIT 2`, prefix+"%")
		if err != nil {
			return "", "", 0
		}
		var ids []string
		for rows.Next() {
			var found string
			if rows.Scan(&found) == nil {
				ids = append(ids, found)
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil || len(ids) != 1 {
			return "", "", 0
		}
		id = ids[0]
	}
	var name, title, prompt string
	// Older Codex databases do not have the separate user-assigned name column.
	_ = db.QueryRow(`SELECT COALESCE(name, '') FROM threads WHERE id = ?`, id).Scan(&name)
	_ = db.QueryRow(`SELECT title, first_user_message FROM threads WHERE id = ?`, id).Scan(&title, &prompt)
	if name = cleanAgentTitle(name); name != "" {
		return id, name, 3
	}
	if title = cleanAgentTitle(title); title != "" {
		return id, title, 1
	}
	return id, cleanAgentTitle(prompt), 1
}

// Read only the beginning of the exact transcript supplied by Claude's hook.
// This restores a useful label for resumed sessions without scanning history.
func claudeFirstPrompt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(io.LimitReader(f, 4<<20))
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "user" || entry.IsMeta || entry.Message.Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(entry.Message.Content, &text) != nil {
			var parts []struct{ Type, Text string }
			_ = json.Unmarshal(entry.Message.Content, &parts)
			for _, part := range parts {
				if part.Type == "text" {
					text += " " + part.Text
				}
			}
		}
		if title := cleanAgentTitle(text); title != "" && !strings.HasPrefix(title, "<") {
			return title
		}
	}
	if scanner.Err() != nil {
		return ""
	}
	return ""
}
