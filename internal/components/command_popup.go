package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// CommandPopup renders the global command palette (Ctrl+; or Cmd+;).
func CommandPopup() *r.Node {
	return r.Div("ws-popup fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(CommandPopupID).
		OnClick(r.JS(HideJS(CommandPopupID))).
		Render(
			r.Div("ws-command-panel w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Attr("role", "dialog").Attr("aria-modal", "true").Attr("aria-label", "Commands").
				Render(
					r.Div("ws-command-search").Render(
						r.I("material-icons-round").Attr("aria-hidden", "true").Text("search"),
						r.Input("").
							ID("command-popup-input").
							Attr("type", "text").
							Attr("placeholder", "Search commands…").
							Attr("aria-label", "Search commands").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();}"),
						r.Button("ws-command-dismiss").Attr("type", "button").Attr("aria-label", "Close commands").OnClick(r.JS(HideJS(CommandPopupID))).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("close")),
					),
					r.Div("max-h-80 overflow-y-auto").ID("command-popup-results"),
					r.Div("ws-command-footer").Render(
						r.Span("").Render(r.El("kbd", "").Text("↑ ↓"), r.Span("").Text("Navigate")),
						r.Span("").Render(r.El("kbd", "").Text("↵"), r.Span("").Text("Run command")),
					),
				),
		)
}
