package libro

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// The chat's look comes from Kodri: turns in white windows on a grey page,
// a timeline with dots, mono steps, round black buttons and a rounded
// message box. Colors are Libro's, so light and dark follow the app.
const (
	chatFocus         = "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--ws-accent)"
	chatCard          = "rounded-2xl bg-(--ws-raised) ring-1 ring-(--ws-line)"
	chatEyebrow       = "font-mono text-[11px] uppercase tracking-[0.08em]"
	chatMuted         = "text-(--ws-muted)"
	chatText          = "mt-1 whitespace-pre-wrap text-[14px] leading-relaxed [overflow-wrap:anywhere]"
	chatButton        = "inline-flex min-h-9 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-full border-0 px-4 text-[13px] font-medium transition-colors disabled:cursor-default disabled:opacity-50 " + chatFocus
	chatSolid         = chatButton + " bg-(--ws-fg) text-(--ws-bg) hover:bg-(--ws-fg)/85"
	chatOutline       = chatButton + " bg-transparent text-(--ws-fg) ring-1 ring-inset ring-(--ws-control-line) hover:bg-(--ws-hover)"
	chatGhostIcon     = "flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-transparent text-(--ws-muted) transition-colors hover:bg-(--ws-hover) hover:text-(--ws-fg) disabled:cursor-default disabled:opacity-50 " + chatFocus
	chatRound         = "flex shrink-0 cursor-pointer items-center justify-center rounded-full border-0 bg-(--ws-fg) text-(--ws-bg) transition-colors hover:bg-(--ws-fg)/85 disabled:cursor-default disabled:opacity-40 " + chatFocus
	chatField         = "h-10 w-full min-w-0 rounded-full border-0 bg-(--ws-raised) px-4 text-[14px] text-(--ws-fg) outline-none ring-1 ring-inset ring-(--ws-control-line) placeholder:text-(--ws-muted) focus:ring-2 focus:ring-(--ws-accent)"
	chatMarkdownClass = "ws-chat-markdown mt-1 text-[14px] leading-relaxed [overflow-wrap:anywhere] " +
		"[&>:first-child]:mt-0 [&>:last-child]:mb-0 [&_p]:my-2 [&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-0.5 " +
		"[&_h1]:mb-1.5 [&_h1]:mt-4 [&_h1]:text-[17px] [&_h1]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-4 [&_h2]:text-[15px] [&_h2]:font-semibold [&_h3]:mt-3 [&_h3]:font-semibold [&_h4]:mt-3 [&_h4]:font-semibold [&_strong]:font-semibold " +
		"[&_a]:text-(--ws-accent) [&_a]:underline [&_code]:rounded [&_code]:bg-(--ws-sunken) [&_code]:px-1 [&_code]:font-mono [&_code]:text-[12.5px] " +
		"[&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-xl [&_pre]:bg-(--ws-sunken) [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 " +
		"[&_table]:my-3 [&_table]:w-full [&_table]:border-collapse [&_table]:text-[13px] [&_th]:border-b [&_th]:border-(--ws-line) [&_th]:py-1.5 [&_th]:pr-4 [&_th]:text-left [&_th]:align-bottom [&_th]:font-semibold " +
		"[&_td]:border-b [&_td]:border-(--ws-line) [&_td]:py-1.5 [&_td]:pr-4 [&_td]:align-top [&_hr]:my-4 [&_hr]:border-(--ws-line) " +
		"[&_blockquote]:my-2 [&_blockquote]:border-l-2 [&_blockquote]:border-(--ws-control-line) [&_blockquote]:pl-3 [&_blockquote]:text-(--ws-muted) " +
		"[&_li:has(>input)]:list-none [&_li>input]:mr-1.5 [&_li>input]:align-middle"
)

func chatIcon(name, class string) *r.Node {
	return r.I("material-icons-round leading-none "+class).Attr("aria-hidden", "true").Text(name)
}

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
	result := r.Result{}.Morph(chatBodyID(appID), renderChatBody(appID, snapshot))
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

// renderChat draws a chat panel: the turns, open questions and the message
// box under them.
func renderChat(app Application, sid string) *r.Node {
	snapshot, _ := tm.ChatSnapshot(app.ID)
	chatRender.Lock()
	chatRender.requests[app.ID] = chatRequestSignature(snapshot.Requests)
	chatRender.Unlock()
	return r.Widget("chat", struct{}{}, "absolute inset-0 flex flex-col bg-(--ws-sunken) text-(--ws-fg)").Key("chat-"+app.ID).
		Attr("data-chat", app.ID).
		Attr("data-terminal-app", app.ID).
		Attr("data-sid", sid).
		Render(
			r.Div("min-h-0 flex-1 overflow-auto [scrollbar-color:var(--ws-line)_transparent] [scrollbar-width:thin]").Attr("data-chat-scroll", "").Render(
				r.Div("mx-auto w-full max-w-3xl px-4 pb-6 pt-5").Render(
					renderChatBody(app.ID, snapshot),
					renderChatRequests(app.ID, snapshot.Requests),
				),
			),
			r.Div("shrink-0 px-4 pb-4").Render(chatComposer()),
		)
}

// chatComposer is Kodri's message box: added images and the text on top,
// the buttons in a row under them. Images are pasted, dropped or picked.
func chatComposer() *r.Node {
	field := r.Textarea("block max-h-[40vh] min-h-10 w-full min-w-0 resize-none border-0 bg-transparent px-3 py-2 text-[14px] leading-6 text-(--ws-fg) outline-none [field-sizing:content] placeholder:text-(--ws-muted)").
		Attr("rows", "1").
		Attr("placeholder", "Message Claude").
		Attr("aria-label", "Message Claude").
		Attr("data-chat-input", "")
	pick := r.Input("sr-only").Attr("type", "file").Attr("accept", "image/*").Attr("multiple", "").
		Attr("aria-label", "Add images").Attr("data-chat-pick", "")
	return r.Form("@container mx-auto flex w-full max-w-3xl flex-col gap-1.5").Attr("data-chat-form", "").Render(
		r.P("px-4 text-[12px] text-red-700 empty:hidden dark:text-red-400").Attr("role", "alert").Attr("data-chat-error", ""),
		r.Div("flex w-full min-w-0 flex-col gap-0.5 rounded-[22px] bg-(--ws-raised) p-1.5 shadow-sm ring-1 ring-(--ws-line) transition-shadow focus-within:ring-2 focus-within:ring-(--ws-accent) data-[drop]:ring-2 data-[drop]:ring-(--ws-accent)").Attr("data-chat-box", "").Render(
			r.Div("flex flex-wrap gap-2 px-1.5 pt-1.5 empty:hidden").Attr("data-chat-tray", ""),
			field,
			r.Div("flex items-center gap-0.5").Render(
				r.Label(chatGhostIcon+" focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-(--ws-accent)").Attr("title", "Add images").Render(chatIcon("add_photo_alternate", "text-[20px]"), pick),
				r.Button(chatGhostIcon).Attr("type", "button").Attr("title", "Voice typing").Attr("aria-label", "Voice typing").
					Attr("data-chat-mic", "").Attr("data-voice-control", "").Render(chatIcon("mic", "text-[20px]")),
				r.Div("ml-auto flex items-center gap-3").Render(
					r.Span("hidden text-[11px] "+chatMuted+" @md:inline").Text("Enter sends · Shift+Enter new line"),
					r.Button(chatRound+" h-9 w-9").Attr("type", "submit").Attr("aria-label", "Send").Attr("title", "Send (Enter)").
						Attr("data-chat-send", "").Render(chatIcon("arrow_upward", "text-[20px]")),
				),
			),
		),
	)
}

// renderChatBody is the part of the chat that updates live. Each message of
// the user starts a turn, a window of its own, as in Kodri. The last window's
// bar has Claude's state; while Claude works, the stop button sits over its
// bottom edge.
func renderChatBody(appID string, snapshot components.ChatSnapshot) *r.Node {
	chatRender.Lock()
	cache := chatRender.items[appID]
	if cache == nil {
		cache = map[string]chatCached{}
		chatRender.items[appID] = cache
	}
	type turn struct {
		key   string
		items []*r.Node
	}
	turns := []turn{{key: "turn-start"}}
	seen := make(map[string]bool, len(snapshot.Items))
	now := time.Now()
	for _, item := range snapshot.Items {
		seen[item.ID] = true
		cached, ok := cache[item.ID]
		if !ok || cached.version != item.Version {
			cached = chatCached{item.Version, chatItem(item, now)}
			cache[item.ID] = cached
		}
		if cached.node == nil {
			continue
		}
		if item.Kind == "user" && len(turns[len(turns)-1].items) > 0 {
			turns = append(turns, turn{key: "turn-" + item.ID})
		}
		turns[len(turns)-1].items = append(turns[len(turns)-1].items, cached.node)
	}
	for id := range cache {
		if !seen[id] {
			delete(cache, id)
		}
	}
	chatRender.Unlock()
	working := snapshot.Running && snapshot.Status == "working" && len(snapshot.Requests) == 0
	last := &turns[len(turns)-1]
	if working && !chatToolRunning(snapshot.Items) {
		last.items = append(last.items, chatRow("chat-working", chatLiveDot, r.P("pb-4 font-mono text-[12px] text-(--ws-accent)").Text("Thinking…")))
	}
	body := r.Div("flex flex-col gap-5").ID(chatBodyID(appID)).Attr("aria-live", "polite")
	if len(last.items) == 0 {
		return body.Render(chatEmpty(snapshot.Dir))
	}
	for i, tn := range turns {
		state := r.Span("w-4").Attr("aria-hidden", "true")
		if i == len(turns)-1 {
			state = chatState(snapshot)
		}
		body.Render(r.Div(chatCard+" flex min-w-0 flex-col").Key(tn.key).Render(
			chatWindowBar(snapshot.Dir, state),
			r.Div("flex min-w-0 flex-col px-4 pt-5 sm:px-5").Render(tn.items...),
		))
	}
	return body.Render(r.If(working, r.Div("pointer-events-none sticky bottom-3 z-10 -mt-11 flex justify-end px-4").Render(
		r.Button("pointer-events-auto flex h-12 w-12 cursor-pointer items-center justify-center rounded-full border-0 bg-red-600 text-white shadow-md transition-colors hover:bg-red-700 "+chatFocus+" dark:bg-red-500 dark:hover:bg-red-400").
			Attr("type", "button").Attr("data-chat-stop", "").Attr("aria-label", "Stop Claude").Attr("title", "Stop Claude").
			Render(chatIcon("stop", "text-[24px]")),
	)))
}

// chatWindowBar is the title bar of a turn's window: three dots, the folder
// Claude works in and the state on the right.
func chatWindowBar(dir string, state *r.Node) *r.Node {
	dot := r.Span("h-2.5 w-2.5 rounded-full bg-(--ws-control-line)")
	return r.Div("flex min-h-10 items-center gap-3 rounded-t-[inherit] border-b border-(--ws-line) bg-(--ws-sunken)/60 px-4 py-2").Render(
		r.Span("flex gap-1.5").Attr("aria-hidden", "true").Render(dot, dot, dot),
		r.Span("min-w-0 flex-1 truncate text-center font-mono text-[11px] "+chatMuted).Attr("aria-hidden", "true").Text(dir),
		state,
	)
}

// chatState is Claude's state in the last window's bar.
func chatState(snapshot components.ChatSnapshot) *r.Node {
	label, dot, text := "Ready", "bg-(--ws-control-line)", chatMuted
	switch {
	case !snapshot.Running:
		label = "Stopped"
	case len(snapshot.Requests) > 0:
		label, dot, text = "Waiting for you", "bg-(--ws-accent)", "text-(--ws-accent)"
	case snapshot.Status == "working":
		label, dot, text = "Working", "bg-(--ws-accent) animate-pulse motion-reduce:animate-none", "text-(--ws-accent)"
	case snapshot.Status == "done":
		label, dot, text = "Done", "bg-[#15803d] dark:bg-[#4ade80]", "text-[#15803d] dark:text-[#4ade80]"
	case snapshot.Status == "error":
		label, dot, text = "Failed", "bg-red-600", "text-red-700 dark:text-red-400"
	}
	return r.Span("inline-flex min-w-0 shrink-0 items-center gap-1.5 font-mono text-[11px] "+text).Attr("role", "status").Render(
		r.Span("h-1.5 w-1.5 shrink-0 rounded-full "+dot).Attr("aria-hidden", "true"),
		r.Span("truncate").Text(label),
	)
}

// chatEmpty says that nothing happened yet, in a card with Kodri's dot grid.
func chatEmpty(dir string) *r.Node {
	text := "Claude works in this project. Paste or drop images, press Enter to send."
	if dir != "" && dir != "." && dir != "/" {
		text = "Claude works in " + dir + ". Paste or drop images, press Enter to send."
	}
	return r.Div(chatCard+" relative isolate mt-[12vh] flex flex-col items-center overflow-hidden px-6 py-12 text-center").Render(
		r.Div("pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(var(--ws-control-line)_1px,transparent_1px)] bg-[size:22px_22px] [mask-image:radial-gradient(ellipse_70%_70%_at_50%_0%,black_20%,transparent_100%)]").Attr("aria-hidden", "true"),
		r.Div("pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(ellipse_60%_60%_at_50%_0%,color-mix(in_srgb,var(--ws-accent)_14%,transparent),transparent_70%)]").Attr("aria-hidden", "true"),
		r.Span("flex h-12 w-12 items-center justify-center rounded-full bg-(--ws-accent)/10 text-(--ws-accent) ring-1 ring-(--ws-accent)/20").Render(chatIcon("forum", "text-[24px]")),
		r.H3("mt-5 text-[20px] font-medium tracking-tight text-balance").Text("What should Claude do?"),
		r.P("mt-2 max-w-md text-[14px] leading-6 text-balance "+chatMuted).Text(text),
	)
}

func chatToolRunning(items []components.ChatItem) bool {
	return len(items) > 0 && items[len(items)-1].Kind == "tool" && items[len(items)-1].State == "running"
}

// The timeline's dots: yours is filled, Claude's a ring, steps are small
// grey dots and the live step has an accent ring.
const (
	chatUserDot  = "h-2.5 w-2.5 bg-(--ws-fg)"
	chatAgentDot = "h-2.5 w-2.5 bg-(--ws-raised) ring-2 ring-inset ring-(--ws-fg)"
	chatStepDot  = "ml-0.5 h-1.5 w-1.5 bg-(--ws-control-line)"
	chatLiveDot  = "h-2 w-2 bg-(--ws-raised) ring-2 ring-(--ws-accent)"
)

// chatRow is one line of the timeline: a dot on the line and content.
func chatRow(id, dot string, content *r.Node) *r.Node {
	return r.Div("group relative flex gap-4").ID(id).Key(id).Render(
		r.Div("flex w-3 shrink-0 flex-col items-center").Attr("aria-hidden", "true").Render(
			r.Span("mt-[7px] shrink-0 rounded-full "+dot),
			r.Span("mt-1.5 w-px flex-1 bg-(--ws-line) group-last:hidden"),
		),
		r.Div("min-w-0 flex-1").Render(content),
	)
}

// chatHead is who wrote a message and when.
func chatHead(who string, at, now time.Time) *r.Node {
	return r.Div("flex items-baseline justify-between gap-3").Render(
		r.Strong("text-[14px] font-semibold").Text(who),
		r.If(!at.IsZero(), r.El("time", "shrink-0 font-mono text-[11px] "+chatMuted).Attr("datetime", at.UTC().Format(time.RFC3339)).Text(chatTime(at, now))),
	)
}

// chatTime is the clock for today, else the weekday or the date too.
func chatTime(at, now time.Time) string {
	at, now = at.Local(), now.Local()
	switch {
	case at.Year() == now.Year() && at.YearDay() == now.YearDay():
		return at.Format("15:04")
	case now.Sub(at) < 6*24*time.Hour && now.After(at):
		return at.Format("Mon 15:04")
	case at.Year() == now.Year():
		return at.Format("2 Jan 15:04")
	}
	return at.Format("2 Jan 2006 15:04")
}

// chatItem draws a message, a tool step or a notice.
func chatItem(item components.ChatItem, now time.Time) *r.Node {
	id := "chat-item-" + item.ID
	switch item.Kind {
	case "user":
		images := ""
		switch {
		case item.Images == 1:
			images = "1 image"
		case item.Images > 1:
			images = fmt.Sprintf("%d images", item.Images)
		}
		return chatRow(id, chatUserDot, r.Div("pb-6").Render(
			chatHead("You", item.At, now),
			r.If(item.Text != "", r.P(chatText).Text(item.Text)),
			r.If(images != "", r.Span("mt-2 inline-flex h-7 items-center gap-1.5 rounded-full bg-(--ws-sunken) px-3 text-[12px] ring-1 ring-(--ws-line) "+chatMuted).
				Render(chatIcon("image", "text-[16px]"), r.Span("").Text(images))),
		))
	case "assistant":
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return chatRow(id, chatAgentDot, r.Div("pb-6").Render(
			chatHead("Claude", item.At, now),
			chatMarkdown(item.Text),
		))
	case "tool":
		dot, class, suffix := chatStepDot, chatMuted, ""
		switch item.State {
		case "running":
			dot, class, suffix = chatLiveDot, "text-(--ws-accent)", " …"
		case "failed":
			class, suffix = "text-red-700 dark:text-red-400", " · failed"
		}
		label := item.Tool
		if item.Text != "" {
			label += " · " + item.Text
		}
		line := r.Span("block min-w-0 truncate font-mono text-[12px] leading-5 "+class).Attr("title", label+suffix).Render(
			r.Span("font-semibold").Text(item.Tool),
			r.If(item.Text != "", r.Span("").Text(" "+item.Text)),
			r.If(suffix != "", r.Span("").Text(suffix)),
		)
		if item.Detail == "" {
			return chatRow(id, dot, r.Div("min-w-0 pb-3").Render(line))
		}
		return chatRow(id, dot, r.El("details", "group/step min-w-0 pb-3").Render(
			r.El("summary", "flex min-w-0 cursor-pointer list-none items-center gap-1 rounded-md [&::-webkit-details-marker]:hidden").Render(
				line,
				chatIcon("expand_more", "shrink-0 text-[16px] "+chatMuted+" transition-transform group-open/step:rotate-180"),
			),
			r.El("pre", "mb-1 mt-1.5 max-h-60 overflow-auto whitespace-pre-wrap rounded-xl bg-(--ws-sunken) p-3 font-mono text-[12px] leading-normal [overflow-wrap:anywhere] "+chatMuted).Text(item.Detail),
		))
	case "notice":
		return chatRow(id, "h-2 w-2 bg-amber-500", r.P("pb-5 whitespace-pre-wrap text-[13px] [overflow-wrap:anywhere] "+chatMuted).Text(item.Text))
	case "error":
		return chatRow(id, "h-2 w-2 bg-red-600", r.P("pb-5 whitespace-pre-wrap text-[13px] text-red-700 [overflow-wrap:anywhere] dark:text-red-400").Text(item.Text))
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
		return r.P(chatText).Text(md)
	}
	return r.Div(chatMarkdownClass).Key("md-"+hex.EncodeToString(sum[:8])).Preserve().
		Attr("data-html", html.String()).
		UnsafeJS("this.innerHTML=this.getAttribute('data-html');this.removeAttribute('data-html');")
}

// renderChatRequests draws Claude's open questions and plans.
func renderChatRequests(appID string, requests []components.ChatRequest) *r.Node {
	cards := make([]*r.Node, 0, len(requests))
	for _, request := range requests {
		cards = append(cards, chatRequestCard(request))
	}
	return r.Div("flex flex-col gap-4 empty:hidden [&:not(:empty)]:mt-5").ID(chatRequestsID(appID)).Render(cards...)
}

func chatAction(class, label, answer string) *r.Node {
	return r.Button(class).Attr("type", "button").Attr("data-chat-answer", answer).Text(label)
}

// chatRequestCard asks the user, as Kodri's question card: Claude's
// questions, or its plan to approve. A typed reply declines and tells Claude
// what to do instead.
func chatRequestCard(request components.ChatRequest) *r.Node {
	card := r.El("section", "flex flex-col gap-4 rounded-2xl bg-(--ws-accent)/5 p-5 ring-1 ring-(--ws-accent)/30").Attr("data-chat-request", request.ID)
	eyebrow := func(text string) *r.Node { return r.Span(chatEyebrow + " text-(--ws-accent)").Text(text) }
	if request.Tool == "ExitPlanMode" {
		return card.Attr("aria-label", "Claude's plan").Render(
			eyebrow("Claude's plan"),
			r.Div("max-h-[50vh] overflow-auto rounded-xl bg-(--ws-raised) px-4 py-3 ring-1 ring-(--ws-line)").Render(chatMarkdown(request.Plan)),
			r.Div("flex flex-wrap gap-2").Render(
				chatAction(chatSolid, "Approve plan", "allow"),
				chatAction(chatOutline, "Keep planning", "deny"),
			),
			chatReplyField("Tell Claude what to change"),
		)
	}
	quick := len(request.Questions) == 1 && !request.Questions[0].MultiSelect
	card.Attr("aria-label", "Claude asks").Render(eyebrow("Claude asks"))
	for i, question := range request.Questions {
		name := fmt.Sprintf("q%d", i)
		kind := "radio"
		if question.MultiSelect {
			kind = "checkbox"
		}
		options := make([]*r.Node, 0, len(question.Options))
		for _, option := range question.Options {
			input := r.Input("peer sr-only").Attr("type", kind).Attr("name", name).Attr("value", option.Label)
			if quick {
				input.Attr("data-chat-quick", "")
			}
			options = append(options, r.Label("flex cursor-pointer flex-col gap-0.5 rounded-xl bg-(--ws-raised) px-4 py-2.5 text-left ring-1 ring-inset ring-(--ws-control-line) transition-colors hover:bg-(--ws-hover) has-[:checked]:ring-2 has-[:checked]:ring-(--ws-accent) has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-(--ws-accent)").Render(
				input,
				r.Span("text-[14px] font-medium [overflow-wrap:anywhere]").Text(option.Label),
				r.If(option.Description != "", r.Span("text-[12.5px] leading-snug "+chatMuted).Text(option.Description)),
			))
		}
		card.Render(r.Fieldset("m-0 flex min-w-0 flex-col gap-2.5 border-0 p-0").Attr("data-question", question.Question).Render(
			r.Legend("mb-2.5 p-0 text-[14px] font-medium leading-relaxed [overflow-wrap:anywhere]").Text(question.Question),
			r.Div("flex flex-col gap-2").Render(options...),
			r.Input(chatField).Attr("type", "text").Attr("name", name+"-other").Attr("placeholder", "Or type your own answer").Attr("aria-label", "Your own answer").Attr("autocomplete", "off"),
		))
	}
	return card.Render(r.Div("flex flex-wrap gap-2").Render(
		chatAction(chatSolid, "Send answers", "questions"),
		chatAction(chatOutline, "Skip", "deny"),
	))
}

func chatReplyField(placeholder string) *r.Node {
	return r.Div("flex items-center gap-2").Render(
		r.Input(chatField+" flex-1").Attr("type", "text").Attr("name", "message").Attr("placeholder", placeholder).Attr("aria-label", placeholder).Attr("autocomplete", "off"),
		r.Button(chatGhostIcon+" h-10 w-10").Attr("type", "button").Attr("data-chat-answer", "reply").Attr("aria-label", "Send reply").Attr("title", "Send reply").
			Render(chatIcon("arrow_upward", "text-[20px]")),
	)
}

type actionChatSendInput struct {
	SID    string   `json:"sid"`
	AppID  string   `json:"appId"`
	Text   string   `json:"text"`
	Images []string `json:"images"` // pasted images saved by /attachments
}

type actionSettingsClaudeViewInput struct {
	Chat *bool `json:"chat"`
}

type actionChatAnswerInput struct {
	SID     string            `json:"sid"`
	AppID   string            `json:"appId"`
	Request string            `json:"request"`
	Allow   bool              `json:"allow"`
	Message string            `json:"message"`
	Answers map[string]string `json:"answers"`
}

func chatError(err error) r.Result {
	return r.Result{}.Run(r.Notify("error", err.Error()))
}

// chatImageLimit is the largest image Claude takes inline: 5 MB in base64.
const chatImageLimit = 3_750_000

// chatImages reads the user's images. Only PNGs saved by /attachments are
// read; an image too large to send inline goes to Claude as its path.
func chatImages(paths []string) ([][]byte, string, error) {
	if len(paths) > 8 {
		return nil, "", errors.New("send at most 8 images at once")
	}
	dir, err := libroDataDir()
	if err != nil {
		return nil, "", err
	}
	dir = filepath.Join(dir, "attachments")
	var images [][]byte
	var large []string
	for _, path := range paths {
		rel, err := filepath.Rel(dir, filepath.Clean(path))
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.Ext(path) != ".png" {
			return nil, "", errors.New("this image is not an attachment")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, "", errors.New("this image is gone, add it again")
		}
		if info.Size() > chatImageLimit {
			large = append(large, "[Image: "+path+"]")
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		images = append(images, data)
	}
	return images, strings.Join(large, "\n"), nil
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
		images, large, err := chatImages(in.Images)
		if err != nil {
			return chatError(err), nil
		}
		text := strings.TrimSpace(strings.TrimSpace(in.Text) + "\n" + large)
		restarted := !tm.IsRunning(in.AppID)
		if err := ensureChat(sid, in.AppID); err != nil {
			return chatError(err), nil
		}
		if err := tm.ChatSend(in.AppID, text, images); err != nil {
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
		err := tm.ChatAnswer(in.AppID, in.Request, components.ChatAnswer{Allow: in.Allow, Message: in.Message, Answers: in.Answers})
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
