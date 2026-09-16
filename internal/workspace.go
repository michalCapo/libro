package libro

import (
	"embed"
	"encoding/json"
	"fmt"
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
	"path/filepath"
)

//go:embed workspace.css workspace.js files.js
var workspaceAssets embed.FS

func workspaceJS(sid string) string {
	css, _ := workspaceAssets.ReadFile("workspace.css")
	js, _ := workspaceAssets.ReadFile("workspace.js")
	filesJS, _ := workspaceAssets.ReadFile("files.js")
	list, _ := json.Marshal(plugins())
	keys, _ := json.Marshal(toolKeybindings())
	defaults, _ := json.Marshal(defaultToolKeybindings())
	return "window.__libroToolKeys=" + string(keys) + ";window.__libroDefaultToolKeys=" + string(defaults) + ";" + fmt.Sprintf("window.__libroWorkspaceSID=%s;window.__libroPlugins=%s;var wsStyle=document.createElement('style');wsStyle.textContent=%s;document.head.appendChild(wsStyle);", components.JSString(sid), list, components.JSString(string(css))) + string(filesJS) + string(js)
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

func workspaceButton(label, icon, js string) *r.Node {
	return r.Button("ws-button").Attr("title", label).Attr("aria-label", label).
		OnClick(r.JS(js)).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text(icon))
}

// Keep the update target for existing server actions without rendering a header.
func renderWorkspaceTopBar(state *AppState, sid string) *r.Node {
	return r.Div("hidden").ID(TopBarID).Attr("aria-hidden", "true")
}

func renderWorkspaceSidebar(sid string) *r.Node {
	return r.El("aside", "ws-sidebar").ID("workspace-projects").Attr("aria-label", "Projects").Render(
		r.Div("ws-sidebar-tools").Render(
			r.Button("ws-sidebar-search").Attr("aria-label", "Search commands").OnClick(r.JS("if(window.__libroOpenCommandPalette)__libroOpenCommandPalette()")).Render(
				r.I("material-icons-round").Attr("aria-hidden", "true").Text("search"),
				r.Span("").Text("Search"),
			),
			workspaceButton("Switch project or worktree", "folder", fmt.Sprintf("if(window.__libroOpenProjectDialogSearch){__libroOpenProjectDialogSearch()}else{__ws.call('project.dialog.open',{sid:%s})}", components.JSString(sid))),
			workspaceButton("Add project", "create_new_folder", fmt.Sprintf("if(window.__libroOpenProjectDialogBrowse){__libroOpenProjectDialogBrowse()}else{__ws.call('project.dialog.open',{sid:%s})}", components.JSString(sid))),
			workspaceButton("New agent session", "edit", "libroWorkspace.launcher('center')"),
		),
		r.Div("ws-sidebar-heading").Render(r.Span("").Text("Projects")),
		r.Div("ws-project-list").ID("workspace-project-list"),
		r.Div("ws-sidebar-footer").Render(
			r.Button("ws-sidebar-action").OnClick(r.JS("libroWorkspace.settings()")).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("settings"), r.Span("").Text("Settings")),
			r.Button("ws-sidebar-action").OnClick(r.JS("libroWorkspace.launcher()")).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("apps"), r.Span("").Text("Apps & plugins")),
			r.Button("ws-sidebar-action").OnClick(r.JS("if(window.__libroOpenCommandPalette)__libroOpenCommandPalette()")).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("tune"), r.Span("").Text("Commands")),
		),
	)
}

func renderWorkspaceEmpty(state *AppState, sid string) *r.Node {
	return renderWorkspaceStrip(state, sid, "")
}

func workspaceProjectLabel(state *AppState) string {
	projectLabel := filepath.Base(state.ActiveProject)
	for _, project := range state.Projects {
		if project.Name == state.ActiveProject && project.Path != "" {
			projectLabel = filepath.Base(project.Path)
			break
		}
	}
	return projectLabel
}

func renderWorkspaceStrip(state *AppState, sid, placeholderID string) *r.Node {
	projectLabel := workspaceProjectLabel(state)
	children := []*r.Node{}
	for i, app := range state.Apps {
		children = append(children, renderAppFrameBase(app, i, i == state.SelectedIndex, sid, app.ID == placeholderID))
	}
	return r.Div("ws-project").ID(projectMainID(state.ActiveProject)).Render(
		r.Div("ws-grid").ID(stripID(state.ActiveProject)).Attr("data-workspace-project", state.ActiveProject).Attr("data-project-label", projectLabel).Render(children...),
	).JS(centerSelectedJS(state))
}

func renderWorkspaceTools() *r.Node {
	return r.El("aside", "ws-tool-rail").Attr("aria-label", "Workspace tools").Render(
		workspaceButton("Toggle projects", "view_sidebar", "libroWorkspace.toggle('projects')"),
		workspaceButton("Terminal", "terminal", "libroWorkspace.tool('terminal')"),
		workspaceButton("Browser", "language", "libroWorkspace.tool('browser')"),
		workspaceButton("Files", "folder_open", "libroWorkspace.tool('files')"),
		workspaceButton("Nvim", "edit", "libroWorkspace.tool('nvim')"),
		workspaceButton("Git", "account_tree", "libroWorkspace.tool('lazyrepo')"),
		workspaceButton("Database", "storage", "libroWorkspace.tool('lazydata')"),
		workspaceButton("More tools", "add", "libroWorkspace.launcher('right')"),
		workspaceButton("Toggle bottom terminal (Ctrl+`)", "vertical_align_bottom", "libroWorkspace.bottom()"),
	)
}
