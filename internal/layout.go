package libro

import (
	"libro/internal/components"

	r "github.com/michalCapo/g-sui/ui"
)

// renderPage renders the full page layout
func renderPage(state *AppState, sid string) *r.Node {
	page := r.Div("h-screen w-screen flex flex-col overflow-hidden").ID("libro-workspace").Render(
		renderTopBar(state, sid),
		renderMainAreaWrapper(state, sid),
		r.Div("ws-statusbar").Render(r.Span("").Text("Local workspace"), r.Span("").Text("Agents, tools, and terminals • Libro")),

		renderProjectDialog(state.ProjectDialogOpen, sid),
		renderCloseDialog(sid),
		renderURLPopup(sid),
		renderResizePopup(sid),
		renderCommandPopup(),
		renderMoveProjectPopup(),
		renderWorktreeCreatePopup(),
		r.Div("hidden").ID(ActionEffectsID).Attr("aria-hidden", "true"),
	)
	page.JS(popupRegistryJS() +
		uxHardenJS() +
		flashCSS() +
		termIconSetupJS() +
		terminalFrameSetupJS() +
		toastSetupJS() +
		appWidthPolicyJS(sid) +
		keyboardShortcutsJS(sid) +
		initHashJS(sid) +
		closeDialogJS(sid) +
		components.BrowserJS() +
		projectDialogJS(sid) +
		projectsJS(state) +
		urlPopupJS(sid) +
		resizePopupJS(sid) +
		commandPopupJS(sid) +
		moveProjectPopupJS(sid) +
		worktreeCreatePopupJS(sid) +
		workspaceJS(sid),
	)
	return page
}
