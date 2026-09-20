package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// ShortcutsDialog renders the keyboard shortcuts reference dialog.
func ShortcutsDialog() *r.Node {
	type shortcut struct {
		keys string
		desc string
	}
	type section struct {
		title     string
		subtitle  string
		shortcuts []shortcut
	}
	sections := []section{
		{"Apps", "", []shortcut{
			{"⌘ + O", "Open plugin launcher"},
			{"⌘ + Enter", "Open terminal in Libro"},
			{"Ctrl + ; / ⌘ + ;", "Command palette"},
			{"Ctrl + Shift + S", "Settings"},
			{"⌘ + Q", "Close current app"},
			{"Ctrl + , / ⌘ + ,", "Increase app width"},
			{"Ctrl + . / ⌘ + .", "Decrease app width"},
			{"⌘ + F", "Toggle full width"},
			{"⌘ + +", "Zoom in (whole app)"},
			{"⌘ + -", "Zoom out (whole app)"},
			{"⌘ + 0", "Reset zoom (whole app)"},
		}},
		{"Navigation", "", []shortcut{
			{"Ctrl + A", "Focus agent"},
			{"Ctrl + A twice", "Hide tools and show only agents"},
			{"Ctrl + H / L", "Previous / next panel"},
			{"Ctrl + 1–9", "Switch to numbered project with open panels"},
			{"Ctrl + Shift + P", "Toggle project sidebar"},
			{"⌘ + H", "Navigate left"},
			{"⌘ + L", "Navigate right"},
			{"⌘ + [", "Move app left"},
			{"⌘ + ]", "Move app right"},
			{"⌘ + Ctrl + Y", "Move app to project"},
			{"⌘ + N", "Open projects"},
		}},
		{"Browser", "", []shortcut{
			{"⌘ + B", "New browser with URL popup"},
			{"⌘ + E", "Open nvim (falls back to vim)"},
			{"⌘/Win + Y", "Open Pi agent if available (MD width)"},
		}},
		{"Browser", "Vim keys — disabled in input fields / insert mode", []shortcut{
			{"g / G", "Go to top / bottom of page"},
			{"j / k", "Scroll down / up"},
			{"h / l", "Scroll left / right"},
			{"o", "Open browser address"},
			{"r", "Reload browser page"},
			{"m", "Cycle viewport size (SM / MD / XL / previous size)"},
			{"M", "Rotate SM / MD / XL viewport portrait or landscape"},
			{"i / Esc", "Enter / exit insert mode"},
		}},
	}

	rows := make([]*r.Node, 0)
	for i, sec := range sections {
		mt := "mt-6"
		if i == 0 {
			mt = "mt-0"
		}
		sectionRows := make([]*r.Node, 0, len(sec.shortcuts))
		for _, s := range sec.shortcuts {
			sectionRows = append(sectionRows,
				r.Div("ws-shortcut-row").Render(
					r.Span("").Text(s.desc),
					r.El("kbd", "").Text(s.keys),
				),
			)
		}
		header := []*r.Node{
			r.El("h2", "ws-shortcut-heading").Text(sec.title),
		}
		if sec.subtitle != "" {
			header = append(header,
				r.Div("ws-shortcut-subtitle").Text(sec.subtitle),
			)
		}
		card := r.Div("ws-shortcut-group").Render(sectionRows...)
		rows = append(rows,
			r.Div(mt).Render(
				append(header, card)...,
			),
		)
	}

	return r.Div("ws-popup fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ShortcutsDialogID).
		OnClick(r.JS(HideJS(ShortcutsDialogID))).
		Render(
			r.Div("ws-shortcuts-panel w-full max-w-2xl mx-4 overflow-hidden").
				Attr("role", "dialog").Attr("aria-modal", "true").Attr("aria-label", "Keyboard shortcuts").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("ws-shortcuts-body").Render(rows...),
					r.Div("px-6 py-3 text-xs text-gray-500 dark:text-zinc-400").Render(
						r.Span("").Text("Esc to close"),
					),
				),
		)
}
