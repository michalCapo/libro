package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// WorktreeCreatePopup renders the popup used to create a new git worktree
// (and matching branch) from the current project's branch.
func WorktreeCreatePopup() *r.Node {
	return r.Div("ws-popup fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(WorktreeCreatePopupID).
		OnClick(r.JS(HideJS(WorktreeCreatePopupID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 rounded-xl shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				Attr("role", "dialog").Attr("aria-modal", "true").Attr("aria-label", "New worktree").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("ws-command-search").Render(
						r.I("material-icons-round").Attr("aria-hidden", "true").Text("search"),
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("worktree-create-input").
							Attr("type", "text").
							Attr("placeholder", "new-branch-name").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
						r.Span("text-[11px] font-mono text-gray-500 dark:text-zinc-500").ID("worktree-create-context").Text(""),
						r.Button("ws-command-dismiss").Attr("type", "button").Attr("aria-label", "Close worktree picker").OnClick(r.JS(HideJS(WorktreeCreatePopupID))).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("close")),
					),
					r.Div("max-h-80 overflow-y-auto").
						ID("worktree-create-branches"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("↑↓ navigate"),
						r.Span("").Text("Enter create"),
						r.Span("").Text("Esc cancel"),
					),
				),
		)
}
