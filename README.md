# Libro

A Go web application using [g-sui](https://github.com/michalCapo/g-sui) that manages and displays web applications in iframes with configurable widths. Users can add, view, and navigate between multiple applications in a horizontal strip layout.

## Architecture

- **Backend**: Go with g-sui (server-rendered UI via WebSocket)
- **Browser Engine**: Headless Chromium via Chrome DevTools Protocol (CDP) for URL apps
- **Frontend**: Minimal client JS for Chrome canvas rendering and input forwarding
- **Styling**: Tailwind CSS classes (built into g-sui)
- **State**: Server-side in-memory (application list, selected index, viewport offset)
- **Communication**: WebSocket for UI interactions + WebSocket for Chrome screencast/input per tab

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

### Application Rendering (Chrome Screencast)
URL applications are rendered using a headless Chromium instance controlled via the Chrome DevTools Protocol (CDP). Each URL app gets its own Chrome tab, with screencast frames streamed to an HTML canvas via WebSocket. This approach bypasses X-Frame-Options and CSP restrictions that block iframe embedding, allowing any website to be displayed. The iframe container has a fixed width based on the app's width setting and takes the full height of the viewport. Applications are displayed side-by-side in a horizontal row, each maintaining its configured width at all times.

### Browser-Like Toolbar
Each application frame has a non-overlapping toolbar above the content, similar to a real browser. The toolbar is always visible and contains:

- **Left side (URL apps)**: Back/forward buttons, globe icon, editable URL input field, copy URL button, reload button
- **Left side (Terminal apps)**: Real application icon (for known commands) or gradient initials badge, command/name label
- **Right side (all apps)**: Width size badges (MD, LG, XL, 2XL, FULL) and close button

Users can see the current URL, copy it to clipboard, edit it and press Enter to navigate to a new URL, or reload the page. Back and forward buttons navigate the browser history. The URL bar automatically updates when the user navigates within the Chrome tab, reflecting the current page URL in real time (via CDP Page.frameNavigated events). The URL is automatically prefixed with `https://` if no scheme is provided. URL changes are persisted to localStorage.

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
- `project.navigate.next` - Switch to the next project (wraps around)
- `project.navigate.prev` - Switch to the previous project (wraps around)

All actions re-render the affected DOM sections via g-sui's Replace/Append.

### Terminal Application Support (ttyd)
Support for terminal-based applications via [ttyd](https://github.com/tsl0922/ttyd). When a user defines a terminal application, it starts on a new port using the `-p` parameter. Terminal applications are editable via the `--writable` parameter.

### Close Application
A close button is available on each application/iframe to remove it from the strip.

### Keyboard Shortcuts
- `Win/Meta+H` — navigate left (previous app)
- `Win/Meta+L` — navigate right (next app)
- `Ctrl+1..9` — select app by position
- `Win/Meta+J` — switch to next project
- `Win/Meta+K` — switch to previous project
- `Win/Meta+/` — open fuzzy search launcher
- `Win/Meta+D` — close the currently selected app

When switching apps, the selected application scrolls into view only if it is off-screen (minimal scroll, no centering). Terminal (ttyd) iframes automatically receive focus when selected, including across project switches.

### Fuzzy Search Launcher
Press `Win/Meta+/` to open a fuzzy search popup that lists all saved applications from localStorage. The user can type to filter the list — matching is fuzzy across app name, command, URL, and type, with scoring that prioritizes word boundaries and consecutive matches. Use arrow keys (Up/Down) to navigate the list, `Enter` to launch the selected app on the right side, `Ctrl+Enter` to launch on the left side, and `Escape` to close. The search is accessible from any context (with or without apps open, from any project).

A **Browser** entry is always available at the bottom of the list. Selecting it opens a new empty browser panel with the URL bar focused.

When typing a URL directly into the search box, a **Browse** entry appears at the top for quick navigation. URL detection covers:
- Full URLs: `https://example.com`, `http://localhost:3000`
- Domain-like strings: `google.com`, `app.example.org/path`
- Local addresses: `localhost`, `127.0.0.1`, `0.0.0.0`, `[::1]`, `[::]`

Local addresses (`localhost`, `127.0.0.1`, `0.0.0.0`, IPv6 loopback) automatically use `http://`. All other addresses default to `https://`.

### Smart Terminal Icons
Terminal applications display real brand icons for known commands instead of generic initials. Icons are resolved by matching the base command name (handling prefixes like `sudo`, `env`, and full paths). Known commands include neovim, vim, claude, node, python, docker, git, go, rust, ruby, kubernetes, terraform, tmux, and many more — sourced from Simple Icons CDN. Generic categories (shells, monitoring tools, build tools) use Material Design icons. Unknown commands fall back to the original colored gradient letter icon.

### Project Support
Projects are tied to folders. The default project is "home" which starts in the home directory. Users can create new projects by defining a folder. When a user clicks on a project, the working directory changes to that folder. All ttyd applications start with `cd <pwd>` to ensure they begin in the project's directory.

Projects can be removed by clicking the close button next to the project name in the project bar. The "home" project cannot be removed. Removing a project cleans up all its running applications (terminals and browser tabs) and removes it from localStorage.

When switching between projects, applications from the previous project are kept alive but hidden. The app remembers position and size of applications per project. When returning to a project, its applications become visible again or are created if this is the first time opening the project in the session.

### Directory Picker for Projects
When adding a new project, a directory picker is used to select the folder. The project name is determined from the folder path but can be changed by the user after selection.

### Application State Preservation
When opening new applications, existing applications (especially ttyd) are not reset and maintain their current state. Closing one application does not affect the state of other running applications.

### Application Position Centering
When a single application is open in a project, it is centered horizontally. When two or more applications are open, they start from the left side.

### Browse (Empty Browser)
The "Browse" button (in both the empty state and side launchers) opens a new empty browser panel. The panel starts with a blank page and the URL bar focused, so the user can immediately type a URL or search query and press Enter to navigate. This behaves like opening a new browser tab.

### Fullscreen Mode
A fullscreen toggle button is available in the project bar next to the theme switcher. It uses the browser's Fullscreen API to enter and exit fullscreen mode. The button label and icon update dynamically to reflect the current state ("Fullscreen" / "Exit").

## Chrome Browser Component

URL applications use a headless Chromium instance instead of iframes. This solves the fundamental limitation that most websites set `X-Frame-Options: sameorigin` which blocks iframe embedding.

### How It Works

1. **One Chrome process**: A single headless Chromium instance is launched on first use, with a persistent profile at `~/.config/libro/chrome-profile`
2. **Per-tab targets**: Each URL app creates a separate Chrome tab via the CDP HTTP API, sharing cookies/sessions/profile
3. **Screencast streaming**: Chrome's `Page.startScreencast` streams PNG frames to the Go backend via CDP WebSocket
4. **Canvas rendering**: Client JS creates an HTML canvas, connects via WebSocket to `/chrome/<appID>/`, and draws received frames
5. **Input forwarding**: Mouse, keyboard, and scroll events on the canvas are captured and forwarded to Chrome via CDP `Input.dispatch*` commands
6. **Navigation tracking**: CDP `Page.frameNavigated` events update the address bar in real-time

### Anti-Bot Detection

The Chrome instance includes stealth measures to bypass bot detection (Cloudflare Turnstile, etc.):
- `--disable-blink-features=AutomationControlled` removes the `navigator.webdriver` flag
- User-Agent override removes "HeadlessChrome" from the UA string
- `Page.addScriptToEvaluateOnNewDocument` injects patches for `navigator.plugins`, `navigator.languages`, `window.chrome`, and Permissions API

### Dependencies

- **chromium-browser** or **google-chrome** must be installed on the system
- **gorilla/websocket** for reliable WebSocket communication (CDP + client)

## Key Technical Decisions

1. **Viewport panning**: CSS `transform: translateX()` on the strip container, controlled by server state. The offset is calculated based on selected app index and total widths.
2. **App container sizing**: Each app container gets a CSS class based on its width setting. Tailwind responsive utilities or inline styles computed server-side.
3. **Session state**: g-sui's WebSocket connection context maintains per-user state. Each connected client has its own `AppState`.
4. **Chrome over iframes**: URL apps use headless Chrome with CDP screencast instead of iframes. This eliminates X-Frame-Options restrictions at the cost of slightly higher resource usage.
5. **Width classes**: Width enum maps to actual CSS max-width values. On mobile, overridden to 100%. Uses Tailwind's responsive prefixes (sm:, md:, lg:, xl:, 2xl:).
