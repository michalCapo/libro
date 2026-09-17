package components

import (
	"fmt"

	r "github.com/michalCapo/g-sui/ui"
)

// CloseDialog renders the quit confirmation dialog (hidden by default).
// It is populated dynamically when the user closes the desktop window.
func CloseDialog(sid string) *r.Node {
	return r.Div("ws-popup fixed inset-0 z-[70] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(CloseDialogID).
		Render(
			r.Div("ws-close-panel").
				Attr("role", "dialog").Attr("aria-modal", "true").Attr("aria-labelledby", "close-dialog-title").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("ws-close-heading").Render(
						r.I("material-icons-round text-amber-500 text-lg").Text("warning"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").ID("close-dialog-title").Text("Quit Libro?"),
					),
					r.Div("ws-close-description").Render(
						r.P("text-sm text-gray-600 dark:text-zinc-400").Text("The following applications are still running:"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("close-dialog-apps"),
					r.Div("ws-close-footer").Render(
						r.Span("").Text("Esc cancel"),
						r.Div("flex items-center gap-2").Render(
							r.Button("ws-close-button").Attr("type", "button").
								Text("Cancel").
								Attr("onclick", HideJS(CloseDialogID)),
							r.Button("ws-close-button ws-close-quit").Attr("type", "button").
								ID("close-dialog-confirm").
								Text("Quit").
								Attr("onclick", fmt.Sprintf("__ws.call('app.close.all',{sid:'%s'});", sid)),
						),
					),
				),
		)
}
