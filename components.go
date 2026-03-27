package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	r "github.com/michalCapo/g-sui/ui"
)

const (
	StripID         = "app-strip"
	DialogID        = "add-dialog"
	MainAreaID      = "main-area"
	NavLeftID       = "nav-left"
	NavRightID      = "nav-right"
	ProjectBarID    = "project-bar"
	ProjectDialogID = "project-dialog"
	DirBrowserID    = "dir-browser"
)

// sidData creates a data map with the session ID included
func sidData(sid string, extra ...any) map[string]any {
	m := map[string]any{"sid": sid}
	for i := 0; i+1 < len(extra); i += 2 {
		if key, ok := extra[i].(string); ok {
			m[key] = extra[i+1]
		}
	}
	return m
}

// renderMainArea renders the entire main area based on current state
func renderMainArea(state *AppState, sid string) *r.Node {
	if len(state.Apps) == 0 {
		return renderEmptyState(sid)
	}
	return renderAppStrip(state, sid)
}

// renderEmptyState renders the saved apps list with "+ Add New" button
func renderEmptyState(sid string) *r.Node {
	container := r.Div("flex-1 flex items-center justify-center").ID(MainAreaID).Render(
		r.Div("flex flex-col items-center gap-2 w-full max-w-md").Render(
			r.Div("flex flex-col gap-1.5 w-full").ID("saved-apps-list"),
			r.Div("flex gap-2 w-full mt-1").Render(
				r.Button("flex-1 flex items-center justify-center gap-2 px-6 py-3 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Text("+ Add New").
					OnClick(&r.Action{Name: "app.dialog.open", Data: sidData(sid)}),
				r.Button("flex-1 flex items-center justify-center gap-2 px-6 py-3 bg-indigo-600 hover:bg-indigo-500 text-white font-mono text-sm font-medium rounded-md cursor-pointer transition-colors duration-75").
					Text("Browse").
					OnClick(r.JS(quickBrowseJS(sid))),
			),
		),
	)
	container.JS(renderSavedAppsJS(sid))
	return container
}

// renderSavedAppsJS returns JS that reads localStorage and renders saved app buttons
func renderSavedAppsJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var c=document.getElementById('saved-apps-list');
	if(!c)return;
	var dk=document.documentElement.classList.contains('dark');
	var apps=JSON.parse(localStorage.getItem('libro-apps')||'[]');
	function termIconHtml(cmd){var ini=(cmd||'T').substring(0,2).toUpperCase();return '<span class="w-5 h-5 shrink-0 rounded-md bg-gradient-to-br from-teal-400 to-emerald-600 dark:from-teal-500 dark:to-emerald-700" style="display:inline-flex;align-items:center;justify-content:center;font-size:9px;font-weight:700;color:#fff;line-height:20px;letter-spacing:.04em;box-shadow:0 1px 3px rgba(20,184,166,.35),inset 0 1px 0 rgba(255,255,255,.15)">'+ini+'</span>';}
	c.innerHTML='';
	apps.forEach(function(app,i){
		var btn=document.createElement('button');
		btn.className=dk
			?'w-full flex items-center gap-3 px-4 py-3 bg-zinc-800/80 hover:bg-zinc-700/80 border border-zinc-700/40 hover:border-zinc-600 rounded-lg cursor-pointer text-left transition-colors duration-75'
			:'w-full flex items-center gap-3 px-4 py-3 bg-white hover:bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg cursor-pointer text-left transition-colors duration-75 shadow-sm';

		var iconHtml='';
		var label='';
		if(app.type==='terminal'){
			iconHtml=termIconHtml(app.command);
			label=app.name||app.command;
		}else{
			try{
				var u=new URL(app.url);
				iconHtml='<img class="w-5 h-5 shrink-0 rounded-sm" src="https://www.google.com/s2/favicons?domain='+encodeURIComponent(u.hostname)+'&sz=32" onerror="this.outerHTML=\'<i class=\\\'material-icons-round text-gray-400 text-lg shrink-0\\\'>language</i>\'">';
				label=app.name||u.hostname.replace(/^www\./,'');
			}catch(e){
				iconHtml='<i class="material-icons-round text-gray-400 text-lg shrink-0">language</i>';
				label=app.name||app.url;
			}
		}
		var txtCls=dk?'text-zinc-200':'text-gray-800';
		var badgeCls=dk?'bg-zinc-700 text-zinc-300':'bg-gray-200 text-gray-600';
		var iconCls=dk?'text-zinc-500 hover:text-zinc-200':'text-gray-400 hover:text-gray-700';
		var delCls=dk?'text-zinc-500 hover:text-red-400':'text-gray-400 hover:text-red-500';
		btn.innerHTML=iconHtml
			+'<span class="flex-1 truncate text-sm '+txtCls+'">'+label+'</span>'
			+'<span class="px-2 py-0.5 text-xs font-mono uppercase tracking-wider rounded shrink-0 '+badgeCls+'">'+app.width+'</span>'
			+'<i class="material-icons-round text-xl libro-edit cursor-pointer '+iconCls+'" data-i="'+i+'">edit</i>'
			+'<i class="material-icons-round text-xl libro-del cursor-pointer '+delCls+'" data-i="'+i+'">close</i>';
		btn.onclick=function(e){
			var idx=parseInt(e.target.dataset.i);
			if(e.target.classList.contains('libro-del')){
				e.stopPropagation();
				apps.splice(idx,1);
				localStorage.setItem('libro-apps',JSON.stringify(apps));
				location.reload();
				return;
			}
			if(e.target.classList.contains('libro-edit')){
				e.stopPropagation();
				localStorage.setItem('libro-edit-idx',String(idx));
				__ws.call('app.dialog.open',{sid:'%s'});
				setTimeout(function(){
					var a=apps[idx];
					if(a.type==='terminal'){
						document.getElementById('tab-terminal-btn').click();
						var cmd=document.getElementById('app-command');
						if(cmd)cmd.value=a.command||'bash';
						var wr=document.getElementById('app-writable');
						if(wr)wr.checked=a.writable!==false;
					}else{
						document.getElementById('tab-url-btn').click();
						var url=document.getElementById('app-url');
						if(url)url.value=a.url||'';
					}
					var nm=document.getElementById('app-name');
					if(nm)nm.value=a.name||'';
					var wr=document.getElementById('width-'+(a.width||'md'));
					if(wr)wr.checked=true;
				},100);
				return;
			}
			__ws.call('app.start',{sid:'%s',type:app.type,url:app.url||'',command:app.command||'',width:app.width||'md',writable:app.writable!==false,name:app.name||''});
		};

		c.appendChild(btn);
	});
})();
`, sid, sid)
}

// renderAppStrip renders the horizontal strip of applications with navigation
func renderAppStrip(state *AppState, sid string) *r.Node {
	hasLeft := state.SelectedIndex > 0
	hasRight := state.SelectedIndex < len(state.Apps)-1

	stripChildren := make([]*r.Node, 0)

	stripChildren = append(stripChildren, renderSideLauncher(sid, "left"))

	leftNavCls := "w-8 shrink-0 flex items-center justify-center bg-gray-200 dark:bg-zinc-800 hover:bg-gray-300 dark:hover:bg-zinc-700 text-gray-500 dark:text-zinc-400 hover:text-teal-600 dark:hover:text-teal-400 text-sm font-mono rounded-md cursor-pointer transition-colors duration-75"
	if !hasLeft {
		leftNavCls += " hidden"
	}
	stripChildren = append(stripChildren,
		r.Button(leftNavCls).ID(NavLeftID).Text("<").
			OnClick(&r.Action{Name: "app.navigate.left", Data: sidData(sid)}),
	)

	for i, app := range state.Apps {
		stripChildren = append(stripChildren, renderAppFrame(app, i, i == state.SelectedIndex, sid))
	}

	rightNavCls := "w-8 shrink-0 flex items-center justify-center bg-gray-200 dark:bg-zinc-800 hover:bg-gray-300 dark:hover:bg-zinc-700 text-gray-500 dark:text-zinc-400 hover:text-teal-600 dark:hover:text-teal-400 text-sm font-mono rounded-md cursor-pointer transition-colors duration-75"
	if !hasRight {
		rightNavCls += " hidden"
	}
	stripChildren = append(stripChildren,
		r.Button(rightNavCls).ID(NavRightID).Text(">").
			OnClick(&r.Action{Name: "app.navigate.right", Data: sidData(sid)}),
	)

	stripChildren = append(stripChildren, renderSideLauncher(sid, "right"))

	strip := r.Div("flex items-stretch h-full min-w-full transition-transform duration-75 ease-out").
		ID(StripID).
		Render(stripChildren...)

	container := r.Div("flex-1 flex items-stretch overflow-hidden").Render(strip)
	container.JS(centerSelectedJS(state.SelectedIndex, len(state.Apps)))

	return r.Div("flex-1 flex items-stretch overflow-hidden relative").ID(MainAreaID).Render(container)
}

func centerSelectedJS(selectedIndex int, totalApps int) string {
	return fmt.Sprintf(`
		(function centerApp() {
			requestAnimationFrame(function() {
				requestAnimationFrame(function() {
					var strip = document.getElementById('%s');
					if (!strip) return;
					var selected = strip.children[%d];
					if (!selected) return;
					var container = strip.parentElement;
					var containerWidth = container.offsetWidth;
					var totalApps = %d;
					if (totalApps === 1) {
						var selectedLeft = selected.offsetLeft;
						var selectedWidth = selected.offsetWidth;
						var offset = selectedLeft - (containerWidth / 2) + (selectedWidth / 2);
						strip.style.transform = 'translateX(' + (-offset) + 'px)';
					} else {
						strip.style.transform = 'translateX(0px)';
					}
				});
			});
		})();
	`, StripID, selectedIndex+2, totalApps)
}

func navigateJS(state *AppState, sid string) string {
	return fmt.Sprintf(`
		(function() {
			var strip = document.getElementById('%s');
			if (!strip) return;
			var selectedIdx = %d;
			var totalApps = %d;
			var offset = 2;

			for (var i = 0; i < totalApps; i++) {
				var child = strip.children[i + offset];
				if (!child) continue;
				if (i === selectedIdx) {
					child.className = child.className.replace(/border-gray-200/g, 'border-teal-500');
					child.className = child.className.replace(/dark:border-zinc-700\/50/g, 'dark:border-teal-500/70');
					child.className = child.className.replace(/border-transparent/g, 'border-teal-500');
					// Remove click overlay from selected app
					var overlay = child.querySelector('[data-click-overlay]');
					if (overlay) overlay.remove();
				} else {
					child.className = child.className.replace(/border-teal-500(?!\/)/g, 'border-gray-200');
					child.className = child.className.replace(/dark:border-teal-500\/70/g, 'dark:border-zinc-700/50');
					// Add click overlay to unselected app if missing
					if (!child.querySelector('[data-click-overlay]')) {
						var ov = document.createElement('div');
						ov.setAttribute('data-click-overlay', '');
						ov.className = 'absolute inset-0 z-20 cursor-pointer';
						ov.onclick = function(idx) {
							return function() { __ws.call('app.select', {"sid": "%s", "index": idx}); };
						}(i);
						child.appendChild(ov);
					}
				}
			}

			var selected = strip.children[selectedIdx + offset];
			if (!selected) return;
			var container = strip.parentElement;
			var containerWidth = container.offsetWidth;
			if (totalApps === 1) {
				var selectedLeft = selected.offsetLeft;
				var selectedWidth = selected.offsetWidth;
				var off = selectedLeft - (containerWidth / 2) + (selectedWidth / 2);
				strip.style.transform = 'translateX(' + (-off) + 'px)';
			} else {
				var stripWidth = strip.scrollWidth;
				if (stripWidth <= containerWidth) {
					strip.style.transform = 'translateX(0px)';
				} else {
					var selectedLeft = selected.offsetLeft;
					var selectedWidth = selected.offsetWidth;
					var off = selectedLeft - (containerWidth / 2) + (selectedWidth / 2);
					strip.style.transform = 'translateX(' + (-off) + 'px)';
				}
			}

			var navLeft = document.getElementById('%s');
			var navRight = document.getElementById('%s');
			if (navLeft) {
				if (selectedIdx > 0) { navLeft.classList.remove('hidden'); }
				else { navLeft.classList.add('hidden'); }
			}
			if (navRight) {
				if (selectedIdx < totalApps - 1) { navRight.classList.remove('hidden'); }
				else { navRight.classList.add('hidden'); }
			}
		})();
	`, StripID, state.SelectedIndex, len(state.Apps), sid, NavLeftID, NavRightID)
}

// renderAppFrame renders a single application iframe with controls
func renderAppFrame(app Application, index int, selected bool, sid string) *r.Node {
	borderClass := "border border-gray-200 dark:border-zinc-700/50"
	if selected {
		borderClass = "border border-teal-500 dark:border-teal-500/70"
	}

	frameID := fmt.Sprintf("frame-%s", app.ID)

	iframeSrc := app.URL
	if app.Type == AppTypeURL {
		iframeSrc = "/proxy?url=" + url.QueryEscape(app.URL)
	}

	// Size badge bar + close
	badgeBase := "px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75"
	badges := make([]*r.Node, 0, len(AllWidths())+1)
	for _, w := range AllWidths() {
		cls := badgeBase + " text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700"
		if w == app.Width {
			cls = badgeBase + " bg-teal-600 text-white"
		}
		badges = append(badges, r.Button(cls).
			Text(strings.ToUpper(string(w))).
			OnClick(&r.Action{
				Name: "app.resize",
				Data: sidData(sid, "id", app.ID, "width", string(w)),
			}))
	}
	badges = append(badges, r.Button(badgeBase+" text-gray-400 dark:text-zinc-500 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-400/10 ml-1").
		Text("CLOSE").
		OnClick(&r.Action{
			Name: "app.close",
			Data: sidData(sid, "id", app.ID),
		}))
	topButtons := r.Div("absolute top-1.5 right-1.5 flex gap-0.5 items-center bg-white/90 dark:bg-zinc-900/80 border border-gray-200 dark:border-transparent rounded-md px-1 py-0.5 opacity-0 group-hover:opacity-100 transition-opacity duration-75 z-30 backdrop-blur-sm").
		Render(badges...)

	var labelNode *r.Node
	if app.Type == AppTypeTerminal {
		initials := strings.ToUpper(app.Command)
		if len(initials) > 2 {
			initials = initials[:2]
		}
		labelText := app.Command
		if app.Name != "" {
			labelText = app.Name
		}
		labelNode = r.Div("absolute top-1.5 left-1.5 flex items-center gap-1.5 px-2 py-0.5 bg-white/90 dark:bg-zinc-900/80 border border-gray-200 dark:border-transparent text-[10px] font-mono tracking-wider uppercase rounded-md opacity-0 group-hover:opacity-100 transition-opacity duration-75 z-30 backdrop-blur-sm").Render(
			r.Span("w-4 h-4 shrink-0 rounded-md bg-gradient-to-br from-teal-400 to-emerald-600 dark:from-teal-500 dark:to-emerald-700").
				Attr("style", "display:inline-flex;align-items:center;justify-content:center;font-size:8px;font-weight:700;color:#fff;line-height:16px;letter-spacing:.04em;box-shadow:0 1px 3px rgba(20,184,166,.35),inset 0 1px 0 rgba(255,255,255,.15)").
				Text(initials),
			r.Span("text-gray-600 dark:text-zinc-300").Text(labelText),
		)
	}

	var clickOverlay *r.Node
	if !selected {
		clickOverlay = r.Div("absolute inset-0 z-20 cursor-pointer").
			Attr("data-click-overlay", "").
			OnClick(&r.Action{
				Name: "app.select",
				Data: sidData(sid, "index", index),
			})
	}

	return r.Div("group relative "+app.Width.ContainerClasses()+" h-full "+borderClass+" rounded-md overflow-hidden bg-white dark:bg-zinc-950 transition-colors duration-75 mx-0.5").
		Attr("data-app-id", app.ID).
		Render(
			topButtons,
			labelNode,
			renderIframe(app, frameID, iframeSrc),
			clickOverlay,
		)
}

func renderIframe(app Application, frameID, iframeSrc string) *r.Node {
	sandbox := "allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-same-origin"
	iframe := r.Iframe("w-full h-full border-0").
		ID(frameID).
		Attr("src", iframeSrc).
		Attr("loading", "lazy").
		Attr("sandbox", sandbox)
	if app.Type == AppTypeTerminal {
		iframe.Attr("scrolling", "no")
	}
	return iframe
}

// insertAppJS returns JS that inserts a new app frame into the existing strip.
// The node is compiled to JS and inserted before nav-right (append) or after nav-left (prepend).
func insertAppJS(node *r.Node, prepend bool) string {
	if prepend {
		return node.ToJSAppend(StripID) + fmt.Sprintf(`
(function(){
	var strip=document.getElementById('%s');
	if(!strip)return;
	var navLeft=document.getElementById('%s');
	if(!navLeft)return;
	var last=strip.lastChild;
	strip.insertBefore(last,navLeft.nextSibling);
})();`, StripID, NavLeftID)
	}
	return node.ToJSAppend(StripID) + fmt.Sprintf(`
(function(){
	var strip=document.getElementById('%s');
	if(!strip)return;
	var navRight=document.getElementById('%s');
	if(!navRight)return;
	var last=strip.lastChild;
	strip.insertBefore(last,navRight);
})();`, StripID, NavRightID)
}

// removeAppJS returns JS that removes an app frame by its app ID from the strip.
func removeAppJS(appID string) string {
	return fmt.Sprintf(`
(function(){
	var el=document.querySelector('[data-app-id="%s"]');
	if(el)el.remove();
})();`, appID)
}

var sideDockCounter int

// renderSideLauncher renders a vertical icon dock: saved app icons + "+" button.
// Built via JS since saved apps are in localStorage. side is "left" or "right".
func renderSideLauncher(sid, side string) *r.Node {
	sideDockCounter++
	dockID := fmt.Sprintf("side-dock-%d", sideDockCounter)
	container := r.Div("shrink-0 flex items-center mx-0.5").Render(
		r.Div("flex flex-col items-center gap-1 py-1").ID(dockID),
	)
	container.JS(fmt.Sprintf(`
(function(){
	var dock=document.getElementById('%s');
	if(!dock)return;
	var dk=document.documentElement.classList.contains('dark');
	var apps=JSON.parse(localStorage.getItem('libro-apps')||'[]');
	var btnCls=dk
		?'w-8 h-8 flex items-center justify-center rounded-md cursor-pointer transition-colors duration-75 hover:bg-zinc-700 relative group/ico'
		:'w-8 h-8 flex items-center justify-center rounded-md cursor-pointer transition-colors duration-75 hover:bg-gray-200 relative group/ico';
	var tipCls=dk
		?'absolute left-full ml-2 px-2 py-1 text-xs rounded bg-zinc-800 text-zinc-200 border border-zinc-700 whitespace-nowrap opacity-0 group-hover/ico:opacity-100 pointer-events-none transition-opacity z-[200] shadow-lg'
		:'absolute left-full ml-2 px-2 py-1 text-xs rounded bg-white text-gray-800 border border-gray-200 whitespace-nowrap opacity-0 group-hover/ico:opacity-100 pointer-events-none transition-opacity z-[200] shadow-lg';

	function termIconHtml2(cmd){var ini=(cmd||'T').substring(0,2).toUpperCase();return '<span class="w-5 h-5 shrink-0 rounded-md bg-gradient-to-br from-teal-400 to-emerald-600 dark:from-teal-500 dark:to-emerald-700" style="display:inline-flex;align-items:center;justify-content:center;font-size:9px;font-weight:700;color:#fff;line-height:20px;letter-spacing:.04em;box-shadow:0 1px 3px rgba(20,184,166,.35),inset 0 1px 0 rgba(255,255,255,.15)">'+ini+'</span>';}
	apps.forEach(function(app){
		var btn=document.createElement('button');
		btn.className=btnCls;
		var lb='';
		if(app.type==='terminal'){
			btn.innerHTML=termIconHtml2(app.command);
			lb=app.name||app.command;
		}else{
			try{
				var u=new URL(app.url);
				btn.innerHTML='<img class="w-5 h-5 rounded-sm" src="https://www.google.com/s2/favicons?domain='+encodeURIComponent(u.hostname)+'&sz=32" onerror="this.outerHTML=\'<i class=\\\'material-icons-round text-gray-400 dark:text-zinc-500 text-lg\\\'>language</i>\'">';
				lb=app.name||u.hostname.replace(/^www\./,'');
			}catch(e){
				btn.innerHTML='<i class="material-icons-round text-gray-400 dark:text-zinc-500 text-lg">language</i>';
				lb=app.name||app.url;
			}
		}
		var tip=document.createElement('span');
		tip.className=tipCls;
		tip.textContent=lb+' ('+app.width+')';
		btn.appendChild(tip);
		btn.onclick=function(){
			__ws.call('app.start',{sid:'%s',type:app.type,url:app.url||'',command:app.command||'',width:app.width||'lg',writable:app.writable!==false,name:app.name||''});
		};
		dock.appendChild(btn);
	});

	var browseBtn=document.createElement('button');
	browseBtn.className=btnCls;
	browseBtn.innerHTML='<i class="material-icons-round '+(dk?'text-zinc-500 hover:text-indigo-400':'text-gray-400 hover:text-indigo-600')+' text-lg">language</i>';
	var browseTip=document.createElement('span');
	browseTip.className=tipCls;
	browseTip.textContent='Quick browse';
	browseBtn.appendChild(browseTip);
	browseBtn.onclick=function(){
		var input=prompt('Enter URL or search query:');
		if(!input||!input.trim())return;
		__ws.call('app.browse',{sid:'%s',query:input.trim()});
	};
	dock.appendChild(browseBtn);

	var addBtn=document.createElement('button');
	addBtn.className=btnCls;
	addBtn.innerHTML='<i class="material-icons-round '+(dk?'text-zinc-500 hover:text-teal-400':'text-gray-400 hover:text-teal-600')+' text-lg">add</i>';
	var addTip=document.createElement('span');
	addTip.className=tipCls;
	addTip.textContent='Add new';
	addBtn.appendChild(addTip);
	addBtn.onclick=function(){__ws.call('app.dialog.open',{sid:'%s',side:'%s'});};
	dock.appendChild(addBtn);
})();
`, dockID, sid, sid, sid, side))
	return container
}

// renderAddDialog renders the add application modal dialog
func renderAddDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	widthOptions := make([]*r.Node, 0)
	for _, w := range AllWidths() {
		radio := r.IRadio("accent-teal-500 cursor-pointer").
			Attr("name", "app-width").
			Attr("value", string(w)).
			ID(fmt.Sprintf("width-%s", w))
		if w == WidthLG {
			radio.Attr("checked", "checked")
		}
		widthOptions = append(widthOptions,
			r.Label("flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-300 dark:border-zinc-700 hover:border-gray-400 dark:hover:border-zinc-500 cursor-pointer transition-colors text-gray-700 dark:text-zinc-300 text-sm font-mono").Render(
				radio,
				r.Span("").Text(strings.ToUpper(string(w))),
			),
		)
	}

	tabSwitchJS := func(showTab string) string {
		return fmt.Sprintf(`
			document.getElementById('tab-url-content').classList.toggle('hidden', '%s' !== 'url');
			document.getElementById('tab-terminal-content').classList.toggle('hidden', '%s' !== 'terminal');
			document.getElementById('tab-url-btn').classList.toggle('border-teal-500', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-teal-600', '%s' === 'url');
			document.getElementById('tab-url-btn').classList.toggle('border-transparent', '%s' !== 'url');
			document.getElementById('tab-url-btn').classList.toggle('text-gray-500', '%s' !== 'url');
			document.getElementById('tab-terminal-btn').classList.toggle('border-teal-500', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-teal-600', '%s' === 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('border-transparent', '%s' !== 'terminal');
			document.getElementById('tab-terminal-btn').classList.toggle('text-gray-500', '%s' !== 'terminal');
			document.getElementById('app-type').value = '%s';
		`, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab, showTab)
	}

	collectIDs := []string{"app-url", "app-command", "app-writable", "app-type", "app-name", "width-md"}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-teal-500 focus:border-teal-500 outline-none transition-colors"

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75"+hiddenClass).
		ID(DialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", DialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-md mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("Add Application"),

					r.IHidden("").ID("app-type").Attr("value", "url"),

					r.Div("flex border-b border-gray-200 dark:border-zinc-700/50 mb-4").Render(
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-teal-500 text-teal-600 cursor-pointer transition-colors").
							ID("tab-url-btn").
							Text("URL").
							OnClick(r.JS(tabSwitchJS("url"))),
						r.Button("px-4 py-2 text-sm font-mono border-b-2 border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 cursor-pointer transition-colors").
							ID("tab-terminal-btn").
							Text("Terminal").
							OnClick(r.JS(tabSwitchJS("terminal"))),
					),

					r.Div("mb-5").ID("tab-url-content").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("URL"),
						r.IUrl(inputCls).
							ID("app-url").
							Attr("placeholder", "https://example.com").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
					),

					r.Div("mb-5 hidden").ID("tab-terminal-content").Render(
						r.Div("mb-3").Render(
							r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Command"),
							r.IText(inputCls+" font-mono").
								ID("app-command").
								Attr("placeholder", "bash").
								Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-add').click();}"),
						),
						r.Label("flex items-center gap-2 cursor-pointer").Render(
							r.ICheckbox("accent-teal-500 cursor-pointer w-4 h-4").
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

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "app.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-add").
							Text("Add").
							OnClick(&r.Action{
								Name:    "app.add",
								Data:    sidData(sid),
								Collect: collectIDs,
							}),
					),
				),
		)
}

// resizeJS returns JS that updates an app frame's width without replacing the DOM
func resizeJS(state *AppState, width Width, appID string) string {
	// Build a map of width value -> container classes
	widthMap := ""
	for _, w := range AllWidths() {
		if widthMap != "" {
			widthMap += ","
		}
		widthMap += fmt.Sprintf("'%s':'%s'", string(w), w.ContainerClasses())
	}

	return fmt.Sprintf(`
(function(){
	var strip = document.getElementById('%s');
	if (!strip) return;
	var el = document.querySelector('[data-app-id="%s"]');
	if (!el) return;

	var widths = {%s};
	var newWidth = '%s';
	var newCls = widths[newWidth];

	// Remove old width classes and apply new ones
	var keep = [];
	var cls = el.className.split(/\s+/);
	var allWidthCls = {};
	for (var k in widths) {
		widths[k].split(/\s+/).forEach(function(c){ allWidthCls[c] = true; });
	}
	cls.forEach(function(c){
		if (!allWidthCls[c]) keep.push(c);
	});
	newCls.split(/\s+/).forEach(function(c){ keep.push(c); });
	el.className = keep.join(' ');

	// Update size badges: highlight active, dim others
	var badges = el.querySelectorAll('[data-click-overlay] ~ div button, .group > div:first-child button, div.absolute button');
	// Find the top buttons bar
	var topBar = el.querySelector('.absolute.top-1\\.5.right-1\\.5');
	if (topBar) {
		var btns = topBar.querySelectorAll('button');
		var sizeLabels = ['MD','LG','XL','2XL','FULL'];
		var activeBase = 'px-1.5 py-0.5 text-[10px] font-mono tracking-wider uppercase rounded-sm cursor-pointer transition-colors duration-75';
		btns.forEach(function(b){
			var txt = b.textContent.trim();
			if (sizeLabels.indexOf(txt) === -1) return;
			if (txt === newWidth.toUpperCase()) {
				b.className = activeBase + ' bg-teal-600 text-white';
			} else {
				b.className = activeBase + ' text-gray-400 dark:text-zinc-500 hover:text-gray-700 dark:hover:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-700';
			}
		});
	}

	// Re-center strip
	var container = strip.parentElement;
	var containerWidth = container.offsetWidth;
	var totalApps = %d;
	requestAnimationFrame(function(){
		if (totalApps === 1) {
			var selectedLeft = el.offsetLeft;
			var selectedWidth = el.offsetWidth;
			var off = selectedLeft - (containerWidth / 2) + (selectedWidth / 2);
			strip.style.transform = 'translateX(' + (-off) + 'px)';
		} else {
			var stripWidth = strip.scrollWidth;
			if (stripWidth <= containerWidth) {
				strip.style.transform = 'translateX(0px)';
			} else {
				var selectedLeft = el.offsetLeft;
				var selectedWidth = el.offsetWidth;
				var off = selectedLeft - (containerWidth / 2) + (selectedWidth / 2);
				strip.style.transform = 'translateX(' + (-off) + 'px)';
			}
		}
	});
})();
`, StripID, appID, widthMap, string(width), len(state.Apps))
}

// renderProjectBar renders the horizontal project switcher bar
func renderProjectBar(state *AppState, sid string) *r.Node {
	buttons := make([]*r.Node, 0, len(state.Projects)+1)

	for _, proj := range state.Projects {
		cls := "px-3 py-1.5 text-xs font-mono rounded-md cursor-pointer transition-colors duration-75 "
		if proj.Name == state.ActiveProject {
			cls += "bg-teal-600 text-white"
		} else {
			cls += "bg-gray-200 dark:bg-zinc-800 text-gray-600 dark:text-zinc-400 hover:bg-gray-300 dark:hover:bg-zinc-700"
		}
		buttons = append(buttons,
			r.Button(cls).
				Text(proj.Name).
				Attr("title", proj.Path).
				OnClick(r.JS(fmt.Sprintf(
					"history.replaceState(null,'','#%s');__ws.call('project.switch',{sid:'%s',name:'%s'});",
					proj.Name, sid, proj.Name,
				))),
		)
	}

	// Add project button
	buttons = append(buttons,
		r.Button("px-2 py-1.5 text-xs font-mono rounded-md cursor-pointer text-gray-400 dark:text-zinc-500 hover:text-teal-600 dark:hover:text-teal-400 hover:bg-gray-200 dark:hover:bg-zinc-800 transition-colors duration-75").
			Text("+").
			OnClick(&r.Action{
				Name: "project.dialog.open",
				Data: sidData(sid),
			}),
	)

	// Show active project path
	activePath := ""
	for _, p := range state.Projects {
		if p.Name == state.ActiveProject {
			activePath = p.Path
			break
		}
	}

	return r.Div("flex items-center gap-1.5 px-3 py-2 border-b border-gray-200 dark:border-zinc-800 shrink-0").
		ID(ProjectBarID).
		Render(
			r.Div("flex items-center gap-1.5").Render(buttons...),
			r.Span("ml-3 text-[11px] font-mono text-gray-400 dark:text-zinc-600 truncate").Text(activePath),
			r.Div("ml-auto").Render(r.ThemeSwitcher()),
		)
}

// renderProjectDialog renders the create project modal
func renderProjectDialog(visible bool, sid string) *r.Node {
	hiddenClass := " hidden"
	if visible {
		hiddenClass = ""
	}

	inputCls := "w-full px-3 py-2 bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-700 rounded-md text-gray-800 dark:text-zinc-200 text-sm placeholder-gray-400 dark:placeholder-zinc-500 focus:ring-1 focus:ring-teal-500 focus:border-teal-500 outline-none transition-colors"

	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "/"
	}

	return r.Div("fixed inset-0 z-50 flex items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-sm transition-opacity duration-75"+hiddenClass).
		ID(ProjectDialogID).
		OnClick(r.JS(fmt.Sprintf("document.getElementById('%s').classList.add('hidden')", ProjectDialogID))).
		Render(
			r.Div("bg-white dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700/50 rounded-lg shadow-2xl p-5 w-full max-w-lg mx-4").
				OnClick(r.JS("event.stopPropagation()")).
				Render(
					r.H2("text-lg font-mono font-bold text-gray-900 dark:text-zinc-100 mb-4 tracking-tight").Text("New Project"),

					// Directory picker
					r.Div("mb-4").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Select Folder"),
						renderDirBrowser(homeDir, sid),
					),

					// Hidden input for selected path
					r.IHidden("").ID("project-path").Attr("value", ""),

					// Name field (auto-populated, editable)
					r.Div("mb-5").Render(
						r.Label("block text-xs font-mono text-gray-500 dark:text-zinc-500 uppercase tracking-wider mb-1.5").Text("Name"),
						r.IText(inputCls).
							ID("project-name").
							Attr("placeholder", "select a folder above").
							Attr("oninput", "this.dataset.auto='false'").
							Attr("onkeydown", "if(event.key==='Enter'){event.preventDefault();document.getElementById('btn-create-project').click();}"),
					),

					r.Div("flex justify-end gap-2 pt-2 border-t border-gray-100 dark:border-zinc-800").Render(
						r.Button("px-4 py-2 text-gray-500 hover:text-gray-700 dark:hover:text-zinc-300 font-mono text-sm rounded-md hover:bg-gray-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer").
							Text("Cancel").
							OnClick(&r.Action{Name: "project.dialog.close", Data: sidData(sid)}),
						r.Button("px-5 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-sm font-medium rounded-md transition-colors cursor-pointer").
							ID("btn-create-project").
							Text("Create").
							OnClick(&r.Action{
								Name:    "project.create",
								Data:    sidData(sid),
								Collect: []string{"project-name", "project-path"},
							}),
					),
				),
		)
}

// renderDirBrowser renders the directory browser component
func renderDirBrowser(currentPath string, sid string) *r.Node {
	dirCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-gray-700 dark:text-zinc-300 hover:bg-teal-50 dark:hover:bg-teal-900/20 rounded cursor-pointer transition-colors"
	selectedCls := "flex items-center gap-2 px-3 py-1.5 text-sm font-mono text-teal-700 dark:text-teal-300 bg-teal-50 dark:bg-teal-900/30 rounded font-medium"

	// Read directories
	entries, err := os.ReadDir(currentPath)
	var dirs []string
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
	}

	// Current path display + select button
	pathBar := r.Div("flex items-center gap-2 mb-2").Render(
		r.Div("flex-1 px-3 py-2 bg-gray-50 dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-md text-xs font-mono text-gray-600 dark:text-zinc-400 truncate").
			Attr("title", currentPath).
			Text(currentPath),
		r.Button("shrink-0 px-3 py-2 bg-teal-600 hover:bg-teal-500 text-white font-mono text-xs font-medium rounded-md transition-colors cursor-pointer").
			Text("Select").
			OnClick(r.JS(fmt.Sprintf(`
				document.getElementById('project-path').value='%s';
				var nameEl=document.getElementById('project-name');
				if(!nameEl.value||nameEl.dataset.auto==='true'){
					nameEl.value='%s';
					nameEl.dataset.auto='true';
				}
				nameEl.dispatchEvent(new Event('input'));
			`, currentPath, filepath.Base(currentPath)))),
	)

	// Parent directory entry
	var items []*r.Node
	parentPath := filepath.Dir(currentPath)
	if parentPath != currentPath {
		items = append(items, r.Div(dirCls).Render(
			r.I("material-icons-round text-gray-400 dark:text-zinc-500 text-base").Text("arrow_upward"),
			r.Span("").Text(".."),
		).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": parentPath},
		}))
	}

	// Directory entries
	for _, d := range dirs {
		fullPath := filepath.Join(currentPath, d)
		items = append(items, r.Div(dirCls).Render(
			r.I("material-icons-round text-amber-500 dark:text-amber-400 text-base").Text("folder"),
			r.Span("truncate").Text(d),
		).OnClick(&r.Action{
			Name: "project.browse",
			Data: map[string]any{"sid": sid, "path": fullPath},
		}))
	}

	// Highlight current path as selected
	_ = selectedCls

	dirList := r.Div("max-h-48 overflow-y-auto space-y-0.5 border border-gray-200 dark:border-zinc-700 rounded-md p-1.5 bg-gray-50/50 dark:bg-zinc-800/50")
	if len(items) == 0 {
		dirList.Render(
			r.Div("px-3 py-2 text-xs font-mono text-gray-400 dark:text-zinc-600 italic").Text("No subdirectories"),
		)
	} else {
		dirList.Render(items...)
	}

	return r.Div("").ID(DirBrowserID).Render(pathBar, dirList)
}

// updateHashJS returns JS that updates the URL hash to the given project name
func updateHashJS(name string) string {
	return fmt.Sprintf("history.replaceState(null,'','#%s');", name)
}

// saveProjectToLocalStorageJS saves a project definition to localStorage
func saveProjectToLocalStorageJS(name, path string) string {
	return fmt.Sprintf(`
(function(){
	var projects=JSON.parse(localStorage.getItem('libro-projects')||'[]');
	var exists=projects.some(function(p){return p.name==='%s';});
	if(!exists){projects.push({name:'%s',path:'%s'});}
	localStorage.setItem('libro-projects',JSON.stringify(projects));
})();
`, name, name, path)
}

// loadProjectsJS returns JS that reads saved projects from localStorage and initializes them
func loadProjectsJS(sid string) string {
	return fmt.Sprintf(`
(function _loadProjects(){
	if(typeof __ws==='undefined'||!__ws.call){setTimeout(_loadProjects,50);return;}
	var projects=JSON.parse(localStorage.getItem('libro-projects')||'[]');
	if(projects.length>0){
		__ws.call('project.init',{sid:'%s',projects:projects});
	}
	var hash=location.hash.replace('#','');
	if(hash&&hash!=='home'){
		setTimeout(function(){__ws.call('project.switch',{sid:'%s',name:hash});},100);
	}
	if(!location.hash){history.replaceState(null,'','#home');}
})();
`, sid, sid)
}

// quickBrowseJS returns inline JS that prompts for a URL and calls app.browse
func quickBrowseJS(sid string) string {
	return fmt.Sprintf(`
(function(){
	var input=prompt('Enter URL or search query:');
	if(!input||!input.trim())return;
	__ws.call('app.browse',{sid:'%s',query:input.trim()});
})();`, sid)
}

func keyboardShortcutsJS(sid string) string {
	return fmt.Sprintf(`
		(function() {
			if (window.__libroKbRegistered) return;
			window.__libroKbRegistered = true;

			function libroKeyHandler(e) {
				if (e.metaKey && (e.key === 'h' || e.key === 'H')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.navigate.left', {"sid": "%s"});
				}
				if (e.metaKey && (e.key === 'l' || e.key === 'L')) {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.navigate.right', {"sid": "%s"});
				}
				if (e.ctrlKey && e.key >= '1' && e.key <= '9') {
					e.preventDefault();
					e.stopImmediatePropagation();
					__ws.call('app.select', {"sid": "%s", "index": parseInt(e.key) - 1});
				}
			}

			document.addEventListener('keydown', libroKeyHandler, true);

			function attachIframeListeners() {
				var iframes = document.querySelectorAll('iframe');
				for (var i = 0; i < iframes.length; i++) {
					if (iframes[i].__libroKbAttached) continue;
					iframes[i].__libroKbAttached = true;
					(function(iframe) {
						function attach() {
							try {
								var doc = iframe.contentDocument || iframe.contentWindow.document;
								if (!doc.__libroKbAttached) {
									doc.__libroKbAttached = true;
									doc.addEventListener('keydown', libroKeyHandler, true);
								}
							} catch(err) {}
						}
						iframe.addEventListener('load', attach);
						attach();
					})(iframes[i]);
				}
			}

			attachIframeListeners();
			window.__libroAttachIframeListeners = attachIframeListeners;

			var obs = new MutationObserver(function() { attachIframeListeners(); });
			obs.observe(document.body, {childList: true, subtree: true});
		})();
	`, sid, sid, sid)
}
