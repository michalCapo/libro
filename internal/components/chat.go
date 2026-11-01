package components

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Chat panels run Claude without a terminal. Claude reads and writes JSON
// lines (stream-json); Libro shows them as messages and tool steps. Tools
// run without asking; Claude's questions and plans wait for the user.

// ChatItem is one line of a chat: a message, a tool step or a notice.
type ChatItem struct {
	ID      string
	Kind    string // user, assistant, tool, notice, error
	Text    string // message text, or the tool's short description
	Tool    string // tool name
	Detail  string // the end of a tool's result
	State   string // tool state: running, done, failed
	Images  int    // images the user sent with the message
	At      time.Time
	Version int // changes with the item, so unchanged items keep their rendering
}

// ChatOption is one choice of a question.
type ChatOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ChatQuestion is one question of Claude's AskUserQuestion tool.
type ChatQuestion struct {
	Question    string       `json:"question"`
	Header      string       `json:"header"`
	Options     []ChatOption `json:"options"`
	MultiSelect bool         `json:"multiSelect"`
}

// ChatRequest is Claude's open question or plan.
type ChatRequest struct {
	ID        string
	Tool      string
	Plan      string // the plan of ExitPlanMode
	Questions []ChatQuestion
	input     json.RawMessage
}

// ChatSnapshot is the chat as the panel shows it.
type ChatSnapshot struct {
	Items    []ChatItem
	Requests []ChatRequest
	Status   string // idle, working, done, error
	Running  bool
	Dir      string // the folder Claude works in
}

// ChatAnswer answers a ChatRequest.
type ChatAnswer struct {
	Allow   bool
	Message string            // why the user declined
	Answers map[string]string // question text -> chosen labels or own text
}

type chatState struct {
	mu       sync.Mutex
	stdin    io.WriteCloser
	writeMu  sync.Mutex
	items    []ChatItem
	index    map[string]int
	requests []ChatRequest
	// streamed holds the text items of an assistant message that arrived as
	// deltas, in order; the final message replaces their text.
	streamed        map[string][]string
	streamedMessage string
	streaming       string
	status          string
	stderr          bytes.Buffer
	nextID          int
	onChange        func()
	replayAt        time.Time // the time of the transcript line being replayed
}

const (
	chatMaxItems    = 2000
	chatMaxDetail   = 600
	chatReplayLimit = 400
)

// StartChat starts command as a chat session for appID. command is the
// agent command with resume flags; only Claude is supported.
func (tm *TerminalManager) StartChat(generation uint64, appID, command, cwd string, environment []string, reportSession func(string), onChange func()) (*TerminalSession, error) {
	tm.launchMu.Lock()
	defer tm.launchMu.Unlock()
	if tm.stopped || generation != tm.stops {
		return nil, ErrTerminalsStopped
	}
	if appID == "" {
		return nil, fmt.Errorf("empty chat id")
	}
	if cwd == "" {
		cwd = "/"
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	tm.mu.Lock()
	if existing := tm.sessions[appID]; existing != nil && !existing.isClosed() {
		tm.mu.Unlock()
		return existing, nil
	}
	tm.mu.Unlock()

	launch, err := prepareChatCommand(command)
	if err != nil {
		return nil, err
	}
	shell := userShell()
	cmd := exec.Command(shell, "-lic", strings.TrimSuffix(terminalShellScript("exec "+launch, shell), "; exec "+shellQuote(shell)))
	cmd.Dir = cwd
	environment = append(environment, "LIBRO_PANEL_ID="+appID)
	if executable, err := os.Executable(); err == nil {
		environment = append(environment, "LIBRO_BIN="+executable)
	}
	cmd.Env = mergeEnvironment(os.Environ(), environment)
	setChatProcessGroup(cmd)
	chat := &chatState{index: map[string]int{}, streamed: map[string][]string{}, status: "idle", onChange: onChange}
	cmd.Stderr = &limitedWriter{buf: &chat.stderr, mu: &chat.mu, max: 8 << 10}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	chat.stdin = stdin
	if id := chatResumeID(command); id != "" {
		chat.replay(claudeTranscriptPath(cmd.Env, id))
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Claude: %w", err)
	}
	s := &TerminalSession{
		ID: appID, AppID: appID, Command: command, Cwd: cwd, Writable: true,
		cmd: cmd, outputDone: make(chan struct{}), processDone: make(chan struct{}),
		clients: make(map[*terminalClient]bool), agentStatus: "idle", reportSession: reportSession,
		chat: chat,
	}
	tm.mu.Lock()
	delete(tm.endedChats, appID)
	s.replayOutput = tm.replayOutput
	s.log = &terminalLog{}
	tm.logs[appID] = s.log
	tm.sessions[appID] = s
	tm.mu.Unlock()
	log.Printf("chat started for app %s: %s", appID, command)
	if err := chat.write(map[string]any{"type": "control_request", "request_id": "libro-init", "request": map[string]any{"subtype": "initialize"}}); err != nil {
		log.Printf("chat %s: initialize: %v", appID, err)
	}
	go func() {
		s.readChat(stdout)
		close(s.outputDone)
	}()
	go tm.waitLoop(s)
	return s, nil
}

// ChatCommand reports whether command can run as a chat.
func ChatCommand(command string) bool {
	_, err := prepareChatCommand(command)
	return err == nil
}

// prepareChatCommand adds the stream-json flags and Libro's MCP server and
// instructions to a Claude command.
func prepareChatCommand(command string) (string, error) {
	parts := agentCommandPattern.FindStringSubmatch(strings.TrimSpace(command))
	if parts == nil {
		return "", fmt.Errorf("chat needs a Claude command")
	}
	kind := filepath.Base(parts[1])
	if kind == "ollama" {
		launch := ollamaClaudePattern.FindStringSubmatch(parts[2])
		if launch == nil {
			return "", fmt.Errorf("chat needs a Claude command")
		}
		parts[1] += launch[1] + " --"
		parts[2] = launch[2]
		kind = "claude"
	}
	if kind != "claude" {
		return "", fmt.Errorf("chat supports only Claude")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	mcpJSON, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"libro": map[string]any{"command": executable, "args": []string{"mcp"}}}})
	settings, _ := json.Marshal(map[string]any{"permissions": map[string]any{"allow": []string{"mcp__libro__application", "mcp__libro__children", "mcp__libro__thread", "mcp__libro__repositories"}}})
	args := " -p --input-format stream-json --output-format stream-json --verbose --include-partial-messages --permission-prompt-tool stdio" +
		" --settings " + shellQuote(string(settings)) + " --mcp-config " + shellQuote(string(mcpJSON)) + " --append-system-prompt " + shellQuote(AgentInstructions)
	return parts[1] + args + parts[2], nil
}

// chatResumeID is the session a command resumes.
func chatResumeID(command string) string {
	args := shellWords(command)
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--resume="); ok {
			return value
		}
		if (arg == "--resume" || arg == "-r") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func claudeTranscriptPath(env []string, sessionID string) string {
	if !agentSessionPattern.MatchString(sessionID) {
		return ""
	}
	dir := ""
	for _, value := range env {
		if v, ok := strings.CutPrefix(value, "CLAUDE_CONFIG_DIR="); ok {
			dir = v
		}
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".claude")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "projects", "*", sessionID+".jsonl"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

type limitedWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if extra := w.buf.Len() - w.max; extra > 0 {
		w.buf.Next(extra)
	}
	return len(p), nil
}

func (c *chatState) write(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if c.stdin == nil {
		return io.ErrClosedPipe
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *chatState) changed() {
	if c.onChange != nil {
		c.onChange()
	}
}

// add requires mu.
func (c *chatState) add(item ChatItem) string {
	if item.ID == "" {
		c.nextID++
		item.ID = fmt.Sprintf("c%d", c.nextID)
	}
	if i, ok := c.index[item.ID]; ok {
		item.Version = c.items[i].Version + 1
		c.items[i] = item
		return item.ID
	}
	item.Version = 1
	if item.At.IsZero() {
		item.At = cmpTime(c.replayAt, time.Now())
	}
	c.items = append(c.items, item)
	if len(c.items) > chatMaxItems {
		c.items = append([]ChatItem(nil), c.items[len(c.items)-chatMaxItems:]...)
		c.index = map[string]int{}
		for i, it := range c.items {
			c.index[it.ID] = i
		}
		return item.ID
	}
	c.index[item.ID] = len(c.items) - 1
	return item.ID
}

// update requires mu.
func (c *chatState) update(id string, change func(*ChatItem)) {
	if i, ok := c.index[id]; ok {
		change(&c.items[i])
		c.items[i].Version++
	}
}

func (c *chatState) replay(path string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	reader := bufio.NewReaderSize(f, 64<<10)
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var entry struct {
				Type        string    `json:"type"`
				IsSidechain bool      `json:"isSidechain"`
				IsMeta      bool      `json:"isMeta"`
				Timestamp   time.Time `json:"timestamp"`
			}
			if json.Unmarshal(line, &entry) == nil && !entry.IsSidechain && !entry.IsMeta && (entry.Type == "user" || entry.Type == "assistant") {
				c.replayAt = entry.Timestamp
				c.handle(line, true)
			}
		}
		if err != nil {
			break
		}
	}
	for i := range c.items {
		if c.items[i].State == "running" {
			c.items[i].State = "done"
		}
	}
	if len(c.items) > chatReplayLimit {
		c.items = append([]ChatItem(nil), c.items[len(c.items)-chatReplayLimit:]...)
		c.index = map[string]int{}
		for i, it := range c.items {
			c.index[it.ID] = i
		}
	}
	c.streamed = map[string][]string{}
	c.replayAt = time.Time{}
}

func cmpTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func (s *TerminalSession) readChat(stdout io.Reader) {
	reader := bufio.NewReaderSize(stdout, 256<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			s.chatLine(line)
		}
		if err != nil {
			return
		}
	}
}

func (s *TerminalSession) chatLine(line []byte) {
	c := s.chat
	var head struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Session string `json:"session_id"`
	}
	if json.Unmarshal(line, &head) != nil {
		return
	}
	c.mu.Lock()
	status := c.handle(line, false)
	if status != "" {
		c.status = status
	}
	c.mu.Unlock()
	if head.Type == "system" && head.Subtype == "init" && head.Session != "" {
		s.setAgentStatus("session:" + head.Session)
	}
	if status != "" {
		s.setAgentStatus(status)
	}
	c.changed()
}

// handle applies one stream-json line and returns the new agent status, if
// any. It requires mu.
func (c *chatState) handle(line []byte, replay bool) string {
	var msg struct {
		Type      string          `json:"type"`
		Subtype   string          `json:"subtype"`
		Parent    *string         `json:"parent_tool_use_id"`
		Event     json.RawMessage `json:"event"`
		Message   json.RawMessage `json:"message"`
		RequestID string          `json:"request_id"`
		Request   json.RawMessage `json:"request"`
		IsError   bool            `json:"is_error"`
		Result    string          `json:"result"`
		Errors    []string        `json:"errors"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return ""
	}
	// Subagents work inside their Task step.
	if msg.Parent != nil && *msg.Parent != "" {
		return ""
	}
	switch msg.Type {
	case "stream_event":
		c.streamEvent(msg.Event)
	case "assistant":
		c.assistant(msg.Message)
	case "user":
		c.user(msg.Message, replay)
	case "control_request":
		return c.controlRequest(msg.RequestID, msg.Request)
	case "control_cancel_request":
		c.removeRequest(msg.RequestID)
		if len(c.requests) == 0 {
			return "working"
		}
	case "result":
		c.streaming = ""
		c.streamed = map[string][]string{}
		c.requests = nil
		for i := range c.items {
			if c.items[i].State == "running" {
				c.items[i].State = "done"
				c.items[i].Version++
			}
		}
		if msg.IsError && msg.Subtype != "error_during_execution" {
			text := msg.Result
			if text == "" {
				text = strings.Join(msg.Errors, "\n")
			}
			if text == "" {
				text = "Claude stopped with an error."
			}
			c.add(ChatItem{Kind: "error", Text: text})
			return "error"
		}
		return "done"
	}
	return ""
}

func (c *chatState) streamEvent(raw json.RawMessage) {
	var event struct {
		Type         string `json:"type"`
		ContentBlock struct {
			Type string `json:"type"`
		} `json:"content_block"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	switch event.Type {
	case "message_start":
		c.streamedMessage = event.Message.ID
	case "content_block_start":
		c.streaming = ""
		if event.ContentBlock.Type == "text" {
			c.streaming = c.add(ChatItem{Kind: "assistant"})
			c.streamed[c.streamedMessage] = append(c.streamed[c.streamedMessage], c.streaming)
		}
	case "content_block_delta":
		if event.Delta.Type == "text_delta" && c.streaming != "" {
			c.update(c.streaming, func(item *ChatItem) { item.Text += event.Delta.Text })
		}
	case "content_block_stop":
		c.streaming = ""
	}
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (c *chatState) assistant(raw json.RawMessage) {
	var message struct {
		ID      string        `json:"id"`
		Content []claudeBlock `json:"content"`
	}
	if json.Unmarshal(raw, &message) != nil {
		return
	}
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
			if ids := c.streamed[message.ID]; len(ids) > 0 {
				c.streamed[message.ID] = ids[1:]
				c.update(ids[0], func(item *ChatItem) { item.Text = block.Text })
				continue
			}
			c.add(ChatItem{Kind: "assistant", Text: block.Text})
		case "tool_use":
			c.add(ChatItem{ID: "t-" + block.ID, Kind: "tool", Tool: block.Name, Text: toolSummary(block.Name, block.Input), State: "running"})
		}
	}
}

func (c *chatState) user(raw json.RawMessage, replay bool) {
	var message struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &message) != nil {
		return
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		if replay && userPrompt(text) {
			c.add(ChatItem{Kind: "user", Text: text})
		}
		return
	}
	var blocks []claudeBlock
	if json.Unmarshal(message.Content, &blocks) != nil {
		return
	}
	var typed []string
	images := 0
	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			result := resultText(block.Content)
			c.update("t-"+block.ToolUseID, func(item *ChatItem) {
				item.State = "done"
				if block.IsError {
					item.State = "failed"
				}
				item.Detail = result
			})
		case "text":
			typed = append(typed, block.Text)
		case "image":
			images++
		}
	}
	text = strings.Join(typed, "\n")
	if replay && (images > 0 && !strings.HasPrefix(strings.TrimSpace(text), "<") || userPrompt(text)) {
		c.add(ChatItem{Kind: "user", Text: text, Images: images})
	}
}

// userPrompt leaves out command wrappers and reminders Claude saves as
// user messages.
func userPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return text != "" && !strings.HasPrefix(text, "<") && !strings.HasPrefix(text, "Caveat:")
}

func resultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		var parts []claudeBlock
		_ = json.Unmarshal(raw, &parts)
		var lines []string
		for _, part := range parts {
			if part.Type == "text" {
				lines = append(lines, part.Text)
			}
		}
		text = strings.Join(lines, "\n")
	}
	text = strings.TrimSpace(text)
	if runes := []rune(text); len(runes) > chatMaxDetail {
		text = "…" + string(runes[len(runes)-chatMaxDetail:])
	}
	return text
}

// toolSummary is the part of a tool's input that says what it does.
func toolSummary(tool string, raw json.RawMessage) string {
	var input map[string]any
	_ = json.Unmarshal(raw, &input)
	text := func(key string) string {
		value, _ := input[key].(string)
		return value
	}
	var summary string
	switch tool {
	case "Bash":
		summary = text("command")
	case "Read", "Write", "Edit", "MultiEdit", "NotebookEdit":
		summary = cmpString(text("file_path"), text("notebook_path"))
	case "Grep", "Glob":
		summary = text("pattern")
	case "WebFetch":
		summary = text("url")
	case "WebSearch":
		summary = text("query")
	case "Task", "Agent":
		summary = text("description")
	case "Skill":
		summary = text("skill")
	case "TodoWrite":
		summary = "Update the task list"
	case "AskUserQuestion":
		var questions struct {
			Questions []ChatQuestion `json:"questions"`
		}
		_ = json.Unmarshal(raw, &questions)
		if len(questions.Questions) > 0 {
			summary = questions.Questions[0].Question
		}
	case "ExitPlanMode":
		summary = "Plan ready for review"
	}
	if summary == "" {
		for _, key := range []string{"description", "command", "path", "file_path", "url", "query", "prompt", "name"} {
			if summary = text(key); summary != "" {
				break
			}
		}
	}
	summary = strings.Join(strings.Fields(summary), " ")
	if runes := []rune(summary); len(runes) > 300 {
		summary = string(runes[:300]) + "…"
	}
	return summary
}

func cmpString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (c *chatState) controlRequest(id string, raw json.RawMessage) string {
	var request struct {
		Subtype  string          `json:"subtype"`
		ToolName string          `json:"tool_name"`
		Input    json.RawMessage `json:"input"`
	}
	if json.Unmarshal(raw, &request) != nil {
		return ""
	}
	if request.Subtype != "can_use_tool" {
		// Libro registers no hooks or SDK MCP servers.
		go func() {
			_ = c.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": "not supported by Libro"}})
		}()
		return ""
	}
	pending := ChatRequest{ID: id, Tool: request.ToolName, input: request.Input}
	var input struct {
		Questions []ChatQuestion `json:"questions"`
		Plan      string         `json:"plan"`
	}
	_ = json.Unmarshal(request.Input, &input)
	switch request.ToolName {
	case "AskUserQuestion":
		pending.Questions = input.Questions
	case "ExitPlanMode":
		pending.Plan = input.Plan
	default:
		// Other tools run without asking. Claude reads its output while
		// Libro writes, so the answer must not wait here.
		go func() {
			_ = c.write(controlAllow(id, request.Input, nil))
		}()
		return ""
	}
	c.requests = append(c.requests, pending)
	return "input"
}

// removeRequest requires mu.
func (c *chatState) removeRequest(id string) (ChatRequest, bool) {
	for i, request := range c.requests {
		if request.ID == id {
			c.requests = append(c.requests[:i:i], c.requests[i+1:]...)
			return request, true
		}
	}
	return ChatRequest{}, false
}

func (tm *TerminalManager) chatSession(appID string) (*TerminalSession, error) {
	s := tm.session(appID)
	if s == nil || s.chat == nil || s.isClosed() {
		return nil, fmt.Errorf("claude is not running")
	}
	return s, nil
}

// ChatSend sends the user's message and PNG images to Claude.
func (tm *TerminalManager) ChatSend(appID, text string, images [][]byte) error {
	s, err := tm.chatSession(appID)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" && len(images) == 0 {
		return nil
	}
	var content any = text
	if len(images) > 0 {
		blocks := make([]map[string]any, 0, len(images)+1)
		for _, image := range images {
			blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString(image)}})
		}
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
		content = blocks
	}
	c := s.chat
	if err = c.write(map[string]any{"type": "user", "session_id": "", "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": content}}); err != nil {
		return fmt.Errorf("claude is not running")
	}
	c.mu.Lock()
	first := true
	for _, item := range c.items {
		if item.Kind == "user" {
			first = false
			break
		}
	}
	c.add(ChatItem{Kind: "user", Text: text, Images: len(images)})
	c.status = "working"
	c.mu.Unlock()
	if first && text != "" {
		s.agentMu.Lock()
		s.setAgentTitle(text, 1)
		s.agentMu.Unlock()
	}
	s.setAgentStatus("working")
	c.changed()
	return nil
}

// ChatInterrupt stops Claude's current turn.
func (tm *TerminalManager) ChatInterrupt(appID string) error {
	s, err := tm.chatSession(appID)
	if err != nil {
		return err
	}
	return s.chat.write(map[string]any{"type": "control_request", "request_id": fmt.Sprintf("libro-stop-%d", time.Now().UnixNano()), "request": map[string]any{"subtype": "interrupt"}})
}

// ChatAnswer answers a question or a plan.
func (tm *TerminalManager) ChatAnswer(appID, requestID string, answer ChatAnswer) error {
	s, err := tm.chatSession(appID)
	if err != nil {
		return err
	}
	c := s.chat
	c.mu.Lock()
	request, ok := c.removeRequest(requestID)
	left := len(c.requests)
	if ok && left == 0 {
		c.status = "working"
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("this request is no longer open")
	}
	message := map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": requestID,
		"response": map[string]any{"behavior": "deny", "message": cmpString(strings.TrimSpace(answer.Message), "The user declined this.")}}}
	if answer.Allow {
		message = controlAllow(requestID, request.input, answer.Answers)
	}
	err = c.write(message)
	if left == 0 {
		s.setAgentStatus("working")
	}
	c.changed()
	return err
}

// controlAllow lets a tool run with its input and the user's answers.
func controlAllow(requestID string, raw json.RawMessage, answers map[string]string) map[string]any {
	var input map[string]any
	_ = json.Unmarshal(raw, &input)
	if input == nil {
		input = map[string]any{}
	}
	if len(answers) > 0 {
		input["answers"] = answers
	}
	return map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": requestID,
		"response": map[string]any{"behavior": "allow", "updatedInput": input}}}
}

// ChatSnapshot returns the chat of appID; ok is false without a chat session.
func (tm *TerminalManager) ChatSnapshot(appID string) (ChatSnapshot, bool) {
	s := tm.session(appID)
	if s == nil || s.chat == nil {
		tm.mu.Lock()
		s = tm.endedChats[appID]
		tm.mu.Unlock()
	}
	if s == nil || s.chat == nil {
		return ChatSnapshot{}, false
	}
	c := s.chat
	c.mu.Lock()
	defer c.mu.Unlock()
	return ChatSnapshot{
		Items:    append([]ChatItem(nil), c.items...),
		Requests: append([]ChatRequest(nil), c.requests...),
		Status:   c.status,
		Running:  !s.isClosed(),
		Dir:      filepath.Base(s.Cwd),
	}, true
}

// chatExited records why the process ended and closes open requests.
func (s *TerminalSession) chatExited(code int) {
	c := s.chat
	c.mu.Lock()
	c.requests = nil
	text := "Claude stopped. Send a message to start it again."
	if code != 0 {
		if detail := strings.TrimSpace(c.stderr.String()); detail != "" {
			lines := strings.Split(detail, "\n")
			if len(lines) > 6 {
				lines = lines[len(lines)-6:]
			}
			text = fmt.Sprintf("Claude stopped (exit code %d): %s\nSend a message to start it again.", code, strings.Join(lines, "\n"))
		}
	}
	c.add(ChatItem{Kind: "notice", Text: text})
	c.status = "idle"
	c.mu.Unlock()
	c.changed()
}
