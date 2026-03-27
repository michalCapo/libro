package main

import (
	r "github.com/michalCapo/g-sui/ui"
)

// renderPage renders the full page layout
func renderPage(state *AppState, sid string) *r.Node {
	page := r.Div("h-screen w-screen flex flex-col bg-gray-100 dark:bg-zinc-900 overflow-hidden").Render(
		// Theme switcher in top-right corner
		r.Div("absolute top-3 right-3 z-40").Render(
			r.ThemeSwitcher(),
		),
		renderProjectBar(state, sid),
		renderMainArea(state, sid),
		renderAddDialog(state.DialogOpen, sid),
		renderProjectDialog(state.ProjectDialogOpen, sid),
	)
	page.JS(keyboardShortcutsJS(sid) + loadProjectsJS(sid))
	return page
}
