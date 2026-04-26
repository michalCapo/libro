package components

import (
	"fmt"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

// AddDialogInput is the input for AddDialog.
type AddDialogInput struct {
	Sid          string
	Visible      bool
	Widths       []string
	DefaultWidth string
}

// AddDialog renders the "Add Application" modal with URL/terminal tabs.
func AddDialog(in AddDialogInput) *r.Node {
	hiddenClass := " hidden"
	if in.Visible {
		hiddenClass = ""
	}

	widthOptions := make([]*r.Node, 0, len(in.Widths))
	for _, w := range in.Widths {
		radio := r.IRadio("accent-blue-500 cursor-pointer").
			Attr("name", "app-width").
			Attr("value", w).
			Attr("onchange", fmt.Sprintf("document.getElementById('app-width').value='%s'", w)).
			ID(fmt.Sprintf("width-%s", w))
		if w == in.DefaultWidth {
			radio.Attr("checked", "checked")
		}
		widthOptions = append(widthOptions,
			r.Label("flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-300 dark:border-zinc-700 hover:border-gray-400 dark:hover:border-zinc-500 cursor-pointer transition-colors text-gray-700 dark:text-zinc-300 text-sm font-mono").Render(
				radio,
				r.Span("").Text(strings.ToUpper(w)),
			),
		)
	}

	tabSwitchJS := func(showTab string) string {
		return fmt.Sprintf(`
			document.getElementById('tab-url-content').classList.toggle('hidden', '%s' !== 'url');
			document.getElementById('tab-terminal-content').classList.toggle('hidden', '%s' !== 'terminal');
			document.getElementById('tab-url-btn').classList.toggle('border-blue-500', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-blue-600', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('border-transparent', '%s' !== 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-gray-500', '%s' !== 'url');
			document.getElementById('tab-terminal-btn').classList.toggle('border-blue-500', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-blue-600', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('border-transparent', '%s' !== 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-gray-500', '%s' !== 'terminal');
			document.getElementById('app-type').value = '%s';
		`, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab)
	}

	collectIDs := []string{"app-url", "app-command", "app-writable", "app-type", "app-name", "app-width", "app-project-specific"}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 outline-none transition-colors"

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75" + hiddenClass).
		ID(AddDialogID).
		OnClick(r.JS(HideJS(AddDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-md mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("Add Application"),

					r.IHidden("").ID("app-type").Attr("value", "terminal"),
					r.IHidden("").ID("app-width").Attr("value", in.DefaultWidth),

					r.Div("flex border-b border-gray-200 dark:border-zinc-700/50 mb-4").Render(
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-blue-500 text-blue-600 cursor-pointer transition-colors").
							ID("tab-terminal-btn").
							Text("Terminal").
							OnClick(r.JS(tabSwitchJS("terminal"))),
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 cursor-pointer transition-colors").
							ID("tab-url-btn").
							Text("URL").
							OnClick(r.JS(tabSwitchJS("url"))),
					),

					r.Div("mb-5 hidden").ID("tab-url-content").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("URL"),
						r.IUrl(inputCls).
							ID("app-url").
							Attr("placeholder", "https://example.com").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
						r.P("text-xs text-gray-400 dark:text-zinc-500 mt-1").Text("Use __dir__ as a placeholder for the project directory."),
					),

					r.Div("mb-5").ID("tab-terminal-content").Render(
						r.Div("mb-3").Render(
							r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Command"),
							r.IText(inputCls+" font-mono").
								ID("app-command").
								Attr("placeholder", "bash").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
							r.P("text-xs text-gray-400 dark:text-zinc-500 mt-1").Text("Use __dir__ as a placeholder for the project directory."),
						),
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-blue-500 cursor-pointer w-4 h-4").
								ID("app-writable").
								Attr("checked", "checked"),
							r.Span("text-sm text-gray-600 dark:text-zinc-400").Text("Writable (allow input)"),
						),
					),

					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Name (optional)"),
						r.IText(inputCls).
							ID("app-name").
							Attr("placeholder", "e.g. My App"),
					),

					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Width"),
						r.Div("flex flex-wrap gap-1.5").Render(widthOptions...),
					),

					r.Div("mb-5").Render(
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-blue-500 cursor-pointer w-4 h-4").
								ID("app-project-specific"),
							r.Span("text-sm text-gray-600 dark:text-zinc-400").Text("Project specific"),
						),
					),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "app.dialog.close", Data: map[string]any{"sid": in.Sid}}),
						r.Button("px-5 py-2 bg-blue-600 hover:bg-blue-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-add").
							Text("Save").
							OnClick(&r.Action{
								Name:    "app.save",
								Data:    map[string]any{"sid": in.Sid},
								Collect: collectIDs,
							}),
					),
				),
		)
}
