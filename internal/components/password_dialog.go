package components

import (
	r "github.com/michalCapo/g-sui/ui"
)

// PasswordDialog renders the encrypted password vault popup.
func PasswordDialog() *r.Node {
	inputCls := "w-full bg-transparent text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 outline-none font-mono"
	noteCls := inputCls + " resize-none leading-relaxed"
	labelCls := "block text-[10px] font-mono uppercase tracking-wider text-gray-400 dark:text-zinc-500 mb-1"
	panelCls := "px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50"

	return r.Div("fixed inset-0 z-[70] flex items-start justify-center pt-[10vh] bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75 hidden").
		ID(PasswordDialogID).
		OnClick(r.JS(HideJS(PasswordDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl w-full max-w-xl mx-4 overflow-hidden").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50 flex items-center gap-3").Render(
						r.I("material-icons-round text-blue-600 dark:text-blue-400 text-lg").Text("password"),
						r.Span("text-sm font-medium text-gray-800 dark:text-zinc-200 flex-1").Text("Passwords"),
						r.Button("hidden text-[11px] font-mono text-blue-600 dark:text-blue-400 hover:underline cursor-pointer").
							ID("password-add-open").
							Attr("onclick", "if(window.__libroPasswordShowAdd)window.__libroPasswordShowAdd();").
							Render(
								r.I("material-icons-round text-sm align-middle mr-0.5").Text("add"),
								r.Span("align-middle").Text("Add"),
							),
					),
					r.Div("").ID("password-setup-pane").Render(
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Master password"),
							r.Input(inputCls).
								ID("password-setup-master").
								Attr("type", "password").
								Attr("autocomplete", "new-password").
								Attr("placeholder", "Create master password").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('password-setup-save').click();}"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Confirm"),
							r.Input(inputCls).
								ID("password-setup-confirm").
								Attr("type", "password").
								Attr("autocomplete", "new-password").
								Attr("placeholder", "Repeat master password").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('password-setup-save').click();}"),
						),
						r.Div("px-4 py-2 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
							r.Span("").Text("Master password is required after restart"),
							r.Button("px-4 py-1 bg-blue-600 hover:bg-blue-500 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
								ID("password-setup-save").
								Attr("onclick", "if(window.__libroPasswordSetup)window.__libroPasswordSetup();").
								Text("Create Vault"),
						),
					),
					r.Div("hidden").ID("password-unlock-pane").Render(
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Master password"),
							r.Input(inputCls).
								ID("password-unlock-master").
								Attr("type", "password").
								Attr("autocomplete", "current-password").
								Attr("placeholder", "Unlock vault").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('password-unlock-save').click();}"),
						),
						r.Div("px-4 py-2 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
							r.Span("").Text("Esc close"),
							r.Button("px-4 py-1 bg-blue-600 hover:bg-blue-500 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
								ID("password-unlock-save").
								Attr("onclick", "if(window.__libroPasswordUnlock)window.__libroPasswordUnlock();").
								Text("Unlock"),
						),
					),
					r.Div("hidden").ID("password-search-pane").Render(
						r.Div("px-4 py-3 border-b border-gray-200 dark:border-zinc-700/50").Render(
							r.Input(inputCls).
								ID("password-search-input").
								Attr("type", "text").
								Attr("placeholder", "Search URL or name...").
								Attr("autocomplete", "off").
								Attr("spellcheck", "false").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();}"),
						),
						r.Div("max-h-80 overflow-y-auto").ID("password-results"),
						r.Div("px-4 py-2 border-t border-gray-100 dark:border-zinc-800 flex items-center gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
							r.Span("").Text("↑↓ navigate"),
							r.Span("").Text("Enter copy password"),
							r.Span("").Text("U copy username"),
							r.Span("").Text("Esc close"),
						),
					),
					r.Div("hidden").ID("password-view-pane").Render(
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Name"),
							r.Div(inputCls).ID("password-view-name"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("URL"),
							r.Div(inputCls).ID("password-view-url"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Username"),
							r.Div(inputCls).ID("password-view-username"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Password"),
							r.Div(inputCls).ID("password-view-password"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Note"),
							r.Div(inputCls+" whitespace-pre-wrap leading-relaxed").ID("password-view-note"),
						),
						r.Div("px-4 py-2 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
							r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
								Text("Back").
								Attr("onclick", "if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();"),
							r.Div("flex items-center gap-2").Render(
								r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
									Attr("onclick", "if(window.__libroPasswordCopyCurrent)window.__libroPasswordCopyCurrent('username');").
									Text("Copy Username"),
								r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
									Attr("onclick", "if(window.__libroPasswordCopyCurrent)window.__libroPasswordCopyCurrent('password');").
									Text("Copy Password"),
								r.Button("px-4 py-1 bg-blue-600 hover:bg-blue-500 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
									Attr("onclick", "if(window.__libroPasswordEditCurrent)window.__libroPasswordEditCurrent();").
									Text("Edit"),
							),
						),
					),
					r.Div("hidden").ID("password-add-pane").Render(
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Name"),
							r.Input(inputCls).ID("password-entry-name").Attr("type", "text").Attr("autocomplete", "off").Attr("placeholder", "Example"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("URL"),
							r.Input(inputCls).ID("password-entry-url").Attr("type", "url").Attr("autocomplete", "off").Attr("placeholder", "https://example.com"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Username"),
							r.Input(inputCls).ID("password-entry-username").Attr("type", "text").Attr("autocomplete", "off").Attr("placeholder", "user@example.com"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Password"),
							r.Input(inputCls).ID("password-entry-password").Attr("type", "password").Attr("autocomplete", "new-password").Attr("placeholder", "Password"),
						),
						r.Div(panelCls).Render(
							r.Label(labelCls).Text("Note"),
							r.Textarea(noteCls).ID("password-entry-note").Attr("rows", "2").Attr("autocomplete", "off").Attr("placeholder", "Recovery codes, security questions, or context"),
						),
						r.Div("px-4 py-2 flex items-center justify-between gap-4 text-[10px] font-mono text-gray-400 dark:text-zinc-600").Render(
							r.Button("px-3 py-1 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-xs rounded hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
								Text("Cancel").
								Attr("onclick", "if(window.__libroPasswordShowSearch)window.__libroPasswordShowSearch();"),
							r.Button("px-4 py-1 bg-blue-600 hover:bg-blue-500 text-white font-mono text-xs font-medium rounded transition-colors cursor-pointer").
								ID("password-entry-save").
								Attr("onclick", "if(window.__libroPasswordSave)window.__libroPasswordSave();").
								Text("Save"),
						),
					),
				),
		)
}
