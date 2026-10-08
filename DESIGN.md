---
name: Libro
description: Agent workspace with strong keyboard support, terminal and web applications, and fixed-width tool panels.
colors:
  bg: "#fcfcfd"
  chrome: "#f7f7f8"
  raised: "#fff"
  line: "#e9e9ec"
  control-line: "#d4d4db"
  fg: "#29292e"
  muted: "#70707b"
  hover: "#efeff2"
  accent: "#2452df"
  success: "#15803d"
  popup: "#f7f7f8"
  popup-row: "#fff"
  popup-muted: "#70707b"
  code-comment: "#6c7580"
  code-keyword: "#cf222e"
  code-string: "#0a3069"
  code-title: "#8250df"
  code-constant: "#0550ae"
  code-property: "#116329"
  code-meta: "#953800"
  code-deletion: "#82071e"
  dark-bg: "#1c1c1f"
  dark-chrome: "#222225"
  dark-raised: "#2b2b30"
  dark-line: "#333339"
  dark-control-line: "#484850"
  dark-fg: "#e6e6eb"
  dark-muted: "#aaaab5"
  dark-hover: "#303036"
  dark-accent: "#8aa7ff"
  dark-success: "#4ade80"
  dark-popup: "#252529"
  dark-popup-row: "#34343b"
  dark-popup-muted: "#aaaab5"
  dark-code-comment: "#8b949e"
  dark-code-keyword: "#ff7b72"
  dark-code-string: "#a5d6ff"
  dark-code-title: "#d2a8ff"
  dark-code-constant: "#79c0ff"
  dark-code-property: "#7ee787"
  dark-code-meta: "#ffa657"
  dark-code-deletion: "#ffa198"
typography:
  body:
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif'
    fontSize: "14px"
    lineHeight: 1.5
  title:
    fontSize: "15px"
  empty-heading:
    fontSize: "28px"
    fontWeight: 500
    letterSpacing: "-0.025em"
  palette-label:
    fontSize: "15px"
    lineHeight: "21px"
  palette-description:
    fontSize: "12px"
    lineHeight: "17px"
  keycap:
    fontFamily: "ui-monospace, monospace"
    fontSize: "11px"
    lineHeight: "18px"
rounded:
  keycap: "4px"
  badge: "6px"
  input: "7px"
  control: "8px"
  action: "9px"
  row: "10px"
  popup: "12px"
  shortcut-group: "14px"
spacing:
  compact: "6px"
  small: "8px"
  inline: "10px"
  regular: "12px"
  large: "24px"
components:
  launch-button:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.fg}"
    rounded: "{rounded.action}"
    padding: "7px 14px"
  icon-button:
    textColor: "{colors.muted}"
    rounded: "{rounded.control}"
    width: "30px"
    height: "30px"
  field:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.fg}"
    rounded: "{rounded.input}"
    padding: "10px 12px"
  selected-project:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.fg}"
    rounded: "{rounded.control}"
    padding: "6px 10px"
  size-badge:
    textColor: "{colors.muted}"
    rounded: "{rounded.badge}"
    padding: "1px 5px"
  note-row:
    textColor: "{colors.fg}"
    padding: "12px 4px"
  palette-row:
    textColor: "{colors.fg}"
    rounded: "{rounded.row}"
    padding: "10px 12px"
---

# Design System: Libro

## Overview

**Creative North Star: "Agent workspace built around terminal and web applications"**

Libro brings multiple agents into one project with strong keyboard support and fixed-width tool panels. It is comparable to T3 Code or the OpenAI and Anthropic desktop apps, but built with a different approach centered on terminal and web applications.

The desktop shell uses near-white content, a softly gray sidebar, subtle dividers, rounded white selected rows, muted icons, compact bordered controls, and restrained blue accents. The dark equivalent keeps the same hierarchy and geometry. The agent workspace is the main content.

**Key Characteristics:**
- Quiet system typography and compact desktop controls.
- Strong keyboard support for navigating projects, agents, and tools.
- Fixed-width app panels separated by fine rules.
- Rounded selection surfaces and softly lifted popups.

Scope: the built workspace shell and its dialogs. Source of truth is `internal/workspace.css`, supported by `workspace.go`, `workspace.js`, `components.go`, and the popup components. Embedded terminals and browser pages retain their own content styling. `PRODUCT.md` fixes the brand and docked workspace structure. This is a record of the implementation, not a new visual direction.

## Colors

### Primary

`accent` supplies blue focus outlines, selected panel underlines, and browser loading indicators. It is a functional accent, not a large decorative surface.

### Neutral

`bg` is the near-white main surface; `chrome` is the soft gray navigation surface; `raised` is the white selected row and control surface. `fg` carries primary text, `muted` secondary text and icons. `line` separates surfaces; `control-line` outlines compact controls; `hover` gives restrained pointer feedback. `popup`, `popup-row`, and `popup-muted` apply the same hierarchy to overlays. Each `dark-` counterpart is applied under `html.dark`.

The muted text token was checked against the sidebar surface at 4.57:1. This does not claim a complete accessibility audit of embedded content. Terminal backgrounds and colors follow the shell theme, including live changes. Settings provides Auto (the default), Light, and Dark. Auto follows the OS color scheme; explicit choices override it and persist through the UI library’s theme storage.

The `code-*` tokens give file previews a restrained syntax palette. Their `dark-code-*` counterparts preserve token roles instead of mechanically inverting the light colors.

## Typography

Use the system sans-serif stack in `body`; there is no separate display font. Body text is compact and regular. Project headings and palette labels use the title scale; toolbar titles use 13px, sidebar labels and result descriptions 12px. Brand text is 16px at weight 550. Empty-state headings use `empty-heading`; supporting copy has a 46ch maximum width and 1.65 line height. Status and size controls use 10px text. Keycaps use the monospace role. Material Icons Round supplies mostly 18–19px muted icons.

## Layout

Project threads contain one center agent panel each. New thread creates a thread directly without opening the agent picker; its configurable shortcut is empty by default. Ctrl+Shift+A opens the agent picker and starts the chosen agent in a new thread. Replace agent starts a fresh session in the current thread and keeps its tools open. Notes and bottom terminal processes are shared across the project; browser and other tools belong to each thread. Project settings choose a shared application or one application per thread, with automatic ports and optional per-thread overrides. Standalone threads keep their existing behavior.

The shell has a 56px titlebar, a 26px statusbar, and left project navigation. The sidebar is 216px wide. At 760px or less it overlays the content between the titlebar and statusbar, with width `min(216px, calc(100vw - 48px))`. It defaults closed on small screens when no preference exists; selecting a project closes it. The brand and trailing status text hide, and empty-state headings become 24px.

The main area uses center, right, and bottom docks with no outer card padding. Each thread has at most one center agent panel. Project-backed threads reuse the same Notes panel and bottom terminals; those panels move with the selected thread without restarting. Shared applications also move with the selected thread; per-thread application panels stay in their own workspaces. The right dock keeps multiple tools alive but shows one selected tool at a time. The agent panel has no size of its own: it fills the space the tools leave, with a 600px minimum. If the selected right tool and a 600px agent do not fit together, the tool overlays the right side. A panel size wider than the visible area shrinks to fit it, so the main row never scrolls sideways. The bottom dock can show the assigned application command below the main row. A shared tab row selects the running agent and tools. All tool panels in a thread share one width, so the row ends with one size picker for them.

Tool width presets are XS 320px, SM 480px, MD 640px, LG 960px, XL 1280px, and 2XL 1920px. These are panel widths, not viewport breakpoints. Legacy 3XL 2560px and MAX (full strip width, formerly called FULL) remain available as steps beyond 2XL; the existing screen-width policy disables 3XL on Full HD or smaller screens. MAX uses available workspace width. Maximizing temporarily fills the workspace and hides other panels.

Toolbars are at least 48px tall and allow controls to wrap. Popups are centered with viewport height limits and internal scrolling. Commands and the app launcher are at most 420px wide with a 24px viewport gutter; other existing dialogs retain their component-specific widths. At small widths, preserve the dock and tab model and overlay the selected right tool instead of stacking panels.

Notes uses a single scrolling column with 16px padding at every panel width. Its toolbar and action row wrap with 8px gaps. The editor stacks fields with 16px gaps; image previews stay within the panel width and a 360px maximum height. Preserve this layout at XS (320px) as well as MD (640px).

## Elevation & Depth

The workspace uses tonal differences and 1px dividers. Panels are flat and square. Popups alone use the shared shadow `0 12px 36px rgb(0 0 0 / 14%)` with a 10% black backdrop and no blur. The mobile sidebar uses a lateral shadow. Bordered launch controls have only a faint shadow where implemented. Exact shadow values and component samples are in `.impeccable/design.json`.

Animations on app panels are disabled. Reduced-motion preferences remove shell animation, transitions, and smooth scrolling.

## Shapes

Use the frontmatter radii by role: small keycaps and size badges, gently rounded controls, rounded selected rows, and larger popup and shortcut-group corners. Preserve square app panels. Borders are single-pixel neutral rules; selection does not require a thick outline.

## Components

- **Navigation:** the sidebar starts with one compact utility row: search grows to fill the space, followed by icon buttons for switching projects, adding a project, and starting a new agent session. Section labels (Projects, Threads) are 12px semibold muted text. Project rows are 32px high with 13px text; the active project uses primary text at weight 600. Threads and worktrees use the full sidebar width beneath their project, without indentation or a tree guide line. Two-line rows keep the title and metadata close together on one subtly bordered surface, including when unselected. The selected thread uses the raised surface, a fine ring, and primary text. Row actions (new thread, settings, remove, overflow) appear only on hover or keyboard focus. Running agents appear below their project as direct session links. Project rows keep their folder icon; thread rows show agent working and done states. Projects without open threads fade to 52% opacity until hovered, focused, or current. A terminal glyph on the project row marks a running bottom command shared by its threads. Standalone threads show their own terminal activity. Shortcut numbers trail all row indicators and actions. Actions remain visible on touch devices.
- **Tools:** the right tool rail is 48px wide with 32px quiet icon buttons. Thin rules split the projects toggle, the tool list, and the add and bottom-terminal actions. The visible right tool uses the raised surface, a fine ring, and primary text. The tab row above the docks is 40px high and matches sidebar rows: tabs are quiet 13px muted text with a hover surface. The selected tab uses the raised surface, a fine ring, and primary text at weight 600. The agent tab starts with the voice button and has no size control. After the tabs, one size group shows the shared tool width as a small monospace keycap.
- **Actions:** launch and new-session buttons are compact bordered raised controls, at least 36px high. Icon buttons are quiet, square controls. Hover uses the neutral hover surface; focus uses the blue outline.
- **Fields:** ordinary popup fields use raised fill, a fine border, and an accent caret. Search fields sit inside a rounded light search surface with an icon and dismiss action. Their inner border and outline are removed; the shared search wrapper shows focus-within feedback.
- **Size controls:** only the selected size appears in the panel toolbar, with raised fill and a fine ring. Hover or click opens a floating size picker with every preset, including the current size. Keyboard users open it with Enter, Space, or Arrow Down; Escape dismisses it. The command resize picker uses rounded rows and radio indicators.
- **Palettes:** commands, apps, and project results use compact rounded rows with muted icons and a 14px primary label. Command and app rows are one line, at least 36px high; the description or scope trails the label in smaller muted text and truncates. Hover uses the hover surface; the selected row uses the popup-row surface with a fine ring. Keyboard hints stay muted; searchable lists show a no-results state.
- **Dialogs:** form labels are 12px semibold muted text that sit close to their field. The submit action uses the accent fill; a destructive submit, such as discarding a thread, uses red.
- **Shortcuts:** search above grouped rows; 50px minimum row height, fine separators, right-aligned outlined keycaps. Long labels wrap; keycaps stay on one line.

### Notes

Notes extends the quiet desktop controls inside a project tool panel. Open it to a list filtered to New, with New, Archived, and All options and an Add note action. Rows are 44px rounded cards with a 2px gap, a wrapping title, and muted state text; hover uses the hover fill. Show an icon and a plain message when the filter or search has no notes. Search matches title and body.

The note list header includes the project name. Lists stay bound to their project and refresh when switching projects.

Selecting a row expands its inline editor in an accordion, with one note open at a time. The open card uses the raised fill with a fine ring and hides its row title, so the title is not shown twice. Search and filters stay visible. Notes have no title field: the first text line of the body is the title, styled at 17px semibold in the editor. Older notes with a separate title get it as the first heading when opened. The Markdown editor is borderless, with a quiet icon toolbar, and keeps pasted images inline with the text.

One action row sits below a fine divider: Send to agent (primary, icon and label), the destination project select, an icon Move button, the status line, and a quiet icon Delete that turns red on hover. Moving requires a saved note with no unsaved changes and a selected destination. The row wraps at narrow widths.

Save note keeps the editor open and shows Saved. Cancel discards draft changes and returns to the list. Send to agent sends the saved note to the active agent in the same project; it is disabled until the note is saved and has no unsaved changes. Keep loading, unsaved, saving, sending, success, and error messages in the status area. Controls reuse raised fills, neutral borders, and blue keyboard-focus outlines. Notes opens with Ctrl+I by default; the shortcut is configurable in Settings.

## Do's and Don'ts

- Do keep tool widths fixed and shared as tools are added; let the agent take the rest.
- Do reuse workspace color tokens for shell controls and popups.
- Do show selected, hover, keyboard-focus, empty, and no-results states.
- Don't turn the app strip into a wrapping or equal-width responsive grid.
- Don't introduce a new brand direction, display font, or decorative accent palette.
- Don't apply shell styling inside third-party browser content or terminal applications.

## Settings and panel controls

Settings is a workspace page reached from the sidebar. The Panels group sets one default tool panel width (XS–2XL or MAX), initially MD (640px). Agent panels have no default width; they fill the remaining space. Changes save to SQLite and apply only to newly opened tool panels; a thread that already has tools gives new tools their shared width. The page also holds theme (Auto, Light, Dark), an agent-done notification sound, agent command management (edit, rename, disable, remove, custom agents), tool command management (edit, disable, remove, and add custom CLI tools), and remappable keyboard shortcuts. Section labels are 12px semibold muted text above rounded groups; empty status lines collapse so sections keep an even gap.

The Editor section follows Tools and uses the existing grouped-row layout: a File editor label and help text beside a select control. It lists enabled CLI tools by name, including custom tools, with Off to clear the selection. Websites and disabled or removed tools are excluded. Nvim is selected by default. Tool name and enabled-state changes update the choices before saving; disabling or removing the selected tool clears the selection. Save persists the editor choice with tool settings across projects.

Files uses a 40px chrome toolbar. The path shows the folder muted and the file name in 13px semibold. Tree rows match the project sidebar: 28px, 6px radius, muted files, foreground folders. The open file is bold; the keyboard cursor row uses the raised fill with a fine ring. Workspace shortcuts win over Files' Ctrl+O and Ctrl+I history moves, so panel and tool shortcuts work while a file is open.

In Files, `e` opens the previewed file or selected tree file in that tool in the right dock. It replaces the source viewer’s Vim end-of-word motion, while navigation results retain `e` to open a result. Shortcut help lists Open in editor; text fields retain normal typing.

Panel toolbars fit within the workspace viewport while terminal content retains its fixed width. The close button comes before the title and appears on panel hover or button keyboard focus; it remains visible for touch input. Worktree rows use their dedicated switch action with project, path, and branch.

### Thread activity

Sidebar thread text and icons use the normal neutral color when idle, blue when working, and green when done, in both themes. Selection changes the row background, not its status color. Working threads show a spinning sync icon and Working label; done threads show a checkmark and Done label. An agent that asks a question, waits for plan approval, or asks for permission also shows as done, even while its subagents still work. Interacting with a completed thread clears its completion indicator and restores its normal text color and chat icon. Repeated done snapshots keep it idle until a new working turn starts. Project interaction acknowledges completed agents in that project.

### Thread completion

The original branch row exposes the same overflow button and right-click menu styling as worktree rows, with Close branch, New thread, and Branch settings; unavailable New thread uses muted text and receives no hover highlight or keyboard navigation focus.

Worktree rows expose an overflow menu and right-click actions. Merge thread, Squash thread, and Create draft PR are searchable actions in Ctrl+;. Each opens a focused review dialog with destination and commit/file summary, without a method dropdown. Successful local integration automatically removes the thread, worktree, and branch. The final button describes the operation and cleanup. Discard is separated at the bottom of the menu. Its dialog has one final discard button, without a branch checkbox or typed confirmation. These controls reuse native dialog/popover behavior, existing surface colors, and standard command buttons.
