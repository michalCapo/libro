package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// URLPopup renders the address popup that floats over a browser app.
// The sid parameter is reserved for future server actions (currently the
// popup is fully driven by injected JS).
func URLPopup(_ string) *r.Node {
	return r.Div("ws-popup absolute inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(URLPopupID).
		OnClick(r.JS(HideJS(URLPopupID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 rounded-xl shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				Attr("role", "dialog").Attr("aria-modal", "true").Attr("aria-label", "Browser address").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("ws-command-search").Render(
						r.I("material-icons-round").Attr("aria-hidden", "true").Text("search"),
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("url-popup-input").
							Attr("type", "text").
							Attr("placeholder", "Enter URL...").
							Attr("aria-label", "Browser address").
							Attr("autocomplete", "off").
							Attr("role", "combobox").Attr("aria-autocomplete", "list").Attr("aria-controls", "url-popup-results").Attr("aria-expanded", "false").
							Attr("spellcheck", "false"),
						r.Button("ws-command-dismiss").Attr("type", "button").Attr("aria-label", "Close browser address").OnClick(r.JS(HideJS(URLPopupID))).Render(r.I("material-icons-round").Attr("aria-hidden", "true").Text("close")),
					),
					r.Div("").ID("url-popup-results").Attr("role", "listbox").Attr("aria-label", "Recent addresses").Attr("hidden", "hidden"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Enter open"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}
