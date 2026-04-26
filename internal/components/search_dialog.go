package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// SearchDialog renders the fuzzy search popup (hidden by default).
// All filtering, navigation, and selection logic runs client-side via JS
// since saved apps are injected via __libroSavedApps from the DB.
func SearchDialog(sid string) *r.Node {
	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(SearchDialogID).
		OnClick(r.JS(HideJS(SearchDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-lg mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
						r.Input("w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono").
							ID("search-input").
							Attr("type", "text").
							Attr("placeholder", "Search applications...").
							Attr("autocomplete", "off").
							Attr("spellcheck", "false").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();}"),
					),
					r.Div("max-h-80 overflow-y-auto").ID("search-results"),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center justify-between text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Div("flex items-center gap-4").Render(
							r.Span("").Text("↑↓ navigate"),
							r.Span("").Text("Enter open"),
							r.Span("").Text("Esc close"),
						),
						r.Span("cursor-pointer hover:text-red-400 transition-colors").
							Attr("onclick", "event.stopPropagation();__ws.call('history.clear',{sid:'"+sid+"'});__ws.call('run.history.clear',{sid:'"+sid+"'});").
							Text("clear history"),
					),
				),
		)
}
