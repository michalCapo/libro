package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// ProjectPickerPopup renders the global project search/picker.
func ProjectPickerPopup(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ProjectPickerID).
		OnClick(r.JS(HideJS(ProjectPickerID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("source"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Projects"),
						r.Button("text-[11px] font-mono text-blue-600 dark:text-blue-400 hover:underline cursor-pointer").
							Attr("title", "Create new project").
							OnClick(&r.Action{Name: "project.dialog.open", Data: map[string]any{"sid": sid}}).
							Render(
								r.I("material-icons-round text-sm align-middle mr-0.5").Text("add"),
								r.Span("align-middle").Text("New"),
							),
					),
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("project-picker-input").
							Attr("type", "text").
							Attr("placeholder", "Type project or branch...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("project-picker-results"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("↑↓ navigate"),
						r.Span("").Text("Enter select"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}
