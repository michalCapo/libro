package libro

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"

	"libro/internal/components"
)

// urlParse is a convenience wrapper around url.Parse.
func urlParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func faviconURL(rawURL string, size int) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if size <= 0 {
		size = 32
	}

	u, err := urlParse(rawURL)
	if err == nil && strings.EqualFold(u.Scheme, "file") {
		return ""
	}
	if err != nil || u.Hostname() == "" {
		u, err = urlParse("https://" + rawURL)
	}
	if err != nil || u.Hostname() == "" || strings.EqualFold(u.Scheme, "file") {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return ""
	}

	target := u.Hostname()
	if u.Scheme != "" && u.Host != "" {
		target = u.Scheme + "://" + u.Host
	}
	return fmt.Sprintf("https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=%s&size=%d", url.QueryEscape(target), size)
}

const (
	MainAreaID = "main-area"
	TopBarID   = "top-bar"

	ProjectDialogID = components.ProjectDialogID

	ShortcutsDialogID     = components.ShortcutsDialogID
	CloseDialogID         = components.CloseDialogID
	URLPopupID            = components.URLPopupID
	ResizePopupID         = components.ResizePopupID
	CommandPopupID        = components.CommandPopupID
	MoveProjectPopupID    = components.MoveProjectPopupID
	WorktreeCreatePopupID = components.WorktreeCreatePopupID
	ActionEffectsID       = "libro-action-effects"
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
	// Editors
	"nvim":   {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"neovim": {URL: "https://cdn.simpleicons.org/neovim/57A143"},
	"vim":    {URL: "https://cdn.simpleicons.org/vim/019733"},
	"vi":     {URL: "https://cdn.simpleicons.org/vim/019733"},
	"emacs":  {URL: "https://cdn.simpleicons.org/gnuemacs/7F5AB6"},
	"nano":   {URL: "https://cdn.simpleicons.org/gnu/A42E2B"},
	"hx":     {URL: "https://cdn.simpleicons.org/helix/2C5CC5"},
	"helix":  {URL: "https://cdn.simpleicons.org/helix/2C5CC5"},
	"code":   {URL: "https://cdn.simpleicons.org/visualstudiocode/007ACC"},
	"zed":    {URL: "https://cdn.simpleicons.org/zedindustries/0CA5E9"},

	// AI assistants
	"claude":  {URL: "https://cdn.simpleicons.org/anthropic/d4a27f"},
	"gemini":  {URL: "https://cdn.simpleicons.org/googlegemini/8E75B2"},
	"gh":      {URL: "https://cdn.simpleicons.org/github/FFFFFF"},
	"copilot": {URL: "https://cdn.simpleicons.org/githubcopilot/FFFFFF"},
	"openai":  {URL: "https://cdn.simpleicons.org/openai/FFFFFF"},
	"chatgpt": {URL: "https://cdn.simpleicons.org/openai/FFFFFF"},
	"aider":   {MaterialIcon: "smart_toy"},
	"codex":   {MaterialIcon: "code"},
	"pi":      {MaterialIcon: "functions"},

	// JavaScript / TypeScript runtimes & package managers
	"node":    {URL: "https://cdn.simpleicons.org/nodedotjs/5FA04E"},
	"nodejs":  {URL: "https://cdn.simpleicons.org/nodedotjs/5FA04E"},
	"npm":     {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"npx":     {URL: "https://cdn.simpleicons.org/npm/CB3837"},
	"yarn":    {URL: "https://cdn.simpleicons.org/yarn/2C8EBB"},
	"pnpm":    {URL: "https://cdn.simpleicons.org/pnpm/F69220"},
	"bun":     {URL: "https://cdn.simpleicons.org/bun/FBF0DF"},
	"bunx":    {URL: "https://cdn.simpleicons.org/bun/FBF0DF"},
	"deno":    {URL: "https://cdn.simpleicons.org/deno/FFFFFF"},
	"tsc":     {URL: "https://cdn.simpleicons.org/typescript/3178C6"},
	"ts-node": {URL: "https://cdn.simpleicons.org/typescript/3178C6"},
	"vite":    {URL: "https://cdn.simpleicons.org/vite/646CFF"},
	"webpack": {URL: "https://cdn.simpleicons.org/webpack/8DD6F9"},
	"esbuild": {URL: "https://cdn.simpleicons.org/esbuild/FFCF00"},

	// Python
	"python":  {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"python3": {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"py":      {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"pip":     {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"pip3":    {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"pipx":    {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"poetry":  {URL: "https://cdn.simpleicons.org/poetry/60A5FA"},
	"uv":      {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"ruff":    {URL: "https://cdn.simpleicons.org/ruff/D7FF64"},
	"jupyter": {URL: "https://cdn.simpleicons.org/jupyter/F37626"},
	"ipython": {URL: "https://cdn.simpleicons.org/python/3776AB"},
	"conda":   {URL: "https://cdn.simpleicons.org/anaconda/44A833"},

	// Containers / orchestration / cloud
	"docker":         {URL: "https://cdn.simpleicons.org/docker/2496ED"},
	"docker-compose": {URL: "https://cdn.simpleicons.org/docker/2496ED"},
	"podman":         {URL: "https://cdn.simpleicons.org/podman/892CA0"},
	"kubectl":        {URL: "https://cdn.simpleicons.org/kubernetes/326CE5"},
	"k9s":            {URL: "https://cdn.simpleicons.org/kubernetes/326CE5"},
	"helm":           {URL: "https://cdn.simpleicons.org/helm/0F1689"},
	"minikube":       {URL: "https://cdn.simpleicons.org/kubernetes/326CE5"},
	"terraform":      {URL: "https://cdn.simpleicons.org/terraform/844FBA"},
	"tofu":           {URL: "https://cdn.simpleicons.org/opentofu/FFDA18"},
	"ansible":        {URL: "https://cdn.simpleicons.org/ansible/EE0000"},
	"vagrant":        {URL: "https://cdn.simpleicons.org/vagrant/1868F2"},
	"pulumi":         {URL: "https://cdn.simpleicons.org/pulumi/8A3391"},
	"aws":            {URL: "https://cdn.simpleicons.org/amazonwebservices/FF9900"},
	"gcloud":         {URL: "https://cdn.simpleicons.org/googlecloud/4285F4"},
	"az":             {URL: "https://cdn.simpleicons.org/microsoftazure/0078D4"},
	"flyctl":         {URL: "https://cdn.simpleicons.org/flydotio/8B5CF6"},
	"fly":            {URL: "https://cdn.simpleicons.org/flydotio/8B5CF6"},
	"vercel":         {URL: "https://cdn.simpleicons.org/vercel/FFFFFF"},
	"netlify":        {URL: "https://cdn.simpleicons.org/netlify/00C7B7"},
	"heroku":         {URL: "https://cdn.simpleicons.org/heroku/430098"},

	// Version control
	"git": {URL: "https://cdn.simpleicons.org/git/F05032"},
	"hg":  {URL: "https://cdn.simpleicons.org/mercurial/999999"},
	"svn": {URL: "https://cdn.simpleicons.org/subversion/809CC9"},

	// Compiled languages
	"go":      {URL: "https://cdn.simpleicons.org/go/00ADD8"},
	"gofmt":   {URL: "https://cdn.simpleicons.org/go/00ADD8"},
	"cargo":   {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"rustc":   {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"rustup":  {URL: "https://cdn.simpleicons.org/rust/DEA584"},
	"zig":     {URL: "https://cdn.simpleicons.org/zig/F7A41D"},
	"nim":     {URL: "https://cdn.simpleicons.org/nim/FFE953"},
	"crystal": {URL: "https://cdn.simpleicons.org/crystal/000000"},
	"gcc":     {URL: "https://cdn.simpleicons.org/gnu/A42E2B"},
	"clang":   {URL: "https://cdn.simpleicons.org/llvm/262D3A"},
	"dotnet":  {URL: "https://cdn.simpleicons.org/dotnet/512BD4"},
	"elixir":  {URL: "https://cdn.simpleicons.org/elixir/4B275F"},
	"iex":     {URL: "https://cdn.simpleicons.org/elixir/4B275F"},
	"mix":     {URL: "https://cdn.simpleicons.org/elixir/4B275F"},
	"erl":     {URL: "https://cdn.simpleicons.org/erlang/A90533"},
	"haskell": {URL: "https://cdn.simpleicons.org/haskell/5D4F85"},
	"ghci":    {URL: "https://cdn.simpleicons.org/haskell/5D4F85"},
	"stack":   {URL: "https://cdn.simpleicons.org/haskell/5D4F85"},

	// Dynamic / scripting
	"ruby":     {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"irb":      {URL: "https://cdn.simpleicons.org/ruby/CC342D"},
	"gem":      {URL: "https://cdn.simpleicons.org/rubygems/E9573F"},
	"bundle":   {URL: "https://cdn.simpleicons.org/rubygems/E9573F"},
	"rails":    {URL: "https://cdn.simpleicons.org/rubyonrails/CC0000"},
	"php":      {URL: "https://cdn.simpleicons.org/php/777BB4"},
	"composer": {URL: "https://cdn.simpleicons.org/composer/885630"},
	"perl":     {URL: "https://cdn.simpleicons.org/perl/39457E"},
	"lua":      {URL: "https://cdn.simpleicons.org/lua/2C2D72"},
	"luajit":   {URL: "https://cdn.simpleicons.org/lua/2C2D72"},
	"r":        {URL: "https://cdn.simpleicons.org/r/276DC3"},
	"julia":    {URL: "https://cdn.simpleicons.org/julia/9558B2"},

	// JVM
	"java":    {URL: "https://cdn.simpleicons.org/openjdk/FFFFFF"},
	"javac":   {URL: "https://cdn.simpleicons.org/openjdk/FFFFFF"},
	"kotlin":  {URL: "https://cdn.simpleicons.org/kotlin/7F52FF"},
	"kotlinc": {URL: "https://cdn.simpleicons.org/kotlin/7F52FF"},
	"scala":   {URL: "https://cdn.simpleicons.org/scala/DC322F"},
	"sbt":     {URL: "https://cdn.simpleicons.org/scala/DC322F"},
	"groovy":  {URL: "https://cdn.simpleicons.org/apachegroovy/4298B8"},
	"swift":   {URL: "https://cdn.simpleicons.org/swift/F05138"},

	// Databases / data tools
	"redis-cli":         {URL: "https://cdn.simpleicons.org/redis/FF4438"},
	"psql":              {URL: "https://cdn.simpleicons.org/postgresql/4169E1"},
	"pg_dump":           {URL: "https://cdn.simpleicons.org/postgresql/4169E1"},
	"mysql":             {URL: "https://cdn.simpleicons.org/mysql/4479A1"},
	"mariadb":           {URL: "https://cdn.simpleicons.org/mariadb/003545"},
	"mongosh":           {URL: "https://cdn.simpleicons.org/mongodb/47A248"},
	"mongo":             {URL: "https://cdn.simpleicons.org/mongodb/47A248"},
	"sqlite3":           {URL: "https://cdn.simpleicons.org/sqlite/003B57"},
	"sqlite":            {URL: "https://cdn.simpleicons.org/sqlite/003B57"},
	"influx":            {URL: "https://cdn.simpleicons.org/influxdb/22ADF6"},
	"clickhouse-client": {URL: "https://cdn.simpleicons.org/clickhouse/FFCC01"},
	"duckdb":            {URL: "https://cdn.simpleicons.org/duckdb/FFF000"},

	// Build & task tools
	"make":   {MaterialIcon: "build"},
	"cmake":  {MaterialIcon: "build"},
	"ninja":  {MaterialIcon: "build"},
	"bazel":  {MaterialIcon: "build"},
	"gradle": {URL: "https://cdn.simpleicons.org/gradle/02303A"},
	"mvn":    {URL: "https://cdn.simpleicons.org/apachemaven/C71A36"},
	"just":   {MaterialIcon: "task_alt"},
	"task":   {MaterialIcon: "task_alt"},

	// Shells
	"bash": {MaterialIcon: "terminal"},
	"zsh":  {MaterialIcon: "terminal"},
	"sh":   {MaterialIcon: "terminal"},
	"fish": {MaterialIcon: "terminal"},
	"dash": {MaterialIcon: "terminal"},
	"ksh":  {MaterialIcon: "terminal"},
	"nu":   {MaterialIcon: "terminal"},
	"pwsh": {URL: "https://cdn.simpleicons.org/powershell/5391FE"},

	// System / monitoring / network
	"ssh":        {MaterialIcon: "vpn_key"},
	"sftp":       {MaterialIcon: "vpn_key"},
	"scp":        {MaterialIcon: "vpn_key"},
	"mosh":       {MaterialIcon: "vpn_key"},
	"telnet":     {MaterialIcon: "vpn_key"},
	"htop":       {MaterialIcon: "monitoring"},
	"btop":       {MaterialIcon: "monitoring"},
	"btm":        {MaterialIcon: "monitoring"},
	"top":        {MaterialIcon: "monitoring"},
	"glances":    {MaterialIcon: "monitoring"},
	"iotop":      {MaterialIcon: "monitoring"},
	"iftop":      {MaterialIcon: "monitoring"},
	"netstat":    {MaterialIcon: "lan"},
	"ss":         {MaterialIcon: "lan"},
	"ping":       {MaterialIcon: "network_ping"},
	"traceroute": {MaterialIcon: "route"},
	"dig":        {MaterialIcon: "dns"},
	"nslookup":   {MaterialIcon: "dns"},
	"curl":       {URL: "https://cdn.simpleicons.org/curl/073551"},
	"wget":       {URL: "https://cdn.simpleicons.org/gnu/A42E2B"},
	"http":       {MaterialIcon: "http"},
	"httpie":     {MaterialIcon: "http"},

	// Multiplexers / launchers
	"screen": {MaterialIcon: "splitscreen"},
	"zellij": {MaterialIcon: "splitscreen"},

	// Filesystem / search
	"ls":   {MaterialIcon: "folder"},
	"eza":  {MaterialIcon: "folder"},
	"exa":  {MaterialIcon: "folder"},
	"tree": {MaterialIcon: "account_tree"},
	"fd":   {MaterialIcon: "search"},
	"find": {MaterialIcon: "search"},
	"rg":   {MaterialIcon: "search"},
	"grep": {MaterialIcon: "search"},
	"ack":  {MaterialIcon: "search"},
	"ag":   {MaterialIcon: "search"},
	"bat":  {MaterialIcon: "description"},
	"cat":  {MaterialIcon: "description"},
	"less": {MaterialIcon: "description"},
	"more": {MaterialIcon: "description"},

	// Misc
	"man":        {MaterialIcon: "menu_book"},
	"tldr":       {MaterialIcon: "menu_book"},
	"jq":         {MaterialIcon: "data_object"},
	"yq":         {MaterialIcon: "data_object"},
	"watch":      {MaterialIcon: "visibility"},
	"systemctl":  {MaterialIcon: "settings_applications"},
	"journalctl": {MaterialIcon: "receipt_long"},
	"crontab":    {MaterialIcon: "schedule"},
	"ffmpeg":     {URL: "https://cdn.simpleicons.org/ffmpeg/007808"},
	"yt-dlp":     {URL: "https://cdn.simpleicons.org/youtube/FF0000"},
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
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("components: simpleicons HEAD %s body close failed: %v", iconURL, cerr)
		}
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

func appContentID(appID string) string {
	return "app-content-" + appID
}

func appFrameStyle(app Application, index int) string {
	width := app.Width.PixelWidth()
	return fmt.Sprintf("order:%d;width:%s;flex:0 0 %s", index, width, width)
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
	return r.Div("ws-body").Render(
		renderWorkspaceSidebar(sid),
		r.Div("flex-1 flex flex-col overflow-hidden relative").ID(MainAreaID).Render(
			renderMainArea(state, sid),
			renderWorkspaceSettings(),
		),
		renderWorkspaceTools(),
	)
}

// renderMainArea renders the entire main area based on current state
func renderMainArea(state *AppState, sid string) *r.Node {
	if len(state.Apps) == 0 {
		return renderWorkspaceEmpty(state, sid)
	}
	return renderAppStrip(state, sid)
}

func renderMainAreaWithPlaceholder(state *AppState, sid, placeholderAppID string) *r.Node {
	if len(state.Apps) == 0 {
		return renderWorkspaceEmpty(state, sid)
	}
	return renderAppStripWithPlaceholder(state, sid, placeholderAppID)
}

func renderMainAreaForProject(state *AppState, sid, projectName string) *r.Node {
	projectState := *state
	projectState.ActiveProject = projectName
	if projectName != state.ActiveProject {
		if snap, ok := state.snapshots[projectName]; ok {
			projectState.Apps = snap.Apps
			projectState.SelectedIndex = snap.SelectedIndex
		} else {
			projectState.Apps = nil
			projectState.SelectedIndex = 0
		}
	}
	return renderMainArea(&projectState, sid)
}

func projectHasRunningApps(state *AppState, projectName string) bool {
	if projectName == state.ActiveProject {
		return len(state.Apps) > 0
	}
	if snap, ok := state.snapshots[projectName]; ok {
		return len(snap.Apps) > 0
	}
	return false
}

// renderAppStrip renders the project's persistent tab groups.
func renderAppStrip(state *AppState, sid string) *r.Node {
	return renderAppStripWithPlaceholder(state, sid, "")
}

func renderAppStripWithPlaceholder(state *AppState, sid, placeholderAppID string) *r.Node {
	return renderWorkspaceStrip(state, sid, placeholderAppID)
}

func selectedAppID(state *AppState) string {
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Apps) {
		return components.JSString(state.Apps[state.SelectedIndex].ID)
	}
	return "''"
}

func centerSelectedJS(state *AppState) string {
	return fmt.Sprintf(`
		window.__libroSelectedApp=%s;
		if (window.__libroSelectedApp && window.__libroApplyBrowserMode && window.__libroGetBrowserMode) {
			window.__libroApplyBrowserMode(window.__libroSelectedApp, window.__libroGetBrowserMode(window.__libroSelectedApp));
		}
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
	`, selectedAppID(state), stripID(state.ActiveProject), len(state.Apps), state.SelectedIndex)
}

// moveAppJS reorders app frames visually using CSS order (no DOM moves,
// so Electron webviews are preserved). Then runs navigateJS for selection visuals.
func moveAppJS(state *AppState, sid string, _ string) string {
	return navigateJS(state, sid)
}

func navigateJS(state *AppState, sid string) string {
	return navigateProjectJS(state.ActiveProject, state.Apps, state.SelectedIndex, sid)
}

func navigateProjectJS(projectName string, apps []Application, selectedIndex int, sid string) string {
	var js strings.Builder
	for i, app := range apps {
		fmt.Fprintf(&js, "var e=document.getElementById(%s);if(e){e.style.order=%s;e.dataset.dock=%s;}", components.JSString("frame-"+app.ID), components.JSString(fmt.Sprint(i)), components.JSString(appDock(app)))
	}
	if selectedIndex >= 0 && selectedIndex < len(apps) {
		fmt.Fprintf(&js, "window.__libroSelectedApp=%s;if(window.libroWorkspace)libroWorkspace.select(%s,false);", components.JSString(apps[selectedIndex].ID), components.JSString(apps[selectedIndex].ID))
	}
	return "(function(){" + js.String() + "})();"
}

// popupRegistryJS registers the global helper used by every popup opener to
// hide any other popup that is currently visible. Pass the popup's element
// (or its ID) as `except` to keep that one open.
func popupRegistryJS() string {
	return fmt.Sprintf(`
(function(){
	var IDS=[%q,%q,%q,%q,%q,%q,%q,%q];
	var FLOATING={};
	[%q,%q].forEach(function(id){FLOATING[id]=true;});
	function keepRoot(){return document.body||document.documentElement;}
	function parkFloating(el){
		if(!el)return;
		try{var active=document.activeElement;if(active&&el.contains&&el.contains(active)&&typeof active.blur==='function')active.blur();}catch(e){}
		var root=keepRoot();
		if(root&&el.parentNode!==root)root.appendChild(el);
	}
	window.__libroParkFloatingPopups=function(except){
		var keep=null;
		if(except)keep=(typeof except==='string')?document.getElementById(except):except;
		Object.keys(FLOATING).forEach(function(id){
			var el=document.getElementById(id);
			if(!el||el===keep)return;
			if(!el.classList.contains('hidden'))el.classList.add('hidden');
			parkFloating(el);
		});
	};
	window.__libroCloseAllPopups=function(except){
		var keep=null;
		if(except){
			keep=(typeof except==='string')?document.getElementById(except):except;
		}
		for(var i=0;i<IDS.length;i++){
			var el=document.getElementById(IDS[i]);
			if(!el||el===keep)continue;
			if(!el.classList.contains('hidden'))el.classList.add('hidden');
			if(FLOATING[IDS[i]])parkFloating(el);
		}
	};
})();
`, URLPopupID, ResizePopupID, CommandPopupID, MoveProjectPopupID, WorktreeCreatePopupID, ShortcutsDialogID, CloseDialogID, ProjectDialogID, URLPopupID, ResizePopupID)
}

func parkFloatingPopupsJS() string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroParkFloatingPopups){window.__libroParkFloatingPopups();return;}
	var root=document.body||document.documentElement;
	[%q,%q].forEach(function(id){
		var el=document.getElementById(id);
		if(!el)return;
		try{var active=document.activeElement;if(active&&el.contains&&el.contains(active)&&typeof active.blur==='function')active.blur();}catch(e){}
		if(!el.classList.contains('hidden'))el.classList.add('hidden');
		if(root&&el.parentNode!==root)root.appendChild(el);
	});
})();
`, URLPopupID, ResizePopupID)
}

func uxHardenJS() string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroUXHardened)return;
	window.__libroUXHardened=true;
	var dialogIDs=[%q,%q,%q,%q,%q,%q];
	var lastFocus={};
	function visible(el){if(!el||el.classList.contains('hidden'))return false;var st=getComputedStyle(el);return st.display!=='none'&&st.visibility!=='hidden'&&st.opacity!=='0';}
	function panelFor(el){return el&&el.firstElementChild?el.firstElementChild:el;}
	function focusables(root){
		if(!root)return [];
		return Array.prototype.slice.call(root.querySelectorAll('button,[href],input,select,textarea,[tabindex]:not([tabindex="-1"])')).filter(function(n){return !n.disabled&&n.getAttribute('aria-hidden')!=='true'&&n.offsetParent!==null;});
	}
	function enhance(el){
		if(!el||el.__libroA11yEnhanced)return;
		el.__libroA11yEnhanced=true;
		var panel=panelFor(el);
		if(panel){
			panel.setAttribute('role','dialog');
			panel.setAttribute('aria-modal','true');
			if(!panel.getAttribute('aria-label')){
				var title=panel.querySelector('span, h1, h2, h3');
				if(title&&title.textContent)panel.setAttribute('aria-label',title.textContent.trim());
			}
			if(!panel.hasAttribute('tabindex'))panel.setAttribute('tabindex','-1');
		}
		el.addEventListener('keydown',function(e){
			if(!visible(el))return;
			if(e.key==='Tab'){
				var nodes=focusables(panel);
				if(!nodes.length){e.preventDefault();if(panel)panel.focus();return;}
				var first=nodes[0], last=nodes[nodes.length-1];
				if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}
				else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}
			}
		},true);
	}
	function watch(el){
		if(!el)return;
		enhance(el);
		var wasVisible=visible(el);
		var obs=new MutationObserver(function(){
			var isVisible=visible(el);
			if(isVisible&&!wasVisible){
				lastFocus[el.id]=document.activeElement;
				setTimeout(function(){var p=panelFor(el);var nodes=focusables(p);(nodes[0]||p||el).focus();},30);
			}else if(!isVisible&&wasVisible){
				var prev=lastFocus[el.id];
				if(prev&&prev.focus)try{prev.focus({preventScroll:true});}catch(e){try{prev.focus();}catch(e2){}}
			}
			wasVisible=isVisible;
		});
		obs.observe(el,{attributes:true,attributeFilter:['class','style']});
	}
	dialogIDs.forEach(function(id){watch(document.getElementById(id));});

	window.__libroConfirmAction=function(title,body,onConfirm){
		var old=document.getElementById('libro-confirm-popover');
		if(old)old.remove();
		var wrap=document.createElement('div');
		wrap.id='libro-confirm-popover';
		wrap.className='ws-popup fixed inset-0 z-[10000] flex items-start justify-center pt-[22vh] bg-black/35 dark:bg-black/55';
		var dk=document.documentElement.classList.contains('dark');
		wrap.innerHTML='<div role="alertdialog" aria-modal="true" aria-label="Confirm action" class="w-full max-w-md mx-4 rounded-lg border shadow-2xl '+(dk?'bg-zinc-900 border-zinc-700 text-zinc-100':'bg-white border-gray-200 text-gray-900')+'">'
			+'<div class="px-4 py-3 border-b '+(dk?'border-zinc-700/50':'border-gray-200')+' flex items-center gap-2"><span class="material-icons-round text-red-500 text-lg">warning</span><div class="text-sm font-medium">'+String(title||'Confirm action').replace(/[&<>]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;'}[c];})+'</div></div>'
			+'<div class="px-4 py-3 text-sm '+(dk?'text-zinc-400':'text-gray-600')+'">'+String(body||'').replace(/[&<>]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;'}[c];}).replace(/\n/g,'<br>')+'</div>'
			+'<div class="px-4 py-3 flex justify-end gap-2"><button data-cancel class="px-3 py-1.5 rounded text-xs font-mono '+(dk?'text-zinc-300 hover:bg-zinc-800':'text-gray-600 hover:bg-gray-100')+'">Cancel</button><button data-confirm class="px-3 py-1.5 rounded bg-red-600 hover:bg-red-500 text-white text-xs font-mono font-medium">Confirm</button></div></div>';
		document.body.appendChild(wrap);
		var cancel=wrap.querySelector('[data-cancel]');
		var confirm=wrap.querySelector('[data-confirm]');
		function close(){wrap.remove();}
		wrap.addEventListener('mousedown',function(e){if(e.target===wrap)close();});
		wrap.addEventListener('keydown',function(e){if(e.key==='Escape'){e.preventDefault();close();}});
		cancel.onclick=close;
		confirm.onclick=function(){close();if(typeof onConfirm==='function')onConfirm();};
		setTimeout(function(){cancel.focus();},0);
	};
})();
`, ProjectDialogID, ShortcutsDialogID, CloseDialogID, CommandPopupID, MoveProjectPopupID, WorktreeCreatePopupID)
}

func flashCSS() string {
	return `(function(){
	if(!document.getElementById('libro-flash-css')){
		var s=document.createElement('style');
		s.id='libro-flash-css';
		s.textContent='@keyframes libro-flash{0%{transform:scale(1);opacity:1}15%{transform:scale(2.5);opacity:.6}100%{transform:scale(1);opacity:1}} @keyframes libro-toast-in{0%{opacity:0;transform:translate(-50%,-50%) scale(.98)}100%{opacity:1;transform:translate(-50%,-50%) scale(1)}} @keyframes libro-toast-out{0%{opacity:1;transform:translate(-50%,-50%) scale(1)}100%{opacity:0;transform:translate(-50%,-50%) scale(.98)}} @keyframes libro-toast-slide-up{0%{transform:translateY(100%);opacity:0}100%{transform:translateY(0);opacity:1}} @keyframes libro-toast-slide-down{0%{transform:translateY(0);opacity:1}100%{transform:translateY(100%);opacity:0}} @keyframes libro-app-select{0%{outline:2px solid rgba(59,130,246,.5)}100%{outline:2px solid transparent}} @keyframes libro-project-switch{0%{opacity:0}100%{opacity:1}} button:focus-visible,input:focus-visible,textarea:focus-visible,[tabindex]:focus-visible{outline:2px solid rgba(59,130,246,.65)!important;outline-offset:2px!important} .scrollbar-none,[id^="app-strip-"]{scrollbar-width:none;-ms-overflow-style:none} .scrollbar-none::-webkit-scrollbar,[id^="app-strip-"]::-webkit-scrollbar{width:0!important;height:0!important;display:none!important}';
		document.head.appendChild(s);
	}

	window.__libroScrollToApp=function(app){
		var strip=app&&app.parentElement;
		if(!strip)return;
		var sr=strip.getBoundingClientRect();
		var ar=app.getBoundingClientRect();
		var viewport=strip.clientWidth||sr.width;
		var spacer=Math.max(0,(viewport-ar.width)/2);
		var first=strip.firstElementChild;
		var last=strip.lastElementChild;
		if(first&&!first.hasAttribute('data-app-id')){
			first.style.flex='0 0 '+spacer+'px';
			first.style.minWidth=spacer+'px';
		}
		if(last&&!last.hasAttribute('data-app-id')){
			last.style.flex='0 0 '+spacer+'px';
			last.style.minWidth=spacer+'px';
		}
		sr=strip.getBoundingClientRect();
		ar=app.getBoundingClientRect();
		var appLeft=ar.left-sr.left+strip.scrollLeft;
		var max=Math.max(0,strip.scrollWidth-strip.clientWidth);
		var target=appLeft+(ar.width/2)-(viewport/2);
		strip.scrollLeft=Math.max(0,Math.min(max,target));
	};

	window.__libroCenterSelectedApp=function(){
		var id=window.__libroSelectedApp||'';
		if(!id)return;
		var app=document.querySelector('[data-app-id="'+String(id).replace(/"/g,'\\"')+'"]');
		if(app&&window.__libroScrollToApp)window.__libroScrollToApp(app);
	};

	if(!window.__libroCenterResizeRegistered){
		window.__libroCenterResizeRegistered=true;
		var scheduleCenter=function(){
			clearTimeout(window.__libroCenterResizeTimer);
			window.__libroCenterResizeTimer=setTimeout(function(){
				if(window.__libroCenterSelectedApp)window.__libroCenterSelectedApp();
			},50);
		};
		window.addEventListener('resize',scheduleCenter);
	}
})();`
}

// toastSetupJS returns JS that registers the global toast function.
func toastSetupJS() string {
	return `
(function(){
	if(window.__libroShowToast)return;
	var timer=null;
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
	return fmt.Sprintf("if(window.__libroShowToast)window.__libroShowToast(%s,%s,%d);", components.JSString(title), components.JSString(subtitle), durationMs)
}

func appWidthPolicyJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var SID=%s;
	var FULL_HD_MAX=1920;
	var widthPixels={xs:320,sm:480,md:640,lg:960,xl:1280,'2xl':1920,'3xl':2560,full:0};

	function screenWidth(){
		var values=[window.innerWidth||0];
		if(window.screen){
			values.push(window.screen.width||0);
			values.push(window.screen.availWidth||0);
		}
		return Math.max.apply(null,values);
	}

	function maxFixedPixels(){
		return screenWidth()<=FULL_HD_MAX?FULL_HD_MAX:0;
	}

	function clampWidth(width){
		var max=maxFixedPixels();
		var px=widthPixels[width]||0;
		if(!max||px===0||px<=max)return width;
		var best='xs';
		Object.keys(widthPixels).forEach(function(k){
			var candidate=widthPixels[k];
			if(candidate>0&&candidate<=max)best=k;
		});
		return best;
	}

	function notifyBlocked(){
		if(window.__libroShowToast)window.__libroShowToast('3XL unavailable','Screen is Full HD or smaller',1800);
	}

	window.__libroAppWidthMaxPixel=maxFixedPixels;
	window.__libroClampAppWidth=clampWidth;
	window.__libroIsAppWidthAllowed=function(width){return clampWidth(width)===width;};
	window.__libroRefreshWidthAvailability=function(root){
		root=root||document;
		root.querySelectorAll('[data-resize-width]').forEach(function(el){
			var width=el.getAttribute('data-resize-width')||'';
			var blocked=clampWidth(width)!==width;
			el.setAttribute('aria-disabled',blocked?'true':'false');
			if(blocked){
				el.setAttribute('title','Requires a screen wider than 1920px');
				el.classList.add('opacity-40');
			}else{
				if(el.getAttribute('title')==='Requires a screen wider than 1920px')el.removeAttribute('title');
				el.classList.remove('opacity-40');
			}
		});
	};
	window.__libroResizeApp=function(appId,width,sid,force){
		var next=clampWidth(width);
		if(next!==width&&!force){
			notifyBlocked();
			return false;
		}
		__ws.call('app.resize',{sid:sid||SID,id:appId,width:next,maxPixel:maxFixedPixels()});
		return true;
	};
	window.__libroResizeSelectedAppStep=function(delta,sid){
		__ws.call('app.resize.step',{sid:sid||SID,delta:delta,maxPixel:maxFixedPixels()});
	};
	window.__libroToggleSelectedAppMax=function(sid){
		__ws.call('app.maximize.toggle',{sid:sid||SID,maxPixel:maxFixedPixels()});
	};

	function enforceExistingWidths(){
		window.__libroRefreshWidthAvailability(document);
		if(!maxFixedPixels())return;
		document.querySelectorAll('[data-app-id]').forEach(function(el){
			if((el.className||'').indexOf('w-[2560px]')===-1)return;
			var appId=el.getAttribute('data-app-id')||'';
			if(appId)window.__libroResizeApp(appId,'3xl',SID,true);
		});
	}
	window.__libroEnforceAppWidthPolicy=enforceExistingWidths;

	window.addEventListener('resize',function(){
		clearTimeout(window.__libroWidthPolicyTimer);
		window.__libroWidthPolicyTimer=setTimeout(enforceExistingWidths,100);
	});
	setTimeout(enforceExistingWidths,100);
})();
`, components.JSString(sid))
}

func renderAppFrame(app Application, index int, selected bool, sid string) *r.Node {
	return renderAppFrameBase(app, index, selected, sid, false)
}

// renderAppFramePlaceholder renders a width-correct app shell without creating
// the Electron webview or terminal instance yet.
func renderAppFramePlaceholder(app Application, index int, selected bool, sid string) *r.Node {
	return renderAppFrameBase(app, index, selected, sid, true)
}

// renderAppFrameBase renders a single application frame with controls.
func renderAppFrameBase(app Application, index int, selected bool, sid string, placeholder bool) *r.Node {
	borderClass := "border-[1px] border-transparent"
	if selected {
		borderClass = "border-[1px] border-blue-500"
	}

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
			Attr("data-resize-width", string(w)).
			Attr("aria-pressed", fmt.Sprint(w == app.Width)).
			Attr("title", w.Label()).
			Attr("aria-label", "Resize panel to "+w.Label()).
			Text(w.ShortLabel()).
			OnClick(r.JS(fmt.Sprintf(
				"this.closest('[popover]').hidePopover();if(window.__libroResizeApp){window.__libroResizeApp(%s,%s,%s)}",
				components.JSString(app.ID),
				components.JSString(string(w)),
				components.JSString(sid),
			))))
	}
	closeButton := r.Button("ws-button ws-panel-close").
		Attr("title", "Close panel").Attr("aria-label", "Close panel").
		Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("close")).
		OnClick(r.JS(fmt.Sprintf("event.stopPropagation();__ws.call('app.close',{sid:%s,id:%s})", components.JSString(sid), components.JSString(app.ID))))

	rightButtons := r.Div("flex gap-0.5 items-center shrink-0").
		Attr("data-size-badges", "").
		Render(
			r.Button("ws-size-trigger").Attr("data-size-trigger", "").
				Attr("aria-label", "Panel size: "+app.Width.ShortLabel()).
				Attr("aria-expanded", "false").Attr("aria-controls", "sizes-"+app.ID).
				Text(app.Width.ShortLabel()),
			r.Div("ws-size-picker").ID("sizes-"+app.ID).Attr("popover", "auto").
				Attr("role", "group").Attr("aria-label", "Panel size").Render(badges...),
		)

	// Left side of toolbar depends on app type
	var leftSide *r.Node
	if appDock(app) == "center" {
		leftSide = r.Div("ws-panel-title").Render(r.Span("").Text(workspaceAppName(app)))
	} else if app.PluginID == "files" {
		leftSide = r.Div("ws-panel-title").Render(r.I("material-icons-round").Text("folder_open"), r.Span("").Text("Files"))
	} else if app.Type == AppTypeURL {
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

		consoleBtn := r.Button(btnCls).
			Attr("title", "Open browser console").Attr("aria-label", "Open browser console").
			OnClick(r.JS(fmt.Sprintf(`if(window.__libroOpenConsole)window.__libroOpenConsole(%s)`, components.JSString(app.ID)))).
			Render(r.I("material-icons-round text-sm").Attr("aria-hidden", "true").Text("code"))

		zoomButtons := r.Div("flex items-center gap-0.5 shrink-0").
			Attr("role", "group").Attr("aria-label", "Browser zoom")
		for _, zoom := range []struct {
			label, icon string
			step        int
		}{
			{"Zoom out", "remove", -1},
			{"Reset zoom to 100%", "restart_alt", 0},
			{"Zoom in", "add", 1},
		} {
			zoomButtons.Render(r.Button(btnCls).
				Attr("title", zoom.label).Attr("aria-label", zoom.label).
				Render(r.I("material-icons-round text-sm").Attr("aria-hidden", "true").Text(zoom.icon)).
				OnClick(r.JS(fmt.Sprintf(`window.__libroWvZoom(%s,%d)`, components.JSString(app.ID), zoom.step))))
		}

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
			Attr("aria-label", "Browser address").
			On("keydown", r.JS(fmt.Sprintf(`if(event.key==='Enter'){event.preventDefault();if(window.__libroNavigateAddress(%s,event.target.value))event.target.blur();}`, components.JSString(app.ID))))

		// Site icon in badge
		globeBadgeCls := "inline-flex items-center justify-center w-6 h-6 rounded shrink-0"
		globeIconCls := "material-icons-round text-sm leading-none"
		if selected {
			globeBadgeCls += " bg-white"
			globeIconCls += " text-black"
		} else {
			globeBadgeCls += " bg-gray-800 dark:bg-zinc-900"
			globeIconCls += " text-white"
		}
		globeIcon := r.I(globeIconCls).Text("language")
		if fav := faviconURL(app.URL, 16); fav != "" {
			globeIcon = r.Img("w-4 h-4 rounded-sm").Attr("src", fav)
		}
		globe := r.Div(globeBadgeCls).Render(globeIcon)

		leftSide = r.Div("flex-1 min-w-0 flex items-center gap-1").
			Render(backBtn, forwardBtn, globe, urlInput, copyBtn, reloadBtn, consoleBtn, zoomButtons)
	} else if app.Type == AppTypeTerminal {
		labelText := workspaceAppName(app)

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
			iconNode = r.I("material-icons-round").Attr("aria-hidden", "true").Text("terminal")
		}
		leftSide = r.Div("ws-panel-title").Render(iconNode, r.Span("").Text(labelText))
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
	toolbar = toolbar.Attr("data-app-toolbar", "")
	if appDock(app) == "center" {
		toolbar = toolbar.Render(r.Div("ws-agent-tab").Render(leftSide, rightButtons))
	} else {
		toolbar = toolbar.Render(closeButton, leftSide, rightButtons)
	}

	return r.Div("group relative flex flex-col "+app.Width.ContainerClasses()+" h-full "+borderClass+" rounded-md overflow-hidden bg-white dark:bg-zinc-950 transition-colors duration-75").
		ID(fmt.Sprintf("frame-%s", app.ID)).
		Attr("data-app-id", app.ID).
		Attr("data-app-name", workspaceAppName(app)).
		Attr("data-app-type", string(app.Type)).
		Attr("data-dock", appDock(app)).
		Attr("data-plugin", pluginForApp(app).ID).
		Attr("style", appFrameStyle(app, index)).
		Render(
			toolbar,
			renderAppContent(app, sid, placeholder, clickOverlay),
		)
}

func renderAppContent(app Application, sid string, placeholder bool, clickOverlay *r.Node) *r.Node {
	content := renderIframe(app, fmt.Sprintf("frame-%s", app.ID), app.URL, sid)
	if placeholder {
		content = renderAppPlaceholder(app)
	}
	return r.Div("relative flex-1 min-h-0").
		ID(appContentID(app.ID)).
		Attr("data-app-content", app.ID).
		Render(
			content,
			clickOverlay,
		)
}

func renderAppPlaceholder(app Application) *r.Node {
	icon := "language"
	label := "Loading browser"
	hint := "If this takes a while, check the URL or close the app."
	if app.Type == AppTypeTerminal {
		icon = "terminal"
		label = "Starting terminal"
		hint = "If it stalls, close this panel and start the command again."
	}
	return r.Div("w-full h-full flex items-center justify-center bg-gray-50 dark:bg-zinc-950").
		Attr("data-app-placeholder", app.ID).
		Render(
			r.Div("flex flex-col items-center gap-2 text-gray-400 dark:text-zinc-600 text-center px-6").Render(
				r.I("material-icons-round text-3xl").Text(icon),
				r.Span("font-mono text-xs text-gray-500 dark:text-zinc-500").Text(label),
				r.Span("font-mono text-[11px] leading-relaxed max-w-xs text-gray-400 dark:text-zinc-700").Text(hint),
			),
		)
}

func renderIframe(app Application, frameID, iframeSrc, sid string) *r.Node {
	if app.PluginID == "files" {
		return renderFiles(app)
	}
	if app.Type == AppTypeURL {
		// Render both the Electron webview and an iframe fallback for plain
		// browser mode. Runtime JS decides which one is visible.
		webviewSrc := app.URL
		if webviewSrc == "" {
			webviewSrc = "about:blank"
		}
		wv := r.El("webview", "").
			ID(fmt.Sprintf("webview-%s", app.ID)).
			Attr("data-webview-app", app.ID).
			Attr("data-sid", sid).
			Attr("src", webviewSrc).
			Attr("partition", "persist:libro").
			Attr("allow", "microphone; camera; display-capture; speaker-selection; autoplay; clipboard-read; clipboard-write; fullscreen").
			Attr("allowpopups", "").
			Attr("style", "display:none;width:100%;height:100%")
		// Force closing tag by adding empty text content
		wv.Text("")
		browserSrc := iframeSrc
		if browserSrc == "" {
			browserSrc = "about:blank"
		}
		browserFallback := r.Iframe("w-full h-full border-0").
			ID("browser-fallback-"+app.ID).
			Attr("data-browser-iframe-app", app.ID).
			Attr("data-sid", sid).
			Attr("data-browser-src", browserSrc).
			Attr("loading", "lazy").
			Attr("style", "display:none")
		devtoolsCloseBtn := r.Button("hidden w-5 h-5 cursor-pointer pointer-events-auto items-center justify-center rounded-full bg-red-500 text-white shadow-sm ring-1 ring-red-600/50 hover:bg-red-600").
			ID(fmt.Sprintf("devtools-close-%s", app.ID)).
			Attr("title", "Close DevTools").
			Attr("onclick", fmt.Sprintf("if(window.__libroCloseConsole)window.__libroCloseConsole('%s')", app.ID)).
			Render(
				r.Span("block w-full text-center text-[9px] leading-5 font-semibold").Text("x"),
			)
		devtoolsControls := r.Div("absolute bottom-3 right-3 z-50 flex items-center gap-2 pointer-events-none").
			ID(fmt.Sprintf("devtools-wrap-%s", app.ID)).
			Render(
				devtoolsCloseBtn,
			)
		webviewWrapper := r.Div("relative flex-1 min-h-0").Render(
			wv,
			browserFallback,
			devtoolsControls,
		)
		devtoolsPanel := r.Div("hidden border-t border-stone-300 dark:border-stone-600 bg-stone-50 dark:bg-zinc-900").
			ID(fmt.Sprintf("devtools-panel-%s", app.ID)).
			Attr("style", "height:320px;min-height:160px;position:relative").
			Render(
				r.Div("w-full h-full").
					ID(fmt.Sprintf("devtools-host-%s", app.ID)),
			)
		fallbackNotice := r.Div("shrink-0 px-3 py-2 text-xs border-b border-gray-200 dark:border-zinc-700 bg-gray-50 dark:bg-zinc-900").
			Attr("data-browser-fallback-notice", app.ID).
			Attr("style", "display:none").
			Render(
				r.Span("").Text("Some sites block embedded previews. Use Libro desktop to browse inside this panel, or "),
				r.El("a", "underline").Text("Open in new tab").
					Attr("data-browser-external-link", app.ID).
					Attr("target", "_blank").Attr("rel", "noopener noreferrer"),
			)
		container := r.Div("w-full h-full absolute inset-0 z-30 flex flex-col").Render(fallbackNotice, webviewWrapper, devtoolsPanel)
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
	terminalID := app.TerminalID
	if terminalID == "" {
		terminalID = app.ID
	}
	return r.Div("relative w-full h-full overflow-hidden bg-zinc-950").
		ID(frameID).
		Attr("data-terminal", terminalID).
		Attr("data-terminal-app", app.ID).
		Attr("data-sid", sid).
		Attr("tabindex", "0").
		Render(
			r.Div("absolute inset-0 flex flex-col items-center justify-center gap-1 text-zinc-500 font-mono text-xs pointer-events-none text-center px-6").
				Render(
					r.Span("").Attr("data-terminal-status", "").Text("Connecting terminal"),
					r.Span("text-[11px] text-zinc-700").Text("If it stalls, close this panel and start the command again."),
				),
		)
}

// insertAppJS returns JS that inserts a new app frame into the existing strip.
// The node is compiled to JS and inserted after the left spacer (prepend) or before the right spacer (append).
func insertAppJS(node *r.Node, _ bool, projectName string) string {
	// CSS order on the app frame (set in renderAppFrame) handles visual positioning,
	// so we just append to the strip — no DOM repositioning needed.
	return node.ToJSAppend(stripID(projectName)) + `(function(){if(window.__libroEnforceAppWidthPolicy)window.__libroEnforceAppWidthPolicy();})();`
}

func settleAppFrameJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var appID=%s;
	function run(){
		if(window.__libroSettleAppFrame)window.__libroSettleAppFrame(appID);
	}
	run();
})();
`, components.JSString(appID))
}

// hideAllProjectsJS returns JS that hides all project divs inside the wrapper.
func hideAllProjectsJS() string {
	return fmt.Sprintf(`
(function(){
	var w=document.getElementById('%s');
	if(!w)return;
	var nodes=w.querySelectorAll(':scope > [id^="project-main-"]');
	for(var i=0;i<nodes.length;i++){
		var el=nodes[i];
		try{var active=document.activeElement;if(active&&el.contains(active)&&typeof active.blur==='function')active.blur();}catch(e){}
		// Keep inactive project DOM mounted so apps keep running, but remove it
		// from layout/paint. visibility:hidden leaves Electron webview and
		// xterm/WebGL compositor overlays behind on some systems.
		el.style.display='none';
		el.style.visibility='hidden';
		el.style.pointerEvents='none';
		el.style.position='absolute';
		el.style.inset='0';
		el.style.zIndex='0';
		el.setAttribute('aria-hidden','true');
	}
})();`, MainAreaID)
}

// showProjectJS returns JS that makes a project div visible.
func showProjectJS(projectName string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.getElementById('%s');
	if(!el)return;
	el.style.display='flex';
	el.style.visibility='visible';
	el.style.pointerEvents='';
	el.style.position='';
	el.style.inset='';
	el.style.zIndex='';
	el.removeAttribute('aria-hidden');
	if(window.__libroFitTerminalFrame){
		el.querySelectorAll('[data-terminal]').forEach(function(term){window.__libroFitTerminalFrame(term,true);});
	}
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

// closeDevtoolsForAppJS returns JS that closes the Electron devtools overlay
// for a specific app before its project is hidden.
func closeDevtoolsForAppJS(appID string) string {
	if appID == "" {
		return ""
	}
	return fmt.Sprintf(`
(function(){
	if(window.__libroCloseConsole) window.__libroCloseConsole(%s);
})();`, components.JSString(appID))
}

// closeDevtoolsForAppsJS closes Electron devtools overlays for all apps in a project
// before that project's DOM is hidden during a project/worktree switch.
func closeDevtoolsForAppsJS(apps []Application) string {
	if len(apps) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(apps))
	var js strings.Builder
	for _, app := range apps {
		if app.ID == "" {
			continue
		}
		if _, ok := seen[app.ID]; ok {
			continue
		}
		seen[app.ID] = struct{}{}
		js.WriteString(closeDevtoolsForAppJS(app.ID))
	}
	return js.String()
}

// focusSelectedAppJS returns JS that focuses the selected app's iframe after a short delay
// and updates the tracked selected app ID for shortcut handlers.
func focusSelectedAppJS(state *AppState) string {
	appID := ""
	if state.SelectedIndex >= 0 && state.SelectedIndex < len(state.Apps) {
		appID = state.Apps[state.SelectedIndex].ID
	}
	return fmt.Sprintf(`
window.__libroSelectedApp=%s;
if(window.__libroCenterSelectedApp) window.__libroCenterSelectedApp();
setTimeout(function(){
	if(window.__libroCenterSelectedApp) window.__libroCenterSelectedApp();
	if(window.__libroFocusApp) window.__libroFocusApp(%d);
}, 30);`, components.JSString(appID), state.SelectedIndex)
}

// removeAppJS disposes the app frame. Browser cookies stay in persist:libro;
// moving a live webview to another parent invalidates its Electron guest.
func removeAppJS(appID string) string {
	return parkFloatingPopupsJS() + fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(el)el.remove();
})();`, appID)
}

// renderURLPopup renders the URL/search popup opened by bare 'o'.
func renderURLPopup(sid string) *r.Node {
	return components.URLPopup(sid)
}

// renderResizePopup renders the app resize popup.
// Uses radio-style buttons navigable with j/k and confirmable with Enter.
func renderResizePopup(sid string) *r.Node {
	widths := AllWidths()
	strs := make([]string, 0, len(widths))
	for _, w := range widths {
		strs = append(strs, string(w))
	}
	return components.ResizePopup(sid, strs)
}

// renderCommandPopup renders the command palette for app-wide and app-specific commands.
func renderCommandPopup() *r.Node {
	return components.CommandPopup()
}

func renderMoveProjectPopup() *r.Node {
	return components.MoveProjectPopup()
}

// commandPopupJS returns the JS that powers the global command palette.
func commandPopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroCommandRegistered)return;
	window.__libroCommandRegistered=true;

	var dlg=document.getElementById('%s');
	var inp=document.getElementById('command-popup-input');
	var res=document.getElementById('command-popup-results');
	var selIdx=0;
	var filtered=[];
	var hoverEnabled=false;

	function armHoverAfterPointerMove(){
		hoverEnabled=false;
		if(!dlg)return;
		var enableHover=function(){
			hoverEnabled=true;
			dlg.removeEventListener('mousemove',enableHover,true);
		};
		dlg.addEventListener('mousemove',enableHover,true);
	}

	function fuzzyMatch(text,query){
		text=(text||'').toLowerCase();query=(query||'').toLowerCase();
		var ti=0,qi=0,score=0,lastMatch=-1;
		while(ti<text.length&&qi<query.length){
			if(text[ti]===query[qi]){
				score+=1;
				if(lastMatch===ti-1)score+=2;
				if(ti===0||text[ti-1]===' '||text[ti-1]==='/'||text[ti-1]==='-'||text[ti-1]==='_')score+=3;
				lastMatch=ti;qi++;
			}
			ti++;
		}
		return qi===query.length?score:0;
	}

	function selectedAppInfo(){
		var appId=window.__libroSelectedApp||'';
		if(!appId)return null;
		var el=document.querySelector('[data-app-id="'+appId+'"]');
		if(!el)return null;
		return {
			id: appId,
			isBrowser: !!el.querySelector('webview[data-webview-app], iframe[data-browser-iframe-app]'),
			isTerminal: !!el.querySelector('[data-terminal]')
		};
	}

	function commandDefinitions(){
		var selected=selectedAppInfo();
		var commands=[
            {id:'new-agent',label:'New agent',scope:'workspace',icon:'add',keywords:'agent codex claude pi launch',run:function(){closePalette();libroWorkspace.launcher('center');}},
            {id:'open-tool',label:'Open tool',scope:'workspace',icon:'apps',keywords:'tools plugins browser files editor git',run:function(){closePalette();libroWorkspace.launcher('right');}},
            {id:'terminal',label:'Toggle terminal tool',scope:'tools',icon:'terminal',keywords:'shell console',run:function(){closePalette();libroWorkspace.tool('terminal');}},
            {id:'browser',label:'Toggle browser',scope:'tools',icon:'language',keywords:'web preview',run:function(){closePalette();libroWorkspace.tool('browser');}},
            {id:'new-browser',label:'New browser panel',scope:'tools',icon:'add',keywords:'web blank browser',run:function(){closePalette();libroWorkspace.newBrowser();}},
            {id:'previous-browser',label:'Previous browser panel',scope:'tools',icon:'chevron_left',keywords:'web browser navigate',run:function(){closePalette();libroWorkspace.navigateBrowser(-1);}},
            {id:'next-browser',label:'Next browser panel',scope:'tools',icon:'chevron_right',keywords:'web browser navigate',run:function(){closePalette();libroWorkspace.navigateBrowser(1);}},
            {id:'files',label:'Toggle files',scope:'tools',icon:'folder_open',keywords:'tree explorer files',run:function(){closePalette();libroWorkspace.tool('files');}},
            {id:'nvim',label:'Toggle Nvim',scope:'tools',icon:'edit',keywords:'editor vim',run:function(){closePalette();libroWorkspace.tool('nvim');}},
            {id:'lazyrepo',label:'Toggle Git',scope:'tools',icon:'account_tree',keywords:'repository changes',run:function(){closePalette();libroWorkspace.tool('lazyrepo');}},
            {id:'lazydata',label:'Toggle database',scope:'tools',icon:'storage',keywords:'data sql',run:function(){closePalette();libroWorkspace.tool('lazydata');}},
            {id:'bottom-terminal',label:'Toggle bottom terminal',scope:'workspace',icon:'vertical_align_bottom',shortcut:'Ctrl+'+String.fromCharCode(96),keywords:'shell bottom',run:function(){closePalette();libroWorkspace.bottom();}},
            {id:'settings',label:'Settings',scope:'workspace',icon:'settings',keywords:'preferences commands shortcuts agents',run:function(){closePalette();libroWorkspace.settings();}},
			{id:'console',label:'App console',scope:'app',icon:'code',keywords:'devtools app console inspector developer tools',run:function(){
				closePalette();
				if(window.libroElectron&&window.libroElectron.toggleDevTools)window.libroElectron.toggleDevTools();
			}},
			{id:'zoom-in',label:'Zoom in',scope:'app',icon:'zoom_in',keywords:'zoom increase larger bigger app scale',run:function(){
				closePalette();
				if(window.libroWorkspace)libroWorkspace.zoom('zoom-in');
			}},
			{id:'zoom-out',label:'Zoom out',scope:'app',icon:'zoom_out',keywords:'zoom decrease smaller app scale',run:function(){
				closePalette();
				if(window.libroWorkspace)libroWorkspace.zoom('zoom-out');
			}},
			{id:'zoom-reset',label:'Reset zoom',scope:'app',icon:'filter_center_focus',keywords:'zoom reset normal default app scale',run:function(){
				closePalette();
				if(window.libroWorkspace)libroWorkspace.zoom('zoom-reset');
			}},

			{id:'project-picker',label:'Switch project',scope:'app',icon:'source',keywords:'projects switch dialog sidebar navigation tree',run:function(){
				closePalette();
				if(window.__libroOpenProjectDialogSearch)window.__libroOpenProjectDialogSearch();
			}},
			{id:'project-new',label:'New project (browse folder)',scope:'app',icon:'create_new_folder',keywords:'new project create folder browse directory path filesystem',run:function(){
				closePalette();
				if(window.__libroOpenProjectDialogBrowse)window.__libroOpenProjectDialogBrowse();
			}},
			{id:'worktree-new',label:'New worktree from current branch',scope:'project',icon:'alt_route',keywords:'worktree branch git fork create new',run:function(){
				closePalette();
				if(window.__libroOpenWorktreeCreate)window.__libroOpenWorktreeCreate();
			}},
			{id:'close-project',label:'Close project',scope:'project',icon:'close',keywords:'close stop all panels terminals processes current project',run:function(){
				closePalette();
				__ws.call('project.close',{sid:window.__libroWorkspaceSID});
			}},
			{id:'project-remove',label:'Remove current project',scope:'project',icon:'delete_outline',keywords:'remove delete current active project unregister forget drop',run:function(){
				closePalette();
				var list=window.__libroProjects||[];
				var active=null;
				for(var i=0;i<list.length;i++){if(list[i].isActive){active=list[i];break;}}
				if(!active){if(window.__libroShowToast)window.__libroShowToast('No active project','',2000);return;}
				if(active.kind==='worktree'){if(window.__libroShowToast)window.__libroShowToast('Cannot remove a worktree from here','Use git worktree remove instead',2500);return;}
				var run=function(){__ws.call('project.remove',{sid:'%s',name:active.name});};
				if(window.__libroConfirmAction){window.__libroConfirmAction('Remove project?', 'Remove project "'+active.name+'" from Libro?\n\nThis only removes it from the project list. Files on disk are kept.', run);}else{run();}
			}},

		];
        if(selected){
            commands.push({id:'close-panel',label:'Close panel',scope:'selected panel',icon:'close',keywords:'close stop terminal agent tool',run:function(){closePalette();__ws.call('app.close',{sid:window.__libroWorkspaceSID,id:selected.id});}});
            commands.push({
				id:'resize',
				label:'Resize panel',
				scope:'selected panel',
				icon:'aspect_ratio',
				keywords:'width active app panel size',
				run:function(){
					closePalette();
					if(window.__libroOpenResizePopup)window.__libroOpenResizePopup();
				},
			});
			commands.push({
				id:'move-project',
				label:'Move to project',
				scope:'selected panel',
				icon:'drive_file_move',
				keywords:'move selected app window to another project transfer switch',
				run:function(){
					closePalette();
					if(window.__libroOpenMoveProject)window.__libroOpenMoveProject();
				},
			});
		}
		if(selected&&selected.isBrowser){
			commands.push({
				id:'console-browser',
				label:'Browser console',
				scope:'selected browser',
				icon:'terminal',
				keywords:'devtools inspector selected browser webview console',
				run:function(){
					closePalette();
					if(window.__libroOpenConsole)window.__libroOpenConsole(selected.id);
				},
			});
		}
		if(selected&&selected.isTerminal){
			commands.push({
				id:'restart-terminal',
				label:'Restart terminal backend',
				scope:'selected terminal',
				icon:'restart_alt',
				keywords:'terminal pty kill restart emergency reset backend websocket session',
				run:function(){
					closePalette();
					__ws.call('app.terminal.restart',{sid:'%s',id:selected.id});
				},
			});
		}
		return commands;
	}

	function render(){
		var dk=document.documentElement.classList.contains('dark');
		res.innerHTML='';
		if(filtered.length===0){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No commands</div>';
			return;
		}
		filtered.forEach(function(cmd,i){
			var row=document.createElement('div');
			var sel=i===selIdx;
			row.className='ws-command-row';
			row.setAttribute('data-selected',String(sel));
			row.innerHTML='<i class="material-icons-round" aria-hidden="true">'+cmd.icon+'</i>'
				+'<span class="ws-command-label">'+cmd.label+'</span>'
				+'<span class="ws-command-scope">'+cmd.scope+'</span>'
				+'<span class="ws-command-enter" aria-hidden="true">↵</span>';
			var shortcut=cmd.shortcut||(window.libroWorkspace&&libroWorkspace.shortcutFor(cmd.id));
			if(shortcut)row.querySelector('.ws-command-enter').textContent=shortcut;
			row.onmouseenter=function(){
				if(!hoverEnabled)return;
				if(selIdx===i)return;
				selIdx=i;
				render();
			};
			row.onclick=function(){execute();};
			res.appendChild(row);
		});
		var sel=res.children[selIdx];
		if(sel)sel.scrollIntoView({block:'nearest'});
	}

	function filter(){
		var q=(inp.value||'').trim();
		var commands=commandDefinitions();
		if(!q){
			filtered=commands;
		}else{
			filtered=commands.map(function(cmd){
				var hay=cmd.id+' '+cmd.label+' '+cmd.scope+' '+(cmd.keywords||'');
				return {cmd:cmd,score:fuzzyMatch(hay,q)};
			}).filter(function(entry){return entry.score>0;})
				.sort(function(a,b){return b.score-a.score;})
				.map(function(entry){return entry.cmd;});
		}
		selIdx=0;
		render();
	}

	function navigateSelection(delta){
		if(delta>0&&selIdx<filtered.length-1){selIdx++;render();}
		else if(delta<0&&selIdx>0){selIdx--;render();}
	}

	function paletteNormalKey(e){
		if(dlg.classList.contains('hidden'))return;
		if(document.activeElement===inp)return;
		if(e.key==='i'){
			e.preventDefault();e.stopImmediatePropagation();inp.focus();return;
		}
		if(e.key==='j'||e.key==='ArrowDown'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(1);return;
		}
		if(e.key==='k'||e.key==='ArrowUp'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(-1);return;
		}
		if(e.key==='Enter'){
			e.preventDefault();e.stopImmediatePropagation();execute();return;
		}
		if(e.key==='Escape'){
			e.preventDefault();e.stopImmediatePropagation();closePalette();return;
		}
	}

	function execute(){
		if(filtered.length===0)return;
		filtered[selIdx].run();
	}

	function openPalette(){
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		armHoverAfterPointerMove();
		setTimeout(function(){inp.focus();},50);
	}

	function closePalette(){
		dlg.classList.add('hidden');
		inp.value='';
		hoverEnabled=false;
	}

	inp.addEventListener('input',filter);
	inp.addEventListener('keydown',function(e){
		e.stopImmediatePropagation();
		if(e.key==='ArrowDown'){
			e.preventDefault();
			navigateSelection(1);
		}else if(e.key==='ArrowUp'){
			e.preventDefault();
			navigateSelection(-1);
		}else if(e.key==='Enter'){
			e.preventDefault();
			execute();
		}else if(e.key==='Escape'){
			e.preventDefault();
			inp.blur();
		}
	});
	document.addEventListener('keydown',paletteNormalKey,true);

window.__libroOpenCommandPalette=openPalette;
})();
`, CommandPopupID, sid, sid)
}

// renderWorktreeCreatePopup renders the popup used to create a new worktree
// from the current branch (Cmd+G).
func renderWorktreeCreatePopup() *r.Node {
	return components.WorktreeCreatePopup()
}

// worktreeCreatePopupJS wires the worktree-create popup: input handling,
// branch submission, and Esc-to-close.
func worktreeCreatePopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('worktree-create-input');}
	function getCtx(){return document.getElementById('worktree-create-context');}
	function getList(){return document.getElementById('worktree-create-branches');}

	function escapeHtml(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}

	function activeProject(){
		var projects=window.__libroProjects||[];
		for(var i=0;i<projects.length;i++){
			if(projects[i].isActive)return projects[i];
		}
		return null;
	}

	function findParentProject(active){
		if(!active)return null;
		if(active.kind!=='worktree')return active;
		var projects=window.__libroProjects||[];
		for(var i=0;i<projects.length;i++){
			if(projects[i].kind==='project'&&projects[i].name===active.name)return projects[i];
		}
		return null;
	}

	function renderBranches(active,parent){
		var list=getList();
		if(!list)return;
		var dk=document.documentElement.classList.contains('dark');
		var branches=(parent&&parent.branches)||[];
		var refs=(parent&&parent.worktreeRefs)||[];
		var refSet={};
		for(var i=0;i<refs.length;i++)refSet[refs[i]]=true;
		var forkBase=active.kind==='worktree'?active.branch:(parent&&parent.currentBranch)||'';
		if(branches.length===0){
			list.innerHTML='<div class="px-3 py-3 text-[11px] font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No branches found</div>';
			return;
		}
		var sorted=branches.slice().sort(function(a,b){
			if(a===forkBase)return -1;
			if(b===forkBase)return 1;
			return a.localeCompare(b);
		});
		var html='';
		for(var j=0;j<sorted.length;j++){
			var b=sorted[j];
			var isFork=b===forkBase;
			var inUse=!!refSet[b]&&!isFork;
			var rowCls='flex items-center gap-2 px-3 py-1.5 text-xs font-mono border-l-2 ';
			if(isFork){
				rowCls+=(dk?'bg-blue-900/30 border-blue-500 text-blue-200':'bg-blue-50 border-blue-500 text-blue-700');
			}else{
				rowCls+='border-transparent '+(dk?'text-zinc-400':'text-gray-600');
			}
			html+='<div class="'+rowCls+'">';
			html+='<i class="material-icons-round text-sm '+(isFork?(dk?'text-blue-300':'text-blue-500'):(dk?'text-zinc-600':'text-gray-400'))+'">'+(isFork?'arrow_right':(inUse?'alt_route':'commit'))+'</i>';
			html+='<span class="flex-1 truncate">'+escapeHtml(b)+'</span>';
			if(isFork){
				html+='<span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded '+(dk?'bg-blue-500/30 text-blue-200':'bg-blue-100 text-blue-700')+'">Fork base</span>';
			}else if(inUse){
				html+='<span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded '+(dk?'bg-zinc-700 text-zinc-400':'bg-gray-200 text-gray-500')+'">In use</span>';
			}
			html+='</div>';
		}
		list.innerHTML=html;
	}

	function openPopup(){
		var dlg=getDlg();
		var inp=getInp();
		var ctx=getCtx();
		if(!dlg||!inp)return;
		var active=activeProject();
		if(!active||!active.isGit){
			if(window.__libroShowToast)window.__libroShowToast('Not a git repository','Switch to a git project first',2200);
			return;
		}
		var parent=findParentProject(active);
		if(ctx){
			var forkBase=active.kind==='worktree'?active.branch:((parent&&parent.currentBranch)||'(unknown)');
			var projName=active.kind==='worktree'?active.name:active.name;
			ctx.textContent='From: '+projName+' @ '+forkBase;
		}
		renderBranches(active,parent);
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		inp.value='';
		setTimeout(function(){inp.focus();},50);
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(dlg)dlg.classList.add('hidden');
		if(inp)inp.value='';
	}

	function submit(){
		var inp=getInp();
		if(!inp)return;
		var name=(inp.value||'').trim();
		if(!name)return;
		closePopup();
		__ws.call('worktree.create',{sid:'%s',branch:name});
	}

	var inp=getInp();
	if(inp){
		inp.addEventListener('keydown',function(e){
			var dlg=getDlg();
			if(!dlg||dlg.classList.contains('hidden'))return;
			e.stopImmediatePropagation();
			if(e.key==='Enter'){e.preventDefault();submit();}
			else if(e.key==='Escape'){e.preventDefault();closePopup();}
		});
	}

	window.__libroOpenWorktreeCreate=openPopup;
})();
`, WorktreeCreatePopupID, sid)
}

// resizePopupJS returns JS that powers the app resize popup.
// Supports j/k keyboard navigation and Enter to confirm.
func resizePopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var currentAppId='';
	var focusedIndex=-1;

	function getDlg(){return document.getElementById('%s');}

	function findSelectedApp(){
		return window.__libroSelectedApp||'';
	}

	function getCurrentWidth(appId){
		var el=document.querySelector('[data-app-id="'+appId+'"]');
		if(!el)return'lg';
		var cls=el.className;
		if(cls.indexOf('w-full')!==-1)return'full';
		if(cls.indexOf('w-[2560px]')!==-1)return'3xl';
		if(cls.indexOf('w-[1920px]')!==-1)return'2xl';
		if(cls.indexOf('w-[1280px]')!==-1)return'xl';
		if(cls.indexOf('w-[960px]')!==-1)return'lg';
		if(cls.indexOf('w-[640px]')!==-1)return'md';
		if(cls.indexOf('w-[320px]')!==-1)return'xs';
		if(cls.indexOf('w-[480px]')!==-1)return'sm';
		return'lg';
	}

	function getBtns(){var d=getDlg();return d?d.querySelectorAll('.resize-btn'):[];}
	function widthAllowed(w){return !window.__libroIsAppWidthAllowed||window.__libroIsAppWidthAllowed(w);}
	function btnAllowed(btn){return btn&&widthAllowed(btn.getAttribute('data-resize-width')||'');}
	function nextAllowedIndex(from,delta){
		var btns=getBtns();
		if(!btns.length)return -1;
		var idx=from;
		for(var i=0;i<btns.length;i++){
			idx=(idx+delta+btns.length)%%btns.length;
			if(btnAllowed(btns[idx]))return idx;
		}
		return -1;
	}

	function highlightFocused(idx){
		var btns=getBtns();
		if(idx<0||idx>=btns.length||!btnAllowed(btns[idx]))return;
		btns.forEach(function(b,i){
			b.setAttribute('aria-selected',String(i===idx));
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
		if(window.__libroRefreshWidthAvailability)window.__libroRefreshWidthAvailability(getDlg());
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
		if(window.__libroClampAppWidth)curWidth=window.__libroClampAppWidth(curWidth);
		if(window.__libroRefreshWidthAvailability)window.__libroRefreshWidthAvailability(dlg);
		var btns=getBtns();
		var idx=0;
		btns.forEach(function(b,i){
			if(b.getAttribute('data-resize-width')===curWidth)idx=i;
		});
		if(!btnAllowed(btns[idx]))idx=nextAllowedIndex(-1,1);
		if(btns.length>0)highlightFocused(idx);
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		setTimeout(function(){dlg.focus();},50);
	}

	function closePopup(){
		var appId=currentAppId;
		var dlg=getDlg();
		if(dlg)dlg.classList.add('hidden');
		if(window.__libroParkFloatingPopups)window.__libroParkFloatingPopups();
		currentAppId='';
		focusedIndex=-1;
		if(appId&&window.__libroFocusAppByID){
			setTimeout(function(){window.__libroFocusAppByID(appId);},0);
		}
	}

	function confirmSelection(){
		var btns=getBtns();
		if(focusedIndex<0||focusedIndex>=btns.length||!currentAppId)return;
		var w=btns[focusedIndex].getAttribute('data-resize-width');
		if(!w)return;
		if(window.__libroResizeApp){
			if(window.__libroResizeApp(currentAppId,w,'%s'))closePopup();
			return;
		}
		__ws.call('app.resize',{sid:'%s',id:currentAppId,width:w});
		closePopup();
	}

	document.addEventListener('click',function(e){
		var btn=e.target.closest('.resize-btn');
		var dlg=getDlg();
		if(!btn||!dlg||!dlg.contains(btn))return;
		e.stopPropagation();
		if(!btnAllowed(btn)){
			if(window.__libroShowToast)window.__libroShowToast('3XL unavailable','Screen is Full HD or smaller',1800);
			return;
		}
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
			var next=nextAllowedIndex(focusedIndex,1);
			highlightFocused(next);
			return;
		}
		if(e.key==='k'||e.key==='ArrowUp'){
			e.preventDefault();e.stopImmediatePropagation();
			var prev=nextAllowedIndex(focusedIndex,-1);
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
`, ResizePopupID, sid, sid)
}

// renderCloseDialog renders the close confirmation dialog (hidden by default).
// It is populated dynamically via JS when the user attempts to close the window.
func renderCloseDialog(sid string) *r.Node {
	return components.CloseDialog(sid)
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
		}
	},true);
})();
`, sid, CloseDialogID)
}

// resizeJS returns JS that updates an app frame's width without replacing the DOM
func resizeJS(_ *AppState, width Width, appID string) string {
	// Build a map of width value -> container classes
	widthMap := ""
	pixelMap := ""
	for _, w := range AllWidths() {
		if widthMap != "" {
			widthMap += ","
		}
		if pixelMap != "" {
			pixelMap += ","
		}
		widthMap += fmt.Sprintf("'%s':'%s'", string(w), w.ContainerClasses())
		pixelMap += fmt.Sprintf("'%s':'%s'", string(w), w.PixelWidth())
	}

	return fmt.Sprintf(`
(function(){
	var el = document.querySelector('[data-app-id="%s"]');
	if (!el) return;

	var widths = {%s};
	var pixels = {%s};
	var newWidth = '%s';
	var newCls = widths[newWidth];
	var newPixel = pixels[newWidth] || '960px';

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
	el.style.width = newPixel;
	el.style.flex = '0 0 ' + newPixel;

	// Update size badges: highlight active, dim others
	var topBar = el.querySelector('[data-size-badges]');
	if (topBar) {
		var trigger = topBar.querySelector('[data-size-trigger]');
		var sizeLabel = newWidth === 'full' ? 'MAX' : newWidth.toUpperCase();
		if(trigger){trigger.textContent=sizeLabel;trigger.setAttribute('aria-label','Panel size: '+sizeLabel);}
		var btns = topBar.querySelectorAll('[data-resize-width]');
		var sizeLabels = ['XS','SM','MD','LG','XL','2XL','3XL','MAX'];
		var activeBase = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
		btns.forEach(function(b){
			var txt = b.textContent.trim();
			if (sizeLabels.indexOf(txt) === -1) return;
			b.setAttribute('aria-pressed',String(txt === sizeLabel));
			var isSelected = el.children[0] && el.children[0].className.indexOf('bg-blue-600') !== -1;
			if (txt === sizeLabel) {
				b.className = activeBase + (isSelected ? ' bg-white/25 text-white' : ' bg-blue-600 text-white');
			} else {
				b.className = activeBase + (isSelected ? ' text-blue-100/70 hover:text-white hover:bg-white/15' : ' text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700');
			}
		});
	}
	if(window.__libroRefreshWidthAvailability)window.__libroRefreshWidthAvailability(el);

	requestAnimationFrame(function(){
		if(window.__libroScrollToApp)window.__libroScrollToApp(el);
		requestAnimationFrame(function(){
			if(window.__libroScrollToApp)window.__libroScrollToApp(el);
		});
		setTimeout(function(){
			if(window.__libroScrollToApp)window.__libroScrollToApp(el);
		}, 80);
	});
	if((window.__libroSelectedApp||'')==='%s'&&window.__libroFocusAppByID){
		window.__libroFocusAppByID('%s');
		setTimeout(function(){window.__libroFocusAppByID('%s');},40);
		setTimeout(function(){window.__libroFocusAppByID('%s');},120);
	}
})();
`, appID, widthMap, pixelMap, string(width), appID, appID, appID, appID)
}

// renderTopBar renders the workspace title and panel controls.
func renderTopBar(state *AppState, sid string) *r.Node {
	return renderWorkspaceTopBar(state, sid)
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
						Attr("src", faviconURL(app.URL, 16))
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
			Attr("data-libro-no-drag", "true").
			Attr("style", "-webkit-app-region:no-drag").
			Attr("aria-label", "Select "+label).
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
	`, state.SelectedIndex, components.JSString(selectedCls), components.JSString(normalCls))
}

// projectDialogJS wires the unified project dialog: project search keeps
// the existing switch behavior, while empty searches fall through to folder
// lookup so the same dialog can open a directory as a new project.
func projectDialogJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroProjectDialogRegistered){
		if(window.__libroProjectDialogBind)window.__libroProjectDialogBind();
		return;
	}
	window.__libroProjectDialogRegistered=true;
	var selectedIdx=0;
	var filtered=[];
	var dirMatches=[];
	var hoverEnabled=false;
	var lookupSeq=0;
	var lookupTimer=0;
	var lookupQuery='';
	var lookupLoading=false;
	var documentKeydownBound=false;

	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('project-input');}
	function getResults(){return document.getElementById('project-results');}
	function query(){return (getInp()&&getInp().value||'').trim();}
	function isPathQuery(q){
		q=q||query();
		return q==='~'||q.indexOf('~/')===0||q.indexOf('./')===0||q.indexOf('../')===0||q.charAt(0)==='/';
	}
	function inDirectoryMode(){return isPathQuery()||(query()!==''&&filtered.length===0);}

	function armHoverAfterPointerMove(){
		hoverEnabled=false;
		var dlg=getDlg();
		if(!dlg)return;
		var enableHover=function(){
			hoverEnabled=true;
			dlg.removeEventListener('mousemove',enableHover,true);
		};
		dlg.addEventListener('mousemove',enableHover,true);
	}

	function fuzzyMatch(text,query){
		text=(text||'').toLowerCase();
		query=(query||'').toLowerCase();
		var ti=0,qi=0,score=0,lastMatch=-1;
		while(ti<text.length&&qi<query.length){
			if(text[ti]===query[qi]){
				score+=1;
				if(lastMatch===ti-1)score+=2;
				if(ti===0||text[ti-1]===' '||text[ti-1]==='/'||text[ti-1]==='.')score+=3;
				lastMatch=ti;
				qi++;
			}
			ti++;
		}
		return qi===query.length?score:0;
	}

	function escapeHtml(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
	function itemIcon(item){return item.kind==='worktree'?'alt_route':(item.isGit?'source':'folder');}
	function homeDir(){
		var dlg=document.getElementById('project-dialog');
		return dlg&&dlg.getAttribute?dlg.getAttribute('data-project-home')||'':'';
	}
	function simplifyPath(path){
		path=path||'';
		var home=homeDir();
		if(home&&path===home)return '~';
		if(home&&path.indexOf(home+'/')===0)return '~/'+path.substring(home.length+1);
		return path;
	}
	function leafName(name){
		name=name||'';
		var parts=name.split('/').filter(Boolean);
		return parts.length?parts[parts.length-1]:name;
	}

	function renderProjectItems(res,dk){
		if(isPathQuery()||filtered.length===0){return false;}
		var html='';
		filtered.forEach(function(item,i){
			var sel=i===selectedIdx;
			var icon=itemIcon(item);
			var primary=item.displayName||leafName(item.path)||item.name;
			var secondary=item.kind==='worktree'?(item.name+' · '+simplifyPath(item.path)):simplifyPath(item.path);
			var activeBadge=item.isActive?'<span class="ml-2 inline-flex items-center justify-center px-1.5 h-4 rounded text-[9px] font-bold leading-none '+(dk?'bg-blue-500/30 text-blue-300':'bg-blue-100 text-blue-700')+'">ACTIVE</span>':'';
			var tempBadge=item.transient?'<span class="ml-2 inline-flex items-center justify-center px-1.5 h-4 rounded text-[9px] font-bold leading-none '+(dk?'bg-zinc-700 text-zinc-300':'bg-gray-100 text-gray-600')+'">TEMP</span>':'';
			html+='<div class="project-item group flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'))
				+'" data-project-idx="'+i+'">';
			html+='<i class="material-icons-round '+(dk?'text-zinc-400':'text-gray-400')+' text-lg">'+icon+'</i>';
			html+='<div class="flex-1 min-w-0"><div class="text-sm truncate '+(dk?'text-zinc-200':'text-gray-800')+'">'+escapeHtml(primary)+activeBadge+tempBadge+'</div>';
			html+='<div class="text-[11px] truncate '+(dk?'text-zinc-500':'text-gray-400')+'">'+escapeHtml(secondary)+'</div></div>';
			html+='<div class="ml-auto shrink-0 flex items-center justify-end gap-2 w-24">';
			if(item.kind!=='worktree'){
				html+='<button data-project-remove="'+i+'" title="Remove project" class="opacity-0 group-hover:opacity-100 hover:opacity-100 transition-opacity p-1 rounded '+(dk?'hover:bg-red-900/40 text-zinc-500 hover:text-red-300':'hover:bg-red-50 text-gray-400 hover:text-red-600')+'"><i class="material-icons-round text-base pointer-events-none">delete_outline</i></button>';
			}else{
				html+='<span class="w-6 h-6 shrink-0"></span>';
			}
			html+='</div></div>';
		});
		res.innerHTML=html;
		res.querySelectorAll('[data-project-idx]').forEach(function(el){
			el.addEventListener('mouseenter',function(){
				if(!hoverEnabled)return;
				var idx=parseInt(el.getAttribute('data-project-idx'),10);
				if(!Number.isNaN(idx)&&idx!==selectedIdx){selectedIdx=idx;render();}
			});
			el.addEventListener('mousedown',function(e){
				if(e.target&&e.target.closest&&e.target.closest('[data-project-remove]'))return;
				e.preventDefault();
				var idx=parseInt(el.getAttribute('data-project-idx'),10);
				if(Number.isNaN(idx))return;
				selectedIdx=idx;
				launchProject();
			});
		});
		res.querySelectorAll('[data-project-remove]').forEach(function(btn){
			btn.addEventListener('mousedown',function(e){e.preventDefault();e.stopPropagation();});
			btn.addEventListener('click',function(e){
				e.preventDefault();e.stopPropagation();
				var idx=parseInt(btn.getAttribute('data-project-remove'),10);
				if(!Number.isNaN(idx))removeAt(idx);
			});
		});
		return true;
	}

	function renderDirectoryItems(res,dk){
		var q=query();
		if(!q){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">Type a project name, branch, folder name, or absolute path</div>';
			return;
		}
		if(lookupLoading&&dirMatches.length===0){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">Searching folders…</div>';
			return;
		}
		if(dirMatches.length===0){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No folder found. Press <span class="text-blue-500">Enter</span> to create it as a new Libro project/folder.</div>';
			return;
		}
		var html='<div class="px-4 py-2 text-[10px] font-mono uppercase tracking-wide '+(dk?'text-zinc-500 bg-zinc-900':'text-gray-400 bg-gray-50')+'">Open folder as new project</div>';
		dirMatches.forEach(function(item,i){
			var sel=i===selectedIdx;
			var icon=item.parent?'arrow_upward':'folder';
			html+='<div class="project-dir-item flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500'):(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'))
				+'" data-dir-idx="'+i+'">';
			html+='<i class="material-icons-round '+(item.parent?(dk?'text-zinc-500':'text-gray-400'):'text-amber-500 dark:text-amber-400')+' text-lg">'+icon+'</i>';
			html+='<div class="flex-1 min-w-0"><div class="text-sm truncate '+(dk?'text-zinc-200':'text-gray-800')+'">'+escapeHtml(item.name)+'</div>';
			html+='<div class="text-[11px] truncate '+(dk?'text-zinc-500':'text-gray-400')+'">'+escapeHtml(item.path)+'</div></div>';
			html+='</div>';
		});
		res.innerHTML=html;
		res.querySelectorAll('[data-dir-idx]').forEach(function(el){
			el.addEventListener('mouseenter',function(){
				if(!hoverEnabled)return;
				var idx=parseInt(el.getAttribute('data-dir-idx'),10);
				if(!Number.isNaN(idx)&&idx!==selectedIdx){selectedIdx=idx;render();}
			});
			el.addEventListener('mousedown',function(e){
				e.preventDefault();
				var idx=parseInt(el.getAttribute('data-dir-idx'),10);
				if(Number.isNaN(idx))return;
				selectedIdx=idx;
				openSelectedDirectory(true);
			});
		});
	}

	function render(){
		var res=getResults();
		if(!res)return;
		var dk=document.documentElement.classList.contains('dark');
		if(renderProjectItems(res,dk)){}else{renderDirectoryItems(res,dk);}
		var selected=res.querySelector('[data-project-idx="'+selectedIdx+'"],[data-dir-idx="'+selectedIdx+'"]');
		if(selected)selected.scrollIntoView({block:'nearest'});
	}

	function scheduleLookup(){
		var q=query();
		clearTimeout(lookupTimer);
		hideCreateConfirm();
		if(!q||(!isPathQuery(q)&&filtered.length>0)){dirMatches=[];lookupLoading=false;render();return;}
		lookupLoading=true;
		render();
		lookupTimer=setTimeout(function(){
			lookupQuery=q;
			__ws.call('project.lookup',{sid:'%s',query:q,seq:++lookupSeq});
		},90);
	}

	function sortProjects(a,b){
		if((b.score||0)!==(a.score||0))return (b.score||0)-(a.score||0);
		return String(a.displayName||a.name||'').localeCompare(String(b.displayName||b.name||''));
	}

	function filter(){
		var q=query();
		var all=(window.__libroProjects||[]).slice();
		if(!q){filtered=all;filtered.sort(sortProjects);}else if(isPathQuery(q)){filtered=[];}else{
			filtered=[];
			all.forEach(function(item){
				var hay=item.name+' '+(item.displayName||'')+' '+(item.branch||'')+' '+(item.path||'');
				var score=fuzzyMatch(hay,q);
				if(score>0){filtered.push(Object.assign({score:score},item));}
			});
			filtered.sort(sortProjects);
		}
		selectedIdx=0;
		for(var i=0;i<filtered.length;i++){if(filtered[i].isActive){selectedIdx=i;break;}}
		scheduleLookup();
		render();
	}

	function hideCreateConfirm(){
		var bar=document.getElementById('project-path-confirm');
		if(bar){bar.classList.add('hidden');bar.dataset.path='';}
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(dlg)dlg.classList.add('hidden');
		if(inp)inp.value='';
		hideCreateConfirm();
		dirMatches=[];
		hoverEnabled=false;
	}

	function launchProject(){
		if(filtered.length===0)return;
		var item=filtered[selectedIdx];
		closePopup();
		if(item.kind==='worktree'){
			__ws.call('worktree.switch',{sid:'%s',project:item.name,path:item.path,branch:item.branch});
		}else{
			history.replaceState(null,'','#'+item.name);
			__ws.call('project.switch',{sid:'%s',name:item.name});
		}
	}

	function selectedDirectory(){
		if(dirMatches.length===0)return null;
		return dirMatches[Math.max(0,Math.min(selectedIdx,dirMatches.length-1))]||null;
	}
	function typedDirectoryPath(){
		var q=query();
		if(!q)return '';
		// If the user typed an explicit directory and ended it with '/', Enter
		// should open that typed directory, not the highlighted child or '..'.
		if(isPathQuery(q)&&/\/$/.test(q)&&q!=='/'){
			return q.replace(/\/+$/,'');
		}
		return '';
	}
	function newProjectPathFromQuery(){
		var q=query();
		if(!q)return '';
		if(q==='~')return homeDir();
		if(q.indexOf('~/')===0)return homeDir()?homeDir()+'/'+q.substring(2):q;
		if(q.charAt(0)==='/')return q;
		if(q.indexOf('./')===0||q.indexOf('../')===0)return q;
		return homeDir()?homeDir()+'/'+q:q;
	}
	function confirmCreateProject(path){
		if(!path)return;
		var run=function(){closePopup();__ws.call('project.create.confirm',{sid:'%s',path:path});};
		if(window.__libroConfirmAction){window.__libroConfirmAction('Create folder?', 'Create this folder and open it as a new Libro project?\n\n'+path, run);}else{run();}
	}
	function openSelectedDirectory(fromClick){
		var typed=fromClick?'':typedDirectoryPath();
		if(typed){
			if(dirMatches.length===0){confirmCreateProject(typed);}else{
				closePopup();
				__ws.call('project.create',{sid:'%s','project-path':typed});
			}
			return;
		}
		var item=selectedDirectory();
		if(!item){confirmCreateProject(newProjectPathFromQuery());return;}
		closePopup();
		__ws.call('project.create',{sid:'%s','project-path':item.path});
	}
	function completeSelectedDirectory(){
		var item=selectedDirectory();
		var inp=getInp();
		if(!item||!inp)return;
		inp.value=simplifyPath(item.path).replace(/\/$/,'')+'/';
		selectedIdx=0;
		dirMatches=[];
		lookupLoading=true;
		render();
		__ws.call('project.lookup',{sid:'%s',query:inp.value,seq:++lookupSeq});
	}

	function navigateSelection(delta){
		var max=inDirectoryMode()?dirMatches.length:filtered.length;
		if(max<=0)return;
		if(delta>0&&selectedIdx<max-1){selectedIdx++;render();}
		else if(delta<0&&selectedIdx>0){selectedIdx--;render();}
	}

	function normalKey(e){
		var dlg=getDlg();
		var inp=getInp();
		if(!dlg||dlg.classList.contains('hidden'))return;
		if(document.activeElement===inp)return;
		if(e.key==='i'){
			e.preventDefault();e.stopImmediatePropagation();if(inp)inp.focus();return;
		}
		if(e.key==='j'||e.key==='ArrowDown'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(1);return;
		}
		if(e.key==='k'||e.key==='ArrowUp'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(-1);return;
		}
		if(e.key==='d'||e.key==='D'){
			e.preventDefault();e.stopImmediatePropagation();if(!inDirectoryMode())removeAt(selectedIdx);return;
		}
		if(e.key==='Enter'){
			e.preventDefault();e.stopImmediatePropagation();if(inDirectoryMode())openSelectedDirectory();else launchProject();return;
		}
		if(e.key==='Escape'){
			e.preventDefault();e.stopImmediatePropagation();closePopup();return;
		}
	}

	function removeAt(idx){
		if(idx<0||idx>=filtered.length)return;
		var item=filtered[idx];
		if(!item||item.kind==='worktree')return;
		var remove=function(){closePopup();__ws.call('project.remove',{sid:'%s',name:item.name});};
		if(window.__libroConfirmAction){window.__libroConfirmAction('Remove project?', 'Remove project "'+item.name+'" from Libro?\n\nThis only removes it from the project list. Files on disk are kept.', remove);}else{remove();}
	}

	function openPopup(){
		var dlg=getDlg();
		bindInput();
		var inp=getInp();
		if(!dlg||!inp)return;
		if(!dlg.classList.contains('hidden')){
			setTimeout(function(){inp.focus();},0);
			return;
		}
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		var scrollTop=function(){var res=getResults();if(res)res.scrollTop=0;};
		scrollTop();
		requestAnimationFrame(scrollTop);
		setTimeout(scrollTop,0);
		armHoverAfterPointerMove();
		setTimeout(function(){inp.focus();scrollTop();},50);
	}
	function openBrowse(){
		openPopup();
		var inp=getInp();
		if(!inp)return;
		inp.value='~/';
		filter();
		setTimeout(function(){inp.focus();inp.setSelectionRange(inp.value.length,inp.value.length);},0);
	}

	function bindCreateConfirm(){
		var cancel=document.getElementById('project-path-confirm-cancel');
		var create=document.getElementById('project-path-confirm-create');
		if(cancel&&!cancel.__libroBound){
			cancel.__libroBound=true;
			cancel.addEventListener('click',function(e){e.preventDefault();hideCreateConfirm();});
		}
		if(create&&!create.__libroBound){
			create.__libroBound=true;
			create.addEventListener('click',function(e){
				e.preventDefault();
				var bar=document.getElementById('project-path-confirm');
				var path=bar&&bar.dataset?bar.dataset.path:'';
				if(path){hideCreateConfirm();closePopup();__ws.call('project.create.confirm',{sid:'%s',path:path});}
			});
		}
	}

	function bindInput(){
		bindCreateConfirm();
		var inp=getInp();
		if(!documentKeydownBound){
			documentKeydownBound=true;
			document.addEventListener('keydown',normalKey,true);
		}
		if(!inp||inp.__libroProjectDialogBound)return;
		inp.__libroProjectDialogBound=true;
		inp.addEventListener('input',filter);
		inp.addEventListener('keydown',function(e){
			var dlg=getDlg();
			if(!dlg||dlg.classList.contains('hidden'))return;
			e.stopImmediatePropagation();
			var max=inDirectoryMode()?dirMatches.length:filtered.length;
			if(e.key==='ArrowDown'){
				e.preventDefault();
				navigateSelection(1);
			}else if(e.key==='ArrowUp'){
				e.preventDefault();
				navigateSelection(-1);
			}else if(e.key==='Tab'){
				e.preventDefault();
				if(inDirectoryMode())completeSelectedDirectory();
			}else if(e.key==='Enter'){
				e.preventDefault();
				if(inDirectoryMode())openSelectedDirectory();else launchProject();
			}else if(e.key==='Escape'){
				e.preventDefault();inp.blur();
			}
		});
	}
	bindInput();
	window.__libroProjectDialogBind=bindInput;
	window.__libroProjectDialogSetDirMatches=function(payload){
		payload=payload||{};
		if(payload.seq&&payload.seq<lookupSeq)return;
		if(payload.query&&payload.query!==query())return;
		dirMatches=payload.matches||[];
		lookupLoading=false;
		if(inDirectoryMode()&&selectedIdx>=dirMatches.length)selectedIdx=0;
		render();
	};
	window.__libroOpenProjectDialog=openPopup;
	window.__libroOpenProjectDialogSearch=openPopup;
	window.__libroOpenProjectDialogBrowse=openBrowse;
	// Backward-compatible aliases for command palette / older Electron preload code.
	window.__libroOpenProjectDialog=openPopup;
	window.__libroOpenProjectDialogSearch=openPopup;
	window.__libroOpenProjectDialogBrowse=openBrowse;
})();
`, ProjectDialogID, sid, sid, sid, sid, sid, sid, sid, sid, sid)
}

func moveProjectPopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var selectedIdx=0;
	var filtered=[];
	var hoverEnabled=false;

	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('move-project-input');}
	function getResults(){return document.getElementById('move-project-results');}
	function targetName(item){return item.kind==='worktree' ? item.name+'/'+item.branch : item.name;}
	function escapeHtml(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}

	function armHoverAfterPointerMove(){
		hoverEnabled=false;
		var dlg=getDlg();
		if(!dlg)return;
		var enableHover=function(){
			hoverEnabled=true;
			dlg.removeEventListener('mousemove',enableHover,true);
		};
		dlg.addEventListener('mousemove',enableHover,true);
	}

	function fuzzyMatch(text,query){
		text=(text||'').toLowerCase();
		query=(query||'').toLowerCase();
		var ti=0,qi=0,score=0,lastMatch=-1;
		while(ti<text.length&&qi<query.length){
			if(text[ti]===query[qi]){
				score+=1;
				if(lastMatch===ti-1)score+=2;
				if(ti===0||text[ti-1]===' '||text[ti-1]==='/'||text[ti-1]==='.')score+=3;
				lastMatch=ti;
				qi++;
			}
			ti++;
		}
		return qi===query.length?score:0;
	}

	function render(){
		var res=getResults();
		if(!res)return;
		var dk=document.documentElement.classList.contains('dark');
		if(filtered.length===0){
			res.innerHTML='<div class="px-4 py-6 text-center text-sm font-mono '+(dk?'text-zinc-500':'text-gray-400')+'">No projects found</div>';
			return;
		}
		var html='';
		filtered.forEach(function(item,i){
			var sel=i===selectedIdx;
			var icon=item.kind==='worktree'?'alt_route':(item.isGit?'source':'folder');
			var primary=item.kind==='worktree'?item.branch:item.name;
			var secondary=item.kind==='worktree'?item.name:item.path;
			var activeBadge=item.isActive?'<span class="ml-2 inline-flex items-center justify-center px-1.5 h-4 rounded text-[9px] font-bold leading-none '+(dk?'bg-blue-500/30 text-blue-300':'bg-blue-100 text-blue-700')+'">ACTIVE</span>':'';
			html+='<div class="move-project-item flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'))
				+'" data-move-project-idx="'+i+'">';
			html+='<i class="material-icons-round '+(dk?'text-zinc-400':'text-gray-400')+' text-lg">'+icon+'</i>';
			html+='<div class="flex-1 min-w-0">';
			html+='<div class="text-sm truncate '+(dk?'text-zinc-200':'text-gray-800')+'">'+escapeHtml(primary)+activeBadge+'</div>';
			html+='<div class="text-[11px] truncate '+(dk?'text-zinc-500':'text-gray-400')+'">'+escapeHtml(secondary)+'</div>';
			html+='</div></div>';
		});
		res.innerHTML=html;
		res.querySelectorAll('[data-move-project-idx]').forEach(function(el){
			el.addEventListener('mouseenter',function(){
				if(!hoverEnabled)return;
				var idx=parseInt(el.getAttribute('data-move-project-idx'),10);
				if(!Number.isNaN(idx)&&idx!==selectedIdx){selectedIdx=idx;render();}
			});
			el.addEventListener('mousedown',function(e){
				e.preventDefault();
				var idx=parseInt(el.getAttribute('data-move-project-idx'),10);
				if(Number.isNaN(idx))return;
				selectedIdx=idx;
				launch();
			});
		});
		var selected=res.querySelector('[data-move-project-idx="'+selectedIdx+'"]');
		if(selected)selected.scrollIntoView({block:'nearest'});
	}

	function filter(){
		var query=(getInp()&&getInp().value||'').trim();
		var all=(window.__libroProjects||[]).slice();
		if(!query){
			filtered=all;
		}else{
			filtered=[];
			all.forEach(function(item){
				var hay=targetName(item)+' '+(item.branch||'')+' '+(item.path||'');
				var score=fuzzyMatch(hay,query);
				if(score>0)filtered.push(Object.assign({score:score},item));
			});
			filtered.sort(function(a,b){return b.score-a.score;});
		}
		selectedIdx=0;
		render();
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(dlg)dlg.classList.add('hidden');
		if(inp)inp.value='';
		hoverEnabled=false;
	}

	function navigateSelection(delta){
		if(delta>0&&selectedIdx<filtered.length-1){selectedIdx++;render();}
		else if(delta<0&&selectedIdx>0){selectedIdx--;render();}
	}

	function normalKey(e){
		var dlg=getDlg();
		var inp=getInp();
		if(!dlg||dlg.classList.contains('hidden'))return;
		if(document.activeElement===inp)return;
		if(e.key==='i'){
			e.preventDefault();e.stopImmediatePropagation();if(inp)inp.focus();return;
		}
		if(e.key==='j'||e.key==='ArrowDown'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(1);return;
		}
		if(e.key==='k'||e.key==='ArrowUp'){
			e.preventDefault();e.stopImmediatePropagation();navigateSelection(-1);return;
		}
		if(e.key==='Enter'){
			e.preventDefault();e.stopImmediatePropagation();launch();return;
		}
		if(e.key==='Escape'){
			e.preventDefault();e.stopImmediatePropagation();closePopup();return;
		}
	}

	function launch(){
		if(filtered.length===0)return;
		var item=filtered[selectedIdx];
		closePopup();
		__ws.call('app.move.to.project',{sid:'%s',target:targetName(item),kind:item.kind,project:item.name,path:item.path,branch:item.branch});
	}

	function openPopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(!dlg||!inp)return;
		var appId=window.__libroSelectedApp||'';
		if(!appId){
			if(window.__libroShowToast)window.__libroShowToast('No selected app','Select or open an app first',1800);
			return;
		}
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		armHoverAfterPointerMove();
		setTimeout(function(){inp.focus();},50);
	}

	var inp=getInp();
	if(inp){
		inp.addEventListener('input',filter);
		inp.addEventListener('keydown',function(e){
			var dlg=getDlg();
			if(!dlg||dlg.classList.contains('hidden'))return;
			e.stopImmediatePropagation();
			if(e.key==='ArrowDown'){
				e.preventDefault();
				navigateSelection(1);
			}else if(e.key==='ArrowUp'){
				e.preventDefault();
				navigateSelection(-1);
			}else if(e.key==='Enter'){
				e.preventDefault();
				launch();
			}else if(e.key==='Escape'){
				e.preventDefault();
				inp.blur();
			}
		});
	}
	document.addEventListener('keydown',normalKey,true);

	window.__libroOpenMoveProject=openPopup;
})();
`, MoveProjectPopupID, sid)
}

// projectsJS publishes the list of projects (and their worktrees) into
// window.__libroProjects for the project dialog.
func projectsJS(state *AppState) string {
	displayProjectName := func(name, path string) string {
		if path == "" {
			return name
		}
		return filepath.Base(path)
	}
	type jsProject struct {
		Kind          string   `json:"kind"`
		Name          string   `json:"name"`
		DisplayName   string   `json:"displayName,omitempty"`
		Path          string   `json:"path"`
		Branch        string   `json:"branch,omitempty"`
		IsGit         bool     `json:"isGit"`
		IsActive      bool     `json:"isActive"`
		Branches      []string `json:"branches,omitempty"`
		CurrentBranch string   `json:"currentBranch,omitempty"`
		WorktreeRefs  []string `json:"worktreeRefs,omitempty"`
		Transient     bool     `json:"transient,omitempty"`
		Command       string   `json:"command,omitempty"`
	}
	var all []jsProject
	for _, p := range state.Projects {
		if p.Virtual {
			continue
		}
		isActive := p.Name == state.ActiveProject
		entry := jsProject{
			Kind:        "project",
			Name:        p.Name,
			DisplayName: displayProjectName(p.Name, p.Path),
			Path:        p.Path,
			IsGit:       p.IsGitRepo,
			IsActive:    isActive,
			Transient:   p.Transient,
			Command:     projectCommand(p.Path),
		}

		if p.IsGitRepo && GitAvailable() {
			if branches, err := GitListBranches(p.Path); err == nil {
				entry.Branches = branches
			}
			entry.CurrentBranch = GitCurrentBranch(p.Path)
		}

		all = append(all, entry)

		if !p.IsGitRepo || !GitAvailable() {
			continue
		}
		wts, err := GitListWorktrees(p.Path)
		if err != nil {
			continue
		}
		var refs []string
		for _, wt := range wts {
			if wt.Branch != "" && wt.Branch != "(detached)" {
				refs = append(refs, wt.Branch)
			}
		}
		// attach worktree refs back to the project entry
		for i := range all {
			if all[i].Name == p.Name && all[i].Kind == "project" {
				all[i].WorktreeRefs = refs
				break
			}
		}
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			isMain := wt.Path == p.Path
			if isMain {
				continue
			}
			vtName := p.Name + "/" + wt.Branch
			wtActive := state.ActiveProject == vtName
			all = append(all, jsProject{
				DisplayName: displayProjectName(wt.Branch, wt.Path),
				Kind:        "worktree",
				Command:     projectCommand(wt.Path),
				Name:        p.Name,
				Path:        wt.Path,
				Branch:      wt.Branch,
				IsGit:       true,
				IsActive:    wtActive,
			})
		}
	}
	b, _ := json.Marshal(all)
	if b == nil {
		b = []byte("[]")
	}
	return threadsJS(state) + fmt.Sprintf("window.__libroActiveProject=%s;window.__libroProjects=%s;if(window.libroWorkspace)libroWorkspace.refresh();", components.JSString(state.ActiveProject), string(b))
}

// renderProjectDialog renders the create project modal
func renderProjectDialog(visible bool, sid string) *r.Node {
	return components.ProjectDialog(sid)
}

// updateHashJS returns JS that updates the URL hash to the given project name
func updateHashJS(name string) string {
	title := "Libro"
	if name != "" {
		title = name + " — Libro"
	}
	return fmt.Sprintf("history.replaceState(null,'',%s);document.title=%s;", components.JSString("#"+name), components.JSString(title))
}

// initHashJS handles hash-based project navigation on page load.
// Projects are now loaded from DB on server side, so only hash switching is needed.
func initHashJS(sid string) string {
	return fmt.Sprintf(`
(function _initHash(){
	if(typeof __ws==='undefined'||!__ws.connected||!__ws.connected()){setTimeout(_initHash,50);return;}
	var hash=location.hash.replace('#','');
	if(hash){
		setTimeout(function(){__ws.call('project.switch',{sid:'%s',name:hash});},100);
	}
	var proj=hash||window.__libroActiveProject||'';
	if(proj&&!hash){history.replaceState(null,'','#'+proj);}
	document.title=proj?proj+' \u2014 Libro':'Libro';
})();
`, sid)
}

// termIconSetupJS returns JS that registers a global icon lookup function
// for terminal commands. All JS icon renderers should call __libroTermIcon(cmd, size).
func termIconSetupJS() string {
	return fmt.Sprintf(`
(function(){
	if(!window.__libroFaviconURL){
		window.__libroFaviconURL=function(raw,size){
			raw=(raw||'').trim();
			if(!raw)return '';
			size=size||32;
			var u;
			try{u=new URL(raw);}catch(e){try{u=new URL('https://'+raw);}catch(e2){return '';}}
			if(!u.hostname||u.protocol==='file:')return '';
			var host=u.hostname.toLowerCase();
			if(host==='localhost'||host.endsWith('.localhost')||!host.includes('.')||host.includes(':')||/^\d+\.\d+\.\d+\.\d+$/.test(host))return '';
			var target=(u.protocol&&u.host)?(u.protocol+'//'+u.host):u.hostname;
			return 'https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url='+encodeURIComponent(target)+'&size='+size;
		};
	}
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

func terminalFrameSetupJS() string {
	return `
		(function() {
			if (window.__libroTerminalFramesRegistered) return;
			window.__libroTerminalFramesRegistered = true;

			var assetPromise = null;
			var observed = new WeakSet();
			var terminals = new Map();
			var terminalInputEncoder = window.TextEncoder ? new TextEncoder() : null;
			var resizeObserver = window.ResizeObserver ? new ResizeObserver(function(entries) {
				entries.forEach(function(entry) {
					if (entry && entry.contentRect && (!entry.contentRect.width || !entry.contentRect.height)) return;
					scheduleFitTerminal(entry.target, false, 80);
				});
			}) : null;
			var intersectionObserver = window.IntersectionObserver ? new IntersectionObserver(function(entries) {
				entries.forEach(function(entry) { if (entry.isIntersecting) scheduleFitTerminal(entry.target, false, 0); });
			}, { threshold: 0.01 }) : null;

			function loadScript(src) {
				return new Promise(function(resolve, reject) {
					var existing = document.querySelector('script[data-libro-src="' + src + '"]');
					if (existing) {
						if (existing.getAttribute('data-loaded') === '1') resolve();
						else existing.addEventListener('load', resolve, { once: true });
						return;
					}
					var script = document.createElement('script');
					script.src = src;
					script.async = false;
					script.setAttribute('data-libro-src', src);
					script.onload = function() { script.setAttribute('data-loaded', '1'); resolve(); };
					script.onerror = reject;
					document.head.appendChild(script);
				});
			}

			function ensureAssets() {
				if (assetPromise) return assetPromise;
				assetPromise = new Promise(function(resolve, reject) {
					if (!document.getElementById('libro-xterm-css')) {
						var link = document.createElement('link');
						link.id = 'libro-xterm-css';
						link.rel = 'stylesheet';
						link.href = '/assets/xterm/xterm.css';
						document.head.appendChild(link);
					}
					if (!document.getElementById('libro-xterm-overrides')) {
						var style = document.createElement('style');
						style.id = 'libro-xterm-overrides';
						style.textContent = '[data-terminal] .xterm,[data-terminal] .terminal{width:100%;height:100%;overflow:hidden!important}' +
							'[data-terminal] .xterm-viewport{scrollbar-width:none!important;-ms-overflow-style:none!important;scrollbar-gutter:stable!important}' +
							'[data-terminal] .xterm-viewport::-webkit-scrollbar{width:0!important;height:0!important;display:none!important}' +
							'[data-terminal] .xterm-screen{overflow:hidden!important}';
						document.head.appendChild(style);
					}
					loadScript('/assets/xterm/xterm.js')
						.then(function() { return loadScript('/assets/xterm/addon-fit.js'); })
						.then(function() { return loadScript('/assets/xterm/addon-webgl.js').catch(function() {}); })
						.then(resolve, reject);
				});
				return assetPromise;
			}

			function termTheme() {
				var dark = document.documentElement.classList.contains('dark');
				return dark ? {
					background: '#1e1e1e', foreground: '#d4d4d4', cursor: '#d4d4d4', selectionBackground: '#264f78'
				} : {
					background: '#fdfdfd', foreground: '#1f2328', cursor: '#1f2328', selectionBackground: '#b5d5ff'
				};
			}

			function wsURL(id, sid) {
				var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
				return proto + '//' + location.host + '/terminal/ws/' + encodeURIComponent(id) + '?sid=' + encodeURIComponent(sid || '');
			}

			function setStatus(el, text) {
				var status = el.querySelector('[data-terminal-status]');
				if (status) status.textContent = text || '';
			}

			function terminalSelection(term) {
				try {
					if (term && term.hasSelection && term.hasSelection()) return term.getSelection() || '';
				} catch (err) {}
				return '';
			}

			function fallbackCopyText(text) {
				var ta = document.createElement('textarea');
				ta.value = text;
				ta.style.position = 'fixed';
				ta.style.opacity = '0';
				document.body.appendChild(ta);
				ta.focus();
				ta.select();
				try { document.execCommand('copy'); } catch (err) {}
				document.body.removeChild(ta);
			}

			function writeClipboardText(text) {
				if (!text) return;
				if (window.libroElectron && window.libroElectron.copyToClipboard) {
					window.libroElectron.copyToClipboard(text);
				} else if (navigator.clipboard && navigator.clipboard.writeText) {
					navigator.clipboard.writeText(text).catch(function() { fallbackCopyText(text); });
				} else {
					fallbackCopyText(text);
				}
			}

			function copyTerminalSelection(term, event) {
				var text = terminalSelection(term);
				if (!text) return false;
				if (event && event.clipboardData) {
					try { event.clipboardData.setData('text/plain', text); } catch (err) {}
				}
				writeClipboardText(text);
				if (event && event.preventDefault) event.preventDefault();
				if (window.__libroShowToast) window.__libroShowToast('Copied terminal text', '', 1200);
				return true;
			}

			function isVisibleProject(project) {
				return !project || (project.style.display !== 'none' && project.style.visibility !== 'hidden' && project.getAttribute('aria-hidden') !== 'true');
			}

			function isVisibleTerminal(el) {
				if (!el || !el.isConnected) return false;
				var rect;
				try { rect = el.getBoundingClientRect(); } catch (err) { return false; }
				if (!rect || rect.width <= 0 || rect.height <= 0) return false;
				var project = el.closest('[id^="project-main-"]');
				return isVisibleProject(project);
			}

			function sendResize(controller, cols, rows) {
				cols = Number(cols) || 0;
				rows = Number(rows) || 0;
				if (!controller || !cols || !rows) return;
				if (controller.lastCols === cols && controller.lastRows === rows) return;
				controller.lastCols = cols;
				controller.lastRows = rows;
				if (controller.ws && controller.ws.readyState === WebSocket.OPEN) {
					controller.ws.send(JSON.stringify({ type: 'resize', cols: cols, rows: rows }));
				}
			}

			function writeTerminalBytes(term, bytes) {
				if (!bytes || !bytes.length) return;
				if (bytes[0] === 0) bytes = bytes.subarray ? bytes.subarray(1) : bytes.slice(1);
				if (!bytes || !bytes.length) return;
				try {
					term.write(bytes);
					return;
				} catch (err) {}
				var text = '';
				try {
					if (window.TextDecoder) text = new TextDecoder().decode(bytes);
				} catch (err2) {}
				if (!text) {
					var chunk = 8192;
					for (var i = 0; i < bytes.length; i += chunk) {
						text += String.fromCharCode.apply(null, Array.prototype.slice.call(bytes, i, i + chunk));
					}
				}
				if (text) term.write(text);
			}

			function sendTerminalInput(controller, data) {
				if (!controller || !data || !controller.ws || controller.ws.readyState !== WebSocket.OPEN) return;
				if (terminalInputEncoder) {
					var encoded = terminalInputEncoder.encode(data);
					var payload = new Uint8Array(encoded.length + 1);
					payload[0] = 0;
					payload.set(encoded, 1);
					controller.ws.send(payload);
				} else {
					controller.ws.send('\x00' + data);
				}
			}

			function stripTerminalFocusReports(data) {
				// TUI apps such as nvim and pi agent often redraw on xterm's
				// DEC focus in/out reports. Libro already tracks selected panels,
				// so suppress these synthetic reports to avoid repaint flicker when
				// the window/project/panel focus changes.
				return data ? String(data).replace(/\x1b\[(?:I|O)/g, '') : '';
			}

			function terminalRect(el) {
				try {
					var rect = el.getBoundingClientRect();
					if (!rect || rect.width <= 0 || rect.height <= 0) return null;
					return { width: Math.round(rect.width), height: Math.round(rect.height) };
				} catch (err) {
					return null;
				}
			}

			function fitTerminal(el, force) {
				var controller = terminals.get(el);
				if (!controller || !controller.fit || !isVisibleTerminal(el)) return;
				var rect = terminalRect(el);
				if (!rect) return;
				if (!force && controller.lastFitWidth === rect.width && controller.lastFitHeight === rect.height) return;
				controller.lastFitWidth = rect.width;
				controller.lastFitHeight = rect.height;
				try {
					controller.fit.fit();
					sendResize(controller, controller.term.cols, controller.term.rows);
				} catch (err) {}
			}

			function scheduleFitTerminal(el, force, delay) {
				var controller = terminals.get(el);
				if (!controller) return;
				if (force) controller.pendingFitForce = true;
				if (controller.fitTimer) clearTimeout(controller.fitTimer);
				controller.fitTimer = setTimeout(function() {
					controller.fitTimer = null;
					if (controller.fitFrame) cancelAnimationFrame(controller.fitFrame);
					controller.fitFrame = requestAnimationFrame(function() {
						controller.fitFrame = null;
						var runForced = !!controller.pendingFitForce;
						controller.pendingFitForce = false;
						fitTerminal(el, runForced);
					});
				}, Math.max(0, Number(delay) || 0));
			}

			function focusTerminal(el) {
				var controller = terminals.get(el);
				if (!controller || !controller.term || !isVisibleTerminal(el)) return;
				if (el.contains(document.activeElement)) return;
				var now = Date.now();
				if (controller.lastFocusAt && now - controller.lastFocusAt < 250) return;
				controller.lastFocusAt = now;
				try { controller.term.focus(); } catch (err) {}
			}

			function initTerminal(el) {
				if (!el || observed.has(el)) return;
				observed.add(el);
				ensureAssets().then(function() {
					if (!el.isConnected || terminals.has(el)) return;
					var terminalID = el.getAttribute('data-terminal') || '';
					var appID = el.getAttribute('data-terminal-app') || terminalID;
					var sid = el.getAttribute('data-sid') || '';
					var Term = window.Terminal;
					var Fit = window.FitAddon && window.FitAddon.FitAddon;
					if (!Term || !Fit) {
						setStatus(el, 'Terminal assets failed to load');
						return;
					}
					el.innerHTML = '';
					var scrollback = 2000;
					var cursorBlink = false;
					try {
						var configuredScrollback = parseInt(window.localStorage && window.localStorage.getItem('libro.terminal.scrollback') || '', 10);
						if (configuredScrollback >= 100 && configuredScrollback <= 50000) scrollback = configuredScrollback;
						cursorBlink = !!(window.localStorage && window.localStorage.getItem('libro.terminal.cursorBlink') === '1');
					} catch (err) {}
					var term = new Term({
						allowProposedApi: true,
						fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
						fontSize: 15,
						lineHeight: 1.12,
						cursorBlink: cursorBlink,
						theme: termTheme(),
						scrollback: scrollback,
						smoothScrollDuration: 0
					});
					var fit = new Fit();
					term.loadAddon(fit);
					term.open(el);
					var webgl = null;
					var Webgl = window.WebglAddon && window.WebglAddon.WebglAddon;
					var useWebgl = true;
					try { useWebgl = !window.localStorage || window.localStorage.getItem('libro.terminal.renderer') !== 'dom'; } catch (err) {}
					if (Webgl && useWebgl) {
						try {
							webgl = new Webgl();
							if (webgl.onContextLoss) webgl.onContextLoss(function() { try { webgl.dispose(); } catch (err) {} webgl = null; });
							term.loadAddon(webgl);
						} catch (err) {
							webgl = null;
						}
					}
					if (term.attachCustomKeyEventHandler) {
						term.attachCustomKeyEventHandler(function(ev) {
							if (!ev || ev.type !== 'keydown') return true;
							var key = String(ev.key || '').toLowerCase();
							var copyCombo = key === 'c' && ((ev.ctrlKey && ev.shiftKey) || ev.metaKey);
							if (copyCombo && copyTerminalSelection(term, ev)) return false;
							return true;
						});
					}
					el.addEventListener('copy', function(ev) { copyTerminalSelection(term, ev); });
					var controller = { term: term, fit: fit, webgl: webgl, ws: null, closed: false, reconnectTimer: null, attempts: 0, lastCols: 0, lastRows: 0, lastFitWidth: 0, lastFitHeight: 0, fitTimer: null, fitFrame: null, pendingFitForce: false, lastFocusAt: 0 };
					terminals.set(el, controller);

					function connect() {
						if (controller.closed || !el.isConnected) return;
						setStatus(el, controller.attempts ? 'Reconnecting terminal' : 'Connecting terminal');
						var ws = new WebSocket(wsURL(terminalID, sid));
						ws.binaryType = 'arraybuffer';
						controller.ws = ws;
						ws.onopen = function() {
							controller.attempts = 0;
							setStatus(el, '');
							scheduleFitTerminal(el, false, 0);
						};
						ws.onmessage = function(ev) {
							if (typeof ev.data !== 'string') {
								if (ev.data instanceof ArrayBuffer) {
									writeTerminalBytes(term, new Uint8Array(ev.data));
								} else if (ev.data && ev.data.arrayBuffer) {
									ev.data.arrayBuffer().then(function(buf) { writeTerminalBytes(term, new Uint8Array(buf)); });
								}
								return;
							}
							var raw = ev.data || '';
							if (raw.charCodeAt && raw.charCodeAt(0) === 0) {
								term.write(raw.slice(1));
								return;
							}
							var msg;
							try { msg = JSON.parse(raw); } catch (err) { return; }
							if (msg.type === 'process-status') {
                                el.dataset.processStatus = msg.data;
                                window.dispatchEvent(new Event('libro-process-status'));
                                return;
                            }
							if (msg.type === 'agent-status') {
								window.__libroAgentStatuses = window.__libroAgentStatuses || {};
								window.__libroAgentStatuses[appID] = msg.data;
								window.dispatchEvent(new Event('libro-agent-status'));
								return;
							}
							if (msg.type === 'output') term.write(msg.data || '');
							else if (msg.type === 'exit') {
                                delete el.dataset.processStatus;
                                window.dispatchEvent(new Event('libro-process-status'));
                                if (window.__libroAgentStatuses) delete window.__libroAgentStatuses[appID];
                                window.dispatchEvent(new Event('libro-agent-status'));
                                term.write('\r\n[process exited: ' + (msg.code || 0) + ']\r\n');
                                if (el.closest('[data-dock="bottom"]') && window.libroWorkspace) {
                                    controller.closed = true;
                                    window.libroWorkspace.terminalExited(appID);
                                }
                            }
							else if (msg.type === 'error') term.write('\r\n[terminal error: ' + (msg.message || 'unknown') + ']\r\n');
						};
						ws.onclose = function() {
                            delete el.dataset.processStatus;
                            window.dispatchEvent(new Event('libro-process-status'));
							if (window.__libroAgentStatuses) delete window.__libroAgentStatuses[appID];
							window.dispatchEvent(new Event('libro-agent-status'));
							if (controller.closed || !el.isConnected) return;
							controller.attempts++;
							// Stop hammering the server if the connection keeps
							// failing (e.g. the terminal id is no longer valid).
							if (controller.attempts >= 4) {
								setStatus(el, 'Terminal session lost (reopen the app)');
								return;
							}
							var delay = Math.min(1200, 150 * controller.attempts);
							setStatus(el, 'Terminal disconnected');
							controller.reconnectTimer = setTimeout(connect, delay);
						};
						ws.onerror = function() { try { ws.close(); } catch (err) {} };
					}

					term.onTitleChange(function(title) {
						const frame = document.getElementById('frame-' + appID);
						if (!frame || frame.dataset.dock !== 'center') return;
						const task = title.replace(/^(Working|Thinking|Waiting|Ready|Starting)(?:\s*[·|—-]\s*|$)/, '').trim();
						if (!task) return;
						frame.dataset.taskTitle = task.slice(0, 240);
						const grid = frame.closest('[data-workspace-project]');
						const threadId = grid && grid.dataset.workspaceProject;
						if (threadId && threadId.indexOf('thread:') === 0 && window.__ws) __ws.call('thread.rename', {id:threadId, name:task.slice(0, 240)});
					});
					term.onData(function(data) {
						data = stripTerminalFocusReports(data);
						if (!data) return;
						sendTerminalInput(controller, data);
					});
					term.onResize(function(size) {
						if (!isVisibleTerminal(el)) return;
						sendResize(controller, size.cols, size.rows);
					});
					controller.restart = function() {
						if (controller.reconnectTimer) clearTimeout(controller.reconnectTimer);
						try {
							if (controller.ws) {
								controller.ws.onclose = null;
								controller.ws.close();
							}
						} catch (err) {}
						controller.attempts = 0;
						setTimeout(connect, 120);
					};
					el.addEventListener('focus', function() { focusTerminal(el); });
					if (resizeObserver) resizeObserver.observe(el);
					if (intersectionObserver) intersectionObserver.observe(el);
					scheduleFitTerminal(el, false, 0);
					connect();
					if ((window.__libroSelectedApp || '') === appID) setTimeout(function() { focusTerminal(el); }, 60);
				}).catch(function() { setStatus(el, 'Terminal assets failed to load'); });
			}

			window.__libroFitTerminalFrame = function(frameOrAppID, force) {
				var el = frameOrAppID;
				if (typeof frameOrAppID === 'string') {
					el = document.querySelector('[data-terminal-app="' + frameOrAppID.replace(/"/g, '\\"') + '"]');
				}
				if (!el) return;
				scheduleFitTerminal(el, !!force, 120);
			};

			window.__libroFocusTerminalFrame = function(frameOrAppID) {
				var el = frameOrAppID;
				if (typeof frameOrAppID === 'string') {
					el = document.querySelector('[data-terminal-app="' + frameOrAppID.replace(/"/g, '\\"') + '"]');
				}
				if (!el) return;
				focusTerminal(el);
			};

			window.__libroSettleAppFrame = function(appID) {
				var app = document.querySelector('[data-app-id="' + String(appID).replace(/"/g, '\\"') + '"]');
				if (!app) return;
				var termEl = app.querySelector('[data-terminal]');
				function settle() {
					if (window.__libroScrollToApp) window.__libroScrollToApp(app);
					app.style.transform = 'translateZ(0)';
					app.getBoundingClientRect();
					if (termEl) scheduleFitTerminal(termEl, false, 0);
					if ((window.__libroSelectedApp || '') === appID && window.__libroFocusAppByID) window.__libroFocusAppByID(appID);
				}
				requestAnimationFrame(function() { settle(); requestAnimationFrame(settle); });
			};

			window.__libroRestartTerminal = function(appID) {
				var el = document.querySelector('[data-terminal-app="' + String(appID).replace(/"/g, '\\"') + '"]');
				var controller = el && terminals.get(el);
				if (controller && controller.restart) controller.restart();
			};

			window.__libroRefreshTerminalThemes = function() {
				terminals.forEach(function(controller) { try { controller.term.options.theme = termTheme(); } catch (err) {} });
			};

			function scan(root) {
				var scope = root && root.querySelectorAll ? root : document;
				if (scope.matches && scope.matches('[data-terminal]')) initTerminal(scope);
				scope.querySelectorAll('[data-terminal]').forEach(initTerminal);
			}

			scan(document);
			new MutationObserver(function(mutations) {
				mutations.forEach(function(mutation) { mutation.addedNodes.forEach(scan); });
				terminals.forEach(function(controller, el) {
					if (el.isConnected) return;
					controller.closed = true;
					clearTimeout(controller.reconnectTimer);
					clearTimeout(controller.fitTimer);
					if (controller.fitFrame) cancelAnimationFrame(controller.fitFrame);
					if (controller.ws) { controller.ws.onclose = null; controller.ws.close(); }
					if (resizeObserver) resizeObserver.unobserve(el);
					if (intersectionObserver) intersectionObserver.unobserve(el);
					controller.term.dispose();
					terminals.delete(el);
					observed.delete(el);
				});
			}).observe(document.body, { childList: true, subtree: true });
			new MutationObserver(function() {
				window.__libroRefreshTerminalThemes();
			}).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
		})();
`
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
				// Find the visible strip (parent project div is not parked off-screen)
				var strips = document.querySelectorAll('[id^="app-strip-"]');
				var strip = null;
				for (var s = 0; s < strips.length; s++) {
					var parent = strips[s].closest('[id^="project-main-"]');
					if (parent && parent.style.display !== 'none' && parent.style.visibility !== 'hidden' && parent.getAttribute('aria-hidden') !== 'true') {
						strip = strips[s];
						break;
					}
				}
				if (!strip) return;
				var sorted = window.__libroSortedApps(strip);
				var container = sorted[idx];
				if (!container) return;
				if (window.__libroScrollToApp) window.__libroScrollToApp(container);

				function focusAttempt() {
					if (document.querySelector('#url-popup:not(.hidden)')) return;
					if ((window.__libroSelectedApp || '') !== (container.getAttribute('data-app-id') || '')) return;
					try { window.focus(); } catch(err) {}

					// Blur only visible, non-target embedded surfaces. Touching hidden
					// webviews while switching projects can make Electron repaint them.
					var allIframes = document.querySelectorAll('iframe');
					for (var i = 0; i < allIframes.length; i++) {
						var iframeProject = allIframes[i].closest('[id^="project-main-"]');
						if (container.contains(allIframes[i]) || !allIframes[i].offsetParent || (iframeProject && iframeProject.getAttribute('aria-hidden') === 'true')) continue;
						try { allIframes[i].contentWindow.blur(); } catch(err) {}
						allIframes[i].blur();
					}
					var allWebviews = document.querySelectorAll('webview');
					for (var j = 0; j < allWebviews.length; j++) {
						var webviewProject = allWebviews[j].closest('[id^="project-main-"]');
						if (container.contains(allWebviews[j]) || !allWebviews[j].offsetParent || (webviewProject && webviewProject.getAttribute('aria-hidden') === 'true')) continue;
						allWebviews[j].blur();
					}

					// Try to focus a webview first, then native terminal, then iframe fallback.
					var webview = container.querySelector('webview');
					if (webview && window.libroElectron) {
						try { webview.focus(); } catch(err) {}
						return;
					}

					var terminal = container.querySelector('[data-terminal]');
					if (terminal) {
						if (window.__libroFocusTerminalFrame) window.__libroFocusTerminalFrame(terminal);
						else try { terminal.focus({ preventScroll: true }); } catch(err) { try { terminal.focus(); } catch(err2) {} }
						return;
					}

					var iframe = container.querySelector('iframe');
					if (!iframe) return;
					iframe.focus();
					try {
						iframe.contentWindow.focus();
					} catch(err) {}
				}

				var terminalTarget = container.querySelector('[data-terminal]');
				focusAttempt();
				if (!terminalTarget) {
					setTimeout(focusAttempt, 40);
					setTimeout(focusAttempt, 120);
					setTimeout(focusAttempt, 260);
				}
			};

			window.__libroFocusAppByID = function(appID) {
				if (!appID) return;
				var strips = document.querySelectorAll('[id^="app-strip-"]');
				var strip = null;
				for (var s = 0; s < strips.length; s++) {
					var parent = strips[s].closest('[id^="project-main-"]');
					if (parent && parent.style.display !== 'none' && parent.style.visibility !== 'hidden' && parent.getAttribute('aria-hidden') !== 'true') {
						strip = strips[s];
						break;
					}
				}
				if (!strip) return;
				var sorted = window.__libroSortedApps(strip);
				for (var i = 0; i < sorted.length; i++) {
					if ((sorted[i].getAttribute('data-app-id') || '') === appID) {
						window.__libroFocusApp(i);
						return;
					}
				}
			};

			window.__libroMoveSelectedApp = function(direction) {
				if (direction === 'left') {
					__ws.call('app.move.left', {"sid": "%s"});
					return;
				}
				if (direction === 'right') {
					__ws.call('app.move.right', {"sid": "%s"});
				}
			};

			function libroKeyHandler(e) {
				var settingsPage=document.getElementById('workspace-settings');
				if(settingsPage&&!settingsPage.hidden){
					if(e.key==='Escape'){e.preventDefault();libroWorkspace.closeSettings();}
					return;
				}
				// Project dialog is browse-first now. A repeated Win/Cmd+N just refocuses
				// the same dialog, it does not toggle modes or open another UI.
				var projectDialog = document.getElementById('project-dialog');
				if (projectDialog && !projectDialog.classList.contains('hidden') && e.metaKey && (e.key === 'n' || e.key === 'N' || e.code === 'KeyN')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					var pickerInput = document.getElementById('project-input');
					if (pickerInput) pickerInput.focus();
					return;
				}

				if (e.metaKey && !e.ctrlKey && (e.key === ',' || e.code === 'Comma')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if(window.__libroResizeSelectedAppStep)window.__libroResizeSelectedAppStep(1,"%s");
					return;
				}
				if (e.metaKey && !e.ctrlKey && (e.key === '.' || e.code === 'Period')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if(window.__libroResizeSelectedAppStep)window.__libroResizeSelectedAppStep(-1,"%s");
					return;
				}
				if (e.metaKey && !e.ctrlKey && (e.key === '[' || e.code === 'BracketLeft')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					window.__libroMoveSelectedApp('left');
					return;
				}
				if (e.metaKey && !e.ctrlKey && (e.key === ']' || e.code === 'BracketRight')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					window.__libroMoveSelectedApp('right');
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
				if (e.metaKey && e.key === 'Enter' && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenTerminalApp) window.__libroOpenTerminalApp();
					return;
				}
				if (e.metaKey && (e.key === 'o' || e.key === 'O' || e.code === 'KeyO')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.libroWorkspace) libroWorkspace.launcher();
					return;
				}
				if (e.metaKey && (e.key === 'b' || e.key === 'B' || e.code === 'KeyB') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenBrowserApp) window.__libroOpenBrowserApp();
					return;
				}
				if (e.metaKey && (e.key === 'e' || e.key === 'E' || e.code === 'KeyE') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenNvimApp) window.__libroOpenNvimApp();
					return;
				}
				if (e.metaKey && (e.key === 'y' || e.key === 'Y' || e.code === 'KeyY') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenPiAgentApp) window.__libroOpenPiAgentApp();
					return;
				}
				if (e.metaKey && (e.key === 'n' || e.key === 'N' || e.code === 'KeyN') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenProjectDialog) window.__libroOpenProjectDialog();
					return;
				}
				if (e.metaKey && e.ctrlKey && (e.key === 'y' || e.key === 'Y' || e.code === 'KeyY')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenMoveProject) window.__libroOpenMoveProject();
					return;
				}
				if ((e.metaKey !== e.ctrlKey) && !e.altKey && !e.shiftKey && (e.key === ';' || e.code === 'Semicolon')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenCommandPalette) window.__libroOpenCommandPalette();
					return;
				}
				if (e.metaKey && (e.key === 'q' || e.key === 'Q') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.close.current', {"sid": "%s"});
					return;
				}

				if (e.metaKey && (e.key === 'f' || e.key === 'F') && !e.ctrlKey) {
					var appId = window.__libroSelectedApp || '';
					var appEl = appId ? document.querySelector('[data-app-id="' + appId + '"]') : null;
					if (!appEl) return;
					e.preventDefault();
					e.stopImmediatePropagation();
					if(window.__libroToggleSelectedAppMax)window.__libroToggleSelectedAppMax("%s");
					return;
				}
			}

			window.__libroOpenTerminalApp = function() {
				__ws.call('app.start',{sid:'%s',type:'terminal',url:'',command:'',writable:true,name:'',iconUrl:'',side:'right'});
			};

			window.__libroOpenBrowserApp = function() {
				__ws.call('app.browse.open',{sid:'%s',side:'right',popup:true});
			};

			window.__libroOpenNvimApp = function() {
				__ws.call('app.nvim.open',{sid:'%s'});
			};

			window.__libroOpenPiAgentApp = function() {
				__ws.call('app.pi.open',{sid:'%s'});
			};

			window.__libroCloseCurrentApp = function() {
				__ws.call('app.close.current', {"sid": "%s"});
			};

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
		`, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid)
}
