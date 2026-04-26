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
			{"⌘ + O", "New app (right of current)"},
			{"⌘ + Ctrl + O", "New app (left of current)"},
			{"⌘ + ;", "Command palette"},
			{"⌘ + W", "Close current app"},
			{"⌘ + R", "Resize app popup"},
			{"⌘ + F", "Toggle full width"},
			{"⌘ + +", "Zoom in (whole app)"},
			{"⌘ + -", "Zoom out (whole app)"},
			{"⌘ + 0", "Reset zoom (whole app)"},
		}},
		{"Navigation", "Win + X toggles an automatic Ctrl + 2-9 assignment for the current project", []shortcut{
			{"⌘ + H", "Navigate left"},
			{"⌘ + L", "Navigate right"},
			{"⌘ + Ctrl + U", "Move app left"},
			{"⌘ + Ctrl + I", "Move app right"},
			{"⌘ + N", "Open project & worktree picker"},
			{"⌘ + G", "Create worktree from current branch"},
			{"⌘ + X", "Assign or remove current project shortcut"},
			{"Ctrl + 1", "Switch to home project"},
			{"Ctrl + 2–9", "Switch to assigned project or worktree"},
			{"Ctrl + 0", "Switch to previous project"},
			{"⌘ + Z", "Toggle zen mode (hide UI)"},
			{"⌘ + Q", "Quit Libro"},
		}},
		{"Search", "⌘ + O or ⌘ + Ctrl + O to open", []shortcut{
			{": query", "Search the internet"},
			{"! command", "Run terminal command"},
		}},
		{"Browser", "", []shortcut{
			{"Ctrl + L", "URL / search popup for browser apps"},
			{"Ctrl + R", "Reload browser page"},
		}},
		{"Browser", "Vim keys — disabled in input fields", []shortcut{
			{"g / G", "Go to top / bottom of page"},
			{"j / k", "Scroll down / up"},
			{"h / l", "Scroll left / right"},
			{"/", "Find in page"},
			{"n / p", "Find next / previous"},
			{"Esc", "Clear search / blur input"},
			{"b / f", "Page back / forward"},
			{"y", "Copy selected text or URL"},
			{"c", "Open DevTools Console tab"},
			{"Enter", "Follow link / click button"},
		}},
	}

	rows := make([]*r.Node, 0)
	for i, sec := range sections {
		mt := "mt-10"
		if i == 0 {
			mt = "mt-0"
		}
		sectionRows := make([]*r.Node, 0, len(sec.shortcuts))
		for _, s := range sec.shortcuts {
			sectionRows = append(sectionRows,
				r.Div("flex items-center justify-between py-2 px-1").Render(
					r.Span("text-sm text-gray-700 dark:text-zinc-300").Text(s.desc),
					r.Span("text-xs font-mono px-2 py-0.5 rounded bg-gray-100 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400").Text(s.keys),
				),
			)
		}
		header := []*r.Node{
			r.Div("px-1 pb-1 text-lg font-semibold uppercase tracking-wider text-gray-500 dark:text-zinc-400").Text(sec.title),
		}
		if sec.subtitle != "" {
			header = append(header,
				r.Div("px-1 pb-1 text-xs text-gray-400 dark:text-zinc-500").Text(sec.subtitle),
			)
		}
		rows = append(rows,
			r.Div(mt).Render(
				append(header, r.Div("").Render(sectionRows...))...,
			),
		)
	}

	return r.Div("fixed inset-0 z-[60] flex items-start justify-center pt-[15vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(ShortcutsDialogID).
		OnClick(r.JS(HideJS(ShortcutsDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-md mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("keyboard"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Keyboard Shortcuts"),
						r.Button("text-gray-400 dark:text-zinc-500 hover:text-gray-600 dark:hover:text-zinc-300 cursor-pointer").
							Attr("onclick", HideJS(ShortcutsDialogID)).
							Render(r.I("material-icons-round text-base").Text("close")),
					),
					r.Div("px-4 py-2 max-h-[60vh] overflow-y-auto").Render(rows...),
					r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
						r.Span("").Text("Esc to close"),
					),
				),
		)
}
