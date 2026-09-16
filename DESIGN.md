---
name: Libro
description: Quiet desktop chrome for a horizontal coding workspace.
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
    rounded: "{rounded.row}"
    padding: "11px 12px"
  size-badge:
    textColor: "{colors.muted}"
    rounded: "{rounded.badge}"
    padding: "1px 5px"
  palette-row:
    textColor: "{colors.fg}"
    rounded: "{rounded.row}"
    padding: "10px 12px"
---

# Design System: Libro

## Overview

**Creative North Star: "T3 Code desktop workspace"**

Libro follows the supplied T3 Code reference: near-white content, softly gray sidebar, subtle dividers, rounded white selected rows, muted icons, compact bordered controls, and restrained blue accents. The dark equivalent keeps the same hierarchy and geometry. The agent workspace is the main content.

**Key Characteristics:**
- Quiet system typography and compact desktop controls.
- Flat app panels separated by fine rules.
- Rounded selection surfaces and softly lifted popups.

Scope: the built workspace shell and its dialogs. Source of truth is `internal/workspace.css`, supported by `workspace.go`, `workspace.js`, `components.go`, and the popup components. Embedded terminals and browser pages retain their own content styling. `PRODUCT.md` fixes the brand and horizontal workspace structure. This is a record of the implementation, not a new visual direction.

## Colors

### Primary

`accent` supplies blue focus outlines, selected panel underlines, and browser loading indicators. It is a functional accent, not a large decorative surface.

### Neutral

`bg` is the near-white main surface; `chrome` is the soft gray navigation surface; `raised` is the white selected row and control surface. `fg` carries primary text, `muted` secondary text and icons. `line` separates surfaces; `control-line` outlines compact controls; `hover` gives restrained pointer feedback. `popup`, `popup-row`, and `popup-muted` apply the same hierarchy to overlays. Each `dark-` counterpart is applied under `html.dark`.

The muted text token was checked against the sidebar surface at 4.57:1. This does not claim a complete accessibility audit of embedded content. Terminal backgrounds and colors follow the shell theme, including live changes. Settings provides Auto (the default), Light, and Dark. Auto follows the OS color scheme; explicit choices override it and persist through the UI library’s theme storage.

## Typography

Use the system sans-serif stack in `body`; there is no separate display font. Body text is compact and regular. Project headings and palette labels use the title scale; toolbar titles use 13px, sidebar labels and result descriptions 12px. Brand text is 16px at weight 550. Empty-state headings use `empty-heading`; supporting copy has a 46ch maximum width and 1.65 line height. Status and size controls use 10px text. Keycaps use the monospace role. Material Icons Round supplies mostly 18–19px muted icons.

## Layout

The shell has a 56px titlebar, a 26px statusbar, and left project navigation. The sidebar is 252px wide, reduced to 216px at viewport widths of 1050px or less. At 760px or less it overlays the content between the titlebar and statusbar, with width `min(252px, calc(100vw - 48px))`. It defaults closed on small screens when no preference exists; selecting a project closes it. The brand and trailing status text hide, and empty-state headings become 24px.

The main area is one horizontal row of full-height, non-shrinking app panels with no gap or outer card padding. Width presets are XS 320px, SM 480px, MD 640px, LG 960px, XL 1280px, and 2XL 1920px. These are panel widths, not viewport breakpoints. Legacy 3XL 2560px and FULL remain available; the existing screen-width policy disables 3XL on Full HD or smaller screens. FULL uses available strip width. Maximizing temporarily fills the strip and hides other panels. Selecting an offscreen panel scrolls it into view.

Toolbars are at least 48px tall and allow controls to wrap. Popups are centered with viewport height limits and internal scrolling. Commands and the app launcher are at most 420px wide with a 24px viewport gutter; other existing dialogs retain their component-specific widths. At small widths, preserve the carousel rather than stacking panels.

## Elevation & Depth

The workspace uses tonal differences and 1px dividers. Panels are flat and square. Popups alone use the shared shadow `0 12px 36px rgb(0 0 0 / 14%)` with a 10% black backdrop and no blur. The mobile sidebar uses a lateral shadow. Bordered launch controls have only a faint shadow where implemented. Exact shadow values and component samples are in `.impeccable/design.json`.

Animations on app panels are disabled. Reduced-motion preferences remove shell animation, transitions, and smooth scrolling.

## Shapes

Use the frontmatter radii by role: small keycaps and size badges, gently rounded controls, rounded selected rows, and larger popup and shortcut-group corners. Preserve square app panels. Borders are single-pixel neutral rules; selection does not require a thick outline.

## Components

- **Navigation:** the sidebar starts with one compact utility row: search grows to fill the space, followed by icon buttons for switching projects, adding a project, and starting a new agent session. Project rows are at least 44px high. Selection uses the raised surface and primary text. Worktrees indent beneath projects. Removal appears on hover or keyboard focus and remains visible on touch devices.
- **Actions:** launch and new-session buttons are compact bordered raised controls, at least 36px high. Icon buttons are quiet, square controls. Hover uses the neutral hover surface; focus uses the blue outline.
- **Fields:** ordinary popup fields use raised fill, a fine border, and an accent caret. Search fields sit inside a rounded light search surface with an icon and dismiss action. Their inner border and outline are removed; the shared search wrapper shows focus-within feedback.
- **Size controls:** only the selected size appears in the panel toolbar, with raised fill and a fine ring. Hover or click opens a floating size picker with every preset, including the current size. Keyboard users open it with Enter, Space, or Arrow Down; Escape dismisses it. The command resize picker uses rounded rows and radio indicators.
- **Palettes:** commands, apps, and project results use rounded rows with muted icons, a primary label, and a smaller description. Command and app rows are at least 62px high. Selection and hover use the popup-row surface. Keyboard hints stay muted; searchable lists show a no-results state.
- **Shortcuts:** search above grouped rows; 50px minimum row height, fine separators, right-aligned outlined keycaps. Long labels wrap; keycaps stay on one line.

## Do's and Don'ts

- Do keep app widths fixed as panels are added and use horizontal scrolling.
- Do reuse workspace color tokens for shell controls and popups.
- Do show selected, hover, keyboard-focus, empty, and no-results states.
- Don't turn the app strip into a wrapping or equal-width responsive grid.
- Don't introduce a new brand direction, display font, or decorative accent palette.
- Don't apply shell styling inside third-party browser content or terminal applications.

## Settings and panel controls

Settings is a workspace page reached from the sidebar. A grouped row provides the global default panel width (XS–2XL), initially MD (640px). Changes save to SQLite and apply only to newly opened panels.

Panel toolbars fit within the workspace viewport while terminal content retains its fixed width. The close button comes before the title and appears on panel hover or button keyboard focus; it remains visible for touch input. Worktree rows use their dedicated switch action with project, path, and branch.
