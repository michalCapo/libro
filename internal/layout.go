package libro

import (
	r "github.com/michalCapo/g-sui/ui"
)

// renderPage renders the full page layout
func renderPage(state *AppState, sid string) *r.Node {
	page := r.Div("h-screen w-screen flex flex-col bg-gray-100 dark:bg-zinc-900 overflow-hidden").Render(
		renderProjectBar(state, sid),
		renderMainAreaWrapper(state, sid),
		renderAddDialog(state.DialogOpen, sid),
		renderProjectDialog(state.ProjectDialogOpen, sid),
		renderSearchDialog(sid),
		renderShortcutsDialog(),
	)
	page.JS(termIconSetupJS() + keyboardShortcutsJS(sid) + loadProjectsJS(sid) + searchDialogJS(sid) + shortcutsDialogJS() + chromeClientJS())
	return page
}
