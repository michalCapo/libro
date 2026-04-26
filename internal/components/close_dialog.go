package components

import (
	"fmt"

	r "github.com/michalCapo/g-sui/ui"
)

// CloseDialog renders the close confirmation dialog (hidden by default).
// It is populated dynamically via JS when the user attempts to close the window.
func CloseDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[70] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(CloseDialogID).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-2").Render(
						r.I("material-icons-round text-base text-amber-500").Text("warning"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200").Text("Close Libro?"),
					),
					r.Div("px-4 py-3").Render(
						r.P("text-sm text-gray-600 dark:text-zinc-400 mb-3").Text("The following applications are still running:"),
						r.Div("max-h-60 overflow-y-auto").ID("close-dialog-apps"),
					),
					r.Div("px-4 py-3 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-end gap-2").Render(
						r.Button("px-3 py-1.5 text-sm rounded-md border border-gray-300 dark:border-zinc-600 text-gray-700 dark:text-zinc-300 hover:bg-gray-100 dark:hover:bg-zinc-800 cursor-pointer").
							Text("Cancel").
							Attr("onclick", HideJS(CloseDialogID)+"if(window.__electronCloseAbort)window.__electronCloseAbort();"),
						r.Button("px-3 py-1.5 text-sm rounded-md bg-red-500 hover:bg-red-600 text-white cursor-pointer").
							ID("close-dialog-confirm").
							Text("Yes, close all").
							Attr("onclick", fmt.Sprintf("__ws.callSilent('app.close.all',{sid:'%s'});", sid)+HideJS(CloseDialogID)+"if(window.libroElectron)window.libroElectron.forceClose();else window.close();"),
					),
				),
		)
}
