package libro

import (
	r "github.com/michalCapo/g-sui/ui"
)

// renderPage renders the full page layout
func renderPage(state *AppState, sid string) *r.Node {
	page := r.Div("h-screen w-screen flex flex-col bg-gray-100 dark:bg-zinc-900 overflow-hidden").Render(
		renderTopBar(state, sid),
		renderMainAreaWrapper(state, sid),
		renderAddDialog(state.DialogOpen, sid),
		renderProjectDialog(state.ProjectDialogOpen, sid),
		renderSearchDialog(sid),
		renderShortcutsDialog(),
		renderCloseDialog(sid),
		renderWorktreeDialog(sid),
		renderURLPopup(sid),
		renderResizePopup(sid),
	)
	page.JS(flashCSS() + termIconSetupJS() + projectToastSetupJS() + keyboardShortcutsJS(sid) + savedAppsJS(state.ActiveProject) + initHashJS(sid) + searchDialogJS(sid) + shortcutsDialogJS() + closeDialogJS(sid) + webviewClientJS() + worktreeDialogJS(sid) + worktreesJS(state) + urlPopupJS(sid) + resizePopupJS(sid))
	return page
}
