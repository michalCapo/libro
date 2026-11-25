package components

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Prompt history comes from the agent's own session record, so prompts typed
// in the agent are listed and a resumed session keeps its earlier prompts.
// Claude and Codex append to a JSONL transcript; the Pi and OpenCode plugins
// rewrite a JSON list in the activity folder.

// Larger prompts are clipped so each update stays a small socket message.
const maxPromptRunes = 100000

// promptLog is used only by the activity watcher goroutine, except source:
// the session record, guarded by agentMu because closing the terminal reads
// Claude's hook too. Claude's hook names it; Codex's index is looked up.
type promptLog struct {
	source  string
	session string      // Codex session whose rollout was looked up
	path    string      // record that offset and prompts belong to
	file    os.FileInfo // last read state of that record
	offset  int64
	prompts []string
}

// findCodexRollout runs on the watcher's one-second Codex tick. Codex indexes
// a rollout and creates its file shortly after the first prompt.
func (s *TerminalSession) findCodexRollout() {
	s.agentMu.Lock()
	id, found := s.agentSessionID, s.promptLog.source != ""
	s.agentMu.Unlock()
	log := &s.promptLog
	if id == log.session && (found || id == "") {
		return
	}
	path := ""
	if id != "" {
		path = codexRolloutPath(s.activity.codexHome, id, false)
		if _, err := os.Stat(path); err != nil {
			path = ""
		}
	}
	log.session = id
	s.agentMu.Lock()
	log.source = path
	s.agentMu.Unlock()
}

// readAgentPrompts sends the prompt list when the session record changes.
func (s *TerminalSession) readAgentPrompts() {
	log := &s.promptLog
	s.agentMu.Lock()
	path := log.source
	// After /new or a resume, the old rollout must not show until the next lookup.
	if s.activity.kind == "codex" && s.agentSessionID != log.session {
		path = ""
	}
	s.agentMu.Unlock()
	if s.activity.kind == "pi" || s.activity.kind == "opencode" {
		path = filepath.Join(s.activity.dir, "prompts.json")
	}
	if path != log.path {
		log.path, log.file, log.offset, log.prompts = path, nil, 0, nil
		s.setAgentPrompts(nil)
	}
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || log.file != nil && os.SameFile(info, log.file) && info.Size() == log.file.Size() && info.ModTime().Equal(log.file.ModTime()) {
		return
	}
	if s.activity.kind == "pi" || s.activity.kind == "opencode" {
		// A file caught mid-write is read again on the next tick.
		var prompts []string
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &prompts) == nil {
			log.file = info
			s.setAgentPrompts(cleanPrompts(prompts))
		}
		return
	}
	parse := claudePrompt
	if s.activity.kind == "codex" {
		parse = codexPrompt
	}
	// A replaced or shorter file is read again from the start.
	if log.file != nil && (!os.SameFile(info, log.file) || info.Size() < log.offset) {
		log.offset, log.prompts = 0, nil
		s.setAgentPrompts(nil)
	}
	added, offset, err := readPromptLines(path, log.offset, parse)
	log.offset = offset
	if err == nil {
		log.file = info
	}
	if len(added) > 0 {
		log.prompts = append(log.prompts, added...)
		s.setAgentPrompts(slices.Clone(log.prompts))
	}
}

// readPromptLines parses complete lines after offset. A line still being
// written stays for the next read.
func readPromptLines(path string, offset int64, parse func([]byte) string) ([]string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer func() { _ = file.Close() }()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	reader := bufio.NewReaderSize(file, 64<<10)
	var prompts []string
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return prompts, offset, nil
			}
			return prompts, offset, err
		}
		offset += int64(len(line))
		if prompt := clipPrompt(parse(line)); prompt != "" {
			prompts = append(prompts, prompt)
		}
	}
}

func (s *TerminalSession) setAgentPrompts(prompts []string) {
	s.mu.Lock()
	if s.closed || s.agentEnded || len(prompts) == 0 && len(s.agentPrompts) == 0 {
		s.mu.Unlock()
		return
	}
	s.agentPrompts = prompts
	s.mu.Unlock()
	s.broadcast(terminalWSMessage{Type: "agent-prompts", Prompts: prompts})
}

func cleanPrompts(prompts []string) []string {
	result := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt = clipPrompt(promptText(prompt)); prompt != "" {
			result = append(result, prompt)
		}
	}
	return result
}

// promptText keeps the prompt as typed, without the blank lines around it.
func promptText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return strings.Trim(text, "\r\n")
}

func clipPrompt(prompt string) string {
	if len(prompt) > maxPromptRunes {
		if runes := []rune(prompt); len(runes) > maxPromptRunes {
			prompt = string(runes[:maxPromptRunes]) + "…"
		}
	}
	return prompt
}

// claudePrompt returns the text the user sent in one transcript line. Tool
// results, command tags, reminders and compaction summaries are not prompts.
func claudePrompt(line []byte) string {
	if !bytes.Contains(line, []byte(`"user"`)) {
		return ""
	}
	var entry struct {
		Type             string `json:"type"`
		IsMeta           bool   `json:"isMeta"`
		IsSidechain      bool   `json:"isSidechain"`
		IsCompactSummary bool   `json:"isCompactSummary"`
		Message          struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "user" || entry.Message.Role != "user" || entry.IsMeta || entry.IsSidechain || entry.IsCompactSummary {
		return ""
	}
	var text string
	if json.Unmarshal(entry.Message.Content, &text) != nil {
		var parts []struct{ Type, Text string }
		_ = json.Unmarshal(entry.Message.Content, &parts)
		var texts []string
		for _, part := range parts {
			if part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
		text = strings.Join(texts, "\n")
	}
	text = promptText(text)
	if claudeInternalTag.MatchString(text) || strings.HasPrefix(text, "[Request interrupted") {
		return ""
	}
	return text
}

// Claude records slash commands, command output and reminders as user
// messages wrapped in hyphenated tags such as <command-name>.
var claudeInternalTag = regexp.MustCompile(`^\s*<[a-z]+(?:-[a-z]+)+>`)

// codexPrompt reads a user message event from a Codex rollout. Newer versions
// record completed UserMessage items; older ones a user_message event.
func codexPrompt(line []byte) string {
	if !bytes.Contains(line, []byte(`"UserMessage"`)) && !bytes.Contains(line, []byte(`"user_message"`)) {
		return ""
	}
	var entry struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Item    struct {
				Type    string `json:"type"`
				Content []struct{ Type, Text string }
			} `json:"item"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "event_msg" {
		return ""
	}
	switch {
	case entry.Payload.Type == "user_message":
		return promptText(entry.Payload.Message)
	case entry.Payload.Type == "item_completed" && entry.Payload.Item.Type == "UserMessage":
		var texts []string
		for _, part := range entry.Payload.Item.Content {
			if part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
		return promptText(strings.Join(texts, "\n"))
	}
	return ""
}
