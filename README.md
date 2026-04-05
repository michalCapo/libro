# Libro

A Go + Electron application using [g-sui](https://github.com/michalCapo/g-sui) that manages and displays web applications and terminal sessions in a horizontal strip layout. URL apps render natively via Electron `<webview>` tags, and terminals run via ttyd iframes.

## Demo

https://github.com/michalCapo/libro/raw/main/demo/demo.mp4

## Architecture

- **Backend**: Go with g-sui (server-rendered UI via WebSocket)
- **Desktop Shell**: Electron with `<webview>` tag support for native web rendering
- **Frontend**: Minimal client JS for webview lifecycle, navigation tracking, and input forwarding
- **Styling**: Tailwind CSS classes (built into g-sui) with full dark mode support
- **State**: Server-side in-memory (application list, selected index, project snapshots)
- **Persistence**: SQLite database (`~/.local/share/libro/libro.db`)
- **Communication**: WebSocket for UI interactions (g-sui)

## Width System

Each width defines a fixed pixel width:

| Width | Fixed Width |
|-------|-------------|
| SM    | 480px       |
| MD    | 640px       |
| LG    | 960px       |
| XL    | 1280px      |
| 2XL   | 1920px      |
| FULL  | 100%        |

Each app uses its configured fixed pixel width. FULL takes 100% of the viewport.

## Features

### Layout & Page Structure
Full-height flex container as a single page app. One route (`/`) renders the application strip. The entire viewport is the application strip with a neutral background to distinguish from app content.

### Empty State
When no applications exist, "Add App" and "Quick Launch" buttons are displayed centered. If the project has no saved applications yet, a short gray guide text explains how to get started — add web apps by URL or terminal commands, use Quick Launch for quick web lookups and terminal commands, and check shortcuts for a productivity boost.

### Add Application Dialog
Modal/dialog overlay triggered by "+" button click. Contains a URL input field (required) and width selection via radio buttons (SM, MD, LG, XL, 2XL, FULL). On submit, validates the URL is not empty, adds the application to state, closes the dialog, and re-renders the strip.

### Application Rendering

**URL apps** render natively using Electron's `<webview>` tag with a shared persistent session (`partition="persist:libro"`). This provides full browser-quality rendering without the X-Frame-Options and CSP restrictions that block iframe embedding. Each webview gets real mouse, keyboard, and scroll input — no screencasting or input forwarding needed.

**Terminal apps** render via ttyd iframes, unchanged from the terminal support described below.

### Browser-Like Toolbar
Each application frame has a non-overlapping toolbar above the content, similar to a real browser. The toolbar is always visible and contains:

- **Left side (URL apps)**: Back/forward buttons, globe icon, editable URL input field, copy URL button, reload button
- **Left side (Terminal apps)**: Real application icon (for known commands) or gradient initials badge, command/name label
- **Right side (all apps)**: Width size badges (SM, MD, LG, XL, 2XL, FULL) and close button

Users can see the current URL, copy it to clipboard, edit it and press Enter to navigate to a new URL, or reload the page. Back and forward buttons navigate the browser history. The URL bar automatically updates when the user navigates within the webview, reflecting the current page URL in real time (via `did-navigate` and `did-navigate-in-page` events). The URL is automatically prefixed with `https://` if no scheme is provided.

### Horizontal Strip Layout
Applications are arranged in a horizontal flexbox row. The currently selected application is centered in the viewport. Applications that don't fit extend beyond the viewport (off-screen left/right). The strip takes the full height of the viewport.

### "+" Add Buttons on Sides
When there is space on the left or right of the application strip, "+" buttons appear as vertical strips on the viewport edges. New applications are always added to the right side of the strip.

### Viewport Panning / Navigation
When applications overflow the viewport, left (`<`) and right (`>`) arrows appear at viewport edges allowing the user to shift/pan the layout. Smooth CSS transitions animate the shift. Partially visible apps appear on edges during panning.

### Application Selection / Focus
One application is always selected and centered. Clicking on a partially visible app selects and centers it. The selected app is marked with a small teal dot on the top-left corner (absolute positioned with a subtle glow), rather than a border change.

### Multi-App Layout on Large Screens
If the selected app doesn't fill the viewport, adjacent apps are shown partially or fully next to it. "+" buttons fill remaining space if no more apps to show.

### Application Reordering
Applications can be reordered within the strip using keyboard shortcuts (`⌘+Ctrl+H` to move left, `⌘+Ctrl+L` to move right). The selected app swaps position with its neighbor.

### Application Sorting
Apps are sorted alphabetically by name (case-insensitive) when added. Manual reordering via move shortcuts overrides the sort order.

### Manage Saved Apps
A manage/edit overlay allows viewing all saved app definitions, editing existing saved apps (name, URL, command, width, etc.), and deleting saved app definitions from the database. Project-specific apps can be flagged to only appear in their associated project.

### Terminal Application Support (ttyd)
Support for terminal-based applications via [ttyd](https://github.com/tsl0922/ttyd). When a user defines a terminal application, it starts on a new port using the `-p` parameter. Terminal applications are editable via the `--writable` parameter. Ports are allocated starting from 7681, automatically skipping any ports already in use.

### Close Application
A close button is available on each application to remove it from the strip. When closing the entire app with running applications, a confirmation dialog is shown to prevent accidental exit.

### Keyboard Shortcuts

**Apps:**
- `⌘ + N` — new app (right of current)
- `⌘ + Ctrl + N` — new app (left of current)
- `⌘ + D` — close current app
- `⌘ + +` — zoom in (whole application)
- `⌘ + -` — zoom out (whole application)

**Navigation:**
- `⌘ + H` — navigate left (previous app)
- `⌘ + L` — navigate right (next app)
- `⌘ + Ctrl + H` — move app left (reorder)
- `⌘ + Ctrl + L` — move app right (reorder)
- `⌘ + B` — toggle project sidebar
- `Ctrl + 1–9` — switch to assigned project by slot
- `Ctrl + 0` — switch to previous project
- `⌘ + G` — git worktrees popup

**Browser:**
- `Ctrl + L` — focus the URL bar of the selected app
- `Ctrl + R` — reload the selected app

**Browser Vim Keys** (disabled in input fields):
- `g / G` — go to top / bottom of page
- `j / k` — scroll down / up
- `h / l` — scroll left / right
- `/` — find in page
- `n / p` — find next / previous
- `Esc` — clear search
- `b / f` — page back / forward
- `Enter` — follow link / click button

Keyboard shortcuts from webview guest pages are intercepted at the Electron main process level (via `web-contents-created` + `before-input-event`) and forwarded to the host page as synthetic KeyboardEvents.

When switching apps, the selected application scrolls into view only if it is off-screen (minimal scroll, no centering). Terminal (ttyd) iframes automatically receive focus when selected, including across project switches.

### Fuzzy Search Launcher
Press `⌘ + /` to open a fuzzy search popup that lists all saved applications from the database. The user can type to filter the list — matching is fuzzy across app name, command, URL, and type, with scoring that prioritizes word boundaries and consecutive matches. Use arrow keys (Up/Down) to navigate the list, `Enter` to launch the selected app on the right side, `Ctrl+Enter` to launch on the left side, and `Escape` to close. The search is accessible from any context (with or without apps open, from any project).

A **Browser** entry is always available at the bottom of the list. Selecting it opens a new empty browser panel with the URL bar focused.

When typing a URL directly into the search box, a **Browse** entry appears at the top for quick navigation. URL detection covers:
- Full URLs: `https://example.com`, `http://localhost:3000`
- Domain-like strings: `google.com`, `app.example.org/path`
- Local addresses: `localhost`, `127.0.0.1`, `0.0.0.0`, `[::1]`, `[::]`

Local addresses (`localhost`, `127.0.0.1`, `0.0.0.0`, IPv6 loopback) automatically use `http://`. All other addresses default to `https://`.

When typing a non-URL command (e.g. `ls`, `htop`, `npm start`), a **Run** entry appears at the top. Selecting it opens a terminal panel that executes the command directly. After the command finishes, the terminal drops into an interactive shell so output remains visible.

The "Quick Launch" button in the top bar, side launcher, and empty state all open this same search popup, providing a single entry point for browsing URLs and running commands.

### Browsing History
Browsed URLs are persisted to the database with timestamps and titles. History entries can be individually deleted or cleared entirely via WebSocket actions.

### Command History
Terminal commands executed via the search launcher are persisted to the database. Run history entries appear in the search popup alongside browsed URL history and saved apps, and can be individually deleted or cleared.

### Smart Terminal Icons
Terminal applications display real brand icons for known commands instead of generic initials. Icons are resolved by matching the base command name (handling prefixes like `sudo`, `env`, and full paths). Known commands include neovim, vim, claude, node, python, docker, git, go, rust, ruby, kubernetes, terraform, tmux, and many more — sourced from Simple Icons CDN. Generic categories (shells, monitoring tools, build tools) use Material Design icons. Unknown commands fall back to the original colored gradient letter icon.

### Project Support
Projects are tied to folders. The default project is "home" which starts in the home directory. Users can create new projects by defining a folder. When a user clicks on a project, the working directory changes to that folder. All ttyd applications start with `cd <pwd>` to ensure they begin in the project's directory.

Projects can be removed by clicking the close button next to the project name in the project bar. The "home" project cannot be removed. Removing a project cleans up all its running applications (terminals and browser tabs) and removes it from the database.

When switching between projects, applications from the previous project are kept alive but hidden. The app remembers position and size of applications per project. When returning to a project, its applications become visible again or are created if this is the first time opening the project in the session.

### Project Sidebar
The project sidebar lists all projects and can be collapsed/expanded with `⌘ + B`. Projects that are git repositories are automatically detected and marked. Each project can be assigned a keyboard navigation slot (Ctrl+2–9) for quick switching.

### Directory Picker for Projects
When adding a new project, a directory picker is used to select the folder. The project name is determined from the folder path but can be changed by the user after selection.

### Git Worktree Support
For projects that are git repositories, a worktree dialog (`⌘ + G`) allows managing git worktrees:
- **List** existing worktrees with their branch names
- **Switch** to a worktree — creates a virtual project linked to the parent project
- **Add** a new worktree for a branch (creates the branch if it doesn't exist)
- **Remove** a worktree and its associated virtual project

Virtual projects (created from worktrees) are not persisted to the database and share the keyboard nav slot of their parent project.

### Application State Preservation
When opening new applications, existing applications (especially ttyd) are not reset and maintain their current state. Closing one application does not affect the state of other running applications.

### Application Position Centering
When a single application is open in a project, it is centered horizontally. When two or more applications are open, they start from the left side.

### Browse (Empty Browser)
The "Browser" entry in the search launcher opens a new empty browser panel. The panel starts with a blank page and the URL bar focused, so the user can immediately type a URL or search query and press Enter to navigate. This behaves like opening a new browser tab.

### Application Zoom
The entire UI can be zoomed in (`⌘ + +`) or out (`⌘ + -`), handled at the Electron level with `webContents.setZoomLevel()`.

### Dark Mode
Full dark mode support using Tailwind `dark:` prefix classes throughout all components. System theme is detected and applied automatically.

### Version Display
The current application version is shown in the header toolbar next to the shortcut buttons. In production builds, the version is injected at compile time via `-ldflags`. In development, it is read from the `VERSION` file at startup.

### Fullscreen Mode
A fullscreen toggle button is available in the project bar next to the theme switcher. It uses the browser's Fullscreen API to enter and exit fullscreen mode. The button label and icon update dynamically to reflect the current state ("Fullscreen" / "Exit").

## Desktop Mode

Libro runs as a desktop application using Electron. The Go backend starts an HTTP/WebSocket server, and Electron opens a `BrowserWindow` pointing to it.

```bash
libro              # starts server + opens Electron window
libro --no-desktop # starts server only (no window)
libro --version    # show version and exit (also -v)
```

The Go binary launches Electron as a child process, passing the server port via `LIBRO_PORT`. If Electron is not installed locally, the Go binary runs `npm install` automatically. When the Electron window is closed, the Go process exits.

Electron is configured with `webviewTag: true` for native web rendering. Keyboard shortcuts that would be consumed by webview guest pages are intercepted at the Electron main process level and forwarded to the host page.

Development builds (`go build -tags dev`) use port `1439` instead of `8100`, so both can run side-by-side.

### Dependencies

- **Electron** (installed via npm alongside the Go binary)
- **Node.js / npm** (required for Electron installation)
- **g-sui** for server-rendered UI over WebSocket
- **modernc.org/sqlite** for persistence (pure Go, no CGO)

## Install

```bash
./install
```

Builds the Go binary for your OS/architecture and installs it to `~/.local/share/libro/` along with the Electron files (`package.json`, `electron/main.js`, `electron/preload.js`). Runs `npm install` to fetch Electron, then creates a symlink at `~/.local/bin/libro`. Works on Linux, macOS, and Windows (via MSYS/Cygwin).

**Supported architectures:** x86_64 (amd64), aarch64/arm64.

On **Linux**, it also installs a `.desktop` entry, sets a custom icon on the binary, and installs the app icon to `~/.local/share/icons/`. On **Windows**, it embeds the icon into the `.exe` via `go-winres` (if installed).

## Deploy

```bash
./deploy           # bump version, build, commit, tag, push
./deploy --dry-run # preview without changes
```

Reads the current version from `VERSION`, increments the patch number (e.g. `0.0.1` → `0.0.2`), builds the binary, commits the version bump, tags it `vX.X.X`, and pushes to git.

## Key Technical Decisions

1. **Electron webviews over iframes/CDP**: URL apps use Electron `<webview>` tags for native rendering. This eliminates X-Frame-Options/CSP restrictions without the complexity and resource overhead of CDP screencast.
2. **Viewport panning**: CSS `transform: translateX()` on the strip container, controlled by server state. The offset is calculated based on selected app index and total widths.
3. **App container sizing**: Each app container gets a CSS class based on its width setting. Tailwind responsive utilities or inline styles computed server-side.
4. **Session state**: g-sui's WebSocket connection context maintains per-user state. Each connected client has its own `AppState`.
5. **SQLite persistence**: Application definitions and project config are stored in a SQLite database using a pure-Go driver (no CGO dependency). WAL mode enabled for concurrent reads.
6. **Width classes**: Width enum maps to actual CSS max-width values. Uses Tailwind's responsive prefixes (sm:, md:, lg:, xl:, 2xl:).
7. **Keyboard forwarding**: Electron's main process intercepts shortcuts from webview guest pages via `before-input-event` and forwards them as synthetic KeyboardEvents to the host page.
8. **Virtual projects**: Git worktrees create temporary virtual projects that share nav slots with their parent project and are not persisted to the database.
9. **Port allocation**: ttyd ports are allocated starting from 7681, automatically skipping occupied ports.
