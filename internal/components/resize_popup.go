package components

import (
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// ResizePopup renders the app width selector popup.
func ResizePopup(_ string, widths []string) *r.Node {
	buttons := make([]*r.Node, 0, len(widths))
	for _, w := range widths {
		buttons = append(buttons,
			r.Div("resize-btn flex items-center gap-3 px-4 py-2 rounded cursor-pointer transition-colors duration-75 text-gray-600 dark:text-zinc-400 hover:bg-gray-100 dark:hover:bg-zinc-800").
				Attr("data-resize-width", w).
				Render(
					r.Div("w-4 h-4 rounded-full border-2 border-gray-300 dark:border-zinc-600 flex items-center justify-center shrink-0").
						Attr("data-radio", "").
						Render(r.Div("w-2 h-2 rounded-full bg-blue-600 hidden").Attr("data-radio-dot", "")),
					r.Span("text-sm font-mono tracking-wider uppercase").Text(strings.ToUpper(w)),
				),
		)
	}

	return r.Div("absolute inset-0 z-[60] flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden outline-none").
		ID(ResizePopupID).
		Attr("tabindex", "-1").
		OnClick(r.JS(HideJS(ResizePopupID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-xs mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-2").Render(
						r.I("material-icons-round text-blue-500 text-lg").Text("aspect_ratio"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200").Text("Resize App"),
					),
					r.Div("px-3 py-2 flex flex-col gap-0").
						ID("resize-popup-buttons").
						Render(buttons...),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("j/k navigate"),
						r.Span("").Text("Enter resize"),
						r.Span("").Text("Esc close"),
					),
				),
		)
}
