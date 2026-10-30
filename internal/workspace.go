package libro

import (
	"embed"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"path/filepath"
	"strings"
)

//go:embed workspace.css workspace.js voice.js files.js file-editor.bundle.js repos.js page_attachments.js tailwind.min.css material-icons-round.woff2
var workspaceAssets embed.FS

func workspaceJS() string {
	var code strings.Builder
	code.WriteString(`window.__libroToolKeys=props.keys;window.__libroDefaultToolKeys=props.defaults;window.__libroPageToolsAutoExecute=props.autoexecute;window.__libroWorkspaceSID=props.sid;window.__libroPlugins=props.plugins;window.__libroAgentAutoUpdate=props.agentUpdates;`)
	code.WriteString(`var wsStyle=document.createElement('style');wsStyle.textContent=props.css;document.head.appendChild(wsStyle);`)
	code.WriteString(components.VoiceInputJS())
	for _, name := range []string{"file-editor.bundle.js", "files.js", "repos.js", "voice.js", "page_attachments.js", "workspace.js"} {
		data, _ := workspaceAssets.ReadFile(name)
		code.Write(data)
	}
	return code.String()
}

// registerStyles serves the compiled Tailwind CSS and the icon font with the
// app, so pages need no CDN. Rebuild them with npm run build:css.
func registerStyles(app *r.App) {
	css, _ := workspaceAssets.ReadFile("tailwind.min.css")
	app.TailwindCSS(css)
	font, _ := workspaceAssets.ReadFile("material-icons-round.woff2")
	app.IconFont(font)
}

func registerWidgets(app *r.App) {
	app.Widget("workspace", "function(el, props){"+workspaceJS()+"}")
	app.Widget("terminal", "function(el){"+terminalFrameSetupJS()+`window.__libroMountTerminal(el);return function(){window.__libroDisposeTerminal(el);};}`)
	app.Widget("files", `function(el){queueMicrotask(function(){if(el.isConnected)window.libroFiles.init();});return function(){window.libroFiles.dispose(el);};}`)
	app.Widget("repos", `function(el){queueMicrotask(function(){if(el.isConnected)window.libroRepos.init();});return function(){window.libroRepos.dispose(el);};}`)
}

func workspaceAppName(app Application) string {
	if app.Name != "" {
		return app.Name
	}
	if app.Type == AppTypeURL {
		if app.URL != "" {
			return app.URL
		}
		return "Browser"
	}
	if app.Command != "" {
		return app.Command
	}
	return "Terminal"
}

func workspaceButton(label, icon string, action *r.Action) *r.Node {
	return r.Button("ws-button").Attr("type", "button").Attr("title", label).Attr("aria-label", label).
		OnClick(action).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text(icon))
}

// Keep the update target for existing server actions without rendering a header.
func renderWorkspaceTopBar(_ *AppState, _ string) *r.Node {
	return r.Div("hidden").ID(TopBarID).Attr("aria-hidden", "true")
}

func renderWorkspaceSidebar(sid string) *r.Node {
	return r.El("aside", "ws-sidebar").ID("workspace-projects").Attr("aria-label", "Projects").Render(
		r.Div("ws-navigation").Render(
			r.Div("ws-sidebar-heading").Render(
				r.Span("").Text("Threads"),
				workspaceButton("Fold sidebar", "keyboard_double_arrow_left", r.UnsafeJS("libroWorkspace.foldSidebar()")).Attr("data-sidebar-fold", ""),
			),
			r.Button("ws-new-thread").Attr("type", "button").Attr("aria-label", "New thread").OnClick(r.UnsafeJS("libroWorkspace.newThread()")).Render(
				r.I("material-icons-round").Attr("aria-hidden", "true").Text("add"),
				r.Span("").Text("New thread"),
				r.El("kbd", "ws-new-thread-key").Attr("aria-hidden", "true"),
			),
			r.Div("ws-project-list").ID("workspace-project-list"),
			r.Div("ws-project-list").ID("workspace-thread-list"),
			// The tools and footer stay pinned below the scrolling lists. A folded sidebar moves the tools
			// out to their own rail; see foldState in workspace.js.
			r.Div("ws-sidebar-bottom").Render(
				renderWorkspaceTools(),
				r.Div("ws-sidebar-footer").Render(
					r.Button("ws-sidebar-search").ID("workspace-all-projects").Attr("type", "button").Attr("aria-expanded", "false").OnClick(r.UnsafeJS("libroWorkspace.toggleProjects()")).Render(
						r.I("material-icons-round").Attr("aria-hidden", "true").Text("folder_open"),
						r.Span("").Text("All projects"),
						r.El("small", "").ID("workspace-project-count"),
					),
					workspaceButton("Add project", "add", r.UnsafeJS("if(window.__libroOpenProjectDialogBrowse){__libroOpenProjectDialogBrowse();}else{__ws.call('project.dialog.open',{sid:el.dataset.sid});}")).Attr("data-sid", sid),
				),
			),
		),
	)
}

func renderWorkspaceEmpty(state *AppState, sid string) *r.Node {
	return renderWorkspaceStrip(state, sid, "")
}

func workspaceProjectLabel(state *AppState) string {
	if thread := state.thread(state.ActiveProject); thread != nil {
		return thread.Name
	}
	if state.ActiveProject == "" {
		return ""
	}
	// A worktree thread shows the name of its project, not of its folder.
	name := state.parentProjectScope(state.ActiveProject)
	projectLabel := filepath.Base(name)
	for _, project := range state.Projects {
		if project.Name == name && (project.Path != "" || project.Label != "") {
			projectLabel = project.displayName()
			break
		}
	}
	return projectLabel
}

func renderWorkspaceStrip(state *AppState, sid, placeholderID string) *r.Node {
	projectLabel := workspaceProjectLabel(state)
	projectScope := state.projectScope(state.ActiveProject)
	thread := state.thread(state.ActiveProject)
	children := []*r.Node{}
	for i, app := range state.Apps {
		children = append(children, renderAppFrameBase(app, i, i == state.SelectedIndex, sid, app.ID == placeholderID))
	}
	node := r.Div("ws-project").ID(projectMainID(state.ActiveProject)).Render(
		r.Div("ws-grid").ID(stripID(state.ActiveProject)).Attr("data-workspace-project", state.ActiveProject).Attr("data-project-scope", projectScope).Attr("data-thread", fmt.Sprint(thread != nil)).Attr("data-managed", fmt.Sprint(thread != nil && thread.Managed)).Attr("data-project-label", projectLabel).Render(children...),
	)
	return node.Render(centerSelectedNode(state))
}

func renderWorkspaceTools() *r.Node {
	return r.El("aside", "ws-tool-rail").Attr("aria-label", "Workspace tools").Render(
		// Only the folded rail shows these: new thread, then one number per thread card.
		r.Div("ws-rail-threads").Render(
			workspaceButton("New thread", "add", r.UnsafeJS("libroWorkspace.newThread()")),
			r.Div("ws-rail-numbers").ID("workspace-rail-threads"),
		),
		r.Div("ws-tool-sizes").ID("workspace-tool-size").Attr("data-size-badges", ""),
		r.Div("ws-voice-control").Attr("data-voice-control", "").Render(
			workspaceButton("Voice typing", "mic", r.UnsafeJS("void window.libroVoice?.toggle()")).Attr("data-voice-button", "global").Attr("aria-pressed", "false"),
			r.Span("ws-voice-status").Attr("role", "status").Attr("popover", "manual").Attr("hidden", "hidden"),
		),
		r.Div("ws-tool-spacer"),
		workspaceButton("Toggle bottom terminal (Ctrl+`)", "vertical_align_bottom", r.UnsafeJS("libroWorkspace.bottom()")),
		workspaceButton("Commands", "tune", r.UnsafeJS("if(window.__libroOpenCommandPalette)__libroOpenCommandPalette()")),
		workspaceButton("Apps & plugins", "apps", r.UnsafeJS("libroWorkspace.launcher()")),
		workspaceButton("Settings", "settings", r.UnsafeJS("libroWorkspace.settings()")),
	)
}
