package libro

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	r "github.com/michalCapo/g-sui/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"libro/internal/components"
)

// Chat panels show Claude as messages instead of a terminal. The Claude
// process runs in the background (components.StartChat); this file draws
// the chat, after Kodri's timeline, and sends the user's input to it.

var chatApp *r.App

// chatAgent reports whether an agent command can run as a chat: built-in
// Claude and custom agents whose command runs Claude.
func chatAgent(command string) bool { return components.ChatCommand(command) }

// claudeChatView reports whether new Claude panels open as a chat. It is on
// unless the user chose the terminal in Settings.
func claudeChatView() bool {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return true
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'claude_view'`).Scan(&value); err != nil {
		return true
	}
	return value != "terminal"
}

func setClaudeChatView(chat bool) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return sql.ErrConnDone
	}
	value := "terminal"
	if chat {
		value = "chat"
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES ('claude_view',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, value)
	return err
}

// setAppChat makes a new agent panel a chat.
func (sm *StateManager) setAppChat(sid, appID string) {
	defer sm.scheduleLayoutSave(sid)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if state := sm.states[sid]; state != nil {
		for i := range state.Apps {
			if state.Apps[i].ID == appID {
				state.Apps[i].Chat = true
			}
		}
	}
}

// startAgentSession starts a panel's process: a chat or a terminal.
func startAgentSession(generation uint64, panel Application, command, pwd string, environment []string, reportSession func(string)) (*components.TerminalSession, error) {
	if panel.Chat {
		id := panel.ID
		return tm.StartChat(generation, id, command, pwd, environment, reportSession, func() { scheduleChatRender(id) })
	}
	return tm.StartWithSessionReporter(generation, panel.ID, command, pwd, panel.Writable, environment, reportSession)
}

func chatBodyID(appID string) string     { return "chat-body-" + appID }
func chatRequestsID(appID string) string { return "chat-requests-" + appID }
func chatStatusID(appID string) string   { return "chat-status-" + appID }

// Live updates are drawn at most every chatRenderDelay per panel.
const chatRenderDelay = 80 * time.Millisecond

var chatRender = struct {
	sync.Mutex
	pending  map[string]bool
	requests map[string]string // the requests last drawn, so typed answers stay
	items    map[string]map[string]chatCached
}{pending: map[string]bool{}, requests: map[string]string{}, items: map[string]map[string]chatCached{}}

type chatCached struct {
	version int
	node    *r.Node
}

func scheduleChatRender(appID string) {
	chatRender.Lock()
	defer chatRender.Unlock()
	if chatRender.pending[appID] {
		return
	}
	chatRender.pending[appID] = true
	time.AfterFunc(chatRenderDelay, func() {
		chatRender.Lock()
		delete(chatRender.pending, appID)
		chatRender.Unlock()
		broadcastChat(appID)
	})
}

func broadcastChat(appID string) {
	if chatApp == nil {
		return
	}
	snapshot, ok := tm.ChatSnapshot(appID)
	if !ok {
		forgetChat(appID)
		return
	}
	result := r.Result{}.Morph(chatBodyID(appID), renderChatBody(appID, snapshot)).
		Morph(chatStatusID(appID), renderChatStatus(appID, snapshot))
	signature := chatRequestSignature(snapshot.Requests)
	chatRender.Lock()
	if chatRender.requests[appID] != signature {
		chatRender.requests[appID] = signature
		result = result.Morph(chatRequestsID(appID), renderChatRequests(appID, snapshot.Requests))
	}
	chatRender.Unlock()
	if err := chatApp.Broadcast(result); err != nil {
		log.Printf("libro: chat update: %v", err)
	}
}

func chatRequestSignature(requests []components.ChatRequest) string {
	ids := make([]string, len(requests))
	for i, request := range requests {
		ids[i] = request.ID
	}
	return strings.Join(ids, ",")
}

// forgetChat drops the cached rendering of a closed panel.
func forgetChat(appID string) {
	chatRender.Lock()
	delete(chatRender.requests, appID)
	delete(chatRender.items, appID)
	chatRender.Unlock()
}

// renderChat draws a chat panel: the timeline, open requests, the state and
// the message box.
func renderChat(app Application, sid string) *r.Node {
	snapshot, _ := tm.ChatSnapshot(app.ID)
	chatRender.Lock()
	chatRender.requests[app.ID] = chatRequestSignature(snapshot.Requests)
	chatRender.Unlock()
	field := r.Textarea("ws-chat-field").
		Attr("rows", "1").
		Attr("placeholder", "Message Claude").
		Attr("aria-label", "Message Claude").
		Attr("data-chat-input", "")
	send := r.Button("ws-chat-send").Attr("type", "submit").Attr("aria-label", "Send").Attr("title", "Send (Enter)").
		Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("arrow_upward"))
	return r.Widget("chat", struct{}{}, "ws-chat").Key("chat-"+app.ID).
		Attr("data-chat", app.ID).
		Attr("data-terminal-app", app.ID).
		Attr("data-sid", sid).
		Render(
			r.Div("ws-chat-scroll").Attr("data-chat-scroll", "").Render(
				r.Div("ws-chat-column").Render(
					renderChatBody(app.ID, snapshot),
					renderChatRequests(app.ID, snapshot.Requests),
				),
			),
			r.Div("ws-chat-dock").Render(
				renderChatStatus(app.ID, snapshot),
				r.Form("ws-chat-composer").Attr("data-chat-form", "").Render(field, send),
			),
		)
}

// renderChatBody is the part of the chat that updates live.
func renderChatBody(appID string, snapshot components.ChatSnapshot) *r.Node {
	chatRender.Lock()
	cache := chatRender.items[appID]
	if cache == nil {
		cache = map[string]chatCached{}
		chatRender.items[appID] = cache
	}
	nodes := make([]*r.Node, 0, len(snapshot.Items)+1)
	seen := make(map[string]bool, len(snapshot.Items))
	for _, item := range snapshot.Items {
		seen[item.ID] = true
		cached, ok := cache[item.ID]
		if !ok || cached.version != item.Version {
			cached = chatCached{item.Version, chatItem(item)}
			cache[item.ID] = cached
		}
		if cached.node != nil {
			nodes = append(nodes, cached.node)
		}
	}
	for id := range cache {
		if !seen[id] {
			delete(cache, id)
		}
	}
	chatRender.Unlock()
	if snapshot.Running && snapshot.Status == "working" && len(snapshot.Requests) == 0 && !chatToolRunning(snapshot.Items) {
		nodes = append(nodes, chatRow("chat-working", "ws-chat-dot is-live", r.P("ws-chat-step is-live").Text("Thinking…")))
	}
	if len(nodes) == 0 {
		nodes = append(nodes, r.Div("ws-chat-empty").Render(
			r.P("").Text("Ask Claude to do something in this project."),
			r.P("ws-chat-hint").Text("Enter sends. Shift+Enter starts a new line."),
		))
	}
	return r.Div("ws-chat-timeline").ID(chatBodyID(appID)).Attr("aria-live", "polite").Render(nodes...)
}

func chatToolRunning(items []components.ChatItem) bool {
	return len(items) > 0 && items[len(items)-1].Kind == "tool" && items[len(items)-1].State == "running"
}

// chatRow is one line of the timeline: a dot on the line and content.
func chatRow(id, dot string, content *r.Node) *r.Node {
	return r.Div("ws-chat-row").ID(id).Key(id).Render(
		r.Div("ws-chat-rail").Attr("aria-hidden", "true").Render(r.Span(dot), r.Span("ws-chat-line")),
		r.Div("ws-chat-content").Render(content),
	)
}

// chatItem draws a message, a tool step or a notice. Your messages have a
// filled dot, Claude's a ring; tool steps are small grey dots.
func chatItem(item components.ChatItem) *r.Node {
	id := "chat-item-" + item.ID
	head := func(who string) *r.Node { return r.Strong("ws-chat-who").Text(who) }
	switch item.Kind {
	case "user":
		return chatRow(id, "ws-chat-dot is-user", r.Div("ws-chat-message").Render(
			head("You"),
			r.P("ws-chat-text").Text(item.Text),
		))
	case "assistant":
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return chatRow(id, "ws-chat-dot is-agent", r.Div("ws-chat-message").Render(
			head("Claude"),
			chatMarkdown(item.Text),
		))
	case "tool":
		dot, class, label := "ws-chat-dot is-step", "ws-chat-step", item.Tool
		if item.Text != "" {
			label += " · " + item.Text
		}
		switch item.State {
		case "running":
			dot, class, label = "ws-chat-dot is-live", "ws-chat-step is-live", label+" …"
		case "failed":
			class, label = "ws-chat-step is-failed", label+" · failed"
		}
		line := r.Span(class).Attr("title", label).Text(label)
		if item.Detail == "" {
			return chatRow(id, dot, r.Div("ws-chat-tool").Render(line))
		}
		return chatRow(id, dot, r.El("details", "ws-chat-tool").Render(
			r.El("summary", "").Render(line),
			r.El("pre", "ws-chat-detail").Text(item.Detail),
		))
	case "notice":
		return chatRow(id, "ws-chat-dot is-notice", r.P("ws-chat-notice").Text(item.Text))
	case "error":
		return chatRow(id, "ws-chat-dot is-error", r.P("ws-chat-notice is-error").Text(item.Text))
	}
	return nil
}

// markdownImage starts any image, also a reference one: ![a][r].
var markdownImage = regexp.MustCompile(`!\[`)

var chatMarkdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

// chatMarkdown draws Claude's Markdown. Images show as links: an image would
// load by itself. Raw HTML and unsafe links are left out by goldmark. The
// text's hash keys the block and Preserve keeps a drawn block as it is, so
// old answers are not laid out again on every update.
func chatMarkdown(md string) *r.Node {
	sum := sha256.Sum256([]byte(md))
	var html bytes.Buffer
	if err := chatMarkdownRenderer.Convert([]byte(markdownImage.ReplaceAllString(md, "[")), &html); err != nil {
		return r.P("ws-chat-text").Text(md)
	}
	return r.Div("ws-chat-markdown").Key("md-"+hex.EncodeToString(sum[:8])).Preserve().
		Attr("data-html", html.String()).
		UnsafeJS("this.innerHTML=this.getAttribute('data-html');this.removeAttribute('data-html');")
}

// renderChatStatus is the state of the chat with the stop button.
func renderChatStatus(appID string, snapshot components.ChatSnapshot) *r.Node {
	label, class := "Ready", "ws-chat-state"
	switch {
	case !snapshot.Running:
		label = "Stopped"
	case len(snapshot.Requests) > 0:
		label, class = "Waiting for you", "ws-chat-state is-waiting"
	case snapshot.Status == "working":
		label, class = "Working", "ws-chat-state is-working"
	case snapshot.Status == "done":
		label, class = "Done", "ws-chat-state is-done"
	case snapshot.Status == "error":
		label, class = "Failed", "ws-chat-state is-failed"
	}
	working := snapshot.Running && snapshot.Status == "working"
	return r.Div("ws-chat-status").ID(chatStatusID(appID)).Render(
		r.Span(class).Attr("role", "status").Render(r.Span("ws-chat-state-dot").Attr("aria-hidden", "true"), r.Span("").Text(label)),
		r.If(working, r.Button("ws-chat-stop").Attr("type", "button").Attr("data-chat-stop", "").Attr("title", "Stop Claude").
			Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("stop"), r.Span("").Text("Stop"))),
	)
}

// renderChatRequests draws Claude's open permission requests and questions.
func renderChatRequests(appID string, requests []components.ChatRequest) *r.Node {
	cards := make([]*r.Node, 0, len(requests))
	for _, request := range requests {
		cards = append(cards, chatRequestCard(request))
	}
	return r.Div("ws-chat-requests").ID(chatRequestsID(appID)).Render(cards...)
}

func chatButton(class, label string, attrs ...string) *r.Node {
	button := r.Button(class).Attr("type", "button").Text(label)
	for i := 0; i+1 < len(attrs); i += 2 {
		button.Attr(attrs[i], attrs[i+1])
	}
	return button
}

// chatRequestCard asks the user: an OK for a tool, a plan, or Claude's
// questions. A typed reply declines and tells Claude what to do instead.
func chatRequestCard(request components.ChatRequest) *r.Node {
	card := r.El("section", "ws-chat-card").Attr("data-chat-request", request.ID)
	switch {
	case len(request.Questions) > 0:
		quick := len(request.Questions) == 1 && !request.Questions[0].MultiSelect
		card.Attr("aria-label", "Claude asks")
		card.Render(r.Span("ws-chat-eyebrow").Text("Claude asks"))
		for i, question := range request.Questions {
			name := fmt.Sprintf("q%d", i)
			kind := "radio"
			if question.MultiSelect {
				kind = "checkbox"
			}
			options := make([]*r.Node, 0, len(question.Options))
			for _, option := range question.Options {
				input := r.Input("ws-chat-choice-input").Attr("type", kind).Attr("name", name).Attr("value", option.Label)
				if quick {
					input.Attr("data-chat-quick", "")
				}
				options = append(options, r.Label("ws-chat-choice").Attr("title", option.Description).Render(
					input,
					r.Span("ws-chat-choice-label").Text(option.Label),
					r.If(option.Description != "", r.Span("ws-chat-choice-help").Text(option.Description)),
				))
			}
			card.Render(r.Fieldset("ws-chat-question").Attr("data-question", question.Question).Render(
				r.Legend("ws-chat-question-text").Text(question.Question),
				r.Div("ws-chat-choices").Render(options...),
				r.Input("ws-chat-input").Attr("type", "text").Attr("name", name+"-other").Attr("placeholder", "Or type your own answer").Attr("aria-label", "Your own answer"),
			))
		}
		card.Render(r.Div("ws-chat-actions").Render(
			chatButton("ws-chat-primary", "Send answers", "data-chat-answer", "questions"),
			chatButton("ws-chat-secondary", "Skip", "data-chat-answer", "deny"),
		))
	case request.Tool == "ExitPlanMode":
		card.Attr("aria-label", "Claude's plan")
		card.Render(
			r.Span("ws-chat-eyebrow").Text("Claude's plan"),
			chatMarkdown(request.Plan),
			r.Div("ws-chat-actions").Render(
				chatButton("ws-chat-primary", "Approve plan", "data-chat-answer", "allow"),
				chatButton("ws-chat-secondary", "Keep planning", "data-chat-answer", "deny"),
			),
			chatReplyField("Tell Claude what to change"),
		)
	default:
		card.Attr("aria-label", "Claude wants to use "+request.Tool)
		card.Render(
			r.Span("ws-chat-eyebrow").Text("Allow "+request.Tool+"?"),
			r.If(request.Summary != "", r.El("pre", "ws-chat-request-text").Text(request.Summary)),
			r.Div("ws-chat-actions").Render(
				chatButton("ws-chat-primary", "Allow", "data-chat-answer", "allow"),
				r.If(request.CanAlways, chatButton("ws-chat-secondary", "Always allow", "data-chat-answer", "always")),
				chatButton("ws-chat-secondary", "Deny", "data-chat-answer", "deny"),
			),
			chatReplyField("Or tell Claude what to do instead"),
		)
	}
	return card
}

func chatReplyField(placeholder string) *r.Node {
	return r.Div("ws-chat-reply").Render(
		r.Input("ws-chat-input").Attr("type", "text").Attr("name", "message").Attr("placeholder", placeholder).Attr("aria-label", placeholder),
		chatButton("ws-chat-icon", "", "data-chat-answer", "reply", "aria-label", "Send reply", "title", "Send reply").
			Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("arrow_upward")),
	)
}

type actionChatSendInput struct {
	SID   string `json:"sid"`
	AppID string `json:"appId"`
	Text  string `json:"text"`
}

type actionSettingsClaudeViewInput struct {
	Chat *bool `json:"chat"`
}

type actionChatAnswerInput struct {
	SID     string            `json:"sid"`
	AppID   string            `json:"appId"`
	Request string            `json:"request"`
	Allow   bool              `json:"allow"`
	Always  bool              `json:"always"`
	Message string            `json:"message"`
	Answers map[string]string `json:"answers"`
}

func chatError(err error) r.Result {
	return r.Result{}.Run(r.Notify("error", err.Error()))
}

// ensureChat starts the panel's Claude again after it stopped, resuming its
// session.
func ensureChat(sid, appID string) error {
	if tm.IsRunning(appID) {
		return nil
	}
	panel, workspace, pwd, found := sm.workspaceApp(sid, appID)
	if !found || !panel.Chat {
		return errors.New("this chat panel is closed")
	}
	command, reportSession := agentLaunchInWorkspace(sid, workspace, panel)
	environment := append(agentEnvironmentList(), "LIBRO_APPLICATION_PATH="+pwd)
	_, err := startAgentSession(tm.Generation(), panel, command, pwd, environment, reportSession)
	return err
}

func registerChatActions(app *r.App) {
	chatApp = app
	registerWorkspaceAction(app, "chat.send", func(_ *r.Context, in actionChatSendInput) (r.Result, error) {
		sid := inputSID(in.SID)
		restarted := !tm.IsRunning(in.AppID)
		if err := ensureChat(sid, in.AppID); err != nil {
			return chatError(err), nil
		}
		if err := tm.ChatSend(in.AppID, in.Text); err != nil {
			return chatError(err), nil
		}
		if restarted {
			return clientScript("if(window.__libroRestartTerminal)window.__libroRestartTerminal(props[0]);", in.AppID), nil
		}
		return r.Result{}, nil
	})
	registerWorkspaceAction(app, "chat.stop", func(_ *r.Context, in actionChatSendInput) (r.Result, error) {
		if err := tm.ChatInterrupt(in.AppID); err != nil {
			return chatError(err), nil
		}
		return r.Result{}, nil
	})
	registerWorkspaceAction(app, "chat.answer", func(_ *r.Context, in actionChatAnswerInput) (r.Result, error) {
		err := tm.ChatAnswer(in.AppID, in.Request, components.ChatAnswer{Allow: in.Allow, Always: in.Always, Message: in.Message, Answers: in.Answers})
		if err != nil {
			scheduleChatRender(in.AppID)
			return chatError(err), nil
		}
		return r.Result{}, nil
	})
	registerWorkspaceAction(app, "settings.claude-view", func(_ *r.Context, in actionSettingsClaudeViewInput) (r.Result, error) {
		chat, ok := inputField(in.Chat)
		saved := ok && setClaudeChatView(chat) == nil
		return clientScript("libroWorkspace.claudeViewSaved(props[0],props[1]);", saved, claudeChatView()), nil
	})
}
