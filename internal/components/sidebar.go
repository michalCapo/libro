// Package components holds presentational UI building blocks that can be
// rendered from pre-computed input — kept in a sub-package so they can be
// developed without depending on libro's mutable session/app state.
package components

import (
	"encoding/json"
	"fmt"

	r "github.com/michalCapo/g-sui/ui"
)

// SidebarInput is everything RenderProjectSidebar needs to draw the sidebar.
// It is built by libro from AppState before calling.
type SidebarInput struct {
	Sid        string
	SidebarID  string
	Collapsed  bool
	ActivePath string
	Projects   []SidebarProject
}

// SidebarProject is one top-level project row.
type SidebarProject struct {
	Name             string
	Path             string
	IsGitRepo        bool
	HasWorktreesUI   bool // show the chevron toggle
	TreeOpen         bool
	IsActive         bool
	IsParentOfActive bool
	Removable        bool // false for "home"
	AppCount         int  // shown only when !IsGitRepo
	Worktrees        []SidebarWorktree
}

// SidebarWorktree is one worktree sub-row under a git project.
type SidebarWorktree struct {
	Branch   string
	Path     string
	IsMain   bool
	IsActive bool
	AppCount int
	NavSlot  int    // 0 = no shortcut assigned
	SlotName string // canonical project name to bind shortcut to
	OnClick  string // pre-built JS for the row body click
}

// Sidebar renders the left sidebar from pre-computed input.
func Sidebar(in SidebarInput) *r.Node {
	if in.Collapsed {
		return r.Div("w-0 shrink-0 overflow-hidden").ID(in.SidebarID)
	}

	items := make([]*r.Node, 0, len(in.Projects)+1)
	for _, proj := range in.Projects {
		items = append(items, renderProjectRow(in.Sid, proj))
	}

	items = append(items,
		r.Button("w-full flex items-center gap-2 px-3 py-2 text-sm font-mono text-gray-400 dark:text-zinc-600 hover:text-blue-600 dark:hover:text-blue-400 hover:bg-gray-100 dark:hover:bg-zinc-800 rounded-md cursor-pointer transition-colors duration-75 mt-1").
			OnClick(&r.Action{Name: "project.dialog.open", Data: map[string]any{"sid": in.Sid}}).
			Render(
				r.I("material-icons-round text-base shrink-0").Text("add"),
				r.Span("").Text("New Project"),
			),
	)

	return r.Div("w-52 shrink-0 flex flex-col border-r border-gray-200 dark:border-zinc-800 bg-gray-50 dark:bg-zinc-900/50 overflow-hidden").
		ID(in.SidebarID).
		Render(
			r.Div("flex-1 overflow-y-auto p-2 flex flex-col gap-0.5").Render(items...),
			r.Div("px-3 py-2 border-t border-gray-200 dark:border-zinc-800").Render(
				r.Span("text-[10px] font-mono text-gray-400 dark:text-zinc-600 truncate block").
					Attr("title", in.ActivePath).
					Text(in.ActivePath),
			),
		)
}

func renderProjectRow(sid string, proj SidebarProject) *r.Node {
	highlight := proj.IsActive || proj.IsParentOfActive

	projCls := "w-full flex items-center gap-2 px-3 py-2 text-sm font-mono rounded-md cursor-pointer transition-colors duration-75 "
	if highlight {
		projCls += "bg-blue-600 text-white"
	} else {
		projCls += "text-gray-700 dark:text-zinc-300 hover:bg-gray-200 dark:hover:bg-zinc-800"
	}

	iconName := "folder"
	if proj.IsGitRepo {
		iconName = "source"
	}

	projLeadingCls := "relative flex items-center justify-center w-5 h-5 shrink-0"
	projLeadingChildren := []*r.Node{
		r.I("material-icons-round text-base shrink-0").Text(iconName),
	}
	if proj.Removable {
		removeCls := "absolute inset-0 flex items-center justify-center rounded cursor-pointer opacity-0 group-hover/projitem:opacity-100 transition-opacity duration-75 "
		if highlight {
			removeCls += "text-blue-200 hover:text-white hover:bg-white/15"
		} else {
			removeCls += "text-red-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-400/10"
		}
		projLeadingChildren[0] = r.I("material-icons-round text-base shrink-0 group-hover/projitem:opacity-0 transition-opacity duration-75").Text(iconName)
		projLeadingChildren = append(projLeadingChildren,
			r.Button(removeCls).
				Attr("title", "Remove project").
				Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('project.remove',{sid:'%s',name:'%s'});", sid, proj.Name)).
				Render(r.I("material-icons-round text-[14px]").Text("close")),
		)
	}

	btnChildren := []*r.Node{
		r.Span(projLeadingCls).Render(projLeadingChildren...),
		r.Span("truncate flex-1 text-left").Text(proj.Name),
	}
	if !proj.IsGitRepo {
		if badge := renderAppCountBadge(proj.AppCount, highlight); badge != nil {
			btnChildren = append(btnChildren, badge)
		}
	}

	projBtn := r.Button("flex-1 min-w-0 flex items-center gap-2 text-left").
		Attr("title", proj.Path).
		Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('project.header.click',{sid:'%s',project:'%s'});", sid, proj.Name)).
		Render(btnChildren...)

	projControls := []*r.Node{}
	if proj.HasWorktreesUI {
		toggleCls := "inline-flex items-center justify-center w-4 h-4 rounded shrink-0 cursor-pointer transition-colors duration-75 "
		if highlight {
			toggleCls += "text-blue-200 hover:bg-white/15 hover:text-white"
		} else {
			toggleCls += "text-gray-400 dark:text-zinc-600 hover:bg-gray-200 dark:hover:bg-zinc-800 hover:text-gray-700 dark:hover:text-zinc-300"
		}
		toggleIcon := "chevron_right"
		if proj.TreeOpen {
			toggleIcon = "expand_more"
		}
		projControls = append(projControls,
			r.Button(toggleCls).
				Attr("title", "Toggle worktrees").
				Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('project.worktrees.toggle',{sid:'%s',project:'%s'});", sid, proj.Name)).
				Render(r.I("material-icons-round text-[14px]").Text(toggleIcon)),
		)
	}

	projItem := r.Div("group/projitem").Render(
		r.Div(projCls).Render(
			projBtn,
			r.Div("ml-2 flex items-center gap-1 shrink-0").Render(projControls...),
		),
	)

	if proj.TreeOpen && len(proj.Worktrees) > 0 {
		wtItems := make([]*r.Node, 0, len(proj.Worktrees))
		for _, wt := range proj.Worktrees {
			wtItems = append(wtItems, renderWorktreeRow(sid, proj.Name, wt))
		}
		projItem.Render(r.Div("mt-0.5").Render(wtItems...))
	}

	return projItem
}

func renderWorktreeRow(sid, projectName string, wt SidebarWorktree) *r.Node {
	wtCls := "w-full flex items-center gap-2 pr-3 py-1.5 text-xs font-mono rounded-md cursor-pointer transition-colors duration-75 "
	if wt.IsActive {
		wtCls += "bg-blue-500/20 text-blue-700 dark:text-blue-300"
	} else {
		wtCls += "text-gray-500 dark:text-zinc-500 hover:bg-gray-100 dark:hover:bg-zinc-800 hover:text-gray-700 dark:hover:text-zinc-300"
	}

	var wtBadge *r.Node
	if wt.NavSlot > 0 {
		cls := "inline-flex items-center justify-center w-4 min-w-4 h-4 rounded text-[10px] font-bold leading-none shrink-0 cursor-pointer "
		if wt.IsActive {
			cls += "bg-blue-500 text-blue-100 hover:bg-red-500"
		} else {
			cls += "bg-gray-300 dark:bg-zinc-700 text-gray-500 dark:text-zinc-500 hover:bg-red-400 hover:text-white"
		}
		wtBadge = r.Span(cls).
			Attr("title", fmt.Sprintf("Remove shortcut Ctrl+%d", wt.NavSlot)).
			Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('nav.slot.remove',{sid:'%s',name:'%s'});", sid, wt.SlotName)).
			Text(fmt.Sprintf("%d", wt.NavSlot))
	} else {
		cls := "inline-flex items-center justify-center w-4 min-w-4 h-4 rounded text-[10px] font-bold leading-none shrink-0 cursor-pointer opacity-0 group-hover/wtitem:opacity-100 transition-opacity duration-75 bg-gray-200 dark:bg-zinc-800 text-gray-400 dark:text-zinc-600 hover:bg-blue-400 hover:text-white"
		wtBadge = r.Span(cls).
			Attr("title", "Add shortcut").
			Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('nav.slot.add',{sid:'%s',name:'%s'});", sid, wt.SlotName)).
			Text("+")
	}

	wtAddCls := "flex items-center justify-center w-4 h-4 rounded cursor-pointer opacity-0 group-hover/wtitem:opacity-100 transition-opacity duration-75 shrink-0 "
	if wt.IsActive {
		wtAddCls += "text-blue-300 hover:text-white hover:bg-white/15"
	} else {
		wtAddCls += "text-gray-400 dark:text-zinc-600 hover:text-blue-500 dark:hover:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-500/10"
	}
	wtAdd := r.Button(wtAddCls).
		Attr("title", "Add worktree").
		Attr("onclick", fmt.Sprintf("event.stopPropagation();if(window.__libroOpenWorktreeCreatePopup)window.__libroOpenWorktreeCreatePopup({project:%s,path:%s,branch:%s});", jsString(projectName), jsString(wt.Path), jsString(wt.Branch))).
		Render(r.I("material-icons-round text-[12px]").Text("add"))

	wtLeadingCls := "relative flex items-center justify-center w-4 h-4 shrink-0"
	wtLeadingChildren := []*r.Node{
		r.I("material-icons-round text-sm shrink-0").Text("alt_route"),
	}
	if !wt.IsMain {
		removeCls := "absolute inset-0 flex items-center justify-center rounded cursor-pointer opacity-0 group-hover/wtitem:opacity-100 transition-opacity duration-75 "
		if wt.IsActive {
			removeCls += "text-blue-300 hover:text-white hover:bg-white/15"
		} else {
			removeCls += "text-red-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-400/10"
		}
		wtLeadingChildren[0] = r.I("material-icons-round text-sm shrink-0 group-hover/wtitem:opacity-0 transition-opacity duration-75").Text("alt_route")
		wtLeadingChildren = append(wtLeadingChildren,
			r.Button(removeCls).
				Attr("title", "Remove worktree").
				Attr("onclick", fmt.Sprintf("event.stopPropagation();__ws.call('worktree.remove',{sid:'%s',project:'%s',path:'%s'});", sid, projectName, wt.Path)).
				Render(r.I("material-icons-round text-[12px]").Text("close")),
		)
	}

	wtBtnChildren := []*r.Node{
		r.Span(wtLeadingCls).Render(wtLeadingChildren...),
		r.Span("truncate flex-1 text-left").Text(wt.Branch),
	}
	if badge := renderAppCountBadge(wt.AppCount, wt.IsActive); badge != nil {
		wtBtnChildren = append(wtBtnChildren, badge)
	}

	wtBtn := r.Button("flex-1 min-w-0 flex items-center gap-2 text-left").
		Attr("title", wt.Path).
		OnClick(r.JS(wt.OnClick)).
		Render(wtBtnChildren...)

	return r.Div("group/wtitem").Render(
		r.Div(wtCls).Render(
			wtBadge,
			wtBtn,
			r.Div("ml-2 flex items-center gap-1 shrink-0").Render(wtAdd),
		),
	)
}

func renderAppCountBadge(count int, active bool) *r.Node {
	if count == 0 {
		return nil
	}
	cls := "inline-flex items-center justify-center min-w-[16px] h-4 rounded-full text-[10px] font-bold leading-none shrink-0 px-1 "
	if active {
		cls += "bg-white text-blue-600"
	} else {
		cls += "bg-blue-100 dark:bg-blue-900/40 text-blue-600 dark:text-blue-400"
	}
	return r.Span(cls).Text(fmt.Sprintf("%d", count))
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
