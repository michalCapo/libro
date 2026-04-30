package components

import (
	"fmt"

	r "github.com/michalCapo/g-sui/ui"
)

// CloseDialog renders the quit confirmation dialog (hidden by default).
// It is populated dynamically when the user runs the quit command.
func CloseDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[70] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(CloseDialogID).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-amber-500 text-lg").Text("warning"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Quit Libro?"),
					),
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.P("text-sm text-gray-600 dark:text-zinc-400").Text("The following applications are still running:"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("close-dialog-apps"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Esc cancel"),
						r.Div("flex items-center gap-2").Render(
							r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
								Text("Cancel").
								Attr("onclick", HideJS(CloseDialogID)+"if(window.__electronCloseAbort)window.__electronCloseAbort();"),
						r.Button("px-4 py-1 bg-red-500 hover:bg-red-600 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
							ID("close-dialog-confirm").
							Text("Quit").
							Attr("onclick", fmt.Sprintf("__ws.callSilent('app.close.all',{sid:'%s'});", sid)+HideJS(CloseDialogID)+"if(window.libroElectron)window.libroElectron.forceClose();else window.close();"),
					),
					),
				),
		)
}
