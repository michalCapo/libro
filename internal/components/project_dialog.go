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

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75" + hiddenClass).
		ID(ProjectDialogID).
		OnClick(r.JS(HideJS(ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-2xl mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("New Project"),

					r.Div("mb-4").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Select Folder"),
						DirBrowser(homeDir, sid),
					),

					r.IHidden("").ID("project-path").Attr("value", homeDir),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "project.dialog.close", Data: map[string]any{"sid": sid}}),
						r.Button("px-5 py-2 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-create-project").
							Text("Create").
							OnClick(&r.Action{
								Name:    "project.create",
								Data:    map[string]any{"sid": sid},
								Collect: []string{"project-path"},
							}),
					),
				),
		)
}
