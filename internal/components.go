package libro

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	r "github.com/michalCapo/g-sui/ui"

	"libro/internal/components"
	"libro/internal/version"
)

// urlParse is a convenience wrapper around url.Parse.
func urlParse(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

const (
	MainAreaID            = "main-area"
	TopBarID              = "top-bar"
	ManageDialogID        = "manage-dialog"
	DialogID              = components.AddDialogID
	ProjectDialogID       = components.ProjectDialogID
	DirBrowserID          = components.DirBrowserID
	SearchDialogID        = components.SearchDialogID
	ShortcutsDialogID     = components.ShortcutsDialogID
	CloseDialogID         = components.CloseDialogID
	ProjectPickerID       = components.ProjectPickerID
	URLPopupID            = components.URLPopupID
	ResizePopupID         = components.ResizePopupID
	CommandPopupID        = components.CommandPopupID
	MoveProjectPopupID    = components.MoveProjectPopupID
	WorktreeCreatePopupID = components.WorktreeCreatePopupID
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
	"codex":   {URL: "https://cdn.simpleicons.org/openai/FFFFFF"},

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
	"git":     {URL: "https://cdn.simpleicons.org/git/F05032"},
	"hg":      {URL: "https://cdn.simpleicons.org/mercurial/999999"},
	"svn":     {URL: "https://cdn.simpleicons.org/subversion/809CC9"},
	"lazygit": {MaterialIcon: "account_tree"},

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
	"tmux":   {URL: "https://cdn.simpleicons.org/tmux/1BB91F"},
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
	return r.Div("flex-1 flex flex-row overflow-hidden relative").Render(
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

func savedAppsProjectName(state *AppState, projectName string) string {
	for _, project := range state.Projects {
		if project.Name == projectName {
			if project.Virtual && project.ParentProject != "" {
				return project.ParentProject
			}
			break
		}
	}
	return projectName
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

func projectHasClosedApps(state *AppState, projectName string) bool {
	if state == nil || projectName == "" {
		return false
	}
	if snap, ok := state.closedSnapshots[projectName]; ok {
		return snap != nil && len(snap.Apps) > 0
	}
	return false
}

// renderEmptyState renders the saved apps list with "+ Add App" button
func renderEmptyState(state *AppState, sid string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(savedAppsProjectName(state, state.ActiveProject))
	projectApps := make([]SavedApp, 0, len(savedApps))
	otherApps := make([]SavedApp, 0, len(savedApps))
	for _, app := range savedApps {
		if app.ProjectSpecific {
			projectApps = append(projectApps, app)
		} else {
			otherApps = append(otherApps, app)
		}
	}
	ordered := append(projectApps, otherApps...)
	appButtons := make([]*r.Node, 0, len(ordered))
	for _, app := range ordered {
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

	actionButtons := []*r.Node{}
	if projectHasClosedApps(state, state.ActiveProject) {
		actionButtons = append(actionButtons,
			r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-emerald-600 hover:bg-emerald-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
				Render(r.I("material-icons-round text-[18px]").Text("history"), r.Span("").Text("Restore")).
				OnClick(&r.Action{Name: "project.apps.open", Data: sidData(sid)}),
		)
	} else {
		actionButtons = append(actionButtons,
			r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
				Render(r.I("material-icons-round text-[18px]").Text("add"), r.Span("").Text("Add App")).
				OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
		)
	}
	actionButtons = append(actionButtons,
		r.Button("flex-1 flex items-center justify-center gap-1 px-6 py-3 bg-indigo-600 hover:bg-indigo-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
			Render(r.I("material-icons-round text-[18px]").Text("search"), r.Span("").Text("Quick Launch")).
			OnClick(&r.Action{Name: "app.run.open", Data: sidData(sid)}),
	)

	container := r.Div("flex-1 flex items-center justify-center px-4 py-6 overflow-y-auto").ID(projectMainID(state.ActiveProject)).Render(
		r.Div("flex flex-col items-center gap-3 w-full max-w-6xl").Render(
			guideNode,
			r.Div("grid w-full grid-cols-2 gap-3 max-[420px]:grid-cols-1 xl:grid-cols-3").Render(appButtons...),
			r.Div("flex gap-2 w-full max-w-xl mt-1 flex-wrap").Render(actionButtons...),
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

	launchBtn := r.Button("flex-1 flex items-center gap-3 px-4 py-3 text-left cursor-pointer transition-colors duration-75").
		Render(func() []*r.Node {
			nodes := []*r.Node{
				iconNode,
				r.Span("flex-1 truncate text-sm text-gray-800 dark:text-zinc-200").Text(label),
			}
			if app.ProjectSpecific {
				projectLabel := app.ProjectName
				if projectLabel == "" {
					projectLabel = "project"
				}
				nodes = append(nodes, r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-amber-100 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400").Text(projectLabel))
			}
			nodes = append(nodes,
				r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Type),
				r.Span("px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Width),
			)
			return nodes
		}()...).
		OnClick(&r.Action{Name: "app.start", Data: map[string]any{
			"sid": sid, "type": app.Type, "url": app.URL,
			"command": app.Command, "width": app.Width,
			"writable": writable, "name": app.Name,
			"iconUrl": app.IconURL,
		}})

	return r.Div("w-full flex items-center bg-white dark:bg-zinc-800/80 hover:bg-gray-50 dark:hover:bg-zinc-700/80 border border-gray-200 dark:border-zinc-700/40 hover:border-gray-300 dark:hover:border-zinc-600 rounded-lg transition-colors duration-75 shadow-sm dark:shadow-none").
		Render(launchBtn)
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
	var wv=((%s)||'md');
	var wr=document.getElementById('width-'+wv);if(wr)wr.checked=true;
	var wh=document.getElementById('app-width');if(wh)wh.value=wv;
	var ps=document.getElementById('app-project-specific');if(ps)ps.checked=%v;
},100);
`, components.JSString(app.Type), components.JSString(app.Command), app.Writable, components.JSString(app.URL), components.JSString(app.Name), components.JSString(app.Width), app.ProjectSpecific)
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

	strip := r.Div("flex-1 min-w-0 flex items-stretch h-full overflow-x-auto overflow-y-hidden gap-0.5 p-0.5 scrollbar-none").
		ID(stripID(state.ActiveProject)).
		Attr("style", "scrollbar-width:none;-ms-overflow-style:none").
		Render(stripChildren...)

	mainArea := r.Div("flex-1 flex items-stretch overflow-hidden relative p-0.5").ID(projectMainID(state.ActiveProject)).
		Render(
			strip,
		)
	mainArea.JS(centerSelectedJS(state))

	return mainArea
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
	return navigateProjectJS(state.ActiveProject, state.Apps, state.SelectedIndex, state.ZenMode, sid)
}

func navigateProjectJS(projectName string, apps []Application, selectedIndex int, zenMode bool, sid string) string {
	// Build JS to set CSS order on all app frames (keeps DOM stable for webviews)
	var orderJS strings.Builder
	for i, app := range apps {
		fmt.Fprintf(&orderJS, "var e=document.querySelector('[data-app-id=\"%s\"]');if(e)e.style.order='%d';", app.ID, i)
	}

	selectedID := "''"
	if selectedIndex >= 0 && selectedIndex < len(apps) {
		selectedID = components.JSString(apps[selectedIndex].ID)
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
					var appID = child.getAttribute('data-app-id') || '';
					var browserMode = window.__libroGetBrowserMode ? window.__libroGetBrowserMode(appID) : 'normal';
					var selectedBorderColor = browserMode === 'insert' ? 'border-emerald-500' : 'border-blue-500';
					var cleanCls = child.className.replace(/border-\[\dpx\]/g, '').replace(/border-t-\[\dpx\]/g, '').replace(/border-blue-500/g, '').replace(/border-emerald-500/g, '').replace(/border-gray-300 dark:border-zinc-600/g, '').replace(/border-transparent/g, '');
					if (zenMode) {
						child.className = cleanCls + ' border-[1px] border-t-[10px] ' + selectedBorderColor;
					} else {
						child.className = cleanCls + ' border-[1px] ' + selectedBorderColor;
					}
					var toolbar = child.children[0];
					// Make toolbar blue for selected app
					if (toolbar) {
						var selectedToolbarColor = browserMode === 'insert' ? ' bg-emerald-600 border-emerald-700' : ' bg-blue-600 border-blue-700';
						toolbar.className = 'flex items-center gap-2 px-1.5 py-1 border-b shrink-0' + selectedToolbarColor;
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
					var zenClose = child.querySelector('[data-zen-close]');
					if (zenClose) zenClose.style.display = zenMode ? 'flex' : 'none';
				} else {
					// Zen mode: gray border for unselected app, normal border otherwise
					var cleanCls2 = child.className.replace(/border-\[\dpx\]/g, '').replace(/border-t-\[\dpx\]/g, '').replace(/border-blue-500/g, '').replace(/border-gray-300 dark:border-zinc-600/g, '').replace(/border-transparent/g, '');
					child.className = cleanCls2 + (zenMode ? ' border-[1px] border-t-[10px] border-gray-300 dark:border-zinc-600' : ' border-[1px] border-transparent');
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
					var zenClose2 = child.querySelector('[data-zen-close]');
					if (zenClose2) zenClose2.style.display = zenMode ? 'flex' : 'none';
				}
			}

			window.__libroSelectedApp = %s;

			var selected = sorted[selectedIdx];
			if (selected) {
				selected.style.animation='none';
				selected.offsetHeight;
				selected.style.animation='libro-app-select .08s ease-out';
				if(window.__libroScrollToApp)window.__libroScrollToApp(selected);
				var selectedTermFrame = selected.querySelector('iframe[data-terminal-iframe]');
				if (selectedTermFrame && window.__libroFitTerminalFrame) {
					window.__libroFitTerminalFrame(selectedTermFrame);
					setTimeout(function(){ window.__libroFitTerminalFrame(selectedTermFrame); }, 250);
				}
				if (window.__libroFocusApp) {
					setTimeout(function() { window.__libroFocusApp(selectedIdx); }, 30);
				}
			}
		})();
	`, orderJS.String(), stripID(projectName), selectedIndex, len(apps), zenMode, sid, selectedID)
}

// popupRegistryJS registers the global helper used by every popup opener to
// hide any other popup that is currently visible. Pass the popup's element
// (or its ID) as `except` to keep that one open.
func popupRegistryJS() string {
	return fmt.Sprintf(`
(function(){
	var IDS=[%q,%q,%q,%q,%q,%q,%q,%q,%q,%q,%q];
	window.__libroCloseAllPopups=function(except){
		var keep=null;
		if(except){
			keep=(typeof except==='string')?document.getElementById(except):except;
		}
		for(var i=0;i<IDS.length;i++){
			var el=document.getElementById(IDS[i]);
			if(!el||el===keep)continue;
			if(!el.classList.contains('hidden'))el.classList.add('hidden');
		}
	};
})();
`, ProjectPickerID, URLPopupID, ResizePopupID, CommandPopupID, MoveProjectPopupID, WorktreeCreatePopupID, SearchDialogID, ShortcutsDialogID, CloseDialogID, DialogID, ProjectDialogID)
}

func flashCSS() string {
	return `(function(){if(!document.getElementById('libro-flash-css')){var s=document.createElement('style');s.id='libro-flash-css';s.textContent='@keyframes libro-flash{0%{transform:scale(1);opacity:1}15%{transform:scale(2.5);opacity:.6}100%{transform:scale(1);opacity:1}} @keyframes libro-toast-in{0%{opacity:0;transform:translate(-50%,-50%) scale(.98)}100%{opacity:1;transform:translate(-50%,-50%) scale(1)}} @keyframes libro-toast-out{0%{opacity:1;transform:translate(-50%,-50%) scale(1)}100%{opacity:0;transform:translate(-50%,-50%) scale(.98)}} @keyframes libro-toast-slide-up{0%{transform:translateY(100%);opacity:0}100%{transform:translateY(0);opacity:1}} @keyframes libro-toast-slide-down{0%{transform:translateY(0);opacity:1}100%{transform:translateY(100%);opacity:0}} @keyframes libro-app-select{0%{outline:2px solid rgba(59,130,246,.5)}100%{outline:2px solid transparent}} @keyframes libro-project-switch{0%{opacity:0}100%{opacity:1}}';document.head.appendChild(s);}window.__libroScrollToApp=function(app){var strip=app.parentElement;if(!strip)return;var sr=strip.getBoundingClientRect();var ar=app.getBoundingClientRect();var appLeft=ar.left-sr.left+strip.scrollLeft;var appRight=appLeft+ar.width;var pad=8;if(ar.width+pad*2>=sr.width){strip.scrollLeft=Math.max(0,appLeft-pad);return;}if(appLeft-pad<strip.scrollLeft){strip.scrollLeft=Math.max(0,appLeft-pad);}else if(appRight+pad>strip.scrollLeft+sr.width){strip.scrollLeft=Math.max(0,appRight+pad-sr.width);}};})();`
}

// projectToastJS returns JS that shows a brief centered toast with the project and branch.
func projectToastJS(name string) string {
	// Split virtual project names like "nisa/test" into project + branch
	proj := name
	branch := ""
	if before, after, ok := strings.Cut(name, "/"); ok {
		proj = before
		branch = after
	}
	return fmt.Sprintf("if(window.__libroProjectToast)window.__libroProjectToast(%s,%s);", components.JSString(proj), components.JSString(branch))
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
	var dlCompleted={};
	var dlToolbar=null;
	function dlEsc(s){
		return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
	}
	function dlCompletedList(){
		return Object.keys(dlCompleted).sort(function(a,b){return Number(a)-Number(b);}).map(function(k){return dlCompleted[k];});
	}
	function dlRemoveCompleted(id){
		delete dlCompleted[id];
		dlRenderToolbar();
	}
	function dlClearCompleted(){
		dlCompleted={};
		dlRenderToolbar();
	}
	function dlEnsureToolbar(){
		if(dlToolbar&&dlToolbar.parentNode)return dlToolbar;
		dlToolbar=document.createElement('div');
		dlToolbar.id='libro-dl-toolbar';
		document.body.appendChild(dlToolbar);
		return dlToolbar;
	}
	function dlRenderToolbar(){
		var files=dlCompletedList();
		if(files.length===0){
			if(dlToolbar&&dlToolbar.parentNode)dlDismiss(dlToolbar);
			dlToolbar=null;
			return;
		}
		var el=dlEnsureToolbar();
		var t=dlTheme();
		el.style.cssText='position:fixed;bottom:0;left:0;right:0;z-index:9999;padding:8px 12px;background:'+t.bg+';border-top:1px solid '+t.border+';display:flex;align-items:center;gap:8px;font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace;font-size:13px;box-shadow:0 -6px 18px rgba(0,0,0,.08);animation:libro-toast-slide-up .04s ease-out forwards;';
		var label=files.length===1?'Downloaded':'Downloads';
		var html='<span class="material-icons-round" style="font-size:18px;color:'+t.accent+';flex:0 0 auto">download_done</span>'+
			'<span style="color:'+t.fg+';font-size:12px;white-space:nowrap;flex:0 0 auto">'+label+'</span>'+
			'<div data-dl-files style="display:flex;align-items:center;gap:6px;flex:1;min-width:0;overflow-x:auto;padding:1px 0"></div>'+
			'<button type="button" data-dl-folder title="Open downloads folder" style="border:0;background:transparent;color:'+t.dim+';cursor:pointer;width:28px;height:28px;display:flex;align-items:center;justify-content:center;border-radius:6px;flex:0 0 auto"><span class="material-icons-round" style="font-size:19px">folder_open</span></button>'+
			'<button type="button" data-dl-close title="Clear downloads" style="border:0;background:transparent;color:'+t.dim+';cursor:pointer;width:28px;height:28px;display:flex;align-items:center;justify-content:center;border-radius:6px;flex:0 0 auto"><span class="material-icons-round" style="font-size:19px">close</span></button>';
		el.innerHTML=html;
		var list=el.querySelector('[data-dl-files]');
		files.forEach(function(file){
			var a=document.createElement('a');
			a.href='#';
			a.dataset.dlId=String(file.id);
			a.style.cssText='display:inline-flex;align-items:center;max-width:260px;min-width:0;padding:5px 8px;border:1px solid '+t.border+';border-radius:6px;color:'+t.accent+';background:'+(document.documentElement.classList.contains('dark')?'rgba(255,255,255,.04)':'rgba(255,255,255,.7)')+';text-decoration:none;cursor:pointer;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;flex:0 1 auto';
			a.title='Open '+file.filename;
			a.innerHTML='<span style="overflow:hidden;text-overflow:ellipsis">'+dlEsc(file.filename)+'</span>';
			a.addEventListener('click',function(e){
				e.preventDefault();
				var id=this.dataset.dlId;
				var item=dlCompleted[id];
				if(item&&window.libroElectron&&window.libroElectron.openPath)window.libroElectron.openPath(item.filePath);
				dlRemoveCompleted(id);
			});
			list.appendChild(a);
		});
		el.querySelector('[data-dl-folder]').addEventListener('click',function(){
			if(window.__libroOpenDownloadsFolder)window.__libroOpenDownloadsFolder();
		});
		el.querySelector('[data-dl-close]').addEventListener('click',function(){dlClearCompleted();});
	}
	window.__libroOpenDownloadsFolder=function(){
		if(window.libroElectron&&window.libroElectron.openDownloadsFolder)window.libroElectron.openDownloadsFolder();
		dlClearCompleted();
	};
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
		var progressEl=document.getElementById('libro-dl-'+id);
		if(progressEl)dlDismiss(progressEl);
		dlCompleted[id]={id:id,filename:filename,filePath:filePath};
		dlRenderToolbar();
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
	return fmt.Sprintf("if(window.__libroShowToast)window.__libroShowToast(%s,%s,%d);", components.JSString(title), components.JSString(subtitle), durationMs)
}

// renderAppFrame renders a single application iframe with controls
func renderAppFrame(app Application, index int, selected bool, sid string, zenMode ...bool) *r.Node {
	zen := len(zenMode) > 0 && zenMode[0]
	var borderClass string
	if zen {
		if selected {
			borderClass = "border-[1px] border-t-[10px] border-blue-500"
		} else {
			borderClass = "border-[1px] border-t-[10px] border-gray-300 dark:border-zinc-600"
		}
	} else {
		if selected {
			borderClass = "border-[1px] border-blue-500"
		} else {
			borderClass = "border-[1px] border-transparent"
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
			On("keydown", r.JS(fmt.Sprintf(`if(event.key==='Enter'){event.preventDefault();var u=event.target.value.trim();if(u&&!u.startsWith('http://')&&!u.startsWith('https://')){if(/\s/.test(u)||(!u.includes('.')&&!u.includes(':'))){u='https://www.google.com/search?q='+encodeURIComponent(u);}else{u=(/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(:|$)/i.test(u)?'http://':'https://')+u;}}event.target.value=u;window.__libroWvNavigate('%s',u);__ws.call('app.url.set',{"sid":"%s","id":"%s","url":u});event.target.blur();var target=document.querySelector('[data-webview-app="%s"], iframe[data-browser-iframe-app="%s"]');if(target)target.focus();}`, app.ID, sid, app.ID, app.ID, app.ID)))

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

	zenCloseBtn := r.Button("hidden absolute top-2 right-2 z-50 w-7 h-7 items-center justify-center rounded-full cursor-pointer bg-black/45 text-white/80 backdrop-blur-sm transition-colors duration-75 hover:bg-red-500/85 hover:text-white").
		Attr("data-zen-close", "").
		Attr("title", "Close app").
		OnClick(&r.Action{
			Name: "app.close",
			Data: sidData(sid, "id", app.ID),
		}).
		Render(r.I("material-icons-round text-[16px] leading-none block").Text("close"))
	if zen {
		zenCloseBtn = zenCloseBtn.Attr("style", "display:flex")
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
			zenCloseBtn,
		)
}

func renderIframe(app Application, frameID, iframeSrc, sid string) *r.Node {
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
			ID(frameID).
			Attr("data-browser-iframe-app", app.ID).
			Attr("data-sid", sid).
			Attr("src", browserSrc).
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
		container := r.Div("w-full h-full absolute inset-0 z-30 flex flex-col").Render(webviewWrapper, devtoolsPanel)
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
		Attr("data-terminal-iframe", app.ID).
		Attr("src", iframeSrc).
		Attr("loading", "eager").
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

func settleAppFrameJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var appID=%s;
	function run(){
		if(window.__libroSettleAppFrame)window.__libroSettleAppFrame(appID);
	}
	run();
	setTimeout(run, 100);
	setTimeout(run, 500);
	setTimeout(run, 1500);
})();
`, components.JSString(appID))
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
setTimeout(function(){
	if(window.__libroFocusApp) window.__libroFocusApp(%d);
}, 30);`, components.JSString(appID), state.SelectedIndex)
}

// removeAppJS returns JS that removes an app frame by its app ID from the strip.
// For URL apps with webviews, the webview element is moved to a hidden pool
// so its session state (cookies, WebSocket connections) stays alive.
func removeAppJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(!el)return;
	['resize-popup','url-popup'].forEach(function(pid){var p=document.getElementById(pid);if(p&&el.contains(p)){p.classList.add('hidden');document.body.appendChild(p);}});
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
	['resize-popup','url-popup'].forEach(function(pid){var p=document.getElementById(pid);if(p&&el.contains(p)){p.classList.add('hidden');document.body.appendChild(p);}});
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

// renderAddDialog renders the add application modal dialog
func renderAddDialog(visible bool, sid string) *r.Node {
	widths := AllWidths()
	strs := make([]string, 0, len(widths))
	for _, w := range widths {
		strs = append(strs, string(w))
	}
	return components.AddDialog(components.AddDialogInput{
		Sid:          sid,
		Visible:      visible,
		Widths:       strs,
		DefaultWidth: string(WidthLG),
	})
}

// renderSearchDialog renders the fuzzy search popup (hidden by default).
// All filtering, navigation, and selection logic runs client-side via JS
// since saved apps are injected via __libroSavedApps from the DB.
func renderSearchDialog(sid string) *r.Node {
	return components.SearchDialog(sid)
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
		if(qi!==query.length)return 0;
		var idx=text.indexOf(query);
		if(idx>=0){
			score+=12;
			if(idx===0)score+=8;
			if(idx===0||text[idx-1]===' '||text[idx-1]==='/'||text[idx-1]==='.'||text[idx-1]==='-'||text[idx-1]==='_')score+=6;
		}
		return score;
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
				if(!hoverEnabled)return;
				if(selIdx===i)return;
				var prev=res.children[selIdx];
				if(prev)prev.className=prev.className.replace(/bg-blue-900\/30|bg-blue-50/g,'').replace(/border-blue-500/g,'border-transparent')+(dk?' hover:bg-zinc-800':' hover:bg-gray-50');
				selIdx=i;
				row.className=row.className.replace(/hover:bg-zinc-800|hover:bg-gray-50/g,'').replace(/border-transparent/g,'border-blue-500')+(dk?' bg-blue-900/30':' bg-blue-50');
			};
			row.onclick=function(){launch();};
			res.appendChild(row);
		});
		// Always add "Add app" button at the bottom
		var addRow=document.createElement('div');
		var addSel=(filtered.length===0&&selIdx===0)||(selIdx===filtered.length);
		var dk=document.documentElement.classList.contains('dark');
		addRow.className='flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 border-t border-gray-100 dark:border-zinc-800 '
			+(addSel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
			:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'));
		addRow.className+=' group';
		var addTxtCls=dk?'text-zinc-200':'text-gray-800';
		addRow.innerHTML='<i class="material-icons-round text-emerald-500 text-lg shrink-0">add_circle</i>'
			+'<div class="flex-1 min-w-0"><div class="text-sm '+addTxtCls+'">Add new application</div></div>';
		addRow.onmouseenter=function(){
			if(!hoverEnabled)return;
			if(selIdx===filtered.length)return;
			var prev=res.children[selIdx];
			if(prev)prev.className=prev.className.replace(/bg-blue-900\/30|bg-blue-50/g,'').replace(/border-blue-500/g,'border-transparent')+(dk?' hover:bg-zinc-800':' hover:bg-gray-50');
			selIdx=filtered.length;
			addRow.className=addRow.className.replace(/hover:bg-zinc-800|hover:bg-gray-50/g,'').replace(/border-transparent/g,'border-blue-500')+(dk?' bg-blue-900/30':' bg-blue-50');
		};
		addRow.onclick=function(){
			dlg.classList.add('hidden');
			inp.value='';
			__ws.call('app.dialog.open',{sid:'%s',side:pendingSide});
		};
		res.appendChild(addRow);
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
		if(selIdx===filtered.length){
			dlg.classList.add('hidden');
			inp.value='';
			__ws.call('app.dialog.open',{sid:'%s',side:pendingSide});
			return;
		}
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
		// Empty browser — use app.browse.open to open a blank tab with URL bar focused
		if(item.isBrowse&&!app.url){
			__ws.call('app.browse.open',{sid:'%s',side:side});
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
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		inp.value='';
		filter();
		armHoverAfterPointerMove();
		setTimeout(function(){inp.focus();},50);
	}

	function closeSearch(){
		dlg.classList.add('hidden');
		inp.value='';
		hoverEnabled=false;
	}

	inp.addEventListener('input',filter);
	inp.addEventListener('keydown',function(e){
		e.stopImmediatePropagation();
		if(e.key==='ArrowDown'){
			e.preventDefault();
			if(selIdx<filtered.length){selIdx++;render();}
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
`, SearchDialogID, sid, sid, sid, sid, sid, sid, sid, sid, sid)
}

// renderURLPopup renders the URL/search popup opened by bare 'o' (works in both zen and non-zen mode).
func renderURLPopup(sid string) *r.Node {
	return components.URLPopup(sid)
}

// urlPopupJS returns JS that powers the URL popup.
func urlPopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var currentAppId='';
	var selectedIdx=-1;
	var filteredURLs=[];
	var originalQuery='';

	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('url-popup-input');}
	function getHistory(){return document.getElementById('url-popup-history');}

	function findSelectedBrowserApp(){
		var appId=window.__libroSelectedApp||'';
		if(!appId)return '';
		var el=document.querySelector('[data-app-id="'+appId+'"]');
		if(!el)return '';
		return el.querySelector('webview[data-webview-app], iframe[data-browser-iframe-app]')?appId:'';
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
		if(selectedIdx<0||selectedIdx>=filteredURLs.length){
			selectedIdx=0;
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

	function openPopup(targetAppId, initialValue){
		var appId=targetAppId||findSelectedBrowserApp();
		if(!appId)return;
		currentAppId=appId;
		var dlg=getDlg();
		if(!dlg)return;
		var contentArea=document.querySelector('[data-app-content="'+appId+'"]');
		if(contentArea)contentArea.appendChild(dlg);
		var inp=getInp();
		var wv=window.__libroWebviews[appId];
		var hasInitialValue=typeof initialValue==='string';
		var currentUrl=hasInitialValue?initialValue:'';
		if(wv&&!hasInitialValue){try{currentUrl=wv.getURL()||'';}catch(e){}}
		if(!currentUrl&&!hasInitialValue){
			var urlInp=document.getElementById('urlinput-'+appId);
			if(urlInp)currentUrl=urlInp.value||'';
		}
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		if(inp){
			inp.value=currentUrl;
		}
		selectedIdx=0;
		originalQuery='';
		renderHistory('');
		function focusPopupInput(){
			var i=getInp();
			if(!i)return;
			var active=document.activeElement;
			if(active&&active!==i&&typeof active.blur==='function'){
				try{active.blur();}catch(e){}
			}
			try{window.focus();}catch(e){}
			try{i.focus({preventScroll:true});}catch(e){try{i.focus();}catch(e2){}}
			try{i.select();}catch(e){}
		}
		focusPopupInput();
		setTimeout(focusPopupInput,20);
		setTimeout(focusPopupInput,80);
		setTimeout(focusPopupInput,180);
		setTimeout(focusPopupInput,320);
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		var historyContainer=getHistory();
		var appId=currentAppId;
		if(dlg)dlg.classList.add('hidden');
		if(inp){try{inp.blur();}catch(e){}inp.value='';}
		currentAppId='';
		selectedIdx=-1;
		originalQuery='';
		if(historyContainer)historyContainer.innerHTML='';
		if(appId){
			var wv=window.__libroWebviews&&window.__libroWebviews[appId];
			function refocus(){
				if((window.__libroSelectedApp||'')!==appId)return;
				try{window.focus();}catch(e){}
				if(wv){try{wv.focus();}catch(e){}}
			}
			refocus();
			setTimeout(refocus,40);
			setTimeout(refocus,120);
			setTimeout(refocus,260);
		}
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

	var inp=getInp();
	if(inp){
		inp.addEventListener('input',function(){
			selectedIdx=0;
			originalQuery=inp.value;
			renderHistory(inp.value);
		});

		inp.addEventListener('keydown',function(e){
			var dlg=getDlg();
			if(!dlg||dlg.classList.contains('hidden'))return;
			e.stopImmediatePropagation();
			if(e.key==='ArrowDown'){
				e.preventDefault();
				if(filteredURLs.length>0){
					selectedIdx=Math.min(selectedIdx+1,filteredURLs.length-1);
					renderHistory(originalQuery);
					inp.value=filteredURLs[selectedIdx];
				}
			}else if(e.key==='ArrowUp'){
				e.preventDefault();
				if(filteredURLs.length>0&&selectedIdx>0){
					selectedIdx--;
					renderHistory(originalQuery);
					inp.value=filteredURLs[selectedIdx];
				}else if(selectedIdx===0){
					selectedIdx=-1;
					renderHistory(originalQuery);
					inp.value=originalQuery;
				}
			}else if(e.key==='Enter'){
				e.preventDefault();
				if(selectedIdx>=0&&filteredURLs[selectedIdx]&&inp&&inp.value===originalQuery){
					var raw=(originalQuery||'').trim();
					var looksLikeDirect=/^https?:\/\//i.test(raw)||/^(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|\[::0?\])(\:\d+)?(\/|$)/i.test(raw)||(/^[^\s]+\.[^\s]+$/.test(raw)&&!raw.includes(' '));
					if(raw&&!looksLikeDirect&&!/\s/.test(raw))inp.value=filteredURLs[selectedIdx];
				}
				navigate();
			}else if(e.key==='Escape'){
				e.preventDefault();
				closePopup();
			}
		});
	}

	window.__libroOpenURLPopup=function(){openPopup();};
	window.__libroOpenURLPopupFor=function(appId,initialValue){openPopup(appId,initialValue);};
})();
`, URLPopupID, sid)
}

// renderResizePopup renders the resize popup for Win+R (works in both zen and non-zen mode).
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
			isTerminal: !!el.querySelector('iframe:not([data-browser-iframe-app])') && !el.querySelector('webview[data-webview-app], iframe[data-browser-iframe-app]')
		};
	}

	function commandDefinitions(){
		var selected=selectedAppInfo();
		var commands=[
			{id:'open',label:'Open',scope:'app',icon:'add',keywords:'launcher quick launch open create app project search run browse',run:function(){
				closePalette();
				if(window.__libroOpenSearch)window.__libroOpenSearch('right');
			}},
			{id:'new',label:'New',scope:'app',icon:'add_box',keywords:'new add create application app popup dialog project',run:function(){
				closePalette();
				__ws.call('app.dialog.open',{sid:'%s'});
			}},
			{id:'apps',label:'Apps',scope:'app',icon:'apps',keywords:'manage saved applications app list',run:function(){
				closePalette();
				__ws.call('app.manage.open',{sid:'%s'});
			}},
			{id:'downloads',label:'Downloads',scope:'app',icon:'folder_open',keywords:'download downloads downloaded files folder directory open file manager',run:function(){
				closePalette();
				if(window.__libroOpenDownloadsFolder)window.__libroOpenDownloadsFolder();
				else if(window.libroElectron&&window.libroElectron.openDownloadsFolder)window.libroElectron.openDownloadsFolder();
			}},
			{id:'terminal',label:'Open terminal',scope:'app',icon:'terminal',keywords:'terminal shell console bash tty panel libro',run:function(){
				closePalette();
				if(window.__libroOpenTerminalApp)window.__libroOpenTerminalApp();
			}},
			{id:'quit',label:'Quit Libro',scope:'app',icon:'close',keywords:'quit close exit app window desktop',run:function(){
				closePalette();
				if(window.__libroShowCloseDialog)window.__libroShowCloseDialog();
			}},
			{id:'close',label:'Close all apps in project',scope:'project',icon:'close',keywords:'close project apps clear strip remove opened apps',run:function(){
				closePalette();
				__ws.call('project.apps.close',{sid:'%s'});
			}},
			{id:'save',label:'Save open apps',scope:'project',icon:'save',keywords:'save persist project apps reopen restore positions order snapshot',run:function(){
				closePalette();
				__ws.call('project.apps.save',{sid:'%s'});
			}},
			{id:'clean',label:'Clean saved apps',scope:'project',icon:'delete_sweep',keywords:'clean clear remove saved persisted reopen closed project apps snapshot database',run:function(){
				closePalette();
				__ws.call('project.apps.clean',{sid:'%s'});
			}},
			{id:'console',label:'App console',scope:'app',icon:'code',keywords:'devtools app console inspector developer tools',run:function(){
				closePalette();
				if(window.libroElectron&&window.libroElectron.toggleDevTools)window.libroElectron.toggleDevTools();
			}},
			{id:'keymap',label:'Keymap',scope:'app',icon:'keyboard',keywords:'shortcuts keyboard help key bindings',run:function(){
				closePalette();
				if(window.__libroOpenShortcuts)window.__libroOpenShortcuts();
			}},
			{id:'zoom-in',label:'Zoom in',scope:'app',icon:'zoom_in',keywords:'zoom increase larger bigger app scale',run:function(){
				closePalette();
				if(window.libroElectron&&window.libroElectron.zoomIn)window.libroElectron.zoomIn();
			}},
			{id:'zoom-out',label:'Zoom out',scope:'app',icon:'zoom_out',keywords:'zoom decrease smaller app scale',run:function(){
				closePalette();
				if(window.libroElectron&&window.libroElectron.zoomOut)window.libroElectron.zoomOut();
			}},
			{id:'zoom-reset',label:'Reset zoom',scope:'app',icon:'filter_center_focus',keywords:'zoom reset normal default app scale',run:function(){
				closePalette();
				if(window.libroElectron&&window.libroElectron.zoomReset)window.libroElectron.zoomReset();
			}},
			{id:'restore',label:'Restore saved apps',scope:'project',icon:'history',keywords:'restore reopen saved project apps snapshot strip',run:function(){
				closePalette();
				__ws.call('project.apps.open',{sid:'%s'});
			}},
			{id:'projects',label:'Projects',scope:'app',icon:'source',keywords:'projects switch picker sidebar navigation tree',run:function(){
				closePalette();
				if(window.__libroOpenProjectPicker)window.__libroOpenProjectPicker();
			}},
			{id:'project-new',label:'New project (browse folder)',scope:'app',icon:'create_new_folder',keywords:'new project create folder browse directory path filesystem',run:function(){
				closePalette();
				if(window.__libroOpenProjectDialog)window.__libroOpenProjectDialog();
			}},
			{id:'worktree-new',label:'New worktree from current branch',scope:'project',icon:'alt_route',keywords:'worktree branch git fork create new',run:function(){
				closePalette();
				if(window.__libroOpenWorktreeCreate)window.__libroOpenWorktreeCreate();
			}},
			{id:'project-remove',label:'Remove current project',scope:'project',icon:'delete_outline',keywords:'remove delete current active project unregister forget drop',run:function(){
				closePalette();
				var list=window.__libroProjects||[];
				var active=null;
				for(var i=0;i<list.length;i++){if(list[i].isActive){active=list[i];break;}}
				if(!active){if(window.__libroShowToast)window.__libroShowToast('No active project','',2000);return;}
				if(active.kind==='worktree'){if(window.__libroShowToast)window.__libroShowToast('Cannot remove a worktree from here','Use git worktree remove instead',2500);return;}
				if(active.name==='home'){if(window.__libroShowToast)window.__libroShowToast('Cannot remove home project','',2000);return;}
				if(!window.confirm('Remove project "'+active.name+'" from Libro?\n\nThis only removes it from the project list — files on disk are kept.'))return;
				__ws.call('project.remove',{sid:'%s',name:active.name});
			}},
			{id:'zen',label:'Zen',scope:'app',icon:'fullscreen',keywords:'focus chrome toggle minimal',run:function(){
				closePalette();
				__ws.call('zen.toggle',{sid:'%s'});
			}},
		];
		if(selected){
			commands.push({
				id:'resize',
				label:'Resize',
				scope:'selected app',
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
				scope:'selected app',
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
				keywords:'terminal ttyd tmux kill restart emergency reset backend websocket session',
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
			var txtCls=dk?'text-zinc-200':'text-gray-800';
			var subCls=dk?'text-zinc-500':'text-gray-400';
			var badgeCls=dk?'bg-zinc-700 text-zinc-400':'bg-gray-200 text-gray-500';
			row.className='flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'));
			row.innerHTML='<i class="material-icons-round text-blue-500 text-lg shrink-0">'+cmd.icon+'</i>'
				+'<div class="flex-1 min-w-0"><div class="text-sm truncate '+txtCls+'">'+cmd.id+'</div>'
				+'<div class="text-[11px] truncate '+subCls+'">'+cmd.label+'</div></div>'
				+'<span class="px-1.5 py-0.5 text-[10px] font-mono uppercase rounded shrink-0 '+badgeCls+'">'+cmd.scope+'</span>';
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
			if(selIdx<filtered.length-1){selIdx++;render();}
		}else if(e.key==='ArrowUp'){
			e.preventDefault();
			if(selIdx>0){selIdx--;render();}
		}else if(e.key==='Enter'){
			e.preventDefault();
			execute();
		}else if(e.key==='Escape'){
			e.preventDefault();
			closePalette();
		}
	});

window.__libroOpenCommandPalette=openPalette;
})();
`, CommandPopupID, sid, sid, sid, sid, sid, sid, sid, sid, sid)
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

// resizePopupJS returns JS that powers the Win+R resize popup.
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
		if(btns.length>0)highlightFocused(idx);
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		setTimeout(function(){dlg.focus();},50);
	}

	function closePopup(){
		var appId=currentAppId;
		var dlg=getDlg();
		if(dlg)dlg.classList.add('hidden');
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
	return components.ShortcutsDialog()
}

// shortcutsDialogJS returns JS to open/close the shortcuts dialog and handle Esc.
func shortcutsDialogJS() string {
	return fmt.Sprintf(`
(function(){
	var dlg=document.getElementById('%s');
	window.__libroOpenShortcuts=function(){
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
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
			if(window.__electronCloseAbort)window.__electronCloseAbort();
		}
	},true);
})();
`, sid, CloseDialogID)
}

// renderManageAppsPage renders the manage apps popup.
func renderManageAppsPage(state *AppState, sid string) *r.Node {
	hiddenClass := " hidden"
	if state.ManageOpen {
		hiddenClass = ""
	}

	savedApps := DBLoadAllSavedApps()

	globalApps := make([]SavedApp, 0, len(savedApps))
	projectApps := make(map[string][]SavedApp)
	for _, app := range savedApps {
		if app.ProjectSpecific {
			projectApps[app.ProjectName] = append(projectApps[app.ProjectName], app)
			continue
		}
		globalApps = append(globalApps, app)
	}
	sort.Slice(globalApps, func(i, j int) bool {
		return strings.ToLower(savedAppDisplayLabel(globalApps[i])) < strings.ToLower(savedAppDisplayLabel(globalApps[j]))
	})

	projectOrder := make([]string, 0, len(projectApps))
	seenProjects := make(map[string]bool, len(projectApps))
	for _, project := range state.Projects {
		if len(projectApps[project.Name]) == 0 {
			continue
		}
		projectOrder = append(projectOrder, project.Name)
		seenProjects[project.Name] = true
	}
	extraProjects := make([]string, 0)
	for projectName := range projectApps {
		if seenProjects[projectName] {
			continue
		}
		extraProjects = append(extraProjects, projectName)
	}
	sort.Slice(extraProjects, func(i, j int) bool {
		return strings.ToLower(extraProjects[i]) < strings.ToLower(extraProjects[j])
	})
	projectOrder = append(projectOrder, extraProjects...)

	sections := make([]*r.Node, 0, len(projectOrder)+1)
	if len(globalApps) > 0 {
		sections = append(sections, renderManageAppSection("Global Apps", "Available across all projects", globalApps, sid))
	}
	for _, projectName := range projectOrder {
		apps := projectApps[projectName]
		sort.Slice(apps, func(i, j int) bool {
			return strings.ToLower(savedAppDisplayLabel(apps[i])) < strings.ToLower(savedAppDisplayLabel(apps[j]))
		})
		sections = append(sections, renderManageAppSection(projectName, "Project-specific apps", apps, sid))
	}

	var listNode *r.Node
	if len(sections) == 0 {
		listNode = r.Div("flex items-center justify-center py-10").Render(
			r.P("text-sm font-mono text-gray-400 dark:text-zinc-500").Text("No saved apps yet"),
		)
	} else {
		listNode = r.Div("px-4 py-4 space-y-6").Render(sections...)
	}

	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[8vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75" + hiddenClass).
		ID(ManageDialogID).
		OnClick(r.JS(components.HideJS(ManageDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-6xl mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("apps"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Manage Apps"),
						r.Button("text-[11px] font-mono text-blue-600 dark:text-blue-400 hover:underline cursor-pointer").
							Attr("title", "Add new app").
							OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}).
							Render(
								r.I("material-icons-round text-sm align-middle mr-0.5").Text("add"),
								r.Span("align-middle").Text("Add App"),
							),
					),
					r.Div("max-h-[60vh] overflow-y-auto").Render(listNode),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Esc close"),
					),
				),
		)
}

func manageAppsJS() string {
	return fmt.Sprintf(`
		(function(){
			var dlg=document.getElementById('%s');
			if(!dlg)return;
			document.addEventListener('keydown',function(e){
				if(e.key==='Escape'&&!dlg.classList.contains('hidden')){
					e.preventDefault();e.stopImmediatePropagation();
					dlg.classList.add('hidden');
				}
			},true);
		})();
	`, ManageDialogID)
}

func renderManageAppSection(title, subtitle string, apps []SavedApp, sid string) *r.Node {
	rows := make([]*r.Node, 0, len(apps))
	for _, app := range apps {
		rows = append(rows, renderManageAppRow(app, sid))
	}

	children := []*r.Node{
		r.Div("px-1").Render(
			r.Div("text-sm font-mono font-bold uppercase tracking-[0.18em] text-gray-900 dark:text-zinc-100").Text(title),
		),
	}
	if subtitle != "" {
		children = append(children, r.Div("px-1 text-[11px] font-mono uppercase tracking-[0.14em] text-gray-400 dark:text-zinc-500").Text(subtitle))
	}
	children = append(children,
		r.Div("grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3").Render(rows...),
	)

	return r.Div("space-y-2").Render(children...)
}

// renderManageAppRow renders a single row in the manage apps page.
func renderManageAppRow(app SavedApp, sid string) *r.Node {
	var iconNode *r.Node
	label := savedAppDisplayLabel(app)

	if app.Type == "terminal" {
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

	badges := make([]*r.Node, 0, 3)
	if app.ProjectSpecific {
		badges = append(badges, r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-amber-100 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400").Text("project"))
	}
	badges = append(badges,
		r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Type),
		r.Span("px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider rounded shrink-0 bg-gray-200 dark:bg-zinc-700 text-gray-600 dark:text-zinc-300").Text(app.Width),
	)

	editBtn := r.Button("flex items-center justify-center w-8 h-8 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-200 hover:bg-gray-200 dark:hover:bg-zinc-700 transition-colors").
		Render(r.I("material-icons-round text-lg").Text("edit")).
		Attr("onclick", fmt.Sprintf(`__ws.callSilent('app.saved.edit',{sid:'%s',dbid:%d});__ws.call('app.dialog.open',{sid:'%s'});`, sid, app.DBID, sid)+
			savedAppEditFillJS(app))

	deleteBtn := r.Button("flex items-center justify-center w-8 h-8 rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10 transition-colors").
		Render(r.I("material-icons-round text-lg").Text("delete")).
		OnClick(&r.Action{Name: "app.saved.delete", Data: sidData(sid, "dbid", float64(app.DBID))})

	headerChildren := []*r.Node{
		iconNode,
		r.Div("min-w-0 flex-1").Render(
			r.Span("block truncate text-sm font-medium text-gray-800 dark:text-zinc-200").Text(label),
		),
	}

	return r.Div("flex h-full min-h-[112px] flex-col gap-3 rounded-xl border border-gray-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/60 px-4 py-3 hover:bg-gray-50 dark:hover:bg-zinc-800/50 transition-colors").Render(
		r.Div("flex items-center gap-3 min-w-0").Render(headerChildren...),
		r.Div("flex flex-wrap items-center gap-2").Render(badges...),
		r.Div("mt-auto flex items-center justify-end gap-2").Render(editBtn, deleteBtn),
	)
}

func savedAppDisplayLabel(app SavedApp) string {
	label := app.Name
	if app.Type == "terminal" {
		if label == "" {
			label = app.Command
		}
		return label
	}
	if label == "" {
		label = app.URL
	}
	return label
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
`, appID, widthMap, string(width), appID, appID, appID, appID)
}

// renderProjectBar renders the horizontal project switcher bar
// renderTopBar renders the top bar with saved app icons, browse/add buttons, and action buttons.
func renderTopBar(state *AppState, sid string) *r.Node {
	savedApps := DBLoadVisibleSavedApps(savedAppsProjectName(state, state.ActiveProject))

	btnCls := "w-9 h-9 flex items-center justify-center rounded-md cursor-pointer transition-colors duration-75 hover:bg-gray-200 dark:hover:bg-zinc-700 relative group/ico"
	tipCls := "absolute top-full mt-1 px-2 py-1 text-xs rounded bg-white dark:bg-zinc-800 text-gray-800 dark:text-zinc-200 border border-gray-200 dark:border-zinc-700 whitespace-nowrap opacity-0 group-hover/ico:opacity-100 pointer-events-none transition-opacity z-[200] shadow-lg"
	noDragStyle := "-webkit-app-region:no-drag"

	// Core action icons — rendered next to the libro logo.
	coreIcons := []*r.Node{
		// Quick launch button (combined browse + run)
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-indigo-600 dark:hover:text-indigo-400 text-xl").Text("search"),
				r.Span(tipCls).Text("Quick launch"),
			).
			OnClick(&r.Action{Name: "app.run.open", Data: sidData(sid)}),
		// Add app
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-blue-600 dark:hover:text-blue-400 text-[18px]").Text("add"),
				r.Span(tipCls).Text("Add app"),
			).
			OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
		// Manage apps
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("apps"),
				r.Span(tipCls).Text("Manage apps"),
			).
			OnClick(&r.Action{Name: "app.manage.open", Data: sidData(sid)}),
		// Commands
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Attr("onclick", "if(window.__libroOpenCommandPalette)window.__libroOpenCommandPalette();").
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-indigo-600 dark:hover:text-indigo-400 text-xl").Text("menu_open"),
				r.Span(tipCls).Text("Commands"),
			),
		// Shortcuts
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Attr("onclick", fmt.Sprintf("document.getElementById('%s').classList.toggle('hidden');", ShortcutsDialogID)).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("keyboard"),
				r.Span(tipCls).Text("Shortcuts"),
			),
		// Console
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Attr("onclick", "if(window.libroElectron&&window.libroElectron.toggleDevTools)window.libroElectron.toggleDevTools();").
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 text-xl").Text("code"),
				r.Span(tipCls).Text("Console"),
			),
		// Zen mode
		r.Button(btnCls).
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			OnClick(&r.Action{Name: "zen.toggle", Data: sidData(sid, "source", "click")}).
			Render(
				r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-amber-600 dark:hover:text-amber-400 text-xl libro-zen-icon").Text(func() string {
					if state.ZenMode {
						return "visibility"
					}
					return "self_improvement"
				}()),
				r.Span(tipCls).Text("Zen mode"),
			),
	}

	// Saved app launchers
	appIcons := make([]*r.Node, 0, len(savedApps))
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
			Attr("data-libro-no-drag", "true").
			Attr("style", noDragStyle).
			Render(iconNode, r.Span(tipCls).Text(label)).
			OnClick(&r.Action{Name: "app.start", Data: map[string]any{
				"sid": sid, "type": app.Type, "url": app.URL,
				"command": app.Command, "width": app.Width,
				"writable": app.Writable, "name": app.Name,
				"iconUrl": app.IconURL,
			}})
		appIcons = append(appIcons, btn)
	}

	// Running apps preview strip
	appPreview := renderAppPreview(state, sid)

	// In zen mode, hide the top bar content
	if state.ZenMode {
		return r.Div("shrink-0").ID(TopBarID)
	}

	return r.Div("flex items-center gap-1.5 px-3 py-1.5 border-b border-gray-200 dark:border-zinc-800 shrink-0").
		ID(TopBarID).
		Attr("style", "-webkit-app-region:drag").
		Attr("ondblclick", "if(event.target&&event.target.closest&&event.target.closest('[data-libro-no-drag]'))return;if(window.libroElectron&&window.libroElectron.toggleMaximize)window.libroElectron.toggleMaximize();").
		Render(
			r.Button("shrink-0 cursor-pointer hover:opacity-70 transition-opacity duration-75 flex items-center").
				Attr("data-libro-no-drag", "true").
				Attr("style", noDragStyle).
				Attr("title", "Open project picker (⌘B)").
				Attr("onclick", "if(window.__libroOpenProjectPicker)window.__libroOpenProjectPicker();").
				Render(
					r.Img("w-7 h-7").Attr("src", "/assets/logo.svg").Attr("alt", "Libro"),
				),
			r.Div("flex items-center gap-0.5").Render(coreIcons...),
			r.Div("w-px h-5 bg-gray-300 dark:bg-zinc-700 mx-1").Render(),
			r.Div("flex items-center gap-0.5").Render(appIcons...),
			r.Div("ml-auto flex items-center gap-1").Render(appPreview),
			r.Span("text-[10px] text-gray-400 dark:text-gray-500 font-mono select-none ml-2").Text("v"+version.Version),
			r.Button(btnCls).
				Attr("data-libro-no-drag", "true").
				Attr("style", noDragStyle).
				Attr("title", "Use quit command").
				Attr("onclick", "if(window.__libroNotifyQuitCommandOnly)window.__libroNotifyQuitCommandOnly();").
				Render(
					r.I("material-icons-round text-gray-400 dark:text-zinc-500 hover:text-red-600 dark:hover:text-red-400 text-xl").Text("close"),
					r.Span(tipCls).Text("Use quit command"),
				),
			r.Div("").
				Attr("data-libro-no-drag", "true").
				Attr("style", noDragStyle).
				Render(r.ThemeSwitcher()),
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
			Attr("data-libro-no-drag", "true").
			Attr("style", "-webkit-app-region:no-drag").
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

// renderProjectPickerPopup renders the project picker modal.
func renderProjectPickerPopup(sid string) *r.Node {
	return components.ProjectPickerPopup(sid)
}

// projectPickerPopupJS wires the project picker popup: search/filter, keyboard
// navigation, and selection. Reads from window.__libroProjects.
func projectPickerPopupJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var selectedIdx=0;
	var filtered=[];
	var hoverEnabled=false;

	function getDlg(){return document.getElementById('%s');}
	function getInp(){return document.getElementById('project-picker-input');}
	function getResults(){return document.getElementById('project-picker-results');}

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
			html+='<div class="project-picker-item flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors duration-75 '
				+(sel?(dk?'bg-blue-900/30 border-l-2 border-blue-500':'bg-blue-50 border-l-2 border-blue-500')
				:(dk?'hover:bg-zinc-800 border-l-2 border-transparent':'hover:bg-gray-50 border-l-2 border-transparent'))
				+'" data-project-idx="'+i+'">';
			html+='<i class="material-icons-round '+(dk?'text-zinc-400':'text-gray-400')+' text-lg">'+icon+'</i>';
			html+='<div class="flex-1 min-w-0">';
			html+='<div class="text-sm truncate '+(dk?'text-zinc-200':'text-gray-800')+'">'+escapeHtml(primary)+activeBadge+'</div>';
			html+='<div class="text-[11px] truncate '+(dk?'text-zinc-500':'text-gray-400')+'">'+escapeHtml(secondary)+'</div>';
			html+='</div>';
			if(item.kind!=='worktree'&&item.name!=='home'){
				html+='<button data-project-remove="'+i+'" title="Remove project" class="ml-2 opacity-0 group-hover:opacity-100 hover:opacity-100 transition-opacity p-1 rounded '+(dk?'hover:bg-red-900/40 text-zinc-500 hover:text-red-300':'hover:bg-red-50 text-gray-400 hover:text-red-600')+'"><i class="material-icons-round text-base pointer-events-none">delete_outline</i></button>';
			}
			html+='</div>';
		});
		res.innerHTML=html;
		res.querySelectorAll('[data-project-idx]').forEach(function(el){
			el.classList.add('group');
			el.addEventListener('mouseenter',function(){
				if(!hoverEnabled)return;
				var idx=parseInt(el.getAttribute('data-project-idx'),10);
				if(!Number.isNaN(idx)&&idx!==selectedIdx){
					selectedIdx=idx;
					render();
				}
			});
			el.addEventListener('mousedown',function(e){
				if(e.target&&e.target.closest&&e.target.closest('[data-project-remove]'))return;
				e.preventDefault();
				var idx=parseInt(el.getAttribute('data-project-idx'),10);
				if(Number.isNaN(idx))return;
				selectedIdx=idx;
				launch();
			});
		});
		res.querySelectorAll('[data-project-remove]').forEach(function(btn){
			btn.addEventListener('mousedown',function(e){e.preventDefault();e.stopPropagation();});
			btn.addEventListener('click',function(e){
				e.preventDefault();
				e.stopPropagation();
				var idx=parseInt(btn.getAttribute('data-project-remove'),10);
				if(Number.isNaN(idx))return;
				removeAt(idx);
			});
		});
		var selected=res.querySelector('[data-project-idx="'+selectedIdx+'"]');
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
				var hay=item.name+' '+(item.branch||'')+' '+(item.path||'');
				var score=fuzzyMatch(hay,query);
				if(score>0){
					filtered.push(Object.assign({score:score},item));
				}
			});
			filtered.sort(function(a,b){return b.score-a.score;});
		}
		selectedIdx=0;
		for(var i=0;i<filtered.length;i++){
			if(filtered[i].isActive){selectedIdx=i;break;}
		}
		render();
	}

	function closePopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(dlg)dlg.classList.add('hidden');
		if(inp)inp.value='';
		hoverEnabled=false;
	}

	function launch(){
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

	function removeAt(idx){
		if(idx<0||idx>=filtered.length)return;
		var item=filtered[idx];
		if(!item||item.kind==='worktree'||item.name==='home')return;
		if(!window.confirm('Remove project "'+item.name+'" from Libro?\n\nThis only removes it from the project list — files on disk are kept.'))return;
		closePopup();
		__ws.call('project.remove',{sid:'%s',name:item.name});
	}

	function openPopup(){
		var dlg=getDlg();
		var inp=getInp();
		if(!dlg||!inp)return;
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
				if(selectedIdx<filtered.length-1){selectedIdx++;render();}
			}else if(e.key==='ArrowUp'){
				e.preventDefault();
				if(selectedIdx>0){selectedIdx--;render();}
			}else if(e.key==='Enter'){
				e.preventDefault();
				launch();
			}else if(e.key==='Escape'){
				e.preventDefault();
				closePopup();
			}
		});
	}

	window.__libroOpenProjectPicker=openPopup;
})();
`, ProjectPickerID, sid, sid, sid)
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
				if(selectedIdx<filtered.length-1){selectedIdx++;render();}
			}else if(e.key==='ArrowUp'){
				e.preventDefault();
				if(selectedIdx>0){selectedIdx--;render();}
			}else if(e.key==='Enter'){
				e.preventDefault();
				launch();
			}else if(e.key==='Escape'){
				e.preventDefault();
				closePopup();
			}
		});
	}

	window.__libroOpenMoveProject=openPopup;
})();
`, MoveProjectPopupID, sid)
}

// projectsJS publishes the list of projects (and their worktrees) into
// window.__libroProjects for the project picker popup.
func projectsJS(state *AppState) string {
	type jsProject struct {
		Kind          string   `json:"kind"`
		Name          string   `json:"name"`
		Path          string   `json:"path"`
		Branch        string   `json:"branch,omitempty"`
		IsGit         bool     `json:"isGit"`
		IsActive      bool     `json:"isActive"`
		Branches      []string `json:"branches,omitempty"`
		CurrentBranch string   `json:"currentBranch,omitempty"`
		WorktreeRefs  []string `json:"worktreeRefs,omitempty"`
	}
	var all []jsProject
	for _, p := range state.Projects {
		if p.Virtual {
			continue
		}
		isActive := p.Name == state.ActiveProject
		entry := jsProject{
			Kind:     "project",
			Name:     p.Name,
			Path:     p.Path,
			IsGit:    p.IsGitRepo,
			IsActive: isActive,
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
				Kind:     "worktree",
				Name:     p.Name,
				Path:     wt.Path,
				Branch:   wt.Branch,
				IsGit:    true,
				IsActive: wtActive,
			})
		}
	}
	b, _ := json.Marshal(all)
	if b == nil {
		b = []byte("[]")
	}
	return fmt.Sprintf("window.__libroProjects=%s;", string(b))
}

// renderProjectDialog renders the create project modal
func renderProjectDialog(visible bool, sid string) *r.Node {
	return components.ProjectDialog(visible, sid)
}

// renderDirBrowser renders the directory browser component
func renderDirBrowser(currentPath string, sid string) *r.Node {
	return components.DirBrowser(currentPath, sid)
}

// projectDialogJS wires the New Project dialog: live filtering of the
// directory listing as the user types a path, Enter to descend into the
// highlighted entry, "Select current" to use the typed path as a project
// root, and the inline confirmation bar shown when the path doesn't exist.
func projectDialogJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	if(window.__libroProjectDialogRegistered)return;
	window.__libroProjectDialogRegistered=true;
	var sid=%s;
	var dlg=document.getElementById(%s);
	if(!dlg)return;
	var inp,hidden,confirmBar;
	var loadedDir='';
	var debounceId=0;

	function refs(){
		inp=document.getElementById('project-path-input');
		hidden=document.getElementById('project-path');
		confirmBar=document.getElementById('project-path-confirm');
	}
	function dirOf(p){
		if(!p)return '/';
		if(p==='/')return '/';
		var i=p.lastIndexOf('/');
		if(i<=0)return '/';
		return p.substring(0,i);
	}
	function nameOf(p){
		if(!p)return '';
		var i=p.lastIndexOf('/');
		return p.substring(i+1);
	}
	function items(){
		return dlg.querySelectorAll('.dir-item');
	}
	function clearHighlight(){
		items().forEach(function(el){el.classList.remove('bg-blue-100','dark:bg-blue-900/40');});
	}
	function highlightFirst(prefix){
		clearHighlight();
		var first=null;
		items().forEach(function(el){
			if(first)return;
			var n=el.getAttribute('data-name')||'';
			if(n==='..')return;
			if(!prefix||n.toLowerCase().indexOf(prefix.toLowerCase())===0){first=el;}
		});
		if(first){
			first.classList.add('bg-blue-100','dark:bg-blue-900/40');
			first.scrollIntoView({block:'nearest'});
		}
		return first;
	}
	function applyFilter(){
		if(!inp)return;
		var v=inp.value||'';
		if(hidden)hidden.value=v;
		var prefix=v.endsWith('/')?'':nameOf(v);
		items().forEach(function(el){
			var n=el.getAttribute('data-name')||'';
			var match=n==='..'||!prefix||n.toLowerCase().indexOf(prefix.toLowerCase())===0;
			el.style.display=match?'':'none';
		});
		highlightFirst(prefix);
		if(confirmBar)confirmBar.classList.add('hidden');
	}
	function ensureLoaded(){
		if(!inp)return;
		var v=inp.value||'';
		var dir=v.endsWith('/')?(v.length>1?v.slice(0,-1):'/'):dirOf(v);
		if(dir===loadedDir)return;
		loadedDir=dir;
		__ws.call('project.browse',{sid:sid,path:dir});
	}
	function descendHighlighted(){
		var sel=dlg.querySelector('.dir-item.bg-blue-100, .dir-item.bg-blue-900\\/40');
		if(!sel)return false;
		var p=sel.getAttribute('data-path');
		if(!p)return false;
		inp.value=p+'/';
		if(hidden)hidden.value=inp.value;
		loadedDir=p;
		__ws.call('project.browse',{sid:sid,path:p});
		return true;
	}

	function onInput(){
		clearTimeout(debounceId);
		debounceId=setTimeout(function(){ensureLoaded();applyFilter();},80);
	}
	function onKey(e){
		if(e.key==='Tab'){
			e.preventDefault();
			var sel=dlg.querySelector('.dir-item.bg-blue-100, .dir-item.bg-blue-900\\/40');
			if(!sel)return;
			var p=sel.getAttribute('data-path');
			if(!p)return;
			inp.value=p;
			if(hidden)hidden.value=inp.value;
			applyFilter();
			return;
		}
		if(e.key==='Enter'){
			e.preventDefault();
			if(!descendHighlighted()){
				__ws.call('project.create',{sid:sid,'project-path':inp.value});
			}
			return;
		}
		if(e.key==='Escape'){
			e.preventDefault();
			__ws.call('project.dialog.close',{sid:sid});
		}
	}

	function bindButtons(){
		var sel=document.getElementById('btn-select-current');
		if(sel&&!sel.__libroBound){
			sel.__libroBound=true;
			sel.addEventListener('click',function(){
				if(!inp)return;
				__ws.call('project.create',{sid:sid,'project-path':inp.value});
			});
		}
		var crt=document.getElementById('btn-create-project');
		if(crt&&!crt.__libroBound){
			crt.__libroBound=true;
			crt.addEventListener('click',function(){
				if(!inp)return;
				__ws.call('project.create',{sid:sid,'project-path':inp.value});
			});
		}
	}

	function bind(){
		refs();
		if(!inp)return;
		if(!inp.__libroBound){
			inp.__libroBound=true;
			inp.addEventListener('input',onInput);
			inp.addEventListener('keydown',onKey);
		}
		bindButtons();
		applyFilter();
		// Inject confirm-bar buttons (Create / Cancel) once.
		if(confirmBar&&!confirmBar.__libroBound){
			confirmBar.__libroBound=true;
			var btns=document.createElement('div');
			btns.className='mt-2 flex items-center gap-2';
			var ok=document.createElement('button');
			ok.className='px-3 py-1 bg-amber-600 hover:bg-amber-500 text-white font-mono text-xs font-medium rounded cursor-pointer';
			ok.textContent='Create folder';
			ok.addEventListener('click',function(){
				var p=confirmBar.dataset.path||(inp&&inp.value)||'';
				__ws.call('project.create.confirm',{sid:sid,path:p});
			});
			var no=document.createElement('button');
			no.className='px-3 py-1 text-amber-700 dark:text-amber-200 font-mono text-xs rounded hover:bg-amber-100 dark:hover:bg-amber-900/40 cursor-pointer';
			no.textContent='Cancel';
			no.addEventListener('click',function(){confirmBar.classList.add('hidden');});
			btns.appendChild(ok);
			btns.appendChild(no);
			confirmBar.appendChild(btns);
		}
	}

	// Re-bind on DOM updates (the dir listing is replaced on every browse).
	var observer=new MutationObserver(function(){bind();});
	observer.observe(dlg,{childList:true,subtree:true});

	function open(){
		if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
		dlg.classList.remove('hidden');
		bind();
		if(inp){
			loadedDir=dirOf(inp.value);
			setTimeout(function(){inp.focus();inp.select();},50);
		}
	}
	window.__libroOpenProjectDialog=open;
	bind();
})();
`, components.JSString(sid), components.JSString(ProjectDialogID))
}

// updateHashJS returns JS that updates the URL hash to the given project name
func updateHashJS(name string) string {
	return fmt.Sprintf("history.replaceState(null,'','#%s');document.title=%s;", name, components.JSString(name+" — Libro"))
}

// savedAppsJS returns JS that sets the global __libroSavedApps and __libroBrowsedURLs variables from DB data.
// Only apps visible in the given project are included (global + project-specific for this project).
func savedAppsJS(state *AppState) string {
	apps := DBLoadVisibleSavedApps(savedAppsProjectName(state, state.ActiveProject))
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

func terminalFrameSetupJS() string {
	return `
		(function() {
			if (window.__libroTerminalFramesRegistered) return;
			window.__libroTerminalFramesRegistered = true;

			var observed = new WeakSet();
			var resizeObserver = null;
			var intersectionObserver = null;
			if (window.ResizeObserver) {
				resizeObserver = new ResizeObserver(function(entries) {
					entries.forEach(function(entry) {
						var frame = entry.target.querySelector && entry.target.querySelector('iframe[data-terminal-iframe]');
						if (frame) window.__libroFitTerminalFrame(frame);
					});
				});
			}
			if (window.IntersectionObserver) {
				intersectionObserver = new IntersectionObserver(function(entries) {
					entries.forEach(function(entry) {
						if (entry.isIntersecting && entry.target) window.__libroFitTerminalFrame(entry.target);
					});
				}, { threshold: 0.01 });
			}

			window.__libroFitTerminalFrame = function(frameOrAppID) {
				var frame = frameOrAppID;
				if (typeof frameOrAppID === 'string') {
					frame = document.querySelector('iframe[data-terminal-iframe="' + frameOrAppID.replace(/"/g, '\\"') + '"]');
				}
				if (!frame || !frame.contentWindow) return;

				function nudge() {
					try {
						frame.style.width = '100%';
						frame.style.height = '100%';
						frame.style.minWidth = '0';
						frame.style.minHeight = '0';
						frame.style.willChange = 'transform';
						frame.style.transform = 'translateZ(0)';
						frame.getBoundingClientRect();
						var ResizeEvent = frame.contentWindow.Event || Event;
						window.dispatchEvent(new Event('resize'));
						frame.contentWindow.dispatchEvent(new ResizeEvent('resize'));
						var doc = frame.contentDocument || frame.contentWindow.document;
						if (doc) {
							if (doc.documentElement) {
								doc.documentElement.style.width = '100%';
								doc.documentElement.style.height = '100%';
								doc.documentElement.style.overflow = 'hidden';
							}
							if (doc.body) {
								doc.body.style.width = '100%';
								doc.body.style.height = '100%';
								doc.body.style.margin = '0';
								doc.body.style.overflow = 'hidden';
							}
							doc.dispatchEvent(new ResizeEvent('resize'));
							if (doc.defaultView) doc.defaultView.dispatchEvent(new ResizeEvent('resize'));
						}
					} catch (err) {}
				}

				requestAnimationFrame(function() {
					nudge();
					requestAnimationFrame(nudge);
				});
				setTimeout(nudge, 80);
				setTimeout(nudge, 180);
				setTimeout(nudge, 420);
				setTimeout(nudge, 900);
			};

			window.__libroSettleAppFrame = function(appID) {
				var app = document.querySelector('[data-app-id="' + String(appID).replace(/"/g, '\\"') + '"]');
				if (!app) return;
				var termFrame = app.querySelector('iframe[data-terminal-iframe]');
				function settle() {
					if (window.__libroScrollToApp) window.__libroScrollToApp(app);
					app.style.transform = 'translateZ(0)';
					app.getBoundingClientRect();
					if (termFrame && window.__libroFitTerminalFrame) window.__libroFitTerminalFrame(termFrame);
					if ((window.__libroSelectedApp || '') === appID && window.__libroFocusAppByID) {
						window.__libroFocusAppByID(appID);
					}
				}
				requestAnimationFrame(function() {
					settle();
					requestAnimationFrame(settle);
				});
				[80, 180, 350, 700, 1200, 2200, 4200].forEach(function(delay) {
					setTimeout(settle, delay);
				});
			};

			function watch(frame) {
				if (!frame || observed.has(frame)) return;
				observed.add(frame);
				frame.addEventListener('load', function() {
					window.__libroFitTerminalFrame(frame);
					var app = frame.closest('[data-app-id]');
					if (app && window.__libroSettleAppFrame) {
						window.__libroSettleAppFrame(app.getAttribute('data-app-id') || '');
					}
				});
				if (intersectionObserver) intersectionObserver.observe(frame);
				var app = frame.closest('[data-app-id]');
				if (app && resizeObserver) resizeObserver.observe(app);
				window.__libroFitTerminalFrame(frame);
			}

			function scan(root) {
				var scope = root && root.querySelectorAll ? root : document;
				if (scope.matches && scope.matches('iframe[data-terminal-iframe]')) watch(scope);
				scope.querySelectorAll('iframe[data-terminal-iframe]').forEach(watch);
			}

			scan(document);
			new MutationObserver(function(mutations) {
				mutations.forEach(function(mutation) {
					mutation.addedNodes.forEach(scan);
				});
			}).observe(document.body, { childList: true, subtree: true });
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

				function focusAttempt() {
					if ((window.__libroSelectedApp || '') !== (container.getAttribute('data-app-id') || '')) return;
					try { window.focus(); } catch(err) {}

					// Blur all other iframes and webviews first
					var allIframes = document.querySelectorAll('iframe');
					for (var i = 0; i < allIframes.length; i++) {
						try { allIframes[i].contentWindow.blur(); } catch(err) {}
						allIframes[i].blur();
					}
					var allWebviews = document.querySelectorAll('webview');
					for (var j = 0; j < allWebviews.length; j++) {
						allWebviews[j].blur();
					}

					// Try to focus a webview first, then fall back to iframe
					var webview = container.querySelector('webview');
					if (webview) {
						try { webview.focus(); } catch(err) {}
						return;
					}

					var iframe = container.querySelector('iframe');
					if (!iframe) return;
					if (iframe.getAttribute('data-terminal-iframe') && window.__libroFitTerminalFrame) {
						window.__libroFitTerminalFrame(iframe);
					}
					iframe.focus();
					try {
						iframe.contentWindow.focus();
						var doc = iframe.contentDocument || iframe.contentWindow.document;
						var termEl = doc.querySelector('.xterm-helper-textarea') || doc.querySelector('textarea') || doc.body;
						if (termEl) {
							termEl.focus();
						}
					} catch(err) {}
				}

				focusAttempt();
				setTimeout(focusAttempt, 40);
				setTimeout(focusAttempt, 120);
				setTimeout(focusAttempt, 260);
			};

			window.__libroFocusAppByID = function(appID) {
				if (!appID) return;
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
				if (e.metaKey && !e.ctrlKey && (e.key === ',' || e.code === 'Comma')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.resize.step', {"sid": "%s", "delta": -1});
					return;
				}
				if (e.metaKey && !e.ctrlKey && (e.key === '.' || e.code === 'Period')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.resize.step', {"sid": "%s", "delta": 1});
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
				if (e.metaKey && e.key === 'Enter' && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenTerminalApp) window.__libroOpenTerminalApp();
					return;
				}
				if (e.metaKey && (e.key === 'o' || e.key === 'O' || e.code === 'KeyO')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenSearch) window.__libroOpenSearch('right');
					return;
				}
				if (e.metaKey && (e.key === 't' || e.key === 'T' || e.code === 'KeyT') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenBrowserApp) window.__libroOpenBrowserApp();
					return;
				}
				if (e.metaKey && e.ctrlKey && e.code === 'KeyN') {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenProjectDialog) window.__libroOpenProjectDialog();
					return;
				}
				if (e.metaKey && (e.key === 'n' || e.key === 'N' || e.code === 'KeyN') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenProjectPicker) window.__libroOpenProjectPicker();
					return;
				}
				if (e.metaKey && e.ctrlKey && (e.key === 'y' || e.key === 'Y' || e.code === 'KeyY')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenMoveProject) window.__libroOpenMoveProject();
					return;
				}
				if (e.metaKey && !e.ctrlKey && (e.key === ';' || e.code === 'Semicolon')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenCommandPalette) window.__libroOpenCommandPalette();
					return;
				}
				if (e.metaKey && (e.key === 'g' || e.key === 'G' || e.code === 'KeyG') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenWorktreeCreate) window.__libroOpenWorktreeCreate();
					return;
				}
				if (e.metaKey && (e.key === 'x' || e.key === 'X' || e.code === 'KeyX') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('nav.slot.toggle.active', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'q' || e.key === 'Q') && !e.ctrlKey) {
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
					var appId = window.__libroSelectedApp || '';
					var appEl = appId ? document.querySelector('[data-app-id="' + appId + '"]') : null;
					if (!appEl) return;
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.maximize.toggle', {"sid": "%s"});
					return;
				}
				if (e.metaKey && (e.key === 'r' || e.key === 'R') && !e.ctrlKey) {
					e.preventDefault();
					e.stopImmediatePropagation();
					if (window.__libroOpenResizePopup) window.__libroOpenResizePopup();
					return;
				}
			}

			window.__libroOpenTerminalApp = function() {
				__ws.call('app.start',{sid:'%s',type:'terminal',url:'',command:'',width:'lg',writable:true,name:'',iconUrl:'',side:'right'});
			};

			window.__libroOpenBrowserApp = function() {
				__ws.call('app.browse.open',{sid:'%s',side:'right',popup:true});
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
		`, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid, sid)
}
