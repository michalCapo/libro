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
			{"⌘ + O", "Open launcher/search on right"},
			{"⌘ + Enter", "Open terminal in Libro"},
			{"⌘ + ;", "Command palette"},
			{"⌘ + Q", "Close current app"},
			{"⌘ + ,", "Decrease app width"},
			{"⌘ + .", "Increase app width"},
			{"⌘ + F", "Toggle full width"},
			{"⌘ + +", "Zoom in (whole app)"},
			{"⌘ + -", "Zoom out (whole app)"},
			{"⌘ + 0", "Reset zoom (whole app)"},
			{"quit", "Quit Libro from command palette"},
		}},
		{"Navigation", "Win + X toggles an automatic Ctrl + 2-9 assignment for the current project", []shortcut{
			{"⌘ + H", "Navigate left"},
			{"⌘ + L", "Navigate right"},
			{"⌘ + [", "Move app left"},
			{"⌘ + ]", "Move app right"},
			{"⌘ + Ctrl + Y", "Move app to project"},
			{"⌘ + N", "Open projects"},
			{"⌘ + G", "Create worktree from current branch"},
			{"⌘ + X", "Assign or remove current project shortcut"},
			{"Ctrl + 1", "Switch to home project"},
			{"Ctrl + 2–9", "Switch to assigned project or worktree"},
			{"Ctrl + 0", "Switch to previous project"},
			{"⌘ + Z", "Toggle zen mode (hide UI)"},
		}},
		{"Search", "⌘ + O to open", []shortcut{
			{": query", "Search the internet"},
			{"! command", "Run terminal command"},
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
			{"o", "URL / search popup for browser apps"},
			{"r", "Reload browser page"},
			{"m", "Cycle mobile view (XS / SM / MD / previous size)"},
			{"/", "Find in page"},
			{"n / N / p", "Find next / previous"},
			{"i / Esc", "Enter / exit insert mode"},
			{"Esc", "Clear search / blur input"},
			{"b / f", "Page back / forward"},
			{"y", "Copy selected text or URL"},
			{"c", "Open DevTools Console tab"},
			{"Enter", "Follow link / click button"},
		}},
	}

	rows := make([]*r.Node, 0)
	for i, sec := range sections {
		mt := "mt-6"
		if i == 0 {
			mt = "mt-0"
		}
		sectionRows := make([]*r.Node, 0, len(sec.shortcuts))
		for j, s := range sec.shortcuts {
			borderCls := "border-t border-gray-100 dark:border-zinc-800"
			if j == 0 {
				borderCls = ""
			}
			sectionRows = append(sectionRows,
				r.Div("flex items-center justify-between gap-3 px-3 py-2.5 "+borderCls).Render(
					r.Div("flex items-center gap-3 min-w-0").Render(
						r.I("material-icons-round text-gray-400 dark:text-zinc-600 text-base").Text("keyboard"),
						r.Span("text-sm text-gray-700 dark:text-zinc-300 truncate").Text(s.desc),
					),
					r.Span("text-[10px] font-mono font-semibold uppercase tracking-wider px-2 py-1 rounded bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400").Text(s.keys),
				),
			)
		}
		header := []*r.Node{
			r.Div("px-1 pb-0.5 text-sm font-bold uppercase tracking-wider text-gray-700 dark:text-zinc-300").Text(sec.title),
		}
		if sec.subtitle != "" {
			header = append(header,
				r.Div("px-1 pb-2 text-[10px] uppercase tracking-wider text-gray-400 dark:text-zinc-500").Text(sec.subtitle),
			)
		} else {
			header = append(header, r.Div("pb-2").Text(""))
		}
		card := r.Div("border border-gray-200 dark:border-zinc-700/50 rounded-lg overflow-hidden").Render(sectionRows...)
		rows = append(rows,
			r.Div(mt).Render(
				append(header, card)...,
			),
		)
	}

	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ShortcutsDialogID).
		OnClick(r.JS(HideJS(ShortcutsDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-2xl mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("keyboard"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Keyboard Shortcuts"),
					),
					r.Div("max-h-[60vh] overflow-y-auto px-4 py-4").Render(rows...),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Esc close"),
					),
				),
		)
}
