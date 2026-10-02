package libro

import (
	r "github.com/michalCapo/g-sui/ui"
	"libro/internal/components"
)

func showCloseDialogJS(projectApps []ProjectApps, title, button, action, sid string) r.Result {
	tree := r.Div("max-h-80 overflow-y-auto").ID("close-dialog-apps")
	for _, pa := range projectApps {
		group := r.Div("mb-2").Render(r.Div("flex items-center gap-1.5 text-xs font-medium text-gray-700 dark:text-zinc-300 mb-1").Render(
			r.I("material-icons-round text-sm").Text("folder"), r.Span().Text(pa.Name)))
		for _, app := range pa.Apps {
			icon, label := "language", app.Name
			if app.Type == AppTypeTerminal {
				icon = "terminal"
				if label == "" {
					label = app.Command
				}
			} else if label == "" {
				label = app.URL
			}
			group.Render(r.Div("flex items-center gap-1.5 ml-5 py-0.5 text-xs text-gray-500 dark:text-zinc-500").Render(
				r.I("material-icons-round text-xs").Text(icon), r.Span("truncate").Text(label)))
		}
		tree.Render(group)
	}
	confirm := actionAppCloseAll.Call(sessionInput{SID: sid})
	if action == "project.close" {
		confirm = actionProjectClose.Call(actionProjectCloseInput{SID: sid})
	}
	return r.Result{}.Replace("close-dialog-apps", tree).Replace("close-dialog-confirm",
		r.Button("ws-close-button ws-close-quit").ID("close-dialog-confirm").Attr("type", "button").Attr("autofocus", "").Text(button).
			OnClick(r.Seq(r.Hide(CloseDialogID), confirm))).
		Add(r.Result{}.Run(r.Seq(r.SetText("close-dialog-title", title), r.UnsafeJS("if(window.__libroCloseAllPopups)window.__libroCloseAllPopups('close-dialog');"), r.Show(components.CloseDialogID), r.Focus("close-dialog-confirm"))))
}
