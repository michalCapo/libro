# Libro - Implementation Plan

## Overview
A Go web application using [g-sui](https://github.com/michalCapo/g-sui) that manages and displays web applications in iframes with configurable widths. Users can add, view, and navigate between multiple applications in a horizontal strip layout.

---

## Architecture

- **Backend**: Go with g-sui (server-rendered UI via WebSocket)
- **Frontend**: Zero custom JS - all UI generated server-side via g-sui node trees
- **Styling**: Tailwind CSS classes (built into g-sui)
- **State**: Server-side in-memory (application list, selected index, viewport offset)
- **Communication**: WebSocket for all interactions (add app, navigate, scroll)

---

## Data Model

### Application
```go
type Application struct {
    ID    string // unique identifier
    URL   string // iframe source URL
    Width string // "xs" | "sm" | "md" | "lg" | "xl"
}
```

### AppState (server-side, per-session)
```go
type AppState struct {
    Apps          []Application // ordered list of applications
    SelectedIndex int           // currently focused/centered app index
    ViewportOffset int          // horizontal scroll offset for panning
}
```

---

## Width System

Widths map to Tailwind-like responsive behavior. Each width defines max-width at different breakpoints:

| Width | Mobile (<640px) | Tablet (640-1024px) | Laptop (1024-1536px) | Desktop (1536-2560px) | 2K+ (>2560px) |
|-------|----------------|---------------------|----------------------|----------------------|---------------|
| xs    | 100%           | 320px               | 320px                | 320px                | 320px         |
| sm    | 100%           | 480px               | 480px                | 480px                | 480px         |
| md    | 100%           | 100%                | 640px                | 640px                | 640px         |
| lg    | 100%           | 100%                | 100%                 | 960px                | 960px         |
| xl    | 100%           | 100%                | 100%                 | 100%                 | 1280px        |

**Key rule from spec**: All widths are responsive to viewport. On mobile, everything is 100%. On larger screens, apps use their configured fixed width. On 2K+ screens, xl apps are centered and space on both sides can show "+" buttons or adjacent apps.

---

## Features & Tasks

### 1. Project Setup
- [ ] Initialize Go module (`go mod init libro`)
- [ ] Add g-sui dependency (`go get github.com/michalCapo/g-sui`)
- [ ] Create `main.go` with g-sui app bootstrap
- [ ] Create `assets/` directory for any static assets (if needed)
- [ ] Configure app to listen on `:1439`

### 2. Layout & Page Structure
- [ ] Create main layout function with full-height flex container
- [ ] Single page app - one route (`/`) that renders the app strip
- [ ] No header/nav needed - the entire viewport is the application strip
- [ ] Background should be neutral/subtle to distinguish from iframe content

### 3. Empty State (No Applications)
- [ ] When no applications exist, show centered "+ Application" button
- [ ] Button should be prominent and clearly visible (as shown in layout.png row 1)
- [ ] Clicking opens the "Add Application" dialog

### 4. Add Application Dialog (Popup)
- [ ] Modal/dialog overlay triggered by "+" button click
- [ ] **URL input field**: text input for the application URL (required)
- [ ] **Width selection**: radio button group with options: xs, sm, md, lg, xl
- [ ] Each radio option should show the width label clearly
- [ ] "Add" / "Cancel" buttons
- [ ] On submit: validate URL is not empty, add application to state, close dialog, re-render strip
- [ ] Use g-sui's form components (NewForm, text field, radio field, submit)

### 5. Application Rendering (Iframe Display)
- [ ] Each application renders as an iframe with `src` set to the app's URL
- [ ] Iframe container has fixed width based on the app's width setting
- [ ] Iframe takes full height of the viewport (minus any minimal padding)
- [ ] Iframe should have no border or minimal border to separate from background
- [ ] Applications are displayed side-by-side in a horizontal row
- [ ] Each application container maintains its configured width at all times ("Applications will always keep the same width" - from layout.png)

### 6. Horizontal Strip Layout
- [ ] Applications arranged in a horizontal flexbox/row
- [ ] The currently selected application is centered in the viewport
- [ ] Applications that don't fit in the viewport extend beyond it (off-screen left/right)
- [ ] The strip is the full height of the viewport

### 7. "+" Add Buttons on Sides
- [ ] When there is space on the left or right of the application strip, show "+" buttons
- [ ] On 2K+ screens with an xl app centered: "+" buttons appear on both sides
- [ ] "+" buttons are vertical strips on the edges of the viewport
- [ ] Clicking a side "+" button opens the Add Application dialog
- [ ] New application is always added to the right side of the strip (from layout.png: "will add new application to right")

### 8. Viewport Panning / Navigation
- [ ] When applications overflow the viewport, user can shift/pan the layout
- [ ] **Left arrow** (`<`): shift viewport to show apps on the left
- [ ] **Right arrow** (`>`): shift viewport to show apps on the right
- [ ] Navigation arrows appear at viewport edges when there are apps off-screen in that direction
- [ ] From layout.png: "will shift layout to left" - adding apps to the right shifts existing apps left
- [ ] Smooth transition/animation when shifting (CSS transition on transform/margin)
- [ ] The viewport panning reveals apps that were previously off-screen
- [ ] Partially visible apps on edges (as shown in layout.png bottom row - app 1 partially visible on left, app 3 partially visible on right)

### 9. Application Selection / Focus
- [ ] One application is always "selected" / in focus (centered)
- [ ] Clicking on a partially visible app could select/center it
- [ ] Selected app is visually distinguished (e.g., slightly elevated, border highlight)

### 10. Responsive Behavior
- [ ] **Mobile (<640px)**: All apps take 100% width, show one app at a time, swipe/arrow to navigate
- [ ] **Tablet (640-1024px)**: xs/sm apps show at fixed width, md/lg/xl take 100%
- [ ] **Laptop (1024-1536px)**: xs/sm/md show at fixed width, lg/xl take 100%
- [ ] **Desktop (1536-2560px)**: xs/sm/md/lg show at fixed width, xl takes 100%
- [ ] **2K+ (>2560px)**: All apps show at fixed width, xl apps centered with space for "+" buttons and adjacent apps on both sides

### 11. Multi-App Layout on Large Screens (2K+)
- [ ] On 2K+ screens: if the selected app doesn't fill the viewport, show adjacent apps
- [ ] Partial or whole applications visible next to the main/selected application
- [ ] "+" buttons fill remaining space if no more apps to show
- [ ] Example from layout.png: center app 2, show app 1 partially on left, app 3 partially on right

### 12. State Management via WebSocket Actions
- [ ] `app.add` - Add new application (receives URL + width from dialog)
- [ ] `app.navigate.left` - Shift viewport left (show previous app)
- [ ] `app.navigate.right` - Shift viewport right (show next app)
- [ ] `app.select` - Select/focus a specific application by index
- [ ] `app.dialog.open` - Open the add application dialog
- [ ] `app.dialog.close` - Close the add application dialog
- [ ] All actions re-render the affected DOM sections via g-sui's Replace/Append

### 13. Persistence (Optional/Future)
- [ ] For MVP: in-memory state (lost on server restart)
- [ ] Future: persist to JSON file or database

---

## Layout.png Analysis - Scene by Scene

### Scene 1: Empty State
- Large empty viewport
- Single centered "+ Application" button at top
- Clean, minimal appearance

### Scene 2: One Application Added
- Single application (1) centered in viewport
- "+" (add application) buttons visible on both left and right sides
- Labels: "add application button" pointing to "+" buttons, "Application" pointing to the iframe

### Scene 3: Two Applications
- Two applications (1, 2) side by side
- "+" buttons on both outer edges
- Both apps visible within viewport

### Scene 4: Three Applications (Viewport Overflow Begins)
- Three applications (1, 2, 3) - may start exceeding viewport
- "+" buttons on edges
- Annotation: "Applications will always keep the same width"
- Annotation: "will add new application to right and slide and left application outside the viewport"

### Scene 5: Panned/Scrolled View
- Applications 1, 2, 3 visible but shifted left
- App 1 is partially off-screen on the left
- App 3 is partially visible on the right (or fully visible with app 8 partially on far right)
- Navigation arrow ">" visible on right edge
- Annotation: "will shift layout to left"
- This shows the panning mechanism in action

---

## File Structure

```
libro/
  main.go              # Entry point, app setup, routes, actions
  state.go             # AppState, Application structs, state management
  components.go        # Reusable UI components (app strip, add dialog, nav arrows)
  layout.go            # Main layout wrapper
  width.go             # Width definitions and CSS class mapping
  go.mod
  go.sum
  spec.md              # Original spec
  layout.png           # Original layout diagram
  PLAN.md              # This file
```

---

## Implementation Order

1. **Project setup** - go mod, main.go, basic g-sui app running
2. **Data model** - Application struct, AppState, width constants
3. **Layout** - Main page layout, full-viewport container
4. **Empty state** - Centered "+" button when no apps
5. **Add dialog** - Modal with URL input + width radio buttons
6. **Single app rendering** - Iframe display with correct width
7. **Horizontal strip** - Multiple apps in a row
8. **"+" side buttons** - Add buttons on viewport edges
9. **Viewport panning** - Left/right navigation arrows, shift logic
10. **Responsive widths** - Tailwind breakpoint classes for each width tier
11. **2K+ layout** - Adjacent app visibility, centered selected app
12. **Polish** - Transitions, partial app visibility, edge cases

---

## Key Technical Decisions

1. **Viewport panning**: Use CSS `transform: translateX()` on the strip container, controlled by server state. The offset is calculated based on selected app index and total widths.

2. **Iframe sizing**: Each iframe container gets a CSS class based on its width setting. Use Tailwind responsive utilities or inline styles computed server-side.

3. **Session state**: Use g-sui's WebSocket connection context to maintain per-user state. Each connected client has its own `AppState`.

4. **No client-side JS needed**: All navigation, panning, and dialog logic happens via g-sui WebSocket actions. The server computes the new layout and sends DOM patches.

5. **Width classes**: Map width enum to actual CSS max-width values. On mobile, override to 100%. Use Tailwind's responsive prefixes (sm:, md:, lg:, xl:, 2xl:).
