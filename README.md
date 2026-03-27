# Libro

A Go web application using [g-sui](https://github.com/michalCapo/g-sui) that manages and displays web applications in iframes with configurable widths. Users can add, view, and navigate between multiple applications in a horizontal strip layout.

## Architecture

- **Backend**: Go with g-sui (server-rendered UI via WebSocket)
- **Frontend**: Zero custom JS - all UI generated server-side via g-sui node trees
- **Styling**: Tailwind CSS classes (built into g-sui)
- **State**: Server-side in-memory (application list, selected index, viewport offset)
- **Communication**: WebSocket for all interactions (add app, navigate, scroll)

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

## Width System

Widths map to Tailwind-like responsive behavior. Each width defines max-width at different breakpoints:

| Width | Mobile (<640px) | Tablet (640-1024px) | Laptop (1024-1536px) | Desktop (1536-2560px) | 2K+ (>2560px) |
|-------|----------------|---------------------|----------------------|----------------------|---------------|
| xs    | 100%           | 320px               | 320px                | 320px                | 320px         |
| sm    | 100%           | 480px               | 480px                | 480px                | 480px         |
| md    | 100%           | 100%                | 640px                | 640px                | 640px         |
| lg    | 100%           | 100%                | 100%                 | 960px                | 960px         |
| xl    | 100%           | 100%                | 100%                 | 100%                 | 1280px        |

All widths are responsive to viewport. On mobile, everything is 100%. On larger screens, apps use their configured fixed width. On 2K+ screens, xl apps are centered and space on both sides can show "+" buttons or adjacent apps.

## Features

### Layout & Page Structure
Full-height flex container as a single page app. One route (`/`) renders the application strip. The entire viewport is the application strip with a neutral background to distinguish from iframe content.

### Empty State
When no applications exist, a centered "+ Application" button is displayed. Clicking it opens the "Add Application" dialog.

### Add Application Dialog
Modal/dialog overlay triggered by "+" button click. Contains a URL input field (required) and width selection via radio buttons (xs, sm, md, lg, xl). On submit, validates the URL is not empty, adds the application to state, closes the dialog, and re-renders the strip.

### Application Rendering (Iframe Display)
Each application renders as an iframe with `src` set to the app's URL. The iframe container has a fixed width based on the app's width setting and takes the full height of the viewport. Applications are displayed side-by-side in a horizontal row, each maintaining its configured width at all times.

### Browser-Like Toolbar
Each application frame has a non-overlapping toolbar above the iframe content, similar to a real browser. The toolbar is always visible and contains:

- **Left side (URL apps)**: Back/forward buttons, globe icon, editable URL input field, copy URL button, reload button
- **Left side (Terminal apps)**: Initials badge with gradient background, command/name label
- **Right side (all apps)**: Width size badges (MD, LG, XL, 2XL, FULL) and close button

Users can see the current URL, copy it to clipboard, edit it and press Enter to navigate to a new URL, or reload the iframe. Back and forward buttons navigate the iframe's browser history. The URL bar automatically updates when the user clicks links inside the iframe, reflecting the current page URL in real time (via postMessage from the injected proxy script). The URL is automatically prefixed with `https://` if no scheme is provided. URL changes are persisted to localStorage.

### Horizontal Strip Layout
Applications are arranged in a horizontal flexbox row. The currently selected application is centered in the viewport. Applications that don't fit extend beyond the viewport (off-screen left/right). The strip takes the full height of the viewport.

### "+" Add Buttons on Sides
When there is space on the left or right of the application strip, "+" buttons appear as vertical strips on the viewport edges. New applications are always added to the right side of the strip.

### Viewport Panning / Navigation
When applications overflow the viewport, left (`<`) and right (`>`) arrows appear at viewport edges allowing the user to shift/pan the layout. Smooth CSS transitions animate the shift. Partially visible apps appear on edges during panning.

### Application Selection / Focus
One application is always selected and centered. Clicking on a partially visible app selects and centers it. The selected app is visually distinguished.

### Responsive Behavior
- **Mobile (<640px)**: All apps take 100% width, one app at a time
- **Tablet (640-1024px)**: xs/sm at fixed width, md/lg/xl take 100%
- **Laptop (1024-1536px)**: xs/sm/md at fixed width, lg/xl take 100%
- **Desktop (1536-2560px)**: xs/sm/md/lg at fixed width, xl takes 100%
- **2K+ (>2560px)**: All apps at fixed width, xl apps centered with space for "+" buttons and adjacent apps

### Multi-App Layout on Large Screens (2K+)
On 2K+ screens, if the selected app doesn't fill the viewport, adjacent apps are shown partially or fully next to it. "+" buttons fill remaining space if no more apps to show.

### State Management via WebSocket Actions
- `app.add` - Add new application (receives URL + width from dialog)
- `app.navigate.left` - Shift viewport left (show previous app)
- `app.navigate.right` - Shift viewport right (show next app)
- `app.select` - Select/focus a specific application by index
- `app.dialog.open` - Open the add application dialog
- `app.dialog.close` - Close the add application dialog
- `app.url.set` - Change URL of a running application (navigates iframe to new URL)
- `app.browse.new` - Open a new empty browser panel with focus on the URL bar

All actions re-render the affected DOM sections via g-sui's Replace/Append.

### Terminal Application Support (ttyd)
Support for terminal-based applications via [ttyd](https://github.com/tsl0922/ttyd). When a user defines a terminal application, it starts on a new port using the `-p` parameter. Terminal applications are editable via the `--writable` parameter.

### Close Application
A close button is available on each application/iframe to remove it from the strip.

### Keyboard Shortcuts
Keyboard shortcuts to switch between applications: `Win/Meta+H` navigates left and `Win/Meta+L` navigates right. The selected application is kept centered after switching.

### Project Support
Projects are tied to folders. The default project is "home" which starts in the home directory. Users can create new projects by defining a folder. When a user clicks on a project, the working directory changes to that folder. All ttyd applications start with `cd <pwd>` to ensure they begin in the project's directory.

When switching between projects, applications from the previous project are kept alive but hidden. The app remembers position and size of applications per project. When returning to a project, its applications become visible again or are created if this is the first time opening the project in the session.

### Directory Picker for Projects
When adding a new project, a directory picker is used to select the folder. The project name is determined from the folder path but can be changed by the user after selection.

### Application State Preservation
When opening new applications, existing applications (especially ttyd) are not reset and maintain their current state. Closing one application does not affect the state of other running applications.

### Application Position Centering
When a single application is open in a project, it is centered horizontally. When two or more applications are open, they start from the left side.

### Browse (Empty Browser)
The "Browse" button (in both the empty state and side launchers) opens a new empty browser panel. The panel starts with a blank page and the URL bar focused, so the user can immediately type a URL or search query and press Enter to navigate. This behaves like opening a new browser tab.

## Key Technical Decisions

1. **Viewport panning**: CSS `transform: translateX()` on the strip container, controlled by server state. The offset is calculated based on selected app index and total widths.
2. **Iframe sizing**: Each iframe container gets a CSS class based on its width setting. Tailwind responsive utilities or inline styles computed server-side.
3. **Session state**: g-sui's WebSocket connection context maintains per-user state. Each connected client has its own `AppState`.
4. **No client-side JS needed**: All navigation, panning, and dialog logic happens via g-sui WebSocket actions. The server computes the new layout and sends DOM patches.
5. **Width classes**: Width enum maps to actual CSS max-width values. On mobile, overridden to 100%. Uses Tailwind's responsive prefixes (sm:, md:, lg:, xl:, 2xl:).
