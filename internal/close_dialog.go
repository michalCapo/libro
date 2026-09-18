package libro

import (
	"fmt"
	"html"
	"strings"

	"libro/internal/components"
)

func showCloseDialogJS(projectApps []ProjectApps, title, button, action, sid string) string {
	// Build tree HTML: project > apps
	var tree strings.Builder
	for _, pa := range projectApps {
		fmt.Fprintf(&tree, `<div class="mb-2"><div class="flex items-center gap-1.5 text-xs font-medium text-gray-700 dark:text-zinc-300 mb-1"><span class="material-icons-round text-sm">folder</span>%s</div>`, html.EscapeString(pa.Name))
		for _, a := range pa.Apps {
			icon := "language"
			label := a.Name
			if a.Type == AppTypeTerminal {
				icon = "terminal"
				if label == "" {
					label = a.Command
				}
			} else {
				if label == "" {
					label = a.URL
				}
			}
			fmt.Fprintf(&tree, `<div class="flex items-center gap-1.5 ml-5 py-0.5 text-xs text-gray-500 dark:text-zinc-500"><span class="material-icons-round text-xs">%s</span><span class="truncate">%s</span></div>`, icon, html.EscapeString(label))
		}
		tree.WriteString(`</div>`)
	}
	return fmt.Sprintf(`(function(){
        var dlg=document.getElementById('%s');
        if(!dlg)return;
        document.getElementById('close-dialog-title').textContent=%s;
        document.getElementById('close-dialog-apps').innerHTML=%s;
        var cb=document.getElementById('close-dialog-confirm');
        cb.textContent=%s;
        cb.onclick=function(){dlg.classList.add('hidden');__ws.call(%s,{sid:%s});};
        if(window.__libroCloseAllPopups)window.__libroCloseAllPopups(dlg);
        dlg.classList.remove('hidden');
        cb.focus();
    })();`, CloseDialogID, components.JSString(title), components.JSString(tree.String()),
		components.JSString(button), components.JSString(action), components.JSString(sid))
}
