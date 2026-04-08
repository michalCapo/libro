package libro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"

	"libro/internal/version"
)

// urlParse is a convenience wrapper around url.Parse.
func urlParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

const (
	DialogID          = "add-dialog"
	MainAreaID        = "main-area"
	ProjectBarID      = "project-bar" // kept for backward compat references
	TopBarID          = "top-bar"
	SidebarID         = "project-sidebar"
	ProjectDialogID   = "project-dialog"
	DirBrowserID      = "dir-browser"
	SearchDialogID    = "search-dialog"
	ShortcutsDialogID = "shortcuts-dialog"
	CloseDialogID     = "close-dialog"
	WorktreeDialogID  = "worktree-dialog"
	ManageDialogID    = "manage-dialog"
	URLPopupID        = "url-popup"
	ResizePopupID     = "resize-popup"
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
	"nvim":      {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"neovim":    {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"vim":       {URL: "https://cdn.simpleicons.org/vim/019733"},
	"vi":        {URL: "https://cdn.simpleicons.org/vim/019733"},
	"claude":    {URL: "https://cdn.simpleicons.org/anthropic/d4a27f"},
	"node":      {URL: "https://cdn.simpleicons.org/nodedotjs/5FA04E"},
	"npm":       {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"npx":       {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"bun":       {URL: "https://cdn.simpleicons.org/bun/FBF0DF"},
	"deno":      {URL: "https://cdn.simpleicons.org/deno/FFFFFF"},
	"python":    {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"python3":   {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"pip":       {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"docker":    {URL: "https://cdn.simpleicons.org/docker/2496ED"},
	"git":       {URL: "https://cdn.simpleicons.org/git/F05032"},
	"go":        {URL: "https://cdn.simpleicons.org/go/00ADD8"},
	"cargo":     {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"rustc":     {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"ruby":      {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"irb":       {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"lua":       {URL: "https://cdn.simpleicons.org/lua/2C2D72"},
	"java":      {URL: "https://cdn.simpleicons.org/openjdk/FFFFFF"},
	"kotlin":    {URL: "https://cdn.simpleicons.org/kotlin/7F52FF"},
	"swift":     {URL: "https://cdn.simpleicons.org/swift/F05138"},
	"redis-cli": {URL: "https://cdn.simpleicons.org/redis/FF4438"},
	"psql":      {URL: "https://cdn.simpleicons.org/postgresql/4169E1"},
	"mysql":     {URL: "https://cdn.simpleicons.org/mysql/4479A1"},
	"mongosh":   {URL: "https://cdn.simpleicons.org/mongodb/47A248"},
	"kubectl":   {URL: "https://cdn.simpleicons.org/kubernetes/326CE5"},
	"terraform": {URL: "https://cdn.simpleicons.org/terraform/844FBA"},
	"ansible":   {URL: "https://cdn.simpleicons.org/ansible/EE0000"},
	"tmux":      {URL: "https://cdn.simpleicons.org/tmux/1BB91F"},
	"bash":      {MaterialIcon: "terminal"},
	"zsh":       {MaterialIcon: "terminal"},
	"sh":        {MaterialIcon: "terminal"},
	"fish":      {MaterialIcon: "terminal"},
	"ssh":       {MaterialIcon: "vpn_key"},
	"htop":      {MaterialIcon: "monitoring"},
	"btop":      {MaterialIcon: "monitoring"},
	"top":       {MaterialIcon: "monitoring"},
	"make":      {MaterialIcon: "build"},
	"cmake":     {MaterialIcon: "build"},
	"gradle":    {MaterialIcon: "build"},
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

// discoverTermIconURL tries to find an icon for a terminal command by checking
// the Simple Icons CDN. Returns the icon URL if found, empty string otherwise.
func discoverTermIconURL(command string) string {
	cmd := extractBaseCmd(command)
	if cmd == "" {
		return ""
	}
	// Already in known icons — no need to discover
	if _, ok := knownTermIcons[cmd]; ok {
		return ""
	}

	// Try the command name directly and common variations against Simple Icons CDN
	candidates := []string{cmd}
	// Also try without trailing digits (e.g. "python3" → "python")
	trimmed := strings.TrimRight(cmd, "0123456789")
	if trimmed != "" && trimmed != cmd {
		candidates = append(candidates, trimmed)
	}

	client := &http.Client{Timeout: 4 * time.Second}
	for _, name := range candidates {
		iconURL := "https://cdn.simpleicons.org/" + name
		resp, err := client.Head(iconURL)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return iconURL
		}
	}
	return ""
}

// extractBaseCmd extracts the base command name from a command string.
func extractBaseCmd(command string) string {
	parts := strings.Fields(command)
	cmd := ""
	for _, p := range parts {
		if p != "sudo" && p != "env" && !strings.Contains(p, "=") {
			cmd = p
			break
		}
	}
	if cmd == "" && len(parts) > 0 {
		cmd = parts[0]
	}
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		cmd = cmd[idx+1:]
	}
	return strings.ToLower(cmd)
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

// renderMainAreaWrapper renders the wrapper that contains the sidebar and all per-project main area divs.
// Only the active project's div is visible; others are hidden to preserve state.
func renderMainAreaWrapper(state *AppState, sid string) *r.Node {
	return r.Div("flex-1 flex flex-row overflow-hidden relative").Render(
		renderProjectSidebar(state, sid),
		r.Div("flex-1 flex flex-col overflow-hidden relative").ID(MainAreaID).Render(
			renderMainArea(state, sid),
			// Hidden pool to keep webview elements alive when their app tab is closed.
			// This preserves session state (cookies, WebSocket connections) for sites
			// like Discord that invalidate tokens when the webview process is destroyed.
			r.Div("hidden").ID("webview-pool"),
		),
	)
}

// renderMainArea renders the entire main area based on current state
func renderMainArea(state *AppState, sid string) *r.Node {
	if len(state.Apps) == 0 {
		return renderEmptyState(state, sid)
	}
	return renderAppStrip(state, sid)
}

// renderEmptyState renders the saved apps list with "+ Add App" button
func renderEmptyState(state *AppState, sid string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(state.ActiveProject)
	appButtons := make([]*r.Node, 0, len(savedApps))
	for _, app := range savedApps {
		appButtons = append(appButtons, renderSavedAppButton(app, sid))
	}

	// Guide text for empty projects (no saved apps yet)
	var guideNode *r.Node
	if len(savedApps) == 0 {
		guideNode = r.Div("text-center mb-4").Render(
			r.P("text-sm text-gray-400 dark:text-zinc-500 leading-relaxed").
				Text("Add web applications by URL or terminal commands to get started. Use Browse to quickly look something up on the web. Check out shortcuts for a productivity boost."),
		)
	}

	container := r.Div("flex-1 flex items-center justify-center").ID(projectMainID(state.ActiveProject)).Render(
		r.Div("flex flex-col items-center gap-2 w-full max-w-md").Render(
			guideNode,
			r.Div("flex flex-col gap-1.5 w-full").Render(appButtons...),
			r.Div("flex gap-2 w-full mt-1").Render(
				r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Render(r.I("material-icons-round text-[18px]").Text("add"), r.Span("").Text("Add App")).
					OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
				r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-indigo-600 hover:bg-indigo-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Render(r.I("material-icons-round text-[18px]").Text("search"), r.Span("").Text("Quick Launch")).
					OnClick(&r.Action{Name: "app.run.new", Data: sidData(sid)}),
			),
		),
	)
	return container
}

// renderSavedAppButton renders a single saved app button (server-side, from DB data).
func renderSavedAppButton(app SavedApp, sid string) *r.Node {
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
		} else if app.IconURL != "" {
			iconNode = r.Img("w-5 h-5 shrink-0 rounded-sm").Attr("src", app.IconURL)
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
					h := strings.TrimPrefix(u.Hostname(), "www.")
					label = h
				}
			}
		}
	}

	writable := app.Writable

	// Launch button (simplified - edit/delete moved to Manage Apps dialog)
	btn := r.Button("w-full flex items-center gap-3 px-4 py-3 bg-white dark:bg-zinc-800/80 hover:bg-gray-50 dark:hover:bg-zinc-700/80 border border-gray-200 dark:border-zinc-700/40 hover:border-gray-300 dark:hover:border-zinc-600 rounded-lg cursor-pointer text-left transition-colors duration-75 shadow-sm dark:shadow-none").
		Render(
			iconNode,
			r.Span("flex-1 truncate text-sm text-gray-800 dark:text-zinc-200").Text(label),
			r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Type),
			r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Width),
		).
		OnClick(&r.Action{Name: "app.start", Data: map[string]any{
			"sid": sid, "type": app.Type, "url": app.URL,
			"command": app.Command, "width": app.Width,
			"writable": writable, "name": app.Name,
			"iconUrl": app.IconURL,
		}})

	return btn
}

// savedAppEditFillJS returns JS that fills the add dialog with saved app data for editing.
func savedAppEditFillJS(app SavedApp) string {
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
	var ps=document.getElementById('app-project-specific');if(ps)ps.checked=%v;
},100);
`, jsString(app.Type), jsString(app.Command), app.Writable, jsString(app.URL), jsString(app.Name), jsString(app.Width), app.ProjectSpecific)
}

// renderAppStrip renders the horizontal strip of applications with navigation
func renderAppStrip(state *AppState, sid string) *r.Node {
	// Build strip children: left spacer + apps + right spacer
	stripChildren := make([]*r.Node, 0, len(state.Apps)+2)
	stripChildren = append(stripChildren, r.Div("flex-1 shrink min-w-0").Attr("style", "order:-1"))
	for i, app := range state.Apps {
		stripChildren = append(stripChildren, renderAppFrame(app, i, i == state.SelectedIndex, sid, state.ZenMode))
	}
	stripChildren = append(stripChildren, r.Div("flex-1 shrink min-w-0").Attr("style", "order:99999"))

	strip := r.Div("flex-1 min-w-0 flex items-stretch h-full overflow-x-auto overflow-y-hidden gap-0.5 p-0.5").
		ID(stripID(state.ActiveProject)).
		Render(stripChildren...)

	mainArea := r.Div("flex-1 flex items-stretch overflow-hidden relative p-0.5").ID(projectMainID(state.ActiveProject)).
		Render(
			strip,
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
					var idx = %d;
					var sorted = window.__libroSortedApps ? window.__libroSortedApps(strip) : Array.from(strip.querySelectorAll(':scope > [data-app-id]'));
					var app = sorted[idx];
					if (app && window.__libroScrollToApp) {
						window.__libroScrollToApp(app);
					}
				});
			});
		})();
	`, stripID(projectName), totalApps, selectedIndex)
}

// moveAppJS reorders app frames visually using CSS order (no DOM moves,
// so Electron webviews are preserved). Then runs navigateJS for selection visuals.
func moveAppJS(state *AppState, sid string, _ string) string {
	return navigateJS(state, sid)
}

func navigateJS(state *AppState, sid string) string {
	// Build JS to set CSS order on all app frames (keeps DOM stable for webviews)
	var orderJS strings.Builder
	for i, app := range state.Apps {
		fmt.Fprintf(&orderJS, "var e=document.querySelector('[data-app-id=\"%s\"]');if(e)e.style.order='%d';", app.ID, i)
	}

	return fmt.Sprintf(`
		(function() {
			%s
			var strip = document.getElementById('%s');
			if (!strip) return;
			var selectedIdx = %d;
			var totalApps = %d;
			var zenMode = %v;
			var sorted = window.__libroSortedApps ? window.__libroSortedApps(strip) : Array.from(strip.querySelectorAll(':scope > [data-app-id]'));

			for (var i = 0; i < totalApps; i++) {
				var child = sorted[i];
				if (!child) continue;
				if (i === selectedIdx) {
					// Zen mode: show blue border around selected app
					if (zenMode) {
						child.className = child.className.replace(/border-\[3px\] border-blue-500/g, '').replace(/border-\[3px\] border-gray-300 dark:border-zinc-600/g, '').replace(/\bborder\b/g, '') + ' border-[3px] border-blue-500';
					} else {
						child.className = child.className.replace(/border-\[3px\] border-blue-500/g, '').replace(/border-\[3px\] border-gray-300 dark:border-zinc-600/g, '').replace(/\bborder\b/g, '') + ' border';
					}
					var toolbar = child.children[0];
					// Make toolbar blue for selected app
					if (toolbar) {
						toolbar.className = 'flex items-center gap-2 px-1.5 py-1 border-b shrink-0 bg-blue-600 border-blue-700';
						// Update nav buttons to white-on-red
						toolbar.querySelectorAll('button[title]').forEach(function(btn){
							btn.className = btn.className.replace(/text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700/g, 'text-blue-100/70 hover:text-white hover:bg-white/15');
						});
						// Update URL input
						var urlInp = toolbar.querySelector('input[type=text]');
						if (urlInp) {
							urlInp.className = urlInp.className.replace(/bg-gray-100 dark:bg-zinc-800/g, 'bg-white/15').replace(/text-gray-600 dark:text-zinc-400/g, 'text-white').replace(/placeholder-gray-400 dark:placeholder-zinc-600/g, 'placeholder-blue-200/50');
						}
					}
					// Update size badges for selected app
					var badges = child.querySelector('[data-size-badges]');
					if (badges) {
						var btns = badges.querySelectorAll('button');
						var sizeLabels = ['SM','MD','LG','XL','2XL','FULL'];
						var activeBase = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
						btns.forEach(function(b){
							var txt = b.textContent.trim();
							if (sizeLabels.indexOf(txt) === -1) {
								// close button
								b.className = activeBase + ' ml-1 flex items-center justify-center text-blue-100/70 hover:text-white hover:bg-white/15';
								return;
							}
							var isActive = b.className.indexOf('bg-blue-600') !== -1 || b.className.indexOf('bg-white/25') !== -1;
							if (isActive) {
								b.className = activeBase + ' bg-white/25 text-white';
							} else {
								b.className = activeBase + ' text-blue-100/70 hover:text-white hover:bg-white/15';
							}
						});
					}
					var overlay = child.querySelector('[data-click-overlay]');
					if (overlay) overlay.remove();
				} else {
					// Zen mode: gray border for unselected app, normal border otherwise
					child.className = child.className.replace(/border-\[3px\] border-blue-500/g, '').replace(/border-\[3px\] border-gray-300 dark:border-zinc-600/g, '').replace(/\bborder\b/g, '') + (zenMode ? ' border-[3px] border-gray-300 dark:border-zinc-600' : ' border');
					var toolbar2 = child.children[0];
					// Revert toolbar to default for unselected app
					if (toolbar2) {
						toolbar2.className = 'flex items-center gap-2 px-1.5 py-1 border-b shrink-0 bg-white dark:bg-zinc-900 border-gray-200 dark:border-zinc-700/50';
						// Revert nav buttons
						toolbar2.querySelectorAll('button[title]').forEach(function(btn){
							btn.className = btn.className.replace(/text-blue-100\/70 hover:text-white hover:bg-white\/15/g, 'text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700');
						});
						// Revert URL input
						var urlInp2 = toolbar2.querySelector('input[type=text]');
						if (urlInp2) {
							urlInp2.className = urlInp2.className.replace(/bg-white\/15/g, 'bg-gray-100 dark:bg-zinc-800').replace(/text-white/g, 'text-gray-600 dark:text-zinc-400').replace(/placeholder-blue-200\/50/g, 'placeholder-gray-400 dark:placeholder-zinc-600');
						}
					}
					// Revert size badges to default for unselected app
					var badges2 = child.querySelector('[data-size-badges]');
					if (badges2) {
						var btns2 = badges2.querySelectorAll('button');
						var sizeLabels2 = ['SM','MD','LG','XL','2XL','FULL'];
						var activeBase2 = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
						btns2.forEach(function(b){
							var txt = b.textContent.trim();
							if (sizeLabels2.indexOf(txt) === -1) {
								// close button
								b.className = activeBase2 + ' ml-1 flex items-center justify-center text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10';
								return;
							}
							var isActive = b.className.indexOf('bg-blue-600') !== -1 || b.className.indexOf('bg-white/25') !== -1;
							if (isActive) {
								b.className = activeBase2 + ' bg-blue-600 text-white';
							} else {
								b.className = activeBase2 + ' text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700';
							}
						});
					}
					if (!child.querySelector('[data-click-overlay]')) {
						var ov = document.createElement('div');
						ov.setAttribute('data-click-overlay', '');
						ov.className = 'absolute inset-0 z-40 cursor-pointer';
						ov.onmousedown = function(idx) {
							return function(e) { e.preventDefault(); __ws.call('app.select', {"sid": "%s", "index": idx}); };
						}(i);
						var iframeWrap = child.children[1];
						if (iframeWrap) { iframeWrap.appendChild(ov); } else { child.appendChild(ov); }
					}
				}
			}

			var selected = sorted[selectedIdx];
			if (selected) {
				selected.style.animation='none';
				selected.offsetHeight;
				selected.style.animation='libro-app-select .08s ease-out';
				if(window.__libroScrollToApp)window.__libroScrollToApp(selected);
				if (window.__libroFocusApp) {
					setTimeout(function() { window.__libroFocusApp(selectedIdx); }, 30);
				}
			}
		})();
	`, orderJS.String(), stripID(state.ActiveProject), state.SelectedIndex, len(state.Apps), state.ZenMode, sid)
}

func flashCSS() string {
	return `(function(){if(!document.getElementById('libro-flash-css')){var s=document.createElement('style');s.id='libro-flash-css';s.textContent='@keyframes libro-flash{0%{transform:scale(1);opacity:1}15%{transform:scale(2.5);opacity:.6}100%{transform:scale(1);opacity:1}} @keyframes libro-toast-in{0%{opacity:0;transform:translate(-50%,-50%) scale(.98)}100%{opacity:1;transform:translate(-50%,-50%) scale(1)}} @keyframes libro-toast-out{0%{opacity:1;transform:translate(-50%,-50%) scale(1)}100%{opacity:0;transform:translate(-50%,-50%) scale(.98)}} @keyframes libro-toast-slide-up{0%{transform:translateY(100%);opacity:0}100%{transform:translateY(0);opacity:1}} @keyframes libro-toast-slide-down{0%{transform:translateY(0);opacity:1}100%{transform:translateY(100%);opacity:0}} @keyframes libro-app-select{0%{outline:2px solid rgba(59,130,246,.5)}100%{outline:2px solid transparent}} @keyframes libro-project-switch{0%{opacity:0}100%{opacity:1}}';document.head.appendChild(s);}window.__libroScrollToApp=function(app){var strip=app.parentElement;if(!strip)return;var sr=strip.getBoundingClientRect();var ar=app.getBoundingClientRect();var appLeft=ar.left-sr.left+strip.scrollLeft;var appRight=appLeft+ar.width;var pad=8;if(appLeft-pad<strip.scrollLeft){strip.scrollLeft=appLeft-pad;}else if(appRight+pad>strip.scrollLeft+sr.width){strip.scrollLeft=appRight+pad-sr.width;}};})();`
}

// projectToastJS returns JS that shows a brief centered toast with the project and branch.
func projectToastJS(name string) string {
	// Split virtual project names like "nisa/test" into project + branch
	proj := name
	branch := ""
	if i := strings.Index(name, "/"); i >= 0 {
		proj = name[:i]
		branch = name[i+1:]
	}
	return fmt.Sprintf("if(window.__libroProjectToast)window.__libroProjectToast(%s,%s);", jsString(proj), jsString(branch))
}

// projectToastSetupJS returns JS that registers the global toast function.
func projectToastSetupJS() string {
	return `
(function(){
	if(window.__libroProjectToast)return;
	if(window.__libroShowToast)return;
	var timer=null;
	window.__libroProjectToast=function(proj,branch){
		var el=document.getElementById('libro-project-toast');
		if(!el){
			el=document.createElement('div');
			el.id='libro-project-toast';
			el.style.cssText='position:fixed;top:38%;left:50%;transform:translate(-50%,-50%) scale(.92);z-index:9999;pointer-events:none;opacity:0;';
			document.body.appendChild(el);
		}
		if(timer){clearTimeout(timer);timer=null;}
		var dk=document.documentElement.classList.contains('dark');
		var bg=dk?'rgba(24,24,37,.88)':'rgba(255,255,255,.92)';
		var border=dk?'rgba(63,63,90,.5)':'rgba(200,200,220,.6)';
		var fg=dk?'#e2e2e8':'#1a1a2e';
		var dim=dk?'#7a7a8e':'#8a8a9e';
		var html='<div style="background:'+bg+';border:1px solid '+border+';backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);border-radius:12px;padding:20px 36px;text-align:center;box-shadow:0 8px 32px rgba(0,0,0,.18)">';
		html+='<div style="font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:28px;font-weight:700;color:'+fg+';letter-spacing:-.02em;line-height:1.2">'+proj.replace(/</g,'&lt;')+'</div>';
		if(branch){html+='<div style="font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:14px;color:'+dim+';margin-top:4px;letter-spacing:.02em"><span style="opacity:.5">&#8627;</span> '+branch.replace(/</g,'&lt;')+'</div>';}
		html+='</div>';
		el.innerHTML=html;
		el.style.animation='libro-toast-in .04s ease-out forwards';
		timer=setTimeout(function(){
			el.style.animation='libro-toast-out .05s ease-in forwards';
			timer=setTimeout(function(){el.style.opacity='0';timer=null;},60);
		},600);
	};
	// Configurable toast with custom message and duration
	window.__libroShowToast=function(title,subtitle,durationMs){
		var dur=durationMs||3000;
		var el=document.getElementById('libro-project-toast');
		if(!el){
			el=document.createElement('div');
			el.id='libro-project-toast';
			el.style.cssText='position:fixed;top:38%;left:50%;transform:translate(-50%,-50%) scale(.92);z-index:9999;pointer-events:none;opacity:0;';
			document.body.appendChild(el);
		}
		if(timer){clearTimeout(timer);timer=null;}
		var dk=document.documentElement.classList.contains('dark');
		var bg=dk?'rgba(24,24,37,.88)':'rgba(255,255,255,.92)';
		var border=dk?'rgba(63,63,90,.5)':'rgba(200,200,220,.6)';
		var fg=dk?'#e2e2e8':'#1a1a2e';
		var dim=dk?'#7a7a8e':'#8a8a9e';
		var html='<div style="background:'+bg+';border:1px solid '+border+';backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);border-radius:12px;padding:20px 36px;text-align:center;box-shadow:0 8px 32px rgba(0,0,0,.18)">';
		html+='<div style="font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:22px;font-weight:600;color:'+fg+';letter-spacing:-.02em;line-height:1.3">'+title.replace(/</g,'&lt;')+'</div>';
		if(subtitle){html+='<div style="font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:14px;color:'+dim+';margin-top:6px;letter-spacing:.02em;max-width:400px;line-height:1.4">'+subtitle.replace(/</g,'&lt;')+'</div>';}
		html+='</div>';
		el.innerHTML=html;
		el.style.animation='libro-toast-in .04s ease-out forwards';
		timer=setTimeout(function(){
			el.style.animation='libro-toast-out .05s ease-in forwards';
			timer=setTimeout(function(){el.style.opacity='0';timer=null;},60);
		},dur);
	};
	// --- Download toast helpers ---
	function dlTheme(){
		var dk=document.documentElement.classList.contains('dark');
		return{bg:dk?'#1e1e2e':'#f8f8fa',border:dk?'rgba(63,63,90,.6)':'rgba(200,200,220,.7)',fg:dk?'#e2e2e8':'#1a1a2e',dim:dk?'#7a7a8e':'#8a8a9e',accent:dk?'#89b4fa':'#3b82f6',bar:dk?'rgba(63,63,90,.4)':'rgba(200,200,220,.4)'};
	}
	function dlBase(id){
		var el=document.getElementById('libro-dl-'+id);
		if(el)return el;
		el=document.createElement('div');
		el.id='libro-dl-'+id;
		var t=dlTheme();
		el.style.cssText='position:fixed;bottom:0;left:0;right:0;z-index:9999;padding:8px 16px;background:'+t.bg+';border-top:1px solid '+t.border+';display:flex;align-items:center;gap:10px;font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:13px;animation:libro-toast-slide-up .04s ease-out forwards;';
		document.body.appendChild(el);
		return el;
	}
	function dlDismiss(el){
		el.style.animation='libro-toast-slide-down .04s ease-in forwards';
		setTimeout(function(){if(el.parentNode)el.remove();},50);
	}
	function formatBytes(b){
		if(b<=0)return '0 B';
		var u=['B','KB','MB','GB'];var i=Math.floor(Math.log(b)/Math.log(1024));
		return (b/Math.pow(1024,i)).toFixed(i?1:0)+' '+u[i];
	}

	// Show progress bar with cancel button while downloading
	window.__libroShowDownloadProgress=function(id,filename,received,total){
		var el=dlBase(id);
		var t=dlTheme();
		var pct=total>0?Math.round(received/total*100):0;
		var sizeText=total>0?formatBytes(received)+' / '+formatBytes(total):'';
		el.innerHTML='<span class="material-icons-round" style="font-size:18px;color:'+t.accent+';animation:spin 1s linear infinite">downloading</span>'+
			'<span style="color:'+t.fg+';white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:300px">'+filename.replace(/</g,'&lt;')+'</span>'+
			'<span style="color:'+t.dim+';font-size:11px;white-space:nowrap">'+sizeText+'</span>'+
			'<div style="flex:1;height:4px;border-radius:2px;background:'+t.bar+';min-width:80px;max-width:200px;overflow:hidden"><div data-dl-bar style="width:'+pct+'%;height:100%;background:'+t.accent+';border-radius:2px;transition:width .2s"></div></div>'+
			'<span data-dl-cancel style="cursor:pointer;color:'+t.dim+';font-size:18px;line-height:1" class="material-icons-round" title="Cancel download">close</span>';
		el.querySelector('[data-dl-cancel]').addEventListener('click',function(){
			if(window.libroElectron&&window.libroElectron.cancelDownload)window.libroElectron.cancelDownload(id);
		});
	};

	// Update progress bar
	window.__libroUpdateDownloadProgress=function(id,received,total){
		var el=document.getElementById('libro-dl-'+id);
		if(!el)return;
		var bar=el.querySelector('[data-dl-bar]');
		if(bar&&total>0)bar.style.width=Math.round(received/total*100)+'%';
		var spans=el.querySelectorAll('span');
		// Update size text (third span-like element)
		var sizeEl=el.children[2];
		if(sizeEl&&total>0)sizeEl.textContent=formatBytes(received)+' / '+formatBytes(total);
	};

	// Download complete — show clickable filename
	window.__libroShowDownloadToast=function(id,filename,filePath){
		var el=dlBase(id);
		var t=dlTheme();
		el.innerHTML='<span class="material-icons-round" style="font-size:18px;color:'+t.accent+'">download_done</span>'+
			'<a href="#" style="color:'+t.accent+';text-decoration:none;cursor:pointer;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:400px" title="Open '+filename.replace(/"/g,'&quot;')+'">'+filename.replace(/</g,'&lt;')+'</a>'+
			'<span style="margin-left:auto;cursor:pointer;color:'+t.dim+';font-size:18px;line-height:1" class="material-icons-round">close</span>';
		el.querySelector('a').addEventListener('click',function(e){
			e.preventDefault();
			if(window.libroElectron&&window.libroElectron.openPath)window.libroElectron.openPath(filePath);
		});
		el.querySelector('span:last-child').addEventListener('click',function(){dlDismiss(el);});
		setTimeout(function(){if(el.parentNode)dlDismiss(el);},8000);
	};

	// Download failed
	window.__libroShowDownloadFailed=function(id,filename){
		var el=dlBase(id);
		var t=dlTheme();
		el.innerHTML='<span class="material-icons-round" style="font-size:18px;color:#ef4444">error</span>'+
			'<span style="color:'+t.fg+'">Download failed: '+filename.replace(/</g,'&lt;')+'</span>'+
			'<span style="margin-left:auto;cursor:pointer;color:'+t.dim+';font-size:18px;line-height:1" class="material-icons-round">close</span>';
		el.querySelector('span:last-child').addEventListener('click',function(){dlDismiss(el);});
		setTimeout(function(){if(el.parentNode)dlDismiss(el);},5000);
	};

	// Remove toast (on cancel)
	window.__libroRemoveDownloadToast=function(id){
		var el=document.getElementById('libro-dl-'+id);
		if(el)dlDismiss(el);
	};
})();
`
}

// showToastJS returns JS that displays a configurable toast message.
// title: main message (required)
// subtitle: secondary message (optional, can be empty)
// durationMs: visibility duration in milliseconds (default 3000)
func showToastJS(title, subtitle string, durationMs int) string {
	if durationMs <= 0 {
		durationMs = 3000
	}
	return fmt.Sprintf("if(window.__libroShowToast)window.__libroShowToast(%s,%s,%d);", jsString(title), jsString(subtitle), durationMs)
}

// renderAppFrame renders a single application iframe with controls
func renderAppFrame(app Application, index int, selected bool, sid string, zenMode ...bool) *r.Node {
	zen := len(zenMode) > 0 && zenMode[0]
	borderClass := "border border-gray-200 dark:border-zinc-700/50"
	if zen {
		if selected {
			borderClass = "border-[3px] border-blue-500"
		} else {
			borderClass = "border-[3px] border-gray-300 dark:border-zinc-600"
		}
	}

	frameID := fmt.Sprintf("frame-%s", app.ID)

	iframeSrc := app.URL

	// Size badge bar + close (right side of toolbar)
	badgeBase := "px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75"
	badges := make([]*r.Node, 0, len(AllWidths())+1)
	for _, w := range AllWidths() {
		var cls string
		if selected {
			if w == app.Width {
				cls = badgeBase + " bg-white/25 text-white"
			} else {
				cls = badgeBase + " text-blue-100/70 hover:text-white hover:bg-white/15"
			}
		} else {
			if w == app.Width {
				cls = badgeBase + " bg-blue-600 text-white"
			} else {
				cls = badgeBase + " text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700"
			}
		}
		badges = append(badges, r.Button(cls).
			Text(strings.ToUpper(string(w))).
			OnClick(&r.Action{
				Name: "app.resize",
				Data: sidData(sid, "id", app.ID, "width", string(w)),
			}))
	}
	closeBtnCls := badgeBase + " ml-1 flex items-center justify-center"
	if selected {
		closeBtnCls += " text-blue-100/70 hover:text-white hover:bg-white/15"
	} else {
		closeBtnCls += " text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10"
	}
	badges = append(badges, r.Button(closeBtnCls).
		Render(r.I("material-icons-round text-[10px] leading-none block").Text("close")).
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
		btnCls := "flex items-center justify-center w-6 h-6 rounded-sm transition-colors duration-75 cursor-pointer shrink-0"
		if selected {
			btnCls += " text-white/80 hover:text-white hover:bg-white/15"
		} else {
			btnCls += " text-gray-600 dark:text-zinc-400 hover:text-gray-800 dark:hover:text-zinc-200 hover:bg-gray-200 dark:hover:bg-zinc-700"
		}

		// Back button
		backBtn := r.Button(btnCls).
			Attr("title", "Back").
			OnClick(r.JS(fmt.Sprintf(`window.__libroWvBack('%s')`, app.ID)))
		backBtn.Render(r.I("material-icons-round text-sm").Text("arrow_back"))

		// Forward button
		forwardBtn := r.Button(btnCls).
			Attr("title", "Forward").
			OnClick(r.JS(fmt.Sprintf(`window.__libroWvForward('%s')`, app.ID)))
		forwardBtn.Render(r.I("material-icons-round text-sm").Text("arrow_forward"))

		// Copy button
		copyBtn := r.Button(btnCls).
			Attr("title", "Copy URL").
			OnClick(r.JS(fmt.Sprintf(`var inp=document.getElementById('%s');if(inp){navigator.clipboard.writeText(inp.value);var btn=event.currentTarget;btn.style.color='rgb(20,184,166)';setTimeout(function(){btn.style.color='';},800);}`, urlInputID)))
		copyBtn.Render(r.I("material-icons-round text-sm").Text("content_copy"))

		// Reload button
		reloadBtn := r.Button(btnCls).
			Attr("title", "Reload").
			OnClick(r.JS(fmt.Sprintf(`window.__libroWvReload('%s')`, app.ID)))
		reloadBtn.Render(r.I("material-icons-round text-sm").Text("refresh"))

		// URL input — on Enter, navigate webview and update server state
		urlInputCls := "flex-1 min-w-0 rounded-sm text-[11px] font-mono outline-none px-2 h-6"
		if selected {
			urlInputCls += " bg-white/15 text-white placeholder-blue-200/50"
		} else {
			urlInputCls += " bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 placeholder-gray-400 dark:placeholder-zinc-600"
		}
		urlInput := r.Input(urlInputCls).
			ID(urlInputID).
			Attr("type", "text").
			Attr("value", app.URL).
			Attr("spellcheck", "false").
			Attr("autocomplete", "off").
			On("keydown", r.JS(fmt.Sprintf(`if(event.key==='Enter'){event.preventDefault();var u=event.target.value.trim();if(u&&!u.startsWith('http://')&&!u.startsWith('https://')){if(/\s/.test(u)||(!u.includes('.')&&!u.includes(':'))){u='https://www.google.com/search?q='+encodeURIComponent(u);}else{u=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(u)?'http://':'https://')+u;}}event.target.value=u;window.__libroWvNavigate('%s',u);__ws.call('app.url.set',{"sid":"%s","id":"%s","url":u});event.target.blur();var wv=document.querySelector('[data-webview-app="%s"]');if(wv)wv.focus();}`, app.ID, sid, app.ID, app.ID)))

		// Globe icon in badge
		globeBadgeCls := "inline-flex items-center justify-center w-6 h-6 rounded shrink-0"
		globeIconCls := "material-icons-round text-sm leading-none"
		if selected {
			globeBadgeCls += " bg-white"
			globeIconCls += " text-black"
		} else {
			globeBadgeCls += " bg-gray-800 dark:bg-zinc-900"
			globeIconCls += " text-white"
		}
		globe := r.Div(globeBadgeCls).Render(r.I(globeIconCls).Text("language"))

		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1").
			Render(backBtn, forwardBtn, globe, urlInput, copyBtn, reloadBtn)
	} else if app.Type == AppTypeTerminal && app.Command == "" {
		// Pending terminal — show command input
		cmdInputID := fmt.Sprintf("cmdinput-%s", app.ID)
		termBadgeCls := "inline-flex items-center justify-center w-6 h-6 rounded shrink-0"
		termIconCls := "material-icons-round text-sm leading-none"
		if selected {
			termBadgeCls += " bg-white"
			termIconCls += " text-black"
		} else {
			termBadgeCls += " bg-gray-800 dark:bg-zinc-900"
			termIconCls += " text-white"
		}
		termBadge := r.Div(termBadgeCls).Render(r.I(termIconCls).Text("terminal"))

		cmdInputCls := "flex-1 min-w-0 rounded-sm text-[11px] font-mono outline-none px-2 h-6"
		if selected {
			cmdInputCls += " bg-white/15 text-white placeholder-blue-200/50"
		} else {
			cmdInputCls += " bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 placeholder-gray-400 dark:placeholder-zinc-600"
		}
		cmdInput := r.Input(cmdInputCls).
			ID(cmdInputID).
			Attr("type", "text").
			Attr("placeholder", "Enter command...").
			Attr("spellcheck", "false").
			Attr("autocomplete", "off").
			On("keydown", r.JS(fmt.Sprintf(`if(event.key==='Enter'){event.preventDefault();var cmd=event.target.value.trim();if(cmd){__ws.call('app.run.start',{"sid":"%s","id":"%s","command":cmd});}}`, sid, app.ID)))

		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1").
			Render(termBadge, cmdInput)
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
				matIconCls := "material-icons-round text-sm shrink-0"
				if selected {
					matIconCls += " text-black"
				} else {
					matIconCls += " text-white"
				}
				iconNode = r.I(matIconCls).Text(info.MaterialIcon)
			}
		} else if app.IconURL != "" {
			iconNode = r.Div("shrink-0 w-4 h-4 flex items-center justify-center").Render(
				r.Div("").Attr("style", fmt.Sprintf(
					"width:16px;height:16px;background:url('%s') center/contain no-repeat",
					app.IconURL,
				)),
			)
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

		termLabelCls := "text-[10px] font-mono tracking-wider uppercase truncate"
		badgeCls := "inline-flex items-center gap-1 px-1.5 py-0.5 rounded"
		if selected {
			termLabelCls += " text-black"
			badgeCls += " bg-white"
		} else {
			termLabelCls += " text-white"
			badgeCls += " bg-gray-800 dark:bg-zinc-900"
		}
		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1.5").Render(
			r.Div(badgeCls).Render(
				iconNode,
				r.Span(termLabelCls).Text(labelText),
			),
		)
	}

	var toolbar *r.Node
	var clickOverlay *r.Node
	if !selected {
		clickOverlay = r.Div("absolute inset-0 z-40 cursor-pointer").
			Attr("data-click-overlay", "").
			On("mousedown", &r.Action{
				Name: "app.select",
				Data: sidData(sid, "index", index),
			})
	}

	// Toolbar: always visible, sits above the iframe
	toolbarCls := "flex items-center gap-2 px-1.5 py-1 border-b shrink-0"
	if selected {
		toolbarCls += " bg-blue-600 border-blue-700"
	} else {
		toolbarCls += " bg-white dark:bg-zinc-900 border-gray-200 dark:border-zinc-700/50"
	}
	toolbar = r.Div(toolbarCls)
	if !selected {
		toolbar = toolbar.OnClick(&r.Action{
			Name: "app.select",
			Data: sidData(sid, "index", index),
		})
	}
	toolbar = toolbar.Attr("data-app-toolbar", "").Render(leftSide, rightButtons)
	if zen {
		toolbar = toolbar.Attr("style", "display:none")
	}

	return r.Div("group relative flex flex-col "+app.Width.ContainerClasses()+" h-full "+borderClass+" rounded-md overflow-hidden bg-white dark:bg-zinc-950 transition-all duration-50").
		ID(fmt.Sprintf("frame-%s", app.ID)).
		Attr("data-app-id", app.ID).
		Attr("style", fmt.Sprintf("order:%d", index)).
		Render(
			toolbar,
			r.Div("relative flex-1 min-h-0").
				Attr("data-app-content", app.ID).
				Render(
					renderIframe(app, frameID, iframeSrc, sid),
					clickOverlay,
				),
		)
}

func renderIframe(app Application, frameID, iframeSrc, sid string) *r.Node {
	if app.Type == AppTypeURL {
		// Electron webview: native rendering, no screencast needed.
		// z-30 to sit above the click overlay (z-20)
		webviewSrc := app.URL
		if webviewSrc == "" {
			webviewSrc = "about:blank"
		}
		wv := r.El("webview", "").
			ID(frameID).
			Attr("data-webview-app", app.ID).
			Attr("data-sid", sid).
			Attr("src", webviewSrc).
			Attr("partition", "persist:libro").
			Attr("allowpopups", "").
			Attr("style", "display:inline-flex;width:100%;height:100%")
		// Force closing tag by adding empty text content
		wv.Text("")
		devToolsBtn := r.Button("absolute bottom-3 right-3 z-40 flex items-center gap-2 h-7 px-2.5 rounded-md bg-stone-100 dark:bg-stone-800 backdrop-blur-md shadow-sm border border-stone-300 dark:border-stone-600 hover:bg-stone-200 dark:hover:bg-stone-700 transition-colors cursor-pointer opacity-0 group-hover:opacity-100").
			ID(fmt.Sprintf("devtools-wrap-%s", app.ID)).
			Attr("title", "Open DevTools").
			Attr("onclick", fmt.Sprintf("var wv=window.__libroWebviews['%s'];if(wv){if(wv.isDevToolsOpened()){wv.closeDevTools();}else{wv.openDevTools();}}", app.ID)).
			Render(
				r.I("material-icons-round text-[14px] text-stone-600 dark:text-stone-300").Text("developer_mode"),
				r.Span("hidden text-[11px] font-semibold tabular-nums text-red-500").
					ID(fmt.Sprintf("devtools-errors-%s", app.ID)).
					Text("0"),
			)

		container := r.Div("w-full h-full absolute inset-0 z-30").Render(wv, devToolsBtn)
		if app.URL == "" {
			container.Render(
				r.Div("absolute inset-0 flex items-center justify-center text-gray-400 dark:text-zinc-600 font-mono text-xs z-10 pointer-events-none").
					Attr("data-webview-loading", "").
					Text("Enter a URL above"),
			)
		}
		return container
	}
	// Pending terminal — show placeholder
	if app.Type == AppTypeTerminal && app.Command == "" {
		return r.Div("w-full h-full flex items-center justify-center bg-gray-50 dark:bg-zinc-950").Render(
			r.Div("flex flex-col items-center gap-2 text-gray-400 dark:text-zinc-600").Render(
				r.I("material-icons-round text-3xl").Text("terminal"),
				r.Span("font-mono text-xs").Text("Enter a command above"),
			),
		)
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
func insertAppJS(node *r.Node, _ bool, projectName string) string {
	// CSS order on the app frame (set in renderAppFrame) handles visual positioning,
	// so we just append to the strip — no DOM repositioning needed.
	return node.ToJSAppend(stripID(projectName))
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
	if(el){el.style.display='flex';el.style.animation='none';el.offsetHeight;el.style.animation='libro-project-switch .06s ease-out';}
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
}, 30);`, selectedIndex)
}

// removeAppJS returns JS that removes an app frame by its app ID from the strip.
// For URL apps with webviews, the webview element is moved to a hidden pool
// so its session state (cookies, WebSocket connections) stays alive.
func removeAppJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(!el)return;
	var wv=el.querySelector('webview[data-webview-app]');
	var pool=document.getElementById('webview-pool');
	if(wv&&pool){
		var origin='';
		try{var u=new URL(wv.src||wv.getAttribute('src'));origin=u.origin;}catch(e){}
		if(origin&&origin!=='null'){
			wv.setAttribute('data-pool-origin',origin);
			wv.style.display='none';
			pool.appendChild(wv);
		}
	}
	el.remove();
})();`, appID)
}

// poolWebviewJS returns JS that moves a specific app's webview to the hidden pool
// before a full DOM replace would destroy it. Used when closing the last app.
func poolWebviewJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(!el)return;
	var wv=el.querySelector('webview[data-webview-app]');
	var pool=document.getElementById('webview-pool');
	if(wv&&pool){
		var origin='';
		try{var u=new URL(wv.src||wv.getAttribute('src'));origin=u.origin;}catch(e){}
		if(origin&&origin!=='null'){
			wv.setAttribute('data-pool-origin',origin);
			wv.style.display='none';
			pool.appendChild(wv);
		}
	}
})();`, appID)
}

// showManageOverlayJS returns JS that removes any existing overlay, then appends the new one to body.
func showManageOverlayJS(node *r.Node) string {
	return removeManageOverlayJS() + node.ToJS()
}

// removeManageOverlayJS returns JS that removes the manage overlay.
func removeManageOverlayJS() string {
	return fmt.Sprintf(`(function(){var el=document.getElementById('%s');if(el)el.remove();})();`, ManageDialogID)
}

// renderSideLauncher renders a vertical icon dock: saved app icons + "+" button.
// Now server-rendered from DB instead of client-side localStorage.
func renderSideLauncher(sid, side, activeProject string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(activeProject)

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
			} else if app.IconURL != "" {
				iconNode = r.Img("w-8 h-8 rounded-sm").Attr("src", app.IconURL)
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
						h := strings.TrimPrefix(u.Hostname(), "www.")
						label = h
					}
				}
			}
		}

		btn := r.Button(btnCls).
			Render(
				iconNode,
				r.Span(tipCls).Text(label),
			).
			OnClick(&r.Action{Name: "app.start", Data: map[string]any{
				"sid": sid, "type": app.Type, "url": app.URL,
				"command": app.Command, "width": app.Width,
				"writable": app.Writable, "name": app.Name, "side": side,
				"iconUrl": app.IconURL,
			}})
		children = append(children, btn)
	}

	// Quick launch button (combined browse + run)
	launchBtn := r.Button(btnCls).
		Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-indigo-600 dark:hover:text-indigo-400 text-2xl").Text("search"),
			r.Span(tipCls).Text("Quick launch"),
		).
		OnClick(&r.Action{Name: "app.run.new", Data: sidData(sid, "side", side)})
	children = append(children, launchBtn)

	// Add button
	addBtn := r.Button(btnCls).
		Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-blue-600 dark:hover:text-blue-400 text-[18px]").Text("add"),
			r.Span(tipCls).Text("Add app"),
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
		radio := r.IRadio("accent-blue-500 cursor-pointer").
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
			document.getElementById('tab-url-btn').classList.toggle('border-blue-500', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-blue-600', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('border-transparent', '%s' !== 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-gray-500', '%s' !== 'url');
			document.getElementById('tab-terminal-btn').classList.toggle('border-blue-500', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-blue-600', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('border-transparent', '%s' !== 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-gray-500', '%s' !== 'terminal');
			document.getElementById('app-type').value = '%s';
		`, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab)
	}

	collectIDs := []string{"app-url", "app-command", "app-writable", "app-type", "app-name", "width-md", "app-project-specific"}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 outline-none transition-colors"

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75" + hiddenClass).
		ID(DialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", DialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-md mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("Add Application"),

					r.IHidden("").ID("app-type").Attr("value", "terminal"),

					r.Div("flex border-b border-gray-200 dark:border-zinc-700/50 mb-4").Render(
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-blue-500 text-blue-600 cursor-pointer transition-colors").
							ID("tab-terminal-btn").
							Text("Terminal").
							OnClick(r.JS(tabSwitchJS("terminal"))),
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 cursor-pointer transition-colors").
							ID("tab-url-btn").
							Text("URL").
							OnClick(r.JS(tabSwitchJS("url"))),
					),

					r.Div("mb-5 hidden").ID("tab-url-content").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("URL"),
						r.IUrl(inputCls).
							ID("app-url").
							Attr("placeholder", "https://example.com").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
						r.P("text-xs text-gray-400 dark:text-zinc-500 mt-1").Text("Use __dir__ as a placeholder for the project directory."),
					),

					r.Div("mb-5").ID("tab-terminal-content").Render(
						r.Div("mb-3").Render(
							r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Command"),
							r.IText(inputCls+" font-mono").
								ID("app-command").
								Attr("placeholder", "bash").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
							r.P("text-xs text-gray-400 dark:text-zinc-500 mt-1").Text("Use __dir__ as a placeholder for the project directory."),
						),
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-blue-500 cursor-pointer w-4 h-4").
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

					r.Div("mb-5").Render(
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-blue-500 cursor-pointer w-4 h-4").
								ID("app-project-specific"),
							r.Span("text-sm text-gray-600 dark:text-zinc-400").Text("Project specific"),
						),
					),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "app.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-add").
							Text("Save").
							OnClick(&r.Action{
								Name:    "app.save",
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
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-between text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Div("flex items-center gap-4").Render(
							r.Span("").Text("↑↓ navigate"),
							r.Span("").Text("Enter open"),
							r.Span("").Text("Esc close"),
						),
						r.Span("cursor-pointer hover:text-red-400 transition-colors").
							Attr("onclick", "event.stopPropagation();__ws.call('history.clear',{sid:'"+sid+"'});__ws.call('run.history.clear',{sid:'"+sid+"'});").
							Text("clear history"),
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

	function getBrowsedURLs(){
		return (window.__libroBrowsedURLs||[]).map(function(u){
			var host='';
			try{host=new URL(u).hostname.replace(/^www\./,'');}catch(e){}
			return {app:{type:'url',url:u,name:host||u,width:'lg',writable:true},isHistory:true};
		});
	}

	function getRunHistory(){
		return (window.__libroRunCommands||[]).map(function(c){
			return {app:{type:'terminal',command:c,name:c,width:'lg',writable:true},isRunHistory:true};
		});
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
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'));
			var iconHtml='';
			var label='';
			var sub='';
			var app=item.app;
			if(item.isSearch){
				iconHtml='<i class="material-icons-round text-blue-500 text-lg shrink-0">search</i>';
				label=app.name;
				sub=app.url;
			}else if(item.isRun){
				iconHtml='<i class="material-icons-round text-emerald-500 text-lg shrink-0">terminal</i>';
				label=app.name;
				sub='';
			}else if(item.isRunHistory){
				iconHtml='<i class="material-icons-round text-gray-400 dark:text-zinc-500 text-lg shrink-0">terminal</i>';
				label=app.command;
				sub='';
			}else if(item.isBrowse){
				iconHtml='<i class="material-icons-round text-blue-500 text-lg shrink-0">public</i>';
				label=app.name||app.url;
				sub=app.url;
			}else if(item.isHistory){
				try{
					var u=new URL(app.url);
					iconHtml='<img class="w-6 h-6 rounded-sm shrink-0" src="https://www.google.com/s2/favicons?domain='+encodeURIComponent(u.hostname)+'&sz=32" onerror="this.outerHTML=\'<i class=\\\'material-icons-round text-gray-400 text-lg\\\'>history</i>\'">';
					label=u.hostname.replace(/^www\./,'');
					sub=app.url;
				}catch(e){
					iconHtml='<i class="material-icons-round text-gray-400 text-lg shrink-0">history</i>';
					label=app.url;
					sub=app.url;
				}
			}else if(app.type==='terminal'){
				iconHtml=window.__libroTermIcon?window.__libroTermIcon(app.command,24,app.iconUrl):'';
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
			var typeBadge=item.isSearch?'search':item.isRun?'run':item.isRunHistory?'history':item.isBrowse?'browser':item.isHistory?'history':app.type;
			var deleteBtn='';
			if(item.isHistory){
				deleteBtn='<i class="material-icons-round text-sm shrink-0 cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity '+(dk?'text-zinc-500 hover:text-red-400':'text-gray-400 hover:text-red-500')+'" data-delete-url="'+app.url.replace(/"/g,'&quot;')+'">close</i>';
			}
			if(item.isRunHistory){
				deleteBtn='<i class="material-icons-round text-sm shrink-0 cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity '+(dk?'text-zinc-500 hover:text-red-400':'text-gray-400 hover:text-red-500')+'" data-delete-cmd="'+app.command.replace(/"/g,'&quot;')+'">close</i>';
			}
			row.className+=' group';
			row.innerHTML=iconHtml
				+'<div class="flex-1 min-w-0"><div class="text-sm truncate '+txtCls+'">'+label+'</div>'
				+(sub!==label?'<div class="text-[11px] truncate '+subCls+'">'+sub+'</div>':'')
				+'</div>'
				+deleteBtn
				+'<span class="px-1.5 py-0.5 text-[10px] font-mono uppercase rounded shrink-0 '+badgeCls+'">'+(app.width||'lg')+'</span>'
				+'<span class="px-1.5 py-0.5 text-[10px] font-mono uppercase rounded shrink-0 '+badgeCls+'">'+typeBadge+'</span>';
			if(item.isHistory){
				var delEl=row.querySelector('[data-delete-url]');
				if(delEl){
					delEl.onclick=function(e){
						e.stopPropagation();
						__ws.call('history.delete',{sid:'%s',url:app.url});
					};
				}
			}
			if(item.isRunHistory){
				var delCmd=row.querySelector('[data-delete-cmd]');
				if(delCmd){
					delCmd.onclick=function(e){
						e.stopPropagation();
						__ws.call('run.history.delete',{sid:'%s',command:app.command});
					};
				}
			}
			row.onmouseenter=function(){
				if(selIdx===i)return;
				var prev=res.children[selIdx];
				if(prev)prev.className=prev.className.replace(/bg-blue-900\/30|bg-blue-50/g,'').replace(/border-blue-500/g,'border-transparent')+(dk?' hover:bg-zinc-800':' hover:bg-gray-50');
				selIdx=i;
				row.className=row.className.replace(/hover:bg-zinc-800|hover:bg-gray-50/g,'').replace(/border-transparent/g,'border-blue-500')+(dk?' bg-blue-900/30':' bg-blue-50');
			};
			row.onclick=function(){launch();};
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
		var history=getBrowsedURLs();
		var runHistory=getRunHistory();
		// Build a set of saved app URLs to avoid duplicates from history
		var savedURLs={};
		apps.forEach(function(a){if(a.url)savedURLs[a.url]=true;});
		var uniqueHistory=history.filter(function(h){return !savedURLs[h.app.url];});
		// Build set of saved terminal commands to avoid duplicates from run history
		var savedCmds={};
		apps.forEach(function(a){if(a.command)savedCmds[a.command]=true;});
		var uniqueRunHistory=runHistory.filter(function(h){return !savedCmds[h.app.command];});
		if(!q){
			filtered=apps.map(function(a){return{app:a,score:1};});
			uniqueHistory.forEach(function(h){filtered.push({app:h.app,score:0.5,isHistory:true});});
			uniqueRunHistory.forEach(function(h){filtered.push({app:h.app,score:0.4,isRunHistory:true});});
			filtered.push(browserEntry);
		}else if(isURL(q)){
			// URL typed — show "Browse <url>" at top, then matching saved apps + history
			var browseURL=q;
			if(!/^https?:\/\//i.test(browseURL))browseURL=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(browseURL)?'http://':'https://')+browseURL;
			filtered=[{app:{type:'url',url:browseURL,name:'Browse: '+q,width:'lg'},score:99999,isBrowse:true}];
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,q);
				if(score>0)filtered.push({app:a,score:score});
			});
			uniqueHistory.forEach(function(h){
				var score=fuzzyMatch(h.app.url+' '+h.app.name,q);
				if(score>0)filtered.push({app:h.app,score:score,isHistory:true});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
		}else if(q.charAt(0)===':'){
			// Colon prefix — internet search
			var searchQ=q.substring(1).trim();
			filtered=[];
			if(searchQ){
				filtered.push({app:{type:'url',url:'https://www.google.com/search?q='+encodeURIComponent(searchQ),name:'Search: '+searchQ,width:'lg'},score:99999,isSearch:true});
			}
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,searchQ||q);
				if(score>0)filtered.push({app:a,score:score});
			});
			uniqueHistory.forEach(function(h){
				var score=fuzzyMatch(h.app.url+' '+h.app.name,searchQ||q);
				if(score>0)filtered.push({app:h.app,score:score,isHistory:true});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
			// Keep search at top
			var sIdx=filtered.findIndex(function(f){return f.isSearch;});
			if(sIdx>0){var s=filtered.splice(sIdx,1)[0];filtered.unshift(s);}
		}else if(q.charAt(0)==='!'){
			// Exclamation prefix — run terminal command
			var runQ=q.substring(1).trim();
			filtered=[];
			if(runQ){
				filtered.push({app:{type:'terminal',command:runQ,name:'Run: '+runQ,width:'lg',writable:true},score:99999,isRun:true});
			}
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,runQ||q);
				if(score>0)filtered.push({app:a,score:score});
			});
			uniqueRunHistory.forEach(function(h){
				var score=fuzzyMatch(h.app.command,runQ||q);
				if(score>0)filtered.push({app:h.app,score:score,isRunHistory:true});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
			// Keep "Run:" at top
			var runIdx=filtered.findIndex(function(f){return f.isRun;});
			if(runIdx>0){var r=filtered.splice(runIdx,1)[0];filtered.unshift(r);}
		}else{
			// Plain text — search saved apps and history only
			filtered=[];
			apps.forEach(function(a){
				var text=(a.name||'')+' '+(a.command||'')+' '+(a.url||'')+' '+a.type;
				var score=fuzzyMatch(text,q);
				if(score>0)filtered.push({app:a,score:score});
			});
			uniqueHistory.forEach(function(h){
				var score=fuzzyMatch(h.app.url+' '+h.app.name,q);
				if(score>0)filtered.push({app:h.app,score:score,isHistory:true});
			});
			uniqueRunHistory.forEach(function(h){
				var score=fuzzyMatch(h.app.command,q);
				if(score>0)filtered.push({app:h.app,score:score,isRunHistory:true});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
			// Add browser if "browser" fuzzy matches query
			if(fuzzyMatch('browser',q)>0)filtered.push(browserEntry);
		}
		selIdx=0;
		render();
	}

	var pendingSide='right';

	function launch(){
		if(filtered.length===0)return;
		var side=pendingSide;
		var item=filtered[selIdx];
		var app=item.app;
		dlg.classList.add('hidden');
		inp.value='';
		// Internet search — open browser with search URL
		if(item.isSearch){
			__ws.call('app.start',{sid:'%s',type:'url',url:app.url,command:'',width:app.width||'lg',writable:true,name:'',iconUrl:'',side:side});
			return;
		}
		// Run command — execute terminal directly
		if(item.isRun||item.isRunHistory){
			__ws.call('app.run.execute',{sid:'%s',command:app.command,side:side});
			return;
		}
		// Empty browser — use app.browse.new to open blank tab with URL bar focused
		if(item.isBrowse&&!app.url){
			__ws.call('app.browse.new',{sid:'%s',side:side});
			return;
		}
		// History items — open directly (URL is already complete)
		if(item.isHistory){
			__ws.call('app.start',{sid:'%s',type:'url',url:app.url,command:'',width:app.width||'lg',writable:true,name:'',iconUrl:'',side:side});
			return;
		}
		__ws.call('app.start',{sid:'%s',type:app.type,url:app.url||'',command:app.command||'',width:app.width||'lg',writable:app.writable!==false,name:app.name||'',iconUrl:app.iconUrl||'',side:side});
	}

	function openSearch(side){
		pendingSide=side||'right';
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
			launch();
		}else if(e.key==='Escape'){
			e.preventDefault();
			closeSearch();
		}
	});

	window.__libroOpenSearch=openSearch;
})();
`, SearchDialogID, sid, sid, sid, sid, sid, sid, sid)
}

// renderURLPopup renders the URL/search popup for Ctrl+L (works in both zen and non-zen mode).
func renderURLPopup(sid string) *r.Node {
	return r.Div("absolute inset-0 z-[60] flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(URLPopupID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", URLPopupID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 flex items-center gap-2").Render(
						r.I("material-icons-round text-blue-500 text-lg").Text("language"),
						r.Input("flex-1 bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("url-popup-input").
							Attr("type", "text").
							Attr("placeholder", "Enter URL or search...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
					),
					r.Div("max-h-[40vh] overflow-y-auto").ID("url-popup-history"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Enter navigate"),
						r.Span("").Text("↑↓ select"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}

// urlPopupJS returns JS that powers the Ctrl+L URL popup.
func urlPopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var currentAppId='';
	var selectedIdx=-1;
	var filteredURLs=[];

	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('url-popup-input');}
	function getHistory(){return document.getElementById('url-popup-history');}

	function findSelectedBrowserApp(){
		var selToolbar=document.querySelector('.bg-blue-600.border-blue-700');
		var appEl=selToolbar?selToolbar.closest('[data-app-id]'):null;
		if(!appEl){
			var strips=document.querySelectorAll('[id^="app-strip-"]');
			for(var s=0;s<strips.length;s++){
				var parent=strips[s].closest('[id^="project-main-"]');
				if(parent&&parent.style.display!=='none'){
					var sorted=window.__libroSortedApps?window.__libroSortedApps(strips[s]):Array.from(strips[s].querySelectorAll(':scope > [data-app-id]'));
					for(var i=0;i<sorted.length;i++){
						if(!sorted[i].querySelector('[data-click-overlay]')){
							appEl=sorted[i];break;
						}
					}
					break;
				}
			}
		}
		if(appEl&&appEl.querySelector('webview[data-webview-app]')){
			return appEl.getAttribute('data-app-id');
		}
		return '';
	}

	function renderHistory(query){
		var historyContainer=getHistory();
		var inp=getInp();
		if(!historyContainer)return;
		var urls=window.__libroBrowsedURLs||[];
		var q=(query||'').trim().toLowerCase();
		if(q){
			filteredURLs=urls.filter(function(u){return u.toLowerCase().indexOf(q)!==-1;});
		}else{
			filteredURLs=urls.slice(0,20);
		}
		if(filteredURLs.length===0){
			historyContainer.innerHTML='';
			selectedIdx=-1;
			return;
		}
		var dk=document.documentElement.classList.contains('dark');
		var html='<div class="border-t border-gray-100 dark:border-zinc-800 py-1">';
		for(var i=0;i<filteredURLs.length&&i<20;i++){
			var u=filteredURLs[i];
			var host='';try{host=new URL(u).hostname.replace(/^www\./,'');}catch(e){host=u;}
			var sel=i===selectedIdx;
			var bg=sel?(dk?'background:#2563eb22':'background:#2563eb15'):'';
			var border=sel?'border-left:2px solid #3b82f6':'border-left:2px solid transparent';
			html+='<div class="px-4 py-1.5 flex items-center gap-2 cursor-pointer text-sm hover:bg-gray-50 dark:hover:bg-zinc-800/50" style="'+bg+';'+border+'" data-url-idx="'+i+'">';
			html+='<span class="material-icons-round text-gray-400 dark:text-zinc-500" style="font-size:14px">history</span>';
			html+='<span class="text-gray-500 dark:text-zinc-400 truncate flex-1 font-mono text-xs">'+u.replace(/</g,'&lt;')+'</span>';
			html+='<span class="text-gray-400 dark:text-zinc-600 text-[10px] shrink-0">'+host.replace(/</g,'&lt;')+'</span>';
			html+='</div>';
		}
		html+='</div>';
		historyContainer.innerHTML=html;
		historyContainer.querySelectorAll('[data-url-idx]').forEach(function(el){
			el.addEventListener('mousedown',function(e){
				e.preventDefault();
				var idx=parseInt(el.getAttribute('data-url-idx'));
				if(filteredURLs[idx]&&inp){
					inp.value=filteredURLs[idx];
					navigate();
				}
			});
		});
	}

	function openPopup(){
		var appId=findSelectedBrowserApp();
		if(!appId)return;
		currentAppId=appId;
		var dlg=getDlg();
		if(!dlg)return;
		var contentArea=document.querySelector('[data-app-content="'+appId+'"]');
		if(contentArea)contentArea.appendChild(dlg);
		var inp=getInp();
		var wv=window.__libroWebviews[appId];
		var currentUrl='';
		if(wv){try{currentUrl=wv.getURL()||'';}catch(e){}}
		if(!currentUrl){
			var urlInp=document.getElementById('urlinput-'+appId);
			if(urlInp)currentUrl=urlInp.value||'';
		}
		dlg.classList.remove('hidden');
		if(inp){
			inp.value=currentUrl;
		}
		selectedIdx=-1;
		renderHistory('');
		setTimeout(function(){var i=getInp();if(i){i.focus();i.select();}},50);
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		var historyContainer=getHistory();
		if(dlg)dlg.classList.add('hidden');
		if(inp)inp.value='';
		currentAppId='';
		selectedIdx=-1;
		if(historyContainer)historyContainer.innerHTML='';
	}

	function navigate(){
		var inp=getInp();
		var u=inp?inp.value.trim():'';
		if(!u||!currentAppId)return;
		if(!u.startsWith('http://')&&!u.startsWith('https://')){
			if(/\s/.test(u)||(!u.includes('.')&&!u.includes(':'))){
				u='https://www.google.com/search?q='+encodeURIComponent(u);
			}else{
				u=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(u)?'http://':'https://')+u;
			}
		}
		var appId=currentAppId;
		closePopup();
		window.__libroWvNavigate(appId,u);
		var urlInp=document.getElementById('urlinput-'+appId);
		if(urlInp)urlInp.value=u;
		__ws.callSilent('app.url.set',{sid:'%s',id:appId,url:u});
		setTimeout(function(){
			var wv=window.__libroWebviews[appId];
			if(wv)wv.focus();
		},100);
	}

	document.addEventListener('input',function(e){
		var inp=getInp();
		if(e.target!==inp)return;
		selectedIdx=-1;
		renderHistory(inp.value);
	});

	document.addEventListener('keydown',function(e){
		var inp=getInp();
		if(e.target!==inp)return;
		var dlg=getDlg();
		if(!dlg||dlg.classList.contains('hidden'))return;
		e.stopImmediatePropagation();
		if(e.key==='ArrowDown'){
			e.preventDefault();
			if(filteredURLs.length>0){
				selectedIdx=Math.min(selectedIdx+1,filteredURLs.length-1);
				renderHistory(inp.value);
				inp.value=filteredURLs[selectedIdx];
			}
		}else if(e.key==='ArrowUp'){
			e.preventDefault();
			if(filteredURLs.length>0&&selectedIdx>0){
				selectedIdx--;
				renderHistory(inp.value);
				inp.value=filteredURLs[selectedIdx];
			}else if(selectedIdx===0){
				selectedIdx=-1;
				renderHistory(inp.value);
			}
		}else if(e.key==='Enter'){
			e.preventDefault();
			navigate();
		}else if(e.key==='Escape'){
			e.preventDefault();
			closePopup();
		}
	});

	window.__libroOpenURLPopup=openPopup;
})();
`, URLPopupID, sid)
}

// renderResizePopup renders the resize popup for Win+R / Win+F (works in both zen and non-zen mode).
// Uses radio-style buttons navigable with j/k and confirmable with Enter.
func renderResizePopup(sid string) *r.Node {
	widths := AllWidths()
	buttons := make([]*r.Node, 0, len(widths))
	for _, w := range widths {
		label := strings.ToUpper(string(w))
		buttons = append(buttons,
			r.Div("resize-btn flex items-center gap-3 px-4 py-2 rounded cursor-pointer transition-colors duration-75 text-gray-600 dark:text-zinc-400 hover:bg-gray-100 dark:hover:bg-zinc-800").
				Attr("data-resize-width", string(w)).
				Render(
					r.Div("w-4 h-4 rounded-full border-2 border-gray-300 dark:border-zinc-600 flex items-center justify-center shrink-0").
						Attr("data-radio", "").
						Render(r.Div("w-2 h-2 rounded-full bg-blue-600 hidden").Attr("data-radio-dot", "")),
					r.Span("text-sm font-mono tracking-wider uppercase").Text(label),
				),
		)
	}

	return r.Div("absolute inset-0 z-[60] flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden outline-none").
		ID(ResizePopupID).
		Attr("tabindex", "-1").
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", ResizePopupID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-xs mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-2").Render(
						r.I("material-icons-round text-blue-500 text-lg").Text("aspect_ratio"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200").Text("Resize App"),
					),
					r.Div("px-3 py-2 flex flex-col gap-0").
						ID("resize-popup-buttons").
						Render(buttons...),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("j/k navigate"),
						r.Span("").Text("Enter resize"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}

// resizePopupJS returns JS that powers the Win+R / Win+F resize popup.
// Supports j/k keyboard navigation and Enter to confirm.
func resizePopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var currentAppId='';
	var focusedIndex=-1;

	function getDlg(){return document.getElementById('%s');}

	function findSelectedApp(){
		var selToolbar=document.querySelector('.bg-blue-600.border-blue-700');
		var appEl=selToolbar?selToolbar.closest('[data-app-id]'):null;
		if(!appEl){
			var strips=document.querySelectorAll('[id^="app-strip-"]');
			for(var s=0;s<strips.length;s++){
				var parent=strips[s].closest('[id^="project-main-"]');
				if(parent&&parent.style.display!=='none'){
					var sorted=window.__libroSortedApps?window.__libroSortedApps(strips[s]):Array.from(strips[s].querySelectorAll(':scope > [data-app-id]'));
					for(var i=0;i<sorted.length;i++){
						if(!sorted[i].querySelector('[data-click-overlay]')){
							appEl=sorted[i];break;
						}
					}
					break;
				}
			}
		}
		return appEl?appEl.getAttribute('data-app-id'):'';
	}

	function getCurrentWidth(appId){
		var el=document.querySelector('[data-app-id="'+appId+'"]');
		if(!el)return'lg';
		var cls=el.className;
		if(cls.indexOf('w-full')!==-1)return'full';
		if(cls.indexOf('w-[1920px]')!==-1)return'2xl';
		if(cls.indexOf('w-[1280px]')!==-1)return'xl';
		if(cls.indexOf('w-[960px]')!==-1)return'lg';
		if(cls.indexOf('w-[640px]')!==-1)return'md';
		if(cls.indexOf('w-[480px]')!==-1)return'sm';
		return'lg';
	}

	function getBtns(){var d=getDlg();return d?d.querySelectorAll('.resize-btn'):[];}

	function highlightFocused(idx){
		var btns=getBtns();
		btns.forEach(function(b,i){
			var radio=b.querySelector('[data-radio]');
			var dot=b.querySelector('[data-radio-dot]');
			if(i===idx){
				b.className='resize-btn flex items-center gap-3 px-4 py-2 rounded cursor-pointer transition-colors duration-75 bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300';
				if(radio)radio.className='w-4 h-4 rounded-full border-2 border-blue-600 flex items-center justify-center shrink-0';
				if(dot)dot.classList.remove('hidden');
			}else{
				b.className='resize-btn flex items-center gap-3 px-4 py-2 rounded cursor-pointer transition-colors duration-75 text-gray-600 dark:text-zinc-400 hover:bg-gray-100 dark:hover:bg-zinc-800';
				if(radio)radio.className='w-4 h-4 rounded-full border-2 border-gray-300 dark:border-zinc-600 flex items-center justify-center shrink-0';
				if(dot)dot.classList.add('hidden');
			}
		});
		focusedIndex=idx;
	}

	function openPopup(){
		var appId=findSelectedApp();
		if(!appId)return;
		currentAppId=appId;
		var dlg=getDlg();
		if(!dlg)return;
		var contentArea=document.querySelector('[data-app-content="'+appId+'"]');
		if(contentArea)contentArea.appendChild(dlg);
		var curWidth=getCurrentWidth(appId);
		var btns=getBtns();
		var idx=0;
		btns.forEach(function(b,i){
			if(b.getAttribute('data-resize-width')===curWidth)idx=i;
		});
		highlightFocused(idx);
		dlg.classList.remove('hidden');
		setTimeout(function(){dlg.focus();},50);
	}

	function closePopup(){
		var dlg=getDlg();
		if(dlg)dlg.classList.add('hidden');
		currentAppId='';
		focusedIndex=-1;
	}

	function confirmSelection(){
		var btns=getBtns();
		if(focusedIndex<0||focusedIndex>=btns.length||!currentAppId)return;
		var w=btns[focusedIndex].getAttribute('data-resize-width');
		if(!w)return;
		__ws.callSilent('app.resize',{sid:'%s',id:currentAppId,width:w});
		closePopup();
	}

	document.addEventListener('click',function(e){
		var btn=e.target.closest('.resize-btn');
		var dlg=getDlg();
		if(!btn||!dlg||!dlg.contains(btn))return;
		e.stopPropagation();
		var btns=getBtns();
		for(var i=0;i<btns.length;i++){
			if(btns[i]===btn){highlightFocused(i);break;}
		}
		confirmSelection();
	});

	document.addEventListener('keydown',function(e){
		var dlg=getDlg();
		if(!dlg||dlg.classList.contains('hidden'))return;
		var btns=getBtns();
		if(e.key==='j'||e.key==='ArrowDown'){
			e.preventDefault();e.stopImmediatePropagation();
			var next=focusedIndex+1;
			if(next>=btns.length)next=0;
			highlightFocused(next);
			return;
		}
		if(e.key==='k'||e.key==='ArrowUp'){
			e.preventDefault();e.stopImmediatePropagation();
			var prev=focusedIndex-1;
			if(prev<0)prev=btns.length-1;
			highlightFocused(prev);
			return;
		}
		if(e.key==='Enter'){
			e.preventDefault();e.stopImmediatePropagation();
			confirmSelection();
			return;
		}
		if(e.key==='Escape'){
			e.preventDefault();e.stopImmediatePropagation();
			closePopup();
			return;
		}
	});

	window.__libroOpenResizePopup=openPopup;
})();
`, ResizePopupID, sid)
}

// renderShortcutsDialog renders the keyboard shortcuts popup (hidden by default).
func renderShortcutsDialog() *r.Node {
	type shortcut struct {
		keys string
		desc string
	}
	type section struct {
		title     string
		subtitle  string
		shortcuts []shortcut
	}
	sections := []section{
		{"Apps", "", []shortcut{
			{"⌘ + N", "New app (right of current)"},
			{"⌘ + Ctrl + N", "New app (left of current)"},
			{"⌘ + W", "Close current app"},
			{"⌘ + R", "Resize app popup"},
			{"⌘ + F", "Toggle full width"},
			{"⌘ + +", "Zoom in (whole app)"},
			{"⌘ + -", "Zoom out (whole app)"},
		}},
		{"Navigation", "", []shortcut{
			{"⌘ + H", "Navigate left"},
			{"⌘ + L", "Navigate right"},
			{"⌘ + Ctrl + H", "Move app left"},
			{"⌘ + Ctrl + L", "Move app right"},
			{"⌘ + B", "Toggle sidebar"},
			{"Ctrl + 1–9", "Switch to assigned project"},
			{"Ctrl + 0", "Switch to previous project"},
			{"⌘ + G", "Git worktrees popup"},
			{"⌘ + Z", "Toggle zen mode (hide UI)"},
			{"⌘ + Q", "Quit Libro"},
		}},
		{"Search", "⌘ + N or ⌘ + Ctrl + N to open", []shortcut{
			{": query", "Search the internet"},
			{"! command", "Run terminal command"},
		}},
		{"Browser", "", []shortcut{
			{"Ctrl + L", "URL / search popup"},
			{"Ctrl + R", "Reload browser page"},
		}},
		{"Browser", "Vim keys — disabled in input fields", []shortcut{
			{"g / G", "Go to top / bottom of page"},
			{"j / k", "Scroll down / up"},
			{"h / l", "Scroll left / right"},
			{"/", "Find in page"},
			{"n / p", "Find next / previous"},
			{"Esc", "Clear search / blur input"},
			{"b / f", "Page back / forward"},
			{"Enter", "Follow link / click button"},
		}},
	}

	rows := make([]*r.Node, 0)
	for i, sec := range sections {
		mt := "mt-10"
		if i == 0 {
			mt = "mt-0"
		}
		sectionRows := make([]*r.Node, 0, len(sec.shortcuts))
		for _, s := range sec.shortcuts {
			sectionRows = append(sectionRows,
				r.Div("flex items-center justify-between py-2 px-1").Render(
					r.Span("text-sm text-gray-700 dark:text-zinc-300").Text(s.desc),
					r.Span("text-xs font-mono px-2 py-0.5 rounded bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400").Text(s.keys),
				),
			)
		}
		header := []*r.Node{
			r.Div("px-1 pb-1 text-lg font-semibold uppercase tracking-wider text-gray-500 dark:text-zinc-400").Text(sec.title),
		}
		if sec.subtitle != "" {
			header = append(header,
				r.Div("px-1 pb-1 text-xs text-gray-400 dark:text-zinc-500").Text(sec.subtitle),
			)
		}
		rows = append(rows,
			r.Div(mt).Render(
				append(header, r.Div("").Render(sectionRows...))...,
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
					r.Div("px-4 py-2 max-h-[60vh] overflow-y-auto").Render(rows...),
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

// renderCloseDialog renders the close confirmation dialog (hidden by default).
// It is populated dynamically via JS when the user attempts to close the window.
func renderCloseDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[70] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(CloseDialogID).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-2").Render(
						r.I("material-icons-round text-base text-amber-500").Text("warning"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200").Text("Close Libro?"),
					),
					r.Div("px-4 py-3").Render(
						r.P("text-sm text-gray-600 dark:text-zinc-400 mb-3").Text("The following applications are still running:"),
						r.Div("max-h-60 overflow-y-auto").ID("close-dialog-apps"),
					),
					r.Div("px-4 py-3 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-end gap-2").Render(
						r.Button("px-3 py-1.5 text-sm rounded-md border border-gray-300 dark:border-zinc-600 text-gray-700 dark:text-zinc-300 hover:bg-gray-100 dark:hover:bg-zinc-800 cursor-pointer").
							Text("Cancel").
							Attr("onclick", fmt.Sprintf("document.getElementById('%s').classList.add('hidden');if(window.__electronCloseAbort)window.__electronCloseAbort();", CloseDialogID)),
						r.Button("px-3 py-1.5 text-sm rounded-md bg-red-500 hover:bg-red-600 text-white cursor-pointer").
							ID("close-dialog-confirm").
							Text("Yes, close all").
							Attr("onclick", fmt.Sprintf("__ws.callSilent('app.close.all',{sid:'%s'});document.getElementById('%s').classList.add('hidden');if(window.libroElectron)window.libroElectron.forceClose();else window.close();", sid, CloseDialogID)),
					),
				),
		)
}

// closeDialogJS returns JS to show/hide the close confirmation dialog.
// It populates the app tree dynamically from server data.
func closeDialogJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	window.__libroShowCloseDialog=function(){
		__ws.call('app.close.check',{sid:'%s'});
	};
	document.addEventListener('keydown',function(e){
		var dlg=document.getElementById('%s');
		if(dlg.classList.contains('hidden')) return;
		if(e.key==='Escape'){
			e.preventDefault();e.stopImmediatePropagation();
			dlg.classList.add('hidden');
			if(window.__electronCloseAbort)window.__electronCloseAbort();
		}
	},true);
})();
`, sid, CloseDialogID)
}

// renderWorktreeDialog renders the worktree add/switch popup dialog (hidden by default).
func renderWorktreeDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(WorktreeDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden');", WorktreeDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("alt_route"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").ID("worktree-dialog-title").Text("Worktrees"),
					),
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("worktree-input").
							Attr("type", "text").
							Attr("placeholder", "Search worktrees or type new branch name...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("worktree-results"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("↑↓ navigate"),
						r.Span("").Text("Enter switch/create"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}

// worktreeDialogJS returns JS that powers the worktree popup behavior.
func worktreeDialogJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroWtRegistered)return;
	window.__libroWtRegistered=true;

	var dlg=document.getElementById('%s');
	var inp=document.getElementById('worktree-input');
	var res=document.getElementById('worktree-results');
	var title=document.getElementById('worktree-dialog-title');
	var selIdx=0;
	var filtered=[];
	var currentProject='';

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

	function getWorktrees(){
		return (window.__libroWorktrees||[]).filter(function(wt){
			return !currentProject||wt.project===currentProject;
		});
	}

	function render(){
		var dk=document.documentElement.classList.contains('dark');
		res.innerHTML='';
		if(filtered.length===0){
			var q=inp.value.trim();
			if(q&&currentProject){
				// Show "create new worktree" option
				var row=document.createElement('div');
				row.className='flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
					+(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500');
				row.innerHTML='<i class="material-icons-round text-blue-500 text-lg">add</i>'
					+'<div class="flex-1 min-w-0"><div class="text-sm '+(dk?'text-zinc-200':'text-gray-800')+'">Create worktree: <b>'+q+'</b></div></div>';
				row.onclick=function(){createWorktree(q);};
				res.appendChild(row);
			} else {
				res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No worktrees found</div>';
			}
			return;
		}
		filtered.forEach(function(item,i){
			var row=document.createElement('div');
			var sel=i===selIdx;
			row.className='flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'));
			var txtCls=dk?'text-zinc-200':'text-gray-800';
			var subCls=dk?'text-zinc-500':'text-gray-400';
			row.innerHTML='<i class="material-icons-round '+(dk?'text-zinc-400':'text-gray-400')+' text-lg">alt_route</i>'
				+'<div class="flex-1 min-w-0"><div class="text-sm truncate '+txtCls+'">'+item.branch+'</div>'
				+'<div class="text-[11px] truncate '+subCls+'">'+item.project+' — '+item.path+'</div></div>';
			row.onmouseenter=function(){
				if(selIdx===i)return;
				var prev=res.children[selIdx];
				if(prev)prev.className=prev.className.replace(/bg-blue-900\/30|bg-blue-50/g,'').replace(/border-blue-500/g,'border-transparent')+(dk?' hover:bg-zinc-800':' hover:bg-gray-50');
				selIdx=i;
				row.className=row.className.replace(/hover:bg-zinc-800|hover:bg-gray-50/g,'').replace(/border-transparent/g,'border-blue-500')+(dk?' bg-blue-900/30':' bg-blue-50');
			};
			row.onclick=function(){launch();};
			res.appendChild(row);
		});
		var sel=res.children[selIdx];
		if(sel)sel.scrollIntoView({block:'nearest'});
	}

	function filter(){
		var q=inp.value.trim();
		var wts=getWorktrees();
		if(!q){
			filtered=wts.map(function(w){return{branch:w.branch,project:w.project,path:w.path,score:1};});
		}else{
			filtered=[];
			wts.forEach(function(w){
				var text=w.branch+' '+w.project+' '+w.path;
				var score=fuzzyMatch(text,q);
				if(score>0)filtered.push({branch:w.branch,project:w.project,path:w.path,score:score});
			});
			filtered.sort(function(a,b){return b.score-a.score;});
		}
		selIdx=0;
		render();
	}

	function launch(){
		if(filtered.length===0){
			var q=inp.value.trim();
			if(q&&currentProject){createWorktree(q);}
			return;
		}
		var item=filtered[selIdx];
		dlg.classList.add('hidden');
		inp.value='';
		__ws.call('worktree.switch',{sid:'%s',project:item.project,path:item.path,branch:item.branch});
	}

	function createWorktree(branch){
		if(!currentProject)return;
		dlg.classList.add('hidden');
		inp.value='';
		__ws.call('worktree.add',{sid:'%s',project:currentProject,branch:branch});
	}

	function openDialog(project){
		currentProject=project||'';
		if(project){
			title.textContent='Worktrees — '+project;
		}else{
			title.textContent='Worktrees';
		}
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		setTimeout(function(){inp.focus();},50);
	}

	function closeDialog(){
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
			launch();
		}else if(e.key==='Escape'){
			e.preventDefault();
			closeDialog();
		}
	});

	window.__libroOpenWorktreeDialog=openDialog;
})();
`, WorktreeDialogID, sid, sid)
}

// worktreesJS returns JS that sets the global __libroWorktrees variable from all git projects.
func worktreesJS(state *AppState) string {
	if !GitAvailable() {
		return "window.__libroWorktrees=[];"
	}
	type jsWorktree struct {
		Project string `json:"project"`
		Branch  string `json:"branch"`
		Path    string `json:"path"`
	}
	var all []jsWorktree
	for _, p := range state.Projects {
		if !p.IsGitRepo || p.Virtual {
			continue
		}
		wts, err := GitListWorktrees(p.Path)
		if err != nil {
			continue
		}
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			all = append(all, jsWorktree{
				Project: p.Name,
				Branch:  wt.Branch,
				Path:    wt.Path,
			})
		}
	}
	b, _ := json.Marshal(all)
	if b == nil {
		b = []byte("[]")
	}
	return fmt.Sprintf("window.__libroWorktrees=%s;", string(b))
}

// renderManageAppsPage renders the manage apps page as a fixed overlay so running apps stay alive.
func renderManageAppsPage(state *AppState, sid string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(state.ActiveProject)

	rows := make([]*r.Node, 0, len(savedApps))
	for _, app := range savedApps {
		rows = append(rows, renderManageAppRow(app, sid))
	}

	var listNode *r.Node
	if len(rows) == 0 {
		listNode = r.Div("flex-1 flex items-center justify-center").Render(
			r.P("text-sm font-mono text-gray-400 dark:text-zinc-500").Text("No saved apps yet"),
		)
	} else {
		listNode = r.Div("flex-1 overflow-y-auto").Render(
			r.Div("max-w-2xl mx-auto w-full px-4").Render(rows...),
		)
	}

	return r.Div("fixed inset-0 z-[55] flex flex-col bg-gray-100 dark:bg-zinc-900").
		ID(ManageDialogID).
		Render(
			// Header bar
			r.Div("shrink-0 border-b border-gray-200 dark:border-zinc-800 py-4 bg-white dark:bg-zinc-900").Render(
				r.Div("max-w-2xl mx-auto w-full flex items-center gap-3 px-4").Render(
					r.Span("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100").Text("Manage Apps"),
					r.Div("ml-auto flex items-center gap-2").Render(
						r.Button("flex items-center gap-1.5 px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors").
							Render(r.I("material-icons-round text-[16px]").Text("add"), r.Span("").Text("Add App")).
							OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
						r.Button("flex items-center justify-center w-9 h-9 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700 transition-colors").
							Attr("title", "Close").
							OnClick(&r.Action{Name: "app.manage.close", Data: sidData(sid)}).
							Render(r.I("material-icons-round text-xl").Text("close")),
					),
				),
			),
			// App list
			listNode,
		)
}

// renderManageAppRow renders a single row in the manage apps page.
func renderManageAppRow(app SavedApp, sid string) *r.Node {
	var iconNode *r.Node
	label := app.Name

	if app.Type == "terminal" {
		if label == "" {
			label = app.Command
		}
		if info := lookupTermIcon(app.Command); info != nil {
			if info.URL != "" {
				iconNode = r.Img("w-6 h-6 shrink-0 rounded-sm").Attr("src", info.URL)
			} else {
				iconNode = r.I("material-icons-round text-xl shrink-0 text-gray-400 dark:text-zinc-500").Text(info.MaterialIcon)
			}
		} else if app.IconURL != "" {
			iconNode = r.Img("w-6 h-6 shrink-0 rounded-sm").Attr("src", app.IconURL)
		} else {
			iconNode = r.I("material-icons-round text-xl shrink-0 text-gray-400 dark:text-zinc-500").Text("terminal")
		}
	} else {
		if label == "" {
			label = app.URL
		}
		iconNode = r.I("material-icons-round text-xl shrink-0 text-gray-400 dark:text-zinc-500").Text("language")
		if app.URL != "" {
			if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
				iconNode = r.Img("w-6 h-6 shrink-0 rounded-sm").
					Attr("src", "https://www.google.com/s2/favicons?domain="+u.Hostname()+"&sz=32")
				if label == app.URL {
					h := strings.TrimPrefix(u.Hostname(), "www.")
					label = h
				}
			}
		}
	}

	badges := []*r.Node{
		r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Type),
		r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Width),
	}
	if app.ProjectSpecific {
		badges = append(badges,
			r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-amber-100 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400").Text("project"),
		)
	}

	editBtn := r.Button("flex items-center justify-center w-8 h-8 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-200 hover:bg-gray-200 dark:hover:bg-zinc-700 transition-colors").
		Render(r.I("material-icons-round text-lg").Text("edit")).
		Attr("onclick", fmt.Sprintf(`__ws.callSilent('app.saved.edit',{sid:'%s',dbid:%d});__ws.call('app.dialog.open',{sid:'%s'});`, sid, app.DBID, sid)+
			savedAppEditFillJS(app))

	deleteBtn := r.Button("flex items-center justify-center w-8 h-8 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10 transition-colors").
		Render(r.I("material-icons-round text-lg").Text("delete")).
		OnClick(&r.Action{Name: "app.saved.delete", Data: sidData(sid, "dbid", float64(app.DBID))})

	children := []*r.Node{iconNode}
	children = append(children, r.Span("flex-1 truncate text-sm text-gray-800 dark:text-zinc-200").Text(label))
	children = append(children, badges...)
	children = append(children, editBtn, deleteBtn)

	return r.Div("flex items-center gap-3 px-4 py-3 border-b border-gray-100 dark:border-zinc-800 hover:bg-gray-50 dark:hover:bg-zinc-800/50 transition-colors").
		Render(children...)
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
		var sizeLabels = ['SM','MD','LG','XL','2XL','FULL'];
		var activeBase = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
		btns.forEach(function(b){
			var txt = b.textContent.trim();
			if (sizeLabels.indexOf(txt) === -1) return;
			var isSelected = el.children[0] && el.children[0].className.indexOf('bg-blue-600') !== -1;
			if (txt === newWidth.toUpperCase()) {
				b.className = activeBase + (isSelected ? ' bg-white/25 text-white' : ' bg-blue-600 text-white');
			} else {
				b.className = activeBase + (isSelected ? ' text-blue-100/70 hover:text-white hover:bg-white/15' : ' text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700');
			}
		});
	}

	requestAnimationFrame(function(){
		if(window.__libroScrollToApp)window.__libroScrollToApp(el);
	});
})();
`, appID, widthMap, string(width))
}

// renderProjectBar renders the horizontal project switcher bar
// renderTopBar renders the top bar with saved app icons, browse/add buttons, and action buttons.
func renderTopBar(state *AppState, sid string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(state.ActiveProject)

	btnCls := "w-9 h-9 flex items-center justify-center rounded-md cursor-pointer transition-colors duration-75 hover:bg-gray-200 dark:hover:bg-zinc-700 relative group/ico"
	tipCls := "absolute top-full mt-1 px-2 py-1 text-xs rounded bg-white dark:bg-zinc-800 text-gray-800 dark:text-zinc-200 border border-gray-200 dark:border-zinc-700 whitespace-nowrap opacity-0 group-hover/ico:opacity-100 pointer-events-none transition-opacity z-[200] shadow-lg"

	appIcons := make([]*r.Node, 0, len(savedApps)+2)
	for _, app := range savedApps {
		var iconNode *r.Node
		label := app.Name

		if app.Type == "terminal" {
			if label == "" {
				label = app.Command
			}
			if info := lookupTermIcon(app.Command); info != nil {
				if info.URL != "" {
					iconNode = r.Img("w-6 h-6 rounded-sm").Attr("src", info.URL)
				} else {
					iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-xl").Text(info.MaterialIcon)
				}
			} else if app.IconURL != "" {
				iconNode = r.Img("w-6 h-6 rounded-sm").Attr("src", app.IconURL)
			} else {
				iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-xl").Text("terminal")
			}
		} else {
			if label == "" {
				label = app.URL
			}
			iconNode = r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-xl").Text("language")
			if app.URL != "" {
				if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
					iconNode = r.Img("w-6 h-6 rounded-sm").
						Attr("src", "https://www.google.com/s2/favicons?domain="+u.Hostname()+"&sz=32")
					if label == app.URL {
						h := strings.TrimPrefix(u.Hostname(), "www.")
						label = h
					}
				}
			}
		}

		btn := r.Button(btnCls).
			Render(iconNode, r.Span(tipCls).Text(label)).
			OnClick(&r.Action{Name: "app.start", Data: map[string]any{
				"sid": sid, "type": app.Type, "url": app.URL,
				"command": app.Command, "width": app.Width,
				"writable": app.Writable, "name": app.Name,
				"iconUrl": app.IconURL,
			}})
		appIcons = append(appIcons, btn)
	}

	// Quick launch button (combined browse + run)
	appIcons = append(appIcons,
		r.Button(btnCls).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-indigo-600 dark:hover:text-indigo-400 text-xl").Text("search"),
				r.Span(tipCls).Text("Quick launch"),
			).
			OnClick(&r.Action{Name: "app.run.new", Data: sidData(sid)}),
	)

	// Add button
	appIcons = append(appIcons,
		r.Button(btnCls).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-blue-600 dark:hover:text-blue-400 text-[18px]").Text("add"),
				r.Span(tipCls).Text("Add app"),
			).
			OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
	)

	// Manage apps button (icon style, matching other app icons)
	appIcons = append(appIcons,
		r.Button(btnCls).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("apps"),
				r.Span(tipCls).Text("Manage apps"),
			).
			OnClick(&r.Action{Name: "app.manage.open", Data: sidData(sid)}),
	)

	// Action buttons in icon style (same as app icons)
	appIcons = append(appIcons,
		r.Button(btnCls).
			Attr("onclick", fmt.Sprintf("document.getElementById('%s').classList.toggle('hidden');", ShortcutsDialogID)).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("keyboard"),
				r.Span(tipCls).Text("Shortcuts"),
			),
		r.Button(btnCls).
			Attr("onclick", "if(window.libroElectron&&window.libroElectron.toggleDevTools)window.libroElectron.toggleDevTools();").
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("code"),
				r.Span(tipCls).Text("Console"),
			),
	)

	// Zen mode button
	appIcons = append(appIcons,
		r.Button(btnCls).
			OnClick(&r.Action{Name: "zen.toggle", Data: sidData(sid)}).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-amber-600 dark:hover:text-amber-400 text-xl libro-zen-icon").Text(func() string {
					if state.ZenMode {
						return "visibility"
					}
					return "self_improvement"
				}()),
				r.Span(tipCls).Text("Zen mode"),
			),
	)

	// Running apps preview strip
	appPreview := renderAppPreview(state, sid)

	// In zen mode, hide the top bar content
	if state.ZenMode {
		return r.Div("shrink-0").ID(TopBarID)
	}

	return r.Div("flex items-center gap-1.5 px-3 py-1.5 border-b border-gray-200 dark:border-zinc-800 shrink-0").
		ID(TopBarID).
		Render(
			r.Button("shrink-0 cursor-pointer hover:opacity-70 transition-opacity duration-75 flex items-center gap-1.5").
				Attr("title", "Toggle sidebar (⌘B)").
				OnClick(&r.Action{Name: "sidebar.toggle", Data: sidData(sid)}).
				Render(
					r.Img("w-7 h-7").Attr("src", "/assets/logo.svg").Attr("alt", "Libro"),
					r.Span("text-[10px] text-gray-400 dark:text-gray-500 font-mono select-none").Text("v"+version.Version),
				),
			r.Div("flex items-center gap-0.5 ml-2").Render(appIcons...),
			r.Div("ml-auto flex items-center gap-1").Render(appPreview),
			r.ThemeSwitcher(),
		)
}

// renderAppPreview renders clickable mini-cards for each running app in the top bar.
// This helps users see and switch between apps when the window is too small to show all of them.
func renderAppPreview(state *AppState, sid string) *r.Node {
	if len(state.Apps) == 0 {
		return r.Div("")
	}

	cards := make([]*r.Node, 0, len(state.Apps))
	for i, app := range state.Apps {
		isSelected := i == state.SelectedIndex

		// Build icon for this app
		var iconNode *r.Node
		if app.Type == AppTypeTerminal {
			if info := lookupTermIcon(app.Command); info != nil {
				if info.URL != "" {
					iconNode = r.Img("w-3.5 h-3.5 rounded-sm shrink-0").Attr("src", info.URL)
				} else if info.MaterialIcon != "" {
					iconNode = r.I("material-icons-round text-[11px] shrink-0 opacity-70").Text(info.MaterialIcon)
				}
			} else if app.IconURL != "" {
				iconNode = r.Img("w-3.5 h-3.5 rounded-sm shrink-0").Attr("src", app.IconURL)
			}
			if iconNode == nil {
				iconNode = r.I("material-icons-round text-[11px] shrink-0 opacity-70").Text("terminal")
			}
		} else {
			if app.URL != "" {
				if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
					iconNode = r.Img("w-3.5 h-3.5 rounded-sm shrink-0").
						Attr("src", "https://www.google.com/s2/favicons?domain="+u.Hostname()+"&sz=16")
				}
			}
			if iconNode == nil {
				iconNode = r.I("material-icons-round text-[11px] shrink-0 opacity-70").Text("language")
			}
		}

		// App label
		label := app.Name
		if label == "" {
			if app.Type == AppTypeTerminal {
				label = app.Command
			} else {
				label = app.URL
				if u, err := urlParse(app.URL); err == nil && u.Hostname() != "" {
					label = strings.TrimPrefix(u.Hostname(), "www.")
				}
			}
		}
		if label == "" {
			label = "untitled"
		}

		// Card styling
		var cardCls string
		if isSelected {
			cardCls = "shrink-0 flex items-center gap-1.5 px-2.5 h-7 rounded-md cursor-pointer transition-all duration-75 bg-blue-600 text-white shadow-sm"
		} else {
			cardCls = "shrink-0 flex items-center gap-1.5 px-2.5 h-7 rounded-md cursor-pointer transition-all duration-75 bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 hover:bg-gray-200 dark:hover:bg-zinc-700 hover:text-gray-800 dark:hover:text-zinc-200"
		}

		card := r.Button(cardCls).
			Attr("title", label).
			OnClick(&r.Action{Name: "app.select", Data: sidData(sid, "index", i)}).
			Render(
				iconNode,
				r.Span("text-[10px] font-medium truncate max-w-[120px] leading-tight whitespace-nowrap").Text(label),
			)
		cards = append(cards, card)
	}

	return r.Div("flex items-center gap-1 ml-3 overflow-x-auto scrollbar-none").
		ID("app-preview-strip").
		Attr("style", "scrollbar-width:none;-ms-overflow-style:none").
		Render(cards...)
}

// updateAppPreviewJS returns JS that updates the selected state of preview cards
// without re-rendering the entire top bar. Used for lightweight navigate/select actions.
func updateAppPreviewJS(state *AppState) string {
	selectedCls := "shrink-0 flex items-center gap-1.5 px-2.5 h-7 rounded-md cursor-pointer transition-all duration-75 bg-blue-600 text-white shadow-sm"
	normalCls := "shrink-0 flex items-center gap-1.5 px-2.5 h-7 rounded-md cursor-pointer transition-all duration-75 bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 hover:bg-gray-200 dark:hover:bg-zinc-700 hover:text-gray-800 dark:hover:text-zinc-200"

	return fmt.Sprintf(`
		(function(){
			var strip = document.getElementById('app-preview-strip');
			if (!strip) return;
			var btns = strip.querySelectorAll(':scope > button');
			for (var i = 0; i < btns.length; i++) {
				if (i === %d) {
					btns[i].className = %s;
					btns[i].scrollIntoView({block:'nearest',inline:'nearest',behavior:'smooth'});
				} else {
					btns[i].className = %s;
				}
			}
		})();
	`, state.SelectedIndex, jsString(selectedCls), jsString(normalCls))
}

// runningAppCount returns the number of running apps for a given project name.
func runningAppCount(state *AppState, projectName string) int {
	if projectName == state.ActiveProject {
		return len(state.Apps)
	}
	if snap, ok := state.snapshots[projectName]; ok {
		return len(snap.Apps)
	}
	return 0
}

// renderAppCountBadge renders a small count badge for running apps. Returns nil if count is 0.
func renderAppCountBadge(count int, active bool) *r.Node {
	if count == 0 {
		return nil
	}
	cls := "inline-flex items-center justify-center min-w-[16px] h-4 rounded-full text-[10px] font-bold leading-none shrink-0 px-1 "
	if active {
		cls += "bg-white text-blue-600"
	} else {
		cls += "bg-blue-100 dark:bg-blue-900/40 text-blue-600 dark:text-blue-400"
	}
	return r.Span(cls).Text(fmt.Sprintf("%d", count))
}

// renderProjectSidebar renders the left sidebar with project tree and worktrees.
func renderProjectSidebar(state *AppState, sid string) *r.Node {
	if state.SidebarCollapsed {
		return r.Div("w-0 shrink-0 overflow-hidden").ID(SidebarID)
	}

	items := make([]*r.Node, 0, len(state.Projects)+1)

	for _, proj := range state.Projects {
		// Skip virtual projects — they're shown as worktree sub-items under their parent
		if proj.Virtual {
			continue
		}

		isActive := proj.Name == state.ActiveProject
		// Also check if active project is a virtual child of this project
		isParentOfActive := false
		for _, p := range state.Projects {
			if p.Virtual && p.ParentProject == proj.Name && p.Name == state.ActiveProject {
				isParentOfActive = true
				break
			}
		}

		// Project button
		projCls := "w-full flex items-center gap-2 px-3 py-2 text-sm font-mono rounded-md cursor-pointer transition-colors duration-75 group/proj "
		if isActive || isParentOfActive {
			projCls += "bg-blue-600 text-white"
		} else {
			projCls += "text-gray-700 dark:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-800"
		}

		iconName := "folder"
		if proj.IsGitRepo {
			iconName = "source"
		}

		// Shortcut badge: home always shows "1", others show assigned nav slot
		var badgeNode *r.Node
		if proj.Name == "home" {
			badgeCls := "inline-flex items-center justify-center w-4 h-4 rounded text-[10px] font-bold leading-none shrink-0 "
			if isActive || isParentOfActive {
				badgeCls += "bg-blue-500 text-blue-100"
			} else {
				badgeCls += "bg-gray-300 dark:bg-zinc-700 text-gray-500 dark:text-zinc-500"
			}
			badgeNode = r.Span(badgeCls).Text("1")
		} else if slot := sm.GetNavSlotForProject(sid, proj.Name); slot > 0 {
			badgeCls := "inline-flex items-center justify-center w-4 h-4 rounded text-[10px] font-bold leading-none shrink-0 "
			if isActive || isParentOfActive {
				badgeCls += "bg-blue-500 text-blue-100"
			} else {
				badgeCls += "bg-gray-300 dark:bg-zinc-700 text-gray-500 dark:text-zinc-500"
			}
			badgeNode = r.Span(badgeCls).Text(fmt.Sprintf("%d", slot))
		}

		projBtn := r.Button(projCls).
			Attr("title", proj.Path).
			OnClick(r.JS(fmt.Sprintf(
				"history.replaceState(null,'','#%s');__ws.call('project.switch',{sid:'%s',name:'%s'});",
				proj.Name, sid, proj.Name,
			)))

		btnChildren := []*r.Node{
			r.I("material-icons-round text-base shrink-0").Text(iconName),
		}
		if badgeNode != nil {
			btnChildren = append(btnChildren, badgeNode)
		}
		btnChildren = append(btnChildren, r.Span("truncate flex-1 text-left").Text(proj.Name))

		// Running app count badge for non-git projects (git repos show counts on worktree sub-items)
		if !proj.IsGitRepo {
			if badge := renderAppCountBadge(runningAppCount(state, proj.Name), isActive || isParentOfActive); badge != nil {
				btnChildren = append(btnChildren, badge)
			}
		}

		// Delete button (hidden until hover, not for home)
		if proj.Name != "home" {
			deleteCls := "flex items-center justify-center w-5 h-5 rounded cursor-pointer opacity-0 group-hover/proj:opacity-100 transition-opacity duration-75 shrink-0 "
			if isActive || isParentOfActive {
				deleteCls += "text-blue-200 hover:text-white hover:bg-white/15"
			} else {
				deleteCls += "text-red-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-400/10"
			}
			btnChildren = append(btnChildren,
				r.Button(deleteCls).
					Attr("title", "Remove project").
					Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('project.remove',{sid:'%s',name:'%s'});", sid, proj.Name)).
					Render(r.I("material-icons-round text-[14px]").Text("close")),
			)
		}

		projBtn.Render(btnChildren...)

		projItem := r.Div("").Render(projBtn)

		// Worktree sub-items for git repos
		if proj.IsGitRepo && GitAvailable() {
			worktrees, err := GitListWorktrees(proj.Path)
			if err == nil && len(worktrees) > 1 {
				wtItems := make([]*r.Node, 0, len(worktrees))
				for _, wt := range worktrees {
					if wt.IsBare {
						continue
					}
					// Check if this worktree is the active virtual project
					vtName := proj.Name + "/" + wt.Branch
					isWtActive := state.ActiveProject == vtName
					// Also highlight if main worktree path matches project path and project is active
					isMainWt := wt.Path == proj.Path
					if isMainWt && isActive {
						isWtActive = true
					}

					wtCls := "w-full flex items-center gap-2 pl-8 pr-3 py-1.5 text-xs font-mono rounded-md cursor-pointer transition-colors duration-75 "
					if isWtActive {
						wtCls += "bg-blue-500/20 text-blue-700 dark:text-blue-300"
					} else {
						wtCls += "text-gray-500 dark:text-zinc-500 hover:bg-gray-100 dark:hover:bg-zinc-800 hover:text-gray-700 dark:hover:text-zinc-300"
					}

					var wtOnClick string
					if isMainWt {
						wtOnClick = fmt.Sprintf(
							"history.replaceState(null,'','#%s');__ws.call('project.switch',{sid:'%s',name:'%s'});",
							proj.Name, sid, proj.Name,
						)
					} else {
						wtOnClick = fmt.Sprintf(
							"__ws.call('worktree.switch',{sid:'%s',project:'%s',path:'%s',branch:'%s'});",
							sid, proj.Name, wt.Path, wt.Branch,
						)
					}

					// Determine running app count for this worktree
					wtProjectName := vtName
					if isMainWt {
						wtProjectName = proj.Name
					}
					wtAppCount := runningAppCount(state, wtProjectName)

					wtBtnChildren := []*r.Node{
						r.I("material-icons-round text-sm shrink-0").Text("alt_route"),
						r.Span("truncate flex-1 text-left").Text(wt.Branch),
					}
					if badge := renderAppCountBadge(wtAppCount, isWtActive); badge != nil {
						wtBtnChildren = append(wtBtnChildren, badge)
					}

					wtBtn := r.Button(wtCls).
						Attr("title", wt.Path).
						OnClick(r.JS(wtOnClick)).
						Render(wtBtnChildren...)

					// Delete worktree button (not for main worktree)
					if !isMainWt {
						wtBtn.Render(
							r.Button("flex items-center justify-center w-4 h-4 rounded cursor-pointer text-gray-400 dark:text-zinc-600 hover:text-red-500 dark:hover:text-red-400 opacity-0 group-hover/proj:opacity-100 transition-opacity duration-75 shrink-0").
								Attr("title", "Remove worktree").
								Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('worktree.remove',{sid:'%s',project:'%s',path:'%s'});", sid, proj.Name, wt.Path)).
								Render(r.I("material-icons-round text-[12px]").Text("close")),
						)
					}

					wtItems = append(wtItems, wtBtn)
				}

				// Add worktree button
				wtItems = append(wtItems,
					r.Button("w-full flex items-center gap-2 pl-8 pr-3 py-1.5 text-xs font-mono rounded-md cursor-pointer text-gray-400 dark:text-zinc-600 hover:text-blue-600 dark:hover:text-blue-400 hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors duration-75").
						Attr("onclick", fmt.Sprintf("__ws.call('worktree.dialog.open',{sid:'%s',project:'%s'});", sid, proj.Name)).
						Render(
							r.I("material-icons-round text-sm shrink-0").Text("add"),
							r.Span("").Text("Add worktree"),
						),
				)

				projItem.Render(r.Div("mt-0.5").Render(wtItems...))
			} else if err == nil && (isActive || isParentOfActive) {
				// Single worktree (or none) — still show add button
				projItem.Render(
					r.Div("mt-0.5").Render(
						r.Button("w-full flex items-center gap-2 pl-8 pr-3 py-1.5 text-xs font-mono rounded-md cursor-pointer text-gray-400 dark:text-zinc-600 hover:text-blue-600 dark:hover:text-blue-400 hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors duration-75").
							Attr("onclick", fmt.Sprintf("__ws.call('worktree.dialog.open',{sid:'%s',project:'%s'});", sid, proj.Name)).
							Render(
								r.I("material-icons-round text-sm shrink-0").Text("add"),
								r.Span("").Text("Add worktree"),
							),
					),
				)
			}
		}

		items = append(items, projItem)
	}

	// Add project button
	items = append(items,
		r.Button("w-full flex items-center gap-2 px-3 py-2 text-sm font-mono text-gray-400 dark:text-zinc-600 hover:text-blue-600 dark:hover:text-blue-400 hover:bg-gray-100 dark:hover:bg-zinc-800 rounded-md cursor-pointer transition-colors duration-75 mt-1").
			OnClick(&r.Action{Name: "project.dialog.open", Data: sidData(sid)}).
			Render(
				r.I("material-icons-round text-base shrink-0").Text("add"),
				r.Span("").Text("New Project"),
			),
	)

	// Active project path display
	activePath := ""
	for _, p := range state.Projects {
		if p.Name == state.ActiveProject {
			activePath = p.Path
			break
		}
	}

	return r.Div("w-52 shrink-0 flex flex-col border-r border-gray-200 dark:border-zinc-800 bg-gray-50 dark:bg-zinc-900/50 overflow-hidden").
		ID(SidebarID).
		Render(
			r.Div("flex-1 overflow-y-auto p-2 flex flex-col gap-0.5").Render(items...),
			r.Div("px-3 py-2 border-t border-gray-200 dark:border-zinc-800").Render(
				r.Span("text-[10px] font-mono text-gray-400 dark:text-zinc-600 truncate block").
					Attr("title", activePath).
					Text(activePath),
			),
		)
}

// renderProjectBar kept as alias for backward compatibility in action responses.
func renderProjectBar(state *AppState, sid string) *r.Node {
	return renderProjectSidebar(state, sid)
}

// renderProjectDialog renders the create project modal
func renderProjectDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "/"
	}

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75" + hiddenClass).
		ID(ProjectDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-2xl mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("New Project"),

					// Directory picker
					r.Div("mb-4").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Select Folder"),
						renderDirBrowser(homeDir, sid),
					),

					// Hidden input for selected path
					r.IHidden("").ID("project-path").Attr("value", homeDir),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "project.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-create-project").
							Text("Create").
							OnClick(&r.Action{
								Name:    "project.create",
								Data:    sidData(sid),
								Collect: []string{"project-path"},
							}),
					),
				),
		)
}

// renderDirBrowser renders the directory browser component
func renderDirBrowser(currentPath string, sid string) *r.Node {
	dirCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-gray-700 dark:text-zinc-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded cursor-pointer transition-colors"
	selectedCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-900/30 rounded font-medium"

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

	// Current path display
	pathBar := r.Div("mb-2").Render(
		r.Div("px-3 py-2 bg-gray-50 dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-md text-xs font-mono text-gray-600 dark:text-zinc-400 truncate").
			Attr("title", currentPath).
			Text(currentPath),
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

	dirList := r.Div("max-h-80 overflow-y-auto space-y-0.5 border border-gray-200 dark:border-zinc-700 rounded-md p-1.5 bg-gray-50/50 dark:bg-zinc-800/50")
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
	return fmt.Sprintf("history.replaceState(null,'','#%s');document.title=%s;", name, jsString(name+" — Libro"))
}

// savedAppsJS returns JS that sets the global __libroSavedApps and __libroBrowsedURLs variables from DB data.
// Only apps visible in the given project are included (global + project-specific for this project).
func savedAppsJS(activeProject string) string {
	apps := DBLoadVisibleSavedApps(activeProject)
	type jsApp struct {
		Type     string `json:"type"`
		URL      string `json:"url,omitempty"`
		Command  string `json:"command,omitempty"`
		Width    string `json:"width"`
		Writable bool   `json:"writable"`
		Name     string `json:"name,omitempty"`
		IconURL  string `json:"iconUrl,omitempty"`
	}
	jsApps := make([]jsApp, len(apps))
	for i, a := range apps {
		jsApps[i] = jsApp{Type: a.Type, URL: a.URL, Command: a.Command, Width: a.Width, Writable: a.Writable, Name: a.Name, IconURL: a.IconURL}
	}
	b, _ := json.Marshal(jsApps)

	browsedURLs := DBLoadBrowsedURLs()
	bu, _ := json.Marshal(browsedURLs)

	runCmds := DBLoadRunCommands()
	rc, _ := json.Marshal(runCmds)
	return fmt.Sprintf("window.__libroSavedApps=%s;window.__libroBrowsedURLs=%s;window.__libroRunCommands=%s;", string(b), string(bu), string(rc))
}

// runCommandsJS returns JS that updates the global __libroRunCommands variable from DB data.
func runCommandsJS() string {
	cmds := DBLoadRunCommands()
	b, _ := json.Marshal(cmds)
	return fmt.Sprintf("window.__libroRunCommands=%s;", string(b))
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
	var proj=location.hash.replace('#','')||'home';
	document.title=proj+' \u2014 Libro';
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

	window.__libroTermIcon=function(command,size,cachedIconUrl){
		size=size||24;
		var cmd=resolveCmd(command);
		var info=icons[cmd];
		if(info&&info.url){
			return '<img src="'+info.url+'" style="width:'+size+'px;height:'+size+'px;object-fit:contain" onerror="this.outerHTML=__libroTermIconFallback(\''+command.replace(/'/g,"\\'")+'\','+size+')">';
		}
		if(info&&info.mi){
			return '<i class="material-icons-round" style="font-size:'+size+'px;color:#9ca3af">'+info.mi+'</i>';
		}
		if(cachedIconUrl){
			return '<img src="'+cachedIconUrl+'" style="width:'+size+'px;height:'+size+'px;object-fit:contain" onerror="this.outerHTML=__libroTermIconFallback(\''+command.replace(/'/g,"\\'")+'\','+size+')">';
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

			window.__libroSortedApps = function(strip) {
				var apps = Array.from(strip.querySelectorAll(':scope > [data-app-id]'));
				apps.sort(function(a, b) {
					return (parseInt(a.style.order) || 0) - (parseInt(b.style.order) || 0);
				});
				return apps;
			};

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
				var sorted = window.__libroSortedApps(strip);
				var container = sorted[idx];
				if (!container) return;

				// Blur all other iframes and webviews first
				var allIframes = document.querySelectorAll('iframe');
				for (var i = 0; i < allIframes.length; i++) {
					try { allIframes[i].contentWindow.blur(); } catch(err) {}
					allIframes[i].blur();
				}
				var allWebviews = document.querySelectorAll('webview');
				for (var i = 0; i < allWebviews.length; i++) {
					allWebviews[i].blur();
				}

				// Try to focus a webview first, then fall back to iframe
				var webview = container.querySelector('webview');
				if (webview) {
					webview.focus();
					return;
				}

				var iframe = container.querySelector('iframe');
				if (!iframe) return;
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
				if (e.metaKey && e.ctrlKey && e.code === 'KeyH') {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.move.left', {"sid": "%s"});
					return;
				}
				if (e.metaKey && e.ctrlKey && e.code === 'KeyL') {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.move.right', {"sid": "%s"});
					return;
				}
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
					__ws.call('project.select', {"sid": "%s", "index": idx});
				}
				if (e.ctrlKey && e.key === '0') {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('project.select.last', {"sid": "%s"});
				}
				if (e.metaKey && e.ctrlKey && (e.key === 'n' || e.key === 'N' || e.code === 'KeyN')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenSearch) window.__libroOpenSearch('left');
					return;
				}
				if (e.metaKey && (e.key === 'n' || e.key === 'N' || e.code === 'KeyN')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenSearch) window.__libroOpenSearch('right');
					return;
				}
				if (e.metaKey && (e.key === 'b' || e.key === 'B') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('sidebar.toggle', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'q' || e.key === 'Q') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroShowCloseDialog) window.__libroShowCloseDialog();
					return;
				}
				if (e.metaKey && (e.key === 'w' || e.key === 'W') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.close.current', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'z' || e.key === 'Z') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('zen.toggle', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'f' || e.key === 'F') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.maximize.toggle', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'g' || e.key === 'G')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenWorktreeDialog) window.__libroOpenWorktreeDialog();
					return;
				}
				if (e.ctrlKey && !e.metaKey && (e.key === 'l' || e.key === 'L')) {
					var selTb = document.querySelector('.bg-blue-600.border-blue-700');
					var appE = selTb ? selTb.closest('[data-app-id]') : null;
					if (appE && appE.querySelector('webview')) {
						e.preventDefault();
						e.stopImmediatePropagation();
						if (window.__libroOpenURLPopup) window.__libroOpenURLPopup();
					}
				}
				if (e.metaKey && (e.key === 'r' || e.key === 'R') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenResizePopup) window.__libroOpenResizePopup();
					return;
				}
				if (e.ctrlKey && !e.metaKey && (e.key === 'r' || e.key === 'R')) {
					var selToolbar2 = document.querySelector('.bg-blue-600.border-blue-700');
					var appEl2 = selToolbar2 ? selToolbar2.closest('[data-app-id]') : null;
					if (appEl2) {
						var appId2 = appEl2.getAttribute('data-app-id');
						if (appId2 && window.__libroWebviews && window.__libroWebviews[appId2]) {
							e.preventDefault();
							e.stopImmediatePropagation();
							window.__libroWvReload(appId2);
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
	`, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid)
}
