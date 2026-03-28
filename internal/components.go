package libro

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	r "github.com/michalCapo/g-sui/ui"

	"libro/internal/version"
)

// urlParse is a convenience wrapper around url.Parse.
func urlParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

const (
	DialogID        = "add-dialog"
	MainAreaID      = "main-area"
	ProjectBarID    = "project-bar"
	ProjectDialogID = "project-dialog"
	DirBrowserID    = "dir-browser"
	SearchDialogID     = "search-dialog"
	ShortcutsDialogID  = "shortcuts-dialog"
)

// termIconInfo stores the icon details for a known terminal command.
type termIconInfo struct {
	// URL to an SVG icon (e.g. Simple Icons CDN), or empty for material icon fallback
	URL string
	// Material icon name, used when URL is empty
	MaterialIcon string
}

// knownTermIcons maps command base names to their icon info.
// Uses Simple Icons CDN (https://cdn.simpleicons.org/{name}/{color}) for brand icons.
var knownTermIcons = map[string]termIconInfo{
	"nvim":       {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"neovim":     {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"vim":        {URL: "https://cdn.simpleicons.org/vim/019733"},
	"vi":         {URL: "https://cdn.simpleicons.org/vim/019733"},
	"claude":     {URL: "https://cdn.simpleicons.org/anthropic/d4a27f"},
	"node":       {URL: "https://cdn.simpleicons.org/nodedotjs/5FA04E"},
	"npm":        {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"npx":        {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"bun":        {URL: "https://cdn.simpleicons.org/bun/FBF0DF"},
	"deno":       {URL: "https://cdn.simpleicons.org/deno/FFFFFF"},
	"python":     {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"python3":    {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"pip":        {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"docker":     {URL: "https://cdn.simpleicons.org/docker/2496ED"},
	"git":        {URL: "https://cdn.simpleicons.org/git/F05032"},
	"go":         {URL: "https://cdn.simpleicons.org/go/00ADD8"},
	"cargo":      {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"rustc":      {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"ruby":       {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"irb":        {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"lua":        {URL: "https://cdn.simpleicons.org/lua/2C2D72"},
	"java":       {URL: "https://cdn.simpleicons.org/openjdk/FFFFFF"},
	"kotlin":     {URL: "https://cdn.simpleicons.org/kotlin/7F52FF"},
	"swift":      {URL: "https://cdn.simpleicons.org/swift/F05138"},
	"redis-cli":  {URL: "https://cdn.simpleicons.org/redis/FF4438"},
	"psql":       {URL: "https://cdn.simpleicons.org/postgresql/4169E1"},
	"mysql":      {URL: "https://cdn.simpleicons.org/mysql/4479A1"},
	"mongosh":    {URL: "https://cdn.simpleicons.org/mongodb/47A248"},
	"kubectl":    {URL: "https://cdn.simpleicons.org/kubernetes/326CE5"},
	"terraform":  {URL: "https://cdn.simpleicons.org/terraform/844FBA"},
	"ansible":    {URL: "https://cdn.simpleicons.org/ansible/EE0000"},
	"tmux":       {URL: "https://cdn.simpleicons.org/tmux/1BB91F"},
	"bash":       {MaterialIcon: "terminal"},
	"zsh":        {MaterialIcon: "terminal"},
	"sh":         {MaterialIcon: "terminal"},
	"fish":       {MaterialIcon: "terminal"},
	"ssh":        {MaterialIcon: "vpn_key"},
	"htop":       {MaterialIcon: "monitoring"},
	"btop":       {MaterialIcon: "monitoring"},
	"top":        {MaterialIcon: "monitoring"},
	"make":       {MaterialIcon: "build"},
	"cmake":      {MaterialIcon: "build"},
	"gradle":     {MaterialIcon: "build"},
}

// lookupTermIcon resolves the icon for a terminal command.
// It checks the base command name (first word, without path).
func lookupTermIcon(command string) *termIconInfo {
	// Extract base command: "sudo nvim foo.txt" → "nvim", "/usr/bin/python3" → "python3"
	cmd := command
	parts := strings.Fields(cmd)
	if len(parts) > 0 {
		cmd = parts[0]
		// Skip sudo/env prefixes
		for _, p := range parts {
			if p != "sudo" && p != "env" && !strings.Contains(p, "=") {
				cmd = p
				break
			}
		}
	}
	// Strip path
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		cmd = cmd[idx+1:]
	}
	cmd = strings.ToLower(cmd)
	if info, ok := knownTermIcons[cmd]; ok {
		return &info
	}
	return nil
}

// knownTermIconsJS returns a JS object literal with the icon mapping for client-side use.
func knownTermIconsJS() string {
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for cmd, info := range knownTermIcons {
		if !first {
			sb.WriteString(",")
		}
		first = false
		if info.URL != "" {
			fmt.Fprintf(&sb, "'%s':{url:'%s'}", cmd, info.URL)
		} else {
			fmt.Fprintf(&sb, "'%s':{mi:'%s'}", cmd, info.MaterialIcon)
		}
	}
	sb.WriteString("}")
	return sb.String()
}

// termIconColors returns a gradient palette [top, bottom, mid] for a terminal command.
func termIconColors(cmd string) (string, string, string) {
	palettes := [][3]string{
		{"#0d9488", "#065f46", "#047857"},
		{"#7c3aed", "#4c1d95", "#5b21b6"},
		{"#2563eb", "#1e3a5f", "#1d4ed8"},
		{"#db2777", "#831843", "#9d174d"},
		{"#d97706", "#78350f", "#92400e"},
		{"#059669", "#064e3b", "#047857"},
		{"#dc2626", "#7f1d1d", "#991b1b"},
		{"#0891b2", "#164e63", "#155e75"},
	}
	h := 0
	for _, c := range cmd {
		h = ((h << 5) - h) + int(c)
	}
	if h < 0 {
		h = -h
	}
	p := palettes[h%len(palettes)]
	return p[0], p[1], p[2]
}

// stripID returns the DOM ID for a project's app strip
func stripID(projectName string) string {
	return "app-strip-" + projectName
}

// sidData creates a data map with the session ID included
func sidData(sid string, extra ...any) map[string]any {
	m := map[string]any{"sid": sid}
	for i := 0; i+1 < len(extra); i += 2 {
		if key, ok := extra[i].(string); ok {
			m[key] = extra[i+1]
		}
	}
	return m
}

// projectMainID returns the DOM ID for a project's main area div
func projectMainID(projectName string) string {
	return "project-main-" + projectName
}

// renderMainAreaWrapper renders the wrapper that contains all per-project main area divs.
// Only the active project's div is visible; others are hidden to preserve state.
func renderMainAreaWrapper(state *AppState, sid string) *r.Node {
	return r.Div("flex-1 flex flex-col overflow-hidden relative").ID(MainAreaID).Render(
		renderMainArea(state, sid),
	)
}

// renderMainArea renders the entire main area based on current state
func renderMainArea(state *AppState, sid string) *r.Node {
	if len(state.Apps) == 0 {
		return renderEmptyState(state, sid)
	}
	return renderAppStrip(state, sid)
}

// renderEmptyState renders the saved apps list with "+ Add New" button
func renderEmptyState(state *AppState, sid string) *r.Node {
	savedApps := DBLoadSavedApps(state.ActiveProject)
	appButtons := make([]*r.Node, 0, len(savedApps))
	for i, app := range savedApps {
		appButtons = append(appButtons, renderSavedAppButton(app, i, sid))
	}

	container := r.Div("flex-1 flex items-center justify-center").ID(projectMainID(state.ActiveProject)).Render(
		r.Div("flex flex-col items-center gap-2 w-full max-w-md").Render(
			r.Div("flex flex-col gap-1.5 w-full").Render(appButtons...),
			r.Div("flex gap-2 w-full mt-1").Render(
				r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Render(r.I("material-icons-round text-[18px]").Text("add"), r.Span("").Text("Add New")).
					OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
				r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-indigo-600 hover:bg-indigo-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Render(r.I("material-icons-round text-[18px]").Text("language"), r.Span("").Text("Browse")).
					OnClick(&r.Action{Name: "app.browse.new", Data: sidData(sid)}),
			),
		),
	)
	return container
}

// renderSavedAppButton renders a single saved app button (server-side, from DB data).
func renderSavedAppButton(app SavedApp, index int, sid string) *r.Node {
	var iconNode *r.Node
	label := app.Name

	if app.Type == "terminal" {
		if label == "" {
			label = app.Command
		}
		if info := lookupTermIcon(app.Command); info != nil {
			if info.URL != "" {
				iconNode = r.Img("w-5 h-5 shrink-0 rounded-sm").Attr("src", info.URL)
			} else {
				iconNode = r.I("material-icons-round text-lg shrink-0 text-gray-400 dark:text-zinc-500").Text(info.MaterialIcon)
			}
		} else {
			iconNode = r.I("material-icons-round text-lg shrink-0 text-gray-400 dark:text-zinc-500").Text("terminal")
		}
	} else {
		if label == "" {
			label = app.URL
		}
		iconNode = r.I("material-icons-round text-lg shrink-0 text-gray-400 dark:text-zinc-500").Text("language")
		if app.URL != "" {
			// Try to extract hostname for favicon
			if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
				iconNode = r.Img("w-5 h-5 shrink-0 rounded-sm").
					Attr("src", "https://www.google.com/s2/favicons?domain="+u.Hostname()+"&sz=32")
				if label == app.URL {
					h := u.Hostname()
					if strings.HasPrefix(h, "www.") {
						h = h[4:]
					}
					label = h
				}
			}
		}
	}

	writable := app.Writable

	// Launch button
	btn := r.Button("w-full flex items-center gap-3 px-4 py-3 bg-white dark:bg-zinc-800/80 hover:bg-gray-50 dark:hover:bg-zinc-700/80 border border-gray-200 dark:border-zinc-700/40 hover:border-gray-300 dark:hover:border-zinc-600 rounded-lg cursor-pointer text-left transition-colors duration-75 shadow-sm dark:shadow-none").
		Render(
			iconNode,
			r.Span("flex-1 truncate text-sm text-gray-800 dark:text-zinc-200").Text(label),
			r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Width),
			r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Type),
			r.I("material-icons-round text-xl cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-200").
				Text("edit").
				OnClick(&r.Action{Name: "app.saved.edit", Data: sidData(sid, "index", index)}).
				Attr("onclick", fmt.Sprintf(`event.stopPropagation();__ws.call('app.saved.edit',{sid:'%s',index:%d});__ws.call('app.dialog.open',{sid:'%s'});`, sid, index, sid)+
					savedAppEditFillJS(app, index)),
			r.I("material-icons-round text-xl cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400").
				Text("close").
				Attr("onclick", fmt.Sprintf(`event.stopPropagation();__ws.call('app.saved.delete',{sid:'%s',index:%d});`, sid, index)),
		).
		OnClick(&r.Action{Name: "app.start", Data: map[string]any{
			"sid": sid, "type": app.Type, "url": app.URL,
			"command": app.Command, "width": app.Width,
			"writable": writable, "name": app.Name,
		}})

	return btn
}

// savedAppEditFillJS returns JS that fills the add dialog with saved app data for editing.
func savedAppEditFillJS(app SavedApp, _ int) string {
	return fmt.Sprintf(`
setTimeout(function(){
	if(%s==='terminal'){
		var tb=document.getElementById('tab-terminal-btn');if(tb)tb.click();
		var cmd=document.getElementById('app-command');if(cmd)cmd.value=%s;
		var wr=document.getElementById('app-writable');if(wr)wr.checked=%v;
	}else{
		var tb=document.getElementById('tab-url-btn');if(tb)tb.click();
		var url=document.getElementById('app-url');if(url)url.value=%s;
	}
	var nm=document.getElementById('app-name');if(nm)nm.value=%s;
	var wr=document.getElementById('width-'+((%s)||'md'));if(wr)wr.checked=true;
},100);
`, jsString(app.Type), jsString(app.Command), app.Writable, jsString(app.URL), jsString(app.Name), jsString(app.Width))
}

// renderAppStrip renders the horizontal strip of applications with navigation
func renderAppStrip(state *AppState, sid string) *r.Node {
	// Build strip children: left spacer + apps + right spacer
	stripChildren := make([]*r.Node, 0, len(state.Apps)+2)
	stripChildren = append(stripChildren, r.Div("flex-1 shrink min-w-0"))
	for i, app := range state.Apps {
		stripChildren = append(stripChildren, renderAppFrame(app, i, i == state.SelectedIndex, sid))
	}
	stripChildren = append(stripChildren, r.Div("flex-1 shrink min-w-0"))

	strip := r.Div("flex-1 min-w-0 flex items-stretch h-full overflow-x-auto overflow-y-hidden gap-4 p-0.5").
		ID(stripID(state.ActiveProject)).
		Render(stripChildren...)

	mainArea := r.Div("flex-1 flex items-stretch overflow-hidden relative p-2").ID(projectMainID(state.ActiveProject)).
		Render(
			renderSideLauncher(sid, "left"),
			strip,
			renderSideLauncher(sid, "right"),
		)
	mainArea.JS(centerSelectedJS(state.SelectedIndex, len(state.Apps), state.ActiveProject))

	return mainArea
}

func centerSelectedJS(selectedIndex int, totalApps int, projectName string) string {
	return fmt.Sprintf(`
		(function centerApp() {
			requestAnimationFrame(function() {
				requestAnimationFrame(function() {
					var strip = document.getElementById('%s');
					if (!strip || %d === 0) return;
					var app = strip.children[%d + 1];
					if (app) {
						var stripRect = strip.getBoundingClientRect();
						var appRect = app.getBoundingClientRect();
						if (appRect.left < stripRect.left || appRect.right > stripRect.right) {
							app.scrollIntoView({block: 'nearest', inline: 'center'});
						}
					}
				});
			});
		})();
	`, stripID(projectName), totalApps, selectedIndex)
}

func navigateJS(state *AppState, sid string) string {
	return fmt.Sprintf(`
		(function() {
			var strip = document.getElementById('%s');
			if (!strip) return;
			var selectedIdx = %d;
			var totalApps = %d;
			var offset = 1;

			for (var i = 0; i < totalApps; i++) {
				var child = strip.children[i + offset];
				if (!child) continue;
				if (i === selectedIdx) {
					child.className = child.className.replace(/\bborder\b/g, 'border-2');
					child.className = child.className.replace(/border-gray-200/g, 'border-teal-500');
					child.className = child.className.replace(/dark:border-zinc-700\/50/g, 'dark:border-teal-500/70');
					child.className = child.className.replace(/border-transparent/g, 'border-teal-500');
					var overlay = child.querySelector('[data-click-overlay]');
					if (overlay) overlay.remove();
				} else {
					child.className = child.className.replace(/border-2/g, 'border');
					child.className = child.className.replace(/border-teal-500(?!\/)/g, 'border-gray-200');
					child.className = child.className.replace(/dark:border-teal-500\/70/g, 'dark:border-zinc-700/50');
					if (!child.querySelector('[data-click-overlay]')) {
						var ov = document.createElement('div');
						ov.setAttribute('data-click-overlay', '');
						ov.className = 'absolute inset-0 z-20 cursor-pointer';
						ov.onclick = function(idx) {
							return function() { __ws.call('app.select', {"sid": "%s", "index": idx}); };
						}(i);
						var iframeWrap = child.children[1];
						if (iframeWrap) { iframeWrap.appendChild(ov); } else { child.appendChild(ov); }
					}
				}
			}

			var selected = strip.children[selectedIdx + offset];
			if (selected) {
				selected.scrollIntoView({behavior: 'smooth', block: 'nearest', inline: 'nearest'});
				if (window.__libroFocusApp) {
					setTimeout(function() { window.__libroFocusApp(selectedIdx); }, 100);
				}
			}
		})();
	`, stripID(state.ActiveProject), state.SelectedIndex, len(state.Apps), sid)
}

// renderAppFrame renders a single application iframe with controls
func renderAppFrame(app Application, index int, selected bool, sid string) *r.Node {
	borderClass := "border border-gray-200 dark:border-zinc-700/50"
	if selected {
		borderClass = "border-2 border-teal-500 dark:border-teal-500/70"
	}

	frameID := fmt.Sprintf("frame-%s", app.ID)

	iframeSrc := app.URL
	if app.Type == AppTypeURL {
		// Chrome-backed tabs don't use iframe src; canvas is initialized via JS
		iframeSrc = ""
	}

	// Size badge bar + close (right side of toolbar)
	badgeBase := "px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75"
	badges := make([]*r.Node, 0, len(AllWidths())+1)
	for _, w := range AllWidths() {
		cls := badgeBase + " text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700"
		if w == app.Width {
			cls = badgeBase + " bg-teal-600 text-white"
		}
		badges = append(badges, r.Button(cls).
			Text(strings.ToUpper(string(w))).
			OnClick(&r.Action{
				Name: "app.resize",
				Data: sidData(sid, "id", app.ID, "width", string(w)),
			}))
	}
	badges = append(badges, r.Button(badgeBase+" text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10 ml-1").
		Text("CLOSE").
		OnClick(&r.Action{
			Name: "app.close",
			Data: sidData(sid, "id", app.ID),
		}))
	rightButtons := r.Div("flex gap-0.5 items-center shrink-0").
		Attr("data-size-badges", "").
		Render(badges...)

	// Left side of toolbar depends on app type
	var leftSide *r.Node
	if app.Type == AppTypeURL {
		urlInputID := fmt.Sprintf("urlinput-%s", app.ID)
		btnCls := "flex items-center justify-center w-6 h-6 rounded-sm text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700 transition-colors duration-75 cursor-pointer shrink-0"

		// Back button
		backBtn := r.Button(btnCls).
			Attr("title", "Back").
			OnClick(r.JS(fmt.Sprintf(`(function(){var c=window.__chromeWS&&window.__chromeWS['%s'];if(c&&c.readyState===1)c.send(JSON.stringify({t:'back'}));})()`, app.ID)))
		backBtn.Render(r.I("material-icons-round text-sm").Text("arrow_back"))

		// Forward button
		forwardBtn := r.Button(btnCls).
			Attr("title", "Forward").
			OnClick(r.JS(fmt.Sprintf(`(function(){var c=window.__chromeWS&&window.__chromeWS['%s'];if(c&&c.readyState===1)c.send(JSON.stringify({t:'fwd'}));})()`, app.ID)))
		forwardBtn.Render(r.I("material-icons-round text-sm").Text("arrow_forward"))

		// Copy button
		copyBtn := r.Button(btnCls).
			Attr("title", "Copy URL").
			OnClick(r.JS(fmt.Sprintf(`var inp=document.getElementById('%s');if(inp){navigator.clipboard.writeText(inp.value);var btn=event.currentTarget;btn.style.color='rgb(20,184,166)';setTimeout(function(){btn.style.color='';},800);}`, urlInputID)))
		copyBtn.Render(r.I("material-icons-round text-sm").Text("content_copy"))

		// Reload button
		reloadBtn := r.Button(btnCls).
			Attr("title", "Reload").
			OnClick(r.JS(fmt.Sprintf(`(function(){var c=window.__chromeWS&&window.__chromeWS['%s'];if(c&&c.readyState===1)c.send(JSON.stringify({t:'reload'}));})()`, app.ID)))
		reloadBtn.Render(r.I("material-icons-round text-sm").Text("refresh"))

		// URL input — on Enter, navigate Chrome tab and update server state
		urlInput := r.Input("flex-1 min-w-0 bg-gray-100 dark:bg-zinc-800 rounded-sm text-[11px] font-mono text-gray-600 dark:text-zinc-400 outline-none placeholder-gray-400 dark:placeholder-zinc-600 px-2 h-6").
			ID(urlInputID).
			Attr("type", "text").
			Attr("value", app.URL).
			Attr("spellcheck", "false").
			Attr("autocomplete", "off").
			On("keydown", r.JS(fmt.Sprintf(`if(event.key==='Enter'){event.preventDefault();var u=event.target.value;if(u&&!u.startsWith('http://')&&!u.startsWith('https://'))u=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(u)?'http://':'https://')+u;var c=window.__chromeWS&&window.__chromeWS['%s'];if(c&&c.readyState===1)c.send(JSON.stringify({t:'nav',url:u}));__ws.call('app.url.set',{"sid":"%s","id":"%s","url":u});}`, app.ID, sid, app.ID)))

		// Globe icon
		globe := r.I("material-icons-round text-sm text-gray-400 dark:text-zinc-500 shrink-0 leading-none").Text("language")

		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1").
			Render(backBtn, forwardBtn, globe, urlInput, copyBtn, reloadBtn)
	} else if app.Type == AppTypeTerminal {
		labelText := app.Command
		if app.Name != "" {
			labelText = app.Name
		}

		var iconNode *r.Node
		if info := lookupTermIcon(app.Command); info != nil {
			if info.URL != "" {
				iconNode = r.Div("shrink-0 w-4 h-4 flex items-center justify-center").Render(
					r.Div("").Attr("style", fmt.Sprintf(
						"width:16px;height:16px;background:url('%s') center/contain no-repeat",
						info.URL,
					)),
				)
			} else if info.MaterialIcon != "" {
				iconNode = r.I("material-icons-round text-sm text-gray-500 dark:text-zinc-400 shrink-0").Text(info.MaterialIcon)
			}
		}
		if iconNode == nil {
			// Fallback: gradient letter icon
			initials := strings.ToUpper(app.Command)
			if len(initials) > 1 {
				initials = initials[:1]
			}
			top, bot, mid := termIconColors(app.Command)
			iconStyle := fmt.Sprintf(
				"display:inline-flex;align-items:center;justify-content:center;width:16px;height:16px;border-radius:4px;position:relative;overflow:hidden;background:linear-gradient(145deg,%s 0%%,%s 60%%,%s 100%%);box-shadow:0 1px 3px rgba(0,0,0,.25),inset 0 1px 0 rgba(255,255,255,.25),inset 0 -1px 0 rgba(0,0,0,.12);font-size:15px;font-weight:800;color:#fff;letter-spacing:.04em;text-shadow:0 1px 1px rgba(0,0,0,.3);font-family:ui-monospace,SFMono-Regular,Menlo,monospace",
				top, mid, bot,
			)
			iconNode = r.Span("shrink-0").Attr("style", iconStyle).Text(initials)
		}

		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1.5").Render(
			iconNode,
			r.Span("text-[10px] font-mono tracking-wider uppercase text-gray-600 dark:text-zinc-300 truncate").Text(labelText),
		)
	}

	// Toolbar: always visible, sits above the iframe
	toolbar := r.Div("flex items-center gap-2 px-1.5 py-1 bg-white dark:bg-zinc-900 border-b border-gray-200 dark:border-zinc-700/50 shrink-0").
		Render(leftSide, rightButtons)

	var clickOverlay *r.Node
	if !selected {
		clickOverlay = r.Div("absolute inset-0 z-20 cursor-pointer").
			Attr("data-click-overlay", "").
			OnClick(&r.Action{
				Name: "app.select",
				Data: sidData(sid, "index", index),
			})
	}

	return r.Div("group relative flex flex-col "+app.Width.ContainerClasses()+" h-full "+borderClass+" rounded-md overflow-hidden bg-white dark:bg-zinc-950 transition-colors duration-75").
		Attr("data-app-id", app.ID).
		Render(
			toolbar,
			r.Div("relative flex-1 min-h-0").Render(
				renderIframe(app, frameID, iframeSrc),
				clickOverlay,
			),
		)
}

func renderIframe(app Application, frameID, iframeSrc string) *r.Node {
	if app.Type == AppTypeURL {
		// Chrome-backed view: z-30 to sit above the click overlay (z-20)
		return r.Div("w-full h-full bg-white dark:bg-zinc-950 absolute inset-0 z-30").
			ID(frameID).
			Attr("data-chrome-app", app.ID).
			Attr("data-chrome-url", app.URL).
			Attr("tabindex", "0").
			Attr("style", "outline:none;overflow:hidden")
	}
	// Terminal apps use iframe (for ttyd)
	sandbox := "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-same-origin"
	iframe := r.Iframe("w-full h-full border-0").
		ID(frameID).
		Attr("src", iframeSrc).
		Attr("loading", "lazy").
		Attr("sandbox", sandbox)
	if app.Type == AppTypeTerminal {
		iframe.Attr("scrolling", "no")
	}
	return iframe
}

// insertAppJS returns JS that inserts a new app frame into the existing strip.
// The node is compiled to JS and inserted after the left spacer (prepend) or before the right spacer (append).
func insertAppJS(node *r.Node, prepend bool, projectName string) string {
	sid := stripID(projectName)
	if prepend {
		return node.ToJSAppend(sid) + fmt.Sprintf(`
(function(){
	var strip=document.getElementById('%s');
	if(!strip||strip.children.length<3)return;
	var newApp=strip.lastChild;
	strip.insertBefore(newApp,strip.children[1]);
})();`, sid)
	}
	return node.ToJSAppend(sid) + fmt.Sprintf(`
(function(){
	var strip=document.getElementById('%s');
	if(!strip||strip.children.length<3)return;
	var newApp=strip.lastChild;
	var rightSpacer=strip.children[strip.children.length-2];
	strip.insertBefore(newApp,rightSpacer);
})();`, sid)
}

// hideAllProjectsJS returns JS that hides all project divs inside the wrapper.
func hideAllProjectsJS() string {
	return fmt.Sprintf(`
(function(){
	var w=document.getElementById('%s');
	if(!w)return;
	for(var i=0;i<w.children.length;i++)w.children[i].style.display='none';
})();`, MainAreaID)
}

// showProjectJS returns JS that makes a project div visible.
func showProjectJS(projectName string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.getElementById('%s');
	if(el)el.style.display='flex';
})();`, projectMainID(projectName))
}

// switchProjectJS returns JS that hides all project divs and shows the target.
// If the target div doesn't exist yet, newContent is appended to the wrapper.
func switchProjectJS(toProject string, newContent *r.Node) string {
	hideJS := hideAllProjectsJS()

	if newContent != nil {
		// Hide all existing, then append new content (which is visible by default)
		return hideJS + newContent.ToJSAppend(MainAreaID)
	}

	// Target already exists in DOM — hide all, show target
	return hideJS + showProjectJS(toProject)
}

// focusSelectedAppJS returns JS that focuses the selected app's iframe after a short delay
func focusSelectedAppJS(selectedIndex int) string {
	return fmt.Sprintf(`
setTimeout(function(){
	if(window.__libroFocusApp) window.__libroFocusApp(%d);
}, 150);`, selectedIndex)
}

// removeAppJS returns JS that removes an app frame by its app ID from the strip.
func removeAppJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(el)el.remove();
})();`, appID)
}

// renderSideLauncher renders a vertical icon dock: saved app icons + "+" button.
// Now server-rendered from DB instead of client-side localStorage.
func renderSideLauncher(sid, side string) *r.Node {
	savedApps := DBLoadAllSavedApps()

	tipPos := "left-full ml-2"
	if side == "right" {
		tipPos = "right-full mr-2"
	}
	btnCls := "w-12 h-12 flex items-center justify-center rounded-md cursor-pointer transition-colors duration-75 hover:bg-gray-200 dark:hover:bg-zinc-700 relative group/ico"
	tipCls := "absolute " + tipPos + " px-2 py-1 text-xs rounded bg-white dark:bg-zinc-800 text-gray-800 dark:text-zinc-200 border border-gray-200 dark:border-zinc-700 whitespace-nowrap opacity-0 group-hover/ico:opacity-100 pointer-events-none transition-opacity z-[200] shadow-lg"

	children := make([]*r.Node, 0, len(savedApps)+2)

	for _, app := range savedApps {
		var iconNode *r.Node
		label := app.Name

		if app.Type == "terminal" {
			if label == "" {
				label = app.Command
			}
			if info := lookupTermIcon(app.Command); info != nil {
				if info.URL != "" {
					iconNode = r.Img("w-8 h-8 rounded-sm").Attr("src", info.URL)
				} else {
					iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-2xl").Text(info.MaterialIcon)
				}
			} else {
				iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-2xl").Text("terminal")
			}
		} else {
			if label == "" {
				label = app.URL
			}
			iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-2xl").Text("language")
			if app.URL != "" {
				if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
					iconNode = r.Img("w-8 h-8 rounded-sm").
						Attr("src", "https://www.google.com/s2/favicons?domain="+u.Hostname()+"&sz=32")
					if label == app.URL {
						h := u.Hostname()
						if strings.HasPrefix(h, "www.") {
							h = h[4:]
						}
						label = h
					}
				}
			}
		}

		btn := r.Button(btnCls).
			Render(
				iconNode,
				r.Span(tipCls).Text(label+" ("+app.Width+")"),
			).
			OnClick(&r.Action{Name: "app.start", Data: map[string]any{
				"sid": sid, "type": app.Type, "url": app.URL,
				"command": app.Command, "width": app.Width,
				"writable": app.Writable, "name": app.Name, "side": side,
			}})
		children = append(children, btn)
	}

	// Browse button
	browseBtn := r.Button(btnCls).
		Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-indigo-600 dark:hover:text-indigo-400 text-2xl").Text("language"),
			r.Span(tipCls).Text("Quick browse"),
		).
		OnClick(&r.Action{Name: "app.browse.new", Data: sidData(sid, "side", side)})
	children = append(children, browseBtn)

	// Add button
	addBtn := r.Button(btnCls).
		Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-teal-600 dark:hover:text-teal-400 text-[18px]").Text("add"),
			r.Span(tipCls).Text("Add new"),
		).
		OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid, "side", side)})
	children = append(children, addBtn)

	return r.Div("shrink-0 flex items-center mx-0.5").Render(
		r.Div("flex flex-col items-center gap-1 py-1").Render(children...),
	)
}

// renderAddDialog renders the add application modal dialog
func renderAddDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	widthOptions := make([]*r.Node, 0)
	for _, w := range AllWidths() {
		radio := r.IRadio("accent-teal-500 cursor-pointer").
			Attr("name", "app-width").
			Attr("value", string(w)).
			ID(fmt.Sprintf("width-%s", w))
		if w == WidthLG {
			radio.Attr("checked", "checked")
		}
		widthOptions = append(widthOptions,
			r.Label("flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-300 dark:border-zinc-700 hover:border-gray-400 dark:hover:border-zinc-500 cursor-pointer transition-colors text-gray-700 dark:text-zinc-300 text-sm font-mono").Render(
				radio,
				r.Span("").Text(strings.ToUpper(string(w))),
			),
		)
	}

	tabSwitchJS := func(showTab string) string {
		return fmt.Sprintf(`
			document.getElementById('tab-url-content').classList.toggle('hidden', '%s' !== 'url');
			document.getElementById('tab-terminal-content').classList.toggle('hidden', '%s' !== 'terminal');
			document.getElementById('tab-url-btn').classList.toggle('border-teal-500', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-teal-600', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('border-transparent', '%s' !== 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-gray-500', '%s' !== 'url');
			document.getElementById('tab-terminal-btn').classList.toggle('border-teal-500', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-teal-600', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('border-transparent', '%s' !== 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-gray-500', '%s' !== 'terminal');
			document.getElementById('app-type').value = '%s';
		`, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab)
	}

	collectIDs := []string{"app-url", "app-command", "app-writable", "app-type", "app-name", "width-md"}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-teal-500 focus:border-teal-500 outline-none transition-colors"

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75"+hiddenClass).
		ID(DialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", DialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-md mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("Add Application"),

					r.IHidden("").ID("app-type").Attr("value", "url"),

					r.Div("flex border-b border-gray-200 dark:border-zinc-700/50 mb-4").Render(
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-teal-500 text-teal-600 cursor-pointer transition-colors").
							ID("tab-url-btn").
							Text("URL").
							OnClick(r.JS(tabSwitchJS("url"))),
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 cursor-pointer transition-colors").
							ID("tab-terminal-btn").
							Text("Terminal").
							OnClick(r.JS(tabSwitchJS("terminal"))),
					),

					r.Div("mb-5").ID("tab-url-content").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("URL"),
						r.IUrl(inputCls).
							ID("app-url").
							Attr("placeholder", "https://example.com").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
					),

					r.Div("mb-5 hidden").ID("tab-terminal-content").Render(
						r.Div("mb-3").Render(
							r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Command"),
							r.IText(inputCls+" font-mono").
								ID("app-command").
								Attr("placeholder", "bash").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
						),
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-teal-500 cursor-pointer w-4 h-4").
								ID("app-writable").
								Attr("checked", "checked"),
							r.Span("text-sm text-gray-600 dark:text-zinc-400").Text("Writable (allow input)"),
						),
					),

					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Name (optional)"),
						r.IText(inputCls).
							ID("app-name").
							Attr("placeholder", "e.g. My App"),
					),

					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Width"),
						r.Div("flex flex-wrap gap-1.5").Render(widthOptions...),
					),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "app.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-add").
							Text("Add").
							OnClick(&r.Action{
								Name:    "app.add",
								Data:    sidData(sid),
								Collect: collectIDs,
							}),
					),
				),
		)
}

// renderSearchDialog renders the fuzzy search popup (hidden by default).
// All filtering, navigation, and selection logic runs client-side via JS
// since saved apps are injected via __libroSavedApps from the DB.
func renderSearchDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(SearchDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", SearchDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("search-input").
							Attr("type", "text").
							Attr("placeholder", "Search applications...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();}"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("search-results"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("↑↓ navigate"),
						r.Span("").Text("Enter open right"),
						r.Span("").Text("Ctrl+Enter open left"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}

// searchDialogJS returns the JS that powers the fuzzy search popup behavior.
func searchDialogJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroSearchRegistered)return;
	window.__libroSearchRegistered=true;

	var dlg=document.getElementById('%s');
	var inp=document.getElementById('search-input');
	var res=document.getElementById('search-results');
	var selIdx=0;
	var filtered=[];

	function fuzzyMatch(text,query){
		text=text.toLowerCase();query=query.toLowerCase();
		var ti=0,qi=0,score=0,lastMatch=-1;
		while(ti<text.length&&qi<query.length){
			if(text[ti]===query[qi]){
				score+=1;
				if(lastMatch===ti-1)score+=2;
				if(ti===0||text[ti-1]===' '||text[ti-1]==='/'||text[ti-1]==='.')score+=3;
				lastMatch=ti;qi++;
			}
			ti++;
		}
		return qi===query.length?score:0;
	}

	function getApps(){
		return window.__libroSavedApps||[];
	}

	function render(){
		var dk=document.documentElement.classList.contains('dark');
		res.innerHTML='';
		if(filtered.length===0){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No matches</div>';
			return;
		}
		filtered.forEach(function(item,i){
			var row=document.createElement('div');
			var sel=i===selIdx;
			row.className='flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-teal-900/30 border-l-2 border-teal-500':'bg-teal-50 border-l-2 border-teal-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'));
			var iconHtml='';
			var label='';
			var sub='';
			var app=item.app;
			if(item.isBrowse){
				iconHtml='<i class="material-icons-round text-teal-500 text-lg shrink-0">public</i>';
				label=app.name||app.url;
				sub=app.url;
			}else if(app.type==='terminal'){
				iconHtml=window.__libroTermIcon?window.__libroTermIcon(app.command,24):'';
				label=app.name||app.command;
				sub=app.command;
			}else{
				try{
					var u=new URL(app.url);
					iconHtml='<img class="w-6 h-6 rounded-sm shrink-0" src="https://www.google.com/s2/favicons?domain='+encodeURIComponent(u.hostname)+'&sz=32" onerror="this.outerHTML=\'<i class=\\\'material-icons-round text-gray-400 text-lg\\\'>language</i>\'">';
					label=app.name||u.hostname.replace(/^www\./,'');
					sub=app.url;
				}catch(e){
					iconHtml='<i class="material-icons-round text-gray-400 text-lg shrink-0">language</i>';
					label=app.name||app.url;
					sub=app.url;
				}
			}
			var txtCls=dk?'text-zinc-200':'text-gray-800';
			var subCls=dk?'text-zinc-500':'text-gray-400';
			var badgeCls=dk?'bg-zinc-700 text-zinc-400':'bg-gray-200 text-gray-500';
			var typeBadge=item.isBrowse?'browser':app.type;
			row.innerHTML=iconHtml
				+'<div class="flex-1 min-w-0"><div class="text-sm truncate '+txtCls+'">'+label+'</div>'
				+(sub!==label?'<div class="text-[11px] truncate '+subCls+'">'+sub+'</div>':'')
				+'</div>'
				+'<span class="px-1.5 py-0.5 text-[10px] font-mono uppercase rounded shrink-0 '+badgeCls+'">'+(app.width||'lg')+'</span>'
				+'<span class="px-1.5 py-0.5 text-[10px] font-mono uppercase rounded shrink-0 '+badgeCls+'">'+typeBadge+'</span>';
			row.onmouseenter=function(){selIdx=i;render();};
			row.onclick=function(e){launch(e.ctrlKey?'left':'right');};
			res.appendChild(row);
		});
		var sel=res.children[selIdx];
		if(sel)sel.scrollIntoView({block:'nearest'});
	}

	function isURL(q){
		if(/^https?:\/\//i.test(q))return true;
		if(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:\d+)?(\/|$)/i.test(q))return true;
		// domain-like: contains dot, no spaces (e.g. google.com, foo.bar/path)
		if(/^[^\s]+\.[^\s]+$/.test(q)&&!q.includes(' '))return true;
		return false;
	}

	var browserEntry={app:{type:'url',url:'',name:'Browser',width:'lg'},score:0,isBrowse:true};

	function filter(){
		var q=inp.value.trim();
		var apps=getApps();
		if(!q){
			filtered=apps.map(function(a){return{app:a,score:1};});
			filtered.push(browserEntry);
		}else if(isURL(q)){
			// URL typed — show "Browse <url>" at top, then matching saved apps
			var browseURL=q;
			if(!/^https?:\/\//i.test(browseURL))browseURL=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(browseURL)?'http://':'https://')+browseURL;
			filtered=[{app:{type:'url',url:browseURL,name:'Browse: '+q,width:'lg'},score:99999,isBrowse:true}];
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,q);
				if(score>0)filtered.push({app:a,score:score});
			});
		}else{
			filtered=[];
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,q);
				if(score>0)filtered.push({app:a,score:score});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
			// Add browser if "browser" fuzzy matches query
			if(fuzzyMatch('browser',q)>0)filtered.push(browserEntry);
		}
		selIdx=0;
		render();
	}

	function launch(side){
		if(filtered.length===0)return;
		var item=filtered[selIdx];
		var app=item.app;
		dlg.classList.add('hidden');
		inp.value='';
		// Empty browser — use app.browse.new to open blank tab with URL bar focused
		if(item.isBrowse&&!app.url){
			__ws.call('app.browse.new',{sid:'%s',side:side});
			return;
		}
		__ws.call('app.start',{sid:'%s',type:app.type,url:app.url||'',command:app.command||'',width:app.width||'lg',writable:app.writable!==false,name:app.name||'',side:side});
	}

	function openSearch(){
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		setTimeout(function(){inp.focus();},50);
	}

	function closeSearch(){
		dlg.classList.add('hidden');
		inp.value='';
	}

	inp.addEventListener('input',filter);
	inp.addEventListener('keydown',function(e){
		e.stopImmediatePropagation();
		if(e.key==='ArrowDown'){
			e.preventDefault();
			if(selIdx<filtered.length-1){selIdx++;render();}
		}else if(e.key==='ArrowUp'){
			e.preventDefault();
			if(selIdx>0){selIdx--;render();}
		}else if(e.key==='Enter'){
			e.preventDefault();
			launch(e.ctrlKey?'left':'right');
		}else if(e.key==='Escape'){
			e.preventDefault();
			closeSearch();
		}
	});

	window.__libroOpenSearch=openSearch;
})();
`, SearchDialogID, sid, sid)
}

// renderShortcutsDialog renders the keyboard shortcuts popup (hidden by default).
func renderShortcutsDialog() *r.Node {
	type shortcut struct {
		keys string
		desc string
	}
	shortcuts := []shortcut{
		{"⌘ + H", "Navigate left"},
		{"⌘ + L", "Navigate right"},
		{"Ctrl + 1–9", "Select app by index"},
		{"⌘ + J", "Next project"},
		{"⌘ + K", "Previous project"},
		{"⌘ + /", "Open search"},
		{"⌘ + D", "Close current app"},
	}

	rows := make([]*r.Node, 0, len(shortcuts))
	for _, s := range shortcuts {
		rows = append(rows,
			r.Div("flex items-center justify-between py-2 px-1 border-b border-gray-100 dark:border-zinc-800 last:border-0").Render(
				r.Span("text-sm text-gray-700 dark:text-zinc-300").Text(s.desc),
				r.Span("text-xs font-mono px-2 py-1 rounded bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400").Text(s.keys),
			),
		)
	}

	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ShortcutsDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", ShortcutsDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center justify-between").Render(
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200").Text("Keyboard Shortcuts"),
						r.Button("text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 cursor-pointer").
							Attr("onclick", fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", ShortcutsDialogID)).
							Render(r.I("material-icons-round text-base").Text("close")),
					),
					r.Div("px-4 py-2 max-h-80 overflow-y-auto").Render(rows...),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Esc to close"),
					),
				),
		)
}

// shortcutsDialogJS returns JS to open/close the shortcuts dialog and handle Esc.
func shortcutsDialogJS() string {
	return fmt.Sprintf(`
(function(){
	var dlg=document.getElementById('%s');
	window.__libroOpenShortcuts=function(){
		dlg.classList.remove('hidden');
	};
	document.addEventListener('keydown',function(e){
		if(e.key==='Escape'&&!dlg.classList.contains('hidden')){
			e.preventDefault();e.stopImmediatePropagation();
			dlg.classList.add('hidden');
		}
	},true);
})();
`, ShortcutsDialogID)
}

// resizeJS returns JS that updates an app frame's width without replacing the DOM
func resizeJS(_ *AppState, width Width, appID string) string {
	// Build a map of width value -> container classes
	widthMap := ""
	for _, w := range AllWidths() {
		if widthMap != "" {
			widthMap += ","
		}
		widthMap += fmt.Sprintf("'%s':'%s'", string(w), w.ContainerClasses())
	}

	return fmt.Sprintf(`
(function(){
	var el = document.querySelector('[data-app-id="%s"]');
	if (!el) return;

	var widths = {%s};
	var newWidth = '%s';
	var newCls = widths[newWidth];

	// Remove old width classes and apply new ones
	var keep = [];
	var cls = el.className.split(/\s+/);
	var allWidthCls = {};
	for (var k in widths) {
		widths[k].split(/\s+/).forEach(function(c){ allWidthCls[c] = true; });
	}
	cls.forEach(function(c){
		if (!allWidthCls[c]) keep.push(c);
	});
	newCls.split(/\s+/).forEach(function(c){ keep.push(c); });
	el.className = keep.join(' ');

	// Update size badges: highlight active, dim others
	var topBar = el.querySelector('[data-size-badges]');
	if (topBar) {
		var btns = topBar.querySelectorAll('button');
		var sizeLabels = ['MD','LG','XL','2XL','FULL'];
		var activeBase = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
		btns.forEach(function(b){
			var txt = b.textContent.trim();
			if (sizeLabels.indexOf(txt) === -1) return;
			if (txt === newWidth.toUpperCase()) {
				b.className = activeBase + ' bg-teal-600 text-white';
			} else {
				b.className = activeBase + ' text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700';
			}
		});
	}

	requestAnimationFrame(function(){
		el.scrollIntoView({behavior: 'smooth', block: 'nearest', inline: 'center'});
	});
})();
`, appID, widthMap, string(width))
}

// renderProjectBar renders the horizontal project switcher bar
func renderProjectBar(state *AppState, sid string) *r.Node {
	buttons := make([]*r.Node, 0, len(state.Projects)+1)

	for _, proj := range state.Projects {
		cls := "px-3 py-1.5 text-xs font-mono rounded-md cursor-pointer transition-colors duration-75 "
		if proj.Name == state.ActiveProject {
			cls += "bg-teal-600 text-white"
		} else {
			cls += "bg-gray-200 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 hover:bg-gray-300 dark:hover:bg-zinc-700"
		}
		projBtn := r.Button(cls).
			Text(proj.Name).
			Attr("title", proj.Path).
			OnClick(r.JS(fmt.Sprintf(
				"history.replaceState(null,'','#%s');__ws.call('project.switch',{sid:'%s',name:'%s'});",
				proj.Name, sid, proj.Name,
			)))

		buttons = append(buttons, projBtn)
	}

	// Add project button
	buttons = append(buttons,
		r.Button("flex items-center justify-center w-7 h-7 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-teal-600 dark:hover:text-teal-400 hover:bg-gray-200 dark:hover:bg-zinc-800 transition-colors duration-75").
			Render(r.I("material-icons-round text-[18px]").Text("add")).
			OnClick(&r.Action{
				Name: "project.dialog.open",
				Data: sidData(sid),
			}),
	)

	// Show active project path
	activePath := ""
	for _, p := range state.Projects {
		if p.Name == state.ActiveProject {
			activePath = p.Path
			break
		}
	}

	return r.Div("flex items-center gap-1.5 px-3 py-2 border-b border-gray-200 dark:border-zinc-800 shrink-0").
		ID(ProjectBarID).
		Render(
			r.Img("w-7 h-7 shrink-0").Attr("src", "/assets/logo.svg").Attr("alt", "Libro"),
			r.Div("flex items-center gap-1.5").Render(buttons...),
			func() *r.Node {
				pathWrapper := r.Div("group ml-3 flex items-center gap-1").Render(
					r.Span("text-[11px] font-mono text-gray-400 dark:text-zinc-600 truncate").Text(activePath),
				)
				if state.ActiveProject != "home" {
					pathWrapper.Render(
						r.Button("flex items-center justify-center w-4 h-4 rounded cursor-pointer text-red-500 dark:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity duration-75").
							Attr("title", "Remove project").
							OnClick(&r.Action{
								Name: "project.remove",
								Data: map[string]any{"sid": sid, "name": state.ActiveProject},
							}).
							Render(r.I("material-icons-round text-[14px]").Text("close")),
					)
				}
				return pathWrapper
			}(),
			r.Div("ml-auto flex items-center gap-1").Render(
				r.Span("text-xs text-gray-400 dark:text-gray-500 font-mono select-none").Text("v"+version.Version),
				r.Button("inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium cursor-pointer border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-200 dark:border-gray-600 dark:hover:bg-gray-700 transition-colors").
					Attr("title", "Keyboard shortcuts").
					Attr("onclick", fmt.Sprintf("document.getElementById('%s').classList.toggle('hidden');", ShortcutsDialogID)).
					Render(
						r.I("material-icons-round text-base").Text("keyboard"),
						r.Span("").Text("Shortcuts"),
					),
				r.Button("inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium cursor-pointer border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-200 dark:border-gray-600 dark:hover:bg-gray-700 transition-colors").
					Attr("title", "Toggle fullscreen").
					Attr("onclick", "if(document.fullscreenElement){document.exitFullscreen();this.querySelector('i').textContent='fullscreen';this.querySelector('span').textContent='Fullscreen';}else{document.documentElement.requestFullscreen();this.querySelector('i').textContent='fullscreen_exit';this.querySelector('span').textContent='Exit';}").
					Render(
						r.I("material-icons-round text-base").Text("fullscreen"),
						r.Span("").Text("Fullscreen"),
					),
				r.ThemeSwitcher(),
			),
		)
}

// renderProjectDialog renders the create project modal
func renderProjectDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-teal-500 focus:border-teal-500 outline-none transition-colors"

	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "/"
	}

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75"+hiddenClass).
		ID(ProjectDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-lg mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("New Project"),

					// Directory picker
					r.Div("mb-4").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Select Folder"),
						renderDirBrowser(homeDir, sid),
					),

					// Hidden input for selected path
					r.IHidden("").ID("project-path").Attr("value", ""),

					// Name field (auto-populated, editable)
					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Name"),
						r.IText(inputCls).
							ID("project-name").
							Attr("placeholder", "select a folder above").
							Attr("oninput", "this.dataset.auto='false'").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-create-project').click();}"),
					),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "project.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-create-project").
							Text("Create").
							OnClick(&r.Action{
								Name:    "project.create",
								Data:    sidData(sid),
								Collect: []string{"project-name", "project-path"},
							}),
					),
				),
		)
}

// renderDirBrowser renders the directory browser component
func renderDirBrowser(currentPath string, sid string) *r.Node {
	dirCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-gray-700 dark:text-zinc-300 hover:bg-teal-50 dark:hover:bg-teal-900/20 rounded cursor-pointer transition-colors"
	selectedCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-teal-700 dark:text-teal-300 bg-teal-50 dark:bg-teal-900/30 rounded font-medium"

	// Read directories
	entries, err := os.ReadDir(currentPath)
	var dirs []string
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
	}

	// Current path display + select button
	pathBar := r.Div("flex items-center gap-2 mb-2").Render(
		r.Div("flex-1 px-3 py-2 bg-gray-50 dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-md text-xs font-mono text-gray-600 dark:text-zinc-400 truncate").
			Attr("title", currentPath).
			Text(currentPath),
		r.Button("shrink-0 px-3 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-xs font-medium rounded-md transition-colors cursor-pointer").
			Text("Select").
			OnClick(r.JS(fmt.Sprintf(`
				document.getElementById('project-path').value='%s';
				var nameEl=document.getElementById('project-name');
				if(!nameEl.value||nameEl.dataset.auto==='true'){
					nameEl.value='%s';
					nameEl.dataset.auto='true';
				}
				nameEl.dispatchEvent(new Event('input'));
			`, currentPath, filepath.Base(currentPath)))),
	)

	// Parent directory entry
	var items []*r.Node
	parentPath := filepath.Dir(currentPath)
	if parentPath != currentPath {
		items = append(items, r.Div(dirCls).Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-base").Text("arrow_upward"),
			r.Span("").Text(".."),
		).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": parentPath},
		}))
	}

	// Directory entries
	for _, d := range dirs {
		fullPath := filepath.Join(currentPath, d)
		items = append(items, r.Div(dirCls).Render(
			r.I("material-icons-round text-amber-500 dark:text-amber-400 text-base").Text("folder"),
			r.Span("truncate").Text(d),
		).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": fullPath},
		}))
	}

	// Highlight current path as selected
	_ = selectedCls

	dirList := r.Div("max-h-48 overflow-y-auto space-y-0.5 border border-gray-200 dark:border-zinc-700 rounded-md p-1.5 bg-gray-50/50 dark:bg-zinc-800/50")
	if len(items) == 0 {
		dirList.Render(
			r.Div("px-3 py-2 text-xs font-mono text-gray-400 dark:text-zinc-600 italic").Text("No subdirectories"),
		)
	} else {
		dirList.Render(items...)
	}

	return r.Div("").ID(DirBrowserID).Render(pathBar, dirList)
}

// updateHashJS returns JS that updates the URL hash to the given project name
func updateHashJS(name string) string {
	return fmt.Sprintf("history.replaceState(null,'','#%s');", name)
}

// savedAppsJS returns JS that sets the global __libroSavedApps variable from DB data.
func savedAppsJS() string {
	apps := DBLoadAllSavedApps()
	type jsApp struct {
		Type     string `json:"type"`
		URL      string `json:"url,omitempty"`
		Command  string `json:"command,omitempty"`
		Width    string `json:"width"`
		Writable bool   `json:"writable"`
		Name     string `json:"name,omitempty"`
	}
	jsApps := make([]jsApp, len(apps))
	for i, a := range apps {
		jsApps[i] = jsApp{Type: a.Type, URL: a.URL, Command: a.Command, Width: a.Width, Writable: a.Writable, Name: a.Name}
	}
	b, _ := json.Marshal(jsApps)
	return fmt.Sprintf("window.__libroSavedApps=%s;", string(b))
}

// initHashJS handles hash-based project navigation on page load.
// Projects are now loaded from DB on server side, so only hash switching is needed.
func initHashJS(sid string) string {
	return fmt.Sprintf(`
(function _initHash(){
	if(typeof __ws==='undefined'||!__ws.call){setTimeout(_initHash,50);return;}
	var hash=location.hash.replace('#','');
	if(hash&&hash!=='home'){
		setTimeout(function(){__ws.call('project.switch',{sid:'%s',name:hash});},100);
	}
	if(!location.hash){history.replaceState(null,'','#home');}
})();
`, sid)
}

// termIconSetupJS returns JS that registers a global icon lookup function
// for terminal commands. All JS icon renderers should call __libroTermIcon(cmd, size).
func termIconSetupJS() string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroTermIcon)return;
	var icons=%s;

	function resolveCmd(command){
		var parts=command.trim().split(/\s+/);
		var cmd=parts[0]||'';
		for(var i=0;i<parts.length;i++){
			if(parts[i]!=='sudo'&&parts[i]!=='env'&&parts[i].indexOf('=')===-1){cmd=parts[i];break;}
		}
		var sl=cmd.lastIndexOf('/');
		if(sl>=0)cmd=cmd.substring(sl+1);
		return cmd.toLowerCase();
	}

	window.__libroTermIcon=function(command,size){
		size=size||24;
		var cmd=resolveCmd(command);
		var info=icons[cmd];
		if(info&&info.url){
			return '<img src="'+info.url+'" style="width:'+size+'px;height:'+size+'px;object-fit:contain" onerror="this.outerHTML=__libroTermIconFallback(\''+command.replace(/'/g,"\\'")+'\','+size+')">';
		}
		if(info&&info.mi){
			return '<i class="material-icons-round" style="font-size:'+size+'px;color:#9ca3af">'+info.mi+'</i>';
		}
		return __libroTermIconFallback(command,size);
	};

	window.__libroTermIconFallback=function(command,size){
		var ini=(command||'T').substring(0,1).toUpperCase();
		var palettes=[['#0d9488','#065f46','#047857'],['#7c3aed','#4c1d95','#5b21b6'],['#2563eb','#1e3a5f','#1d4ed8'],['#db2777','#831843','#9d174d'],['#d97706','#78350f','#92400e'],['#059669','#064e3b','#047857'],['#dc2626','#7f1d1d','#991b1b'],['#0891b2','#164e63','#155e75']];
		var hash=0;for(var i=0;i<command.length;i++)hash=((hash<<5)-hash)+command.charCodeAt(i);
		var p=palettes[Math.abs(hash)%%palettes.length];
		var r=Math.round(size*0.3);
		return '<span style="display:inline-flex;align-items:center;justify-content:center;width:'+size+'px;height:'+size+'px;border-radius:'+r+'px;position:relative;overflow:hidden;background:linear-gradient(145deg,'+p[0]+' 0%%,'+p[2]+' 60%%,'+p[1]+' 100%%);box-shadow:0 1px 4px rgba(0,0,0,.25),inset 0 1px 0 rgba(255,255,255,.25),inset 0 -1px 0 rgba(0,0,0,.12);font-size:'+(size*0.5)+'px;font-weight:800;color:#fff;letter-spacing:.04em;text-shadow:0 1px 1px rgba(0,0,0,.3);font-family:ui-monospace,SFMono-Regular,Menlo,monospace"><span style="position:absolute;inset:0;border-radius:'+r+'px;background:linear-gradient(180deg,rgba(255,255,255,.2) 0%%,rgba(255,255,255,.05) 40%%,transparent 60%%);pointer-events:none"></span><span style="position:relative;z-index:1">'+ini+'</span></span>';
	};
})();
`, knownTermIconsJS())
}

func keyboardShortcutsJS(sid string) string {
	return fmt.Sprintf(`
		(function() {
			if (window.__libroKbRegistered) return;
			window.__libroKbRegistered = true;

			window.__libroFocusApp = function(idx) {
				// Find the visible strip (parent project div has display != none)
				var strips = document.querySelectorAll('[id^="app-strip-"]');
				var strip = null;
				for (var s = 0; s < strips.length; s++) {
					var parent = strips[s].closest('[id^="project-main-"]');
					if (parent && parent.style.display !== 'none') {
						strip = strips[s];
						break;
					}
				}
				if (!strip) return;
				var container = strip.children[idx + 1];
				if (!container) return;
				var iframe = container.querySelector('iframe');
				if (!iframe) return;
				// Blur all other iframes first
				var allIframes = document.querySelectorAll('iframe');
				for (var i = 0; i < allIframes.length; i++) {
					if (allIframes[i] !== iframe) {
						try { allIframes[i].contentWindow.blur(); } catch(err) {}
						allIframes[i].blur();
					}
				}
				// Focus target iframe and click into it so xterm.js picks up focus
				iframe.focus();
				try {
					iframe.contentWindow.focus();
					var doc = iframe.contentDocument || iframe.contentWindow.document;
					var termEl = doc.querySelector('.xterm-helper-textarea') || doc.querySelector('textarea') || doc.body;
					if (termEl) {
						termEl.focus();
					}
				} catch(err) {}
			};

			function libroKeyHandler(e) {
				if (e.metaKey && (e.key === 'h' || e.key === 'H')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.navigate.left', {"sid": "%s"});
				}
				if (e.metaKey && (e.key === 'l' || e.key === 'L')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.navigate.right', {"sid": "%s"});
				}
				if (e.ctrlKey && e.key >= '1' && e.key <= '9') {
					e.preventDefault();
					e.stopImmediatePropagation();
					var idx = parseInt(e.key) - 1;
					__ws.call('app.select', {"sid": "%s", "index": idx});
					window.__libroFocusApp(idx);
				}
				if (e.metaKey && (e.key === 'j' || e.key === 'J')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('project.navigate.next', {"sid": "%s"});
				}
				if (e.metaKey && (e.key === 'k' || e.key === 'K')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('project.navigate.prev', {"sid": "%s"});
				}
				if (e.metaKey && e.key === '/') {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenSearch) window.__libroOpenSearch();
				}
				if (e.metaKey && (e.key === 'd' || e.key === 'D')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					var strips = document.querySelectorAll('[id^="app-strip-"]');
					for (var s = 0; s < strips.length; s++) {
						var parent = strips[s].closest('[id^="project-main-"]');
						if (parent && parent.style.display !== 'none') {
							var sel = strips[s].querySelector('.border-teal-500[data-app-id]');
							if (sel) {
								__ws.call('app.close', {"sid": "%s", "id": sel.getAttribute('data-app-id')});
							}
							break;
						}
					}
				}
			}

			document.addEventListener('keydown', libroKeyHandler, true);

			function attachIframeListeners() {
				var iframes = document.querySelectorAll('iframe');
				for (var i = 0; i < iframes.length; i++) {
					if (iframes[i].__libroKbAttached) continue;
					iframes[i].__libroKbAttached = true;
					(function(iframe) {
						function attach() {
							try {
								var doc = iframe.contentDocument || iframe.contentWindow.document;
								if (!doc.__libroKbAttached) {
									doc.__libroKbAttached = true;
									doc.addEventListener('keydown', libroKeyHandler, true);
								}
							} catch(err) {}
						}
						iframe.addEventListener('load', attach);
						attach();
					})(iframes[i]);
				}
			}

			attachIframeListeners();
			window.__libroAttachIframeListeners = attachIframeListeners;

			var obs = new MutationObserver(function() { attachIframeListeners(); });
			obs.observe(document.body, {childList: true, subtree: true});

			// Listen for URL navigation messages from proxied iframes
			window.addEventListener('message', function(e) {
				if (!e.data || !e.data.libroNav) return;
				var iframes = document.querySelectorAll('iframe');
				for (var i = 0; i < iframes.length; i++) {
					try {
						if (iframes[i].contentWindow === e.source) {
							var id = iframes[i].id;
							var appId = id.replace('frame-', '');
							var input = document.getElementById('urlinput-' + appId);
							if (input) input.value = e.data.libroNav;
							break;
						}
					} catch(err) {}
				}
			});
		})();
	`, sid, sid, sid, sid, sid, sid)
}
