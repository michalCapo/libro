package components

import (
	"log"
	"os"

	r "github.com/michalCapo/g-sui/ui"
)

// ProjectDialog renders the unified Projects dialog: existing project search and
// directory lookup for opening folders as projects.
func ProjectDialog(sid string) *r.Node {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("components: ProjectDialog os.UserHomeDir failed: %v", err)
	}
	if homeDir == "" {
		homeDir = "/"
	}

	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ProjectDialogID).
		Attr("data-project-home", homeDir).
		OnClick(r.JS(HideJS(ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-2xl mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("folder_open"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Projects"),
					),
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("project-input").
							Attr("type", "text").
							Attr("placeholder", "Type project, branch, folder, or path...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("project-results"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("↑↓ navigate"),
						r.Span("").Text("Tab complete / enter folder"),
						r.Span("").Text("Enter open"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}
