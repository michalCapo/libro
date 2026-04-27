package components

import (
	"log"
	"os"

	r "github.com/michalCapo/g-sui/ui"
)

// ProjectDialog renders the "New Project" modal containing a directory browser.
func ProjectDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("components: ProjectDialog os.UserHomeDir failed: %v", err)
	}
	if homeDir == "" {
		homeDir = "/"
	}

	return r.Div("fixed inset-0 z-50 flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75"+hiddenClass).
		ID(ProjectDialogID).
		OnClick(r.JS(HideJS(ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-2xl mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("create_new_folder"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("New Project"),
					),

					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("project-path-input").
							Attr("type", "text").
							Attr("value", homeDir).
							Attr("placeholder", "/path/to/project").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false"),
					),

					r.IHidden("").ID("project-path").Attr("value", homeDir),

					r.Div("max-h-[60vh] overflow-y-auto").Render(
						DirBrowser(homeDir, sid),
					),

					r.Div("hidden px-4 py-3 border-t border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 text-xs text-amber-800 dark:text-amber-200 font-mono").
						ID("project-path-confirm").
						Render(
							r.Span("").ID("project-path-confirm-msg").Text(""),
						),

					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Div("flex items-center gap-4").Render(
							r.Span("").Text("↑↓ navigate"),
							r.Span("").Text("Enter descend"),
							r.Span("").Text("Esc close"),
						),
						r.Div("flex items-center gap-2").Render(
							r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
								Text("Cancel").
								OnClick(&r.Action{Name: "project.dialog.close", Data: map[string]any{"sid": sid}}),
							r.Button("px-3 py-1 text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 font-mono text-xs rounded hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors cursor-pointer").
								ID("btn-select-current").
								Text("Select current"),
							r.Button("px-4 py-1 bg-blue-600 hover:bg-blue-500 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
								ID("btn-create-project").
								Text("Create"),
						),
					),
				),
		)
}
