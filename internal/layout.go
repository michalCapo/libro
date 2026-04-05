package libro

import (
	r "github.com/michalCapo/g-sui/ui"
)

// introToastJS returns JS that shows the app intro toast after a short delay.
func introToastJS() string {
	return showToastJS(
		"Libro",
		"A Go + Electron application manager that displays web apps and terminal sessions in a horizontal strip layout. Use ⌘+/ for quick launch, ⌘+B for projects, and ⌘+N to add apps.",
		5000,
	)
}

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
	)
	// Show intro toast on first page load (when no apps running)
	introJS := ""
	if len(state.Apps) == 0 {
		introJS = "setTimeout(function(){" + introToastJS() + "},800);"
	}
	page.JS(flashCSS() + termIconSetupJS() + projectToastSetupJS() + keyboardShortcutsJS(sid) + savedAppsJS(state.ActiveProject) + initHashJS(sid) + searchDialogJS(sid) + shortcutsDialogJS() + closeDialogJS(sid) + webviewClientJS() + worktreeDialogJS(sid) + worktreesJS(state) + introJS)
	return page
}
