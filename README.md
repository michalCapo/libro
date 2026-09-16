# Libro

Libro is a Go + Electron desktop workspace for CLI coding agents. Run Codex, Pi, Claude, or another agent alongside browser tools and terminals in a horizontal row of fixed-width panels. Switch projects on the left.

The right sidebar opens Terminal, Git, and Browser tools. The sidebar shows one tool at a time at its selected width. Switching tool tabs keeps their sessions running. The bottom panel runs one shell terminal per project. Hide it to keep its session running, then reopen it from the titlebar. Use panel width controls to resize panels. Open **Commands** to launch a tool, or **New agent session** to run an agent or custom command.

Tools use a small local plugin manifest. See [Creating plugins](docs/plugins.md) for the API and examples.

![Demo](demo/demo.gif)

## What It Does

- Opens agent, tool, and terminal tab groups in one desktop window.
- Keeps per-project strip state in memory while you switch between projects.
- Saves reusable app definitions in SQLite, either globally or per project.
- Integrates Git worktrees into the project picker and shortcut system.
- Supports browser-oriented shortcuts, including URL popup, vim-style navigation, and per-panel DevTools.

## Install

Release downloads are single self-contained binaries. On first launch, Libro extracts its bundled Electron runtime into the user cache directory and starts the desktop app.

[![Download Linux amd64](https://img.shields.io/badge/Linux-amd64-1f6feb?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-linux-amd64)
[![Download Linux arm64](https://img.shields.io/badge/Linux-arm64-1f6feb?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-linux-arm64)
[![Download macOS amd64](https://img.shields.io/badge/macOS-amd64-111827?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-darwin-amd64)
[![Download macOS arm64](https://img.shields.io/badge/macOS-arm64-111827?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-darwin-arm64)
[![Download Windows amd64](https://img.shields.io/badge/Windows-amd64-0ea5e9?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-windows-amd64.exe)

## Build From Source

Prerequisites:

- Go `1.26.x`
- Node.js `22.12+` + `npm`
- `git` if you want worktree integration

Run from the repo:

```bash
go run .
```

Desktop mode is the default. If local Electron is missing, Libro installs the repo's runtime dependencies with `npm install --no-fund --no-audit --omit=dev` and then launches Electron.

Useful entry points:

```bash
go run . --no-desktop
go run . --version
./install
```

`./install` builds Libro, installs it under `~/.local/share/libro`, refreshes the Electron app files used by repo-based installs, and creates a launcher symlink in `~/.local/bin`.

## Runtime Model

- Go server: serves the host UI over HTTP/WebSocket using [`g-sui`](https://github.com/michalCapo/g-sui)
- Electron shell: opens the host page in a frameless `BrowserWindow`
- Browser apps: render in native Electron `<webview>`s with a persistent `persist:libro` session
- Terminal apps: render native Go PTY sessions over WebSocket inside the strip
- Persistence: SQLite database plus a few settings stored in `libro.db`
- Fallback: if no Electron runtime is available, Libro opens the UI in the default browser

Native window-close requests are intentionally ignored. Quitting is routed through the `quit` command so Libro can flush session data cleanly.

## Data Locations

- Linux: `~/.local/share/libro/libro.db`
- macOS: `~/Library/Application Support/libro/libro.db`
- Windows: `%APPDATA%/libro/libro.db`

Bundled Electron runtimes extracted from release binaries are stored under the user cache directory, in a versioned `libro/desktop/...` path.

## Features

### Apps

- Browser apps support editable URLs, address popup, reload, and DevTools.
- Terminal apps default to the user's shell when no command is provided.
- Panels sit side by side at fixed widths. New panels default to MD (640px) without resizing existing panels. Change this in **Settings → Default panel width**; the preference is saved for all projects.
- Choose XS (320px), SM (480px), MD (640px), LG (960px), XL (1280px), or 2XL (1920px) from the panel toolbar. Existing 3XL and full-width options remain available.
- Scroll horizontally or use the previous/next panel buttons to reach panels outside the viewport. Browser viewport shortcuts remain available.

### Search And Commands

- `⌘ + O` opens the plugin launcher.
- `⌘ + ;` opens the command palette for app and project actions.

### Projects And Worktrees

- `home` is the default project.
- Projects are tied to directories.
- Each project keeps its own in-memory running strip while inactive projects stay hidden.
- Git repositories expose worktrees in the project picker.
- `⌘ + N` opens the project/worktree picker.
- `⌘ + G` creates a new worktree from the current branch.

### Browser Workflow

- Plain-key browser navigation is supported in normal mode (outside input fields, not in insert mode):
  - `o` open browser address popup
  - `r` reload page
  - `m` cycle selected browser through mobile `xs`, `sm`, `md`, and its previous size
  - `g / G` top / bottom
  - `j / k` scroll down / up
  - `h / l` scroll left / right
  - `i` enter insert mode; `Esc` exits insert mode

## Keyboard Shortcuts

### Apps

- `⌘ + O` open launcher/search popup on the right
- `⌘ + Enter` open terminal in Libro
- `⌘ + E` open `nvim` in Libro, falling back to `vim`; shows a notice if neither is installed
- `⌘/Win + Y` open Pi agent in Libro if available, using `MD` width
- `⌘ + ;` open command palette
- `⌘ + Q` close current app
- `⌘ + ,` decrease selected app width
- `⌘ + .` increase selected app width
- `⌘ + F` toggle full width
- `⌘ + +` zoom in
- `⌘ + -` zoom out
- `⌘ + 0` reset zoom
- `quit` quit Libro from the command palette

### Navigation

- `⌘ + H` select app to the left
- `⌘ + L` select app to the right
- `⌘ + [` move app left
- `⌘ + ]` move app right
- `⌘ + Enter` open terminal in Libro
- `⌘ + Ctrl + Y` move app to another project
- `⌘ + N` open project and worktree picker
- `⌘ + G` create worktree from current branch

### Browser

- `⌘ + B` new browser with URL popup
- `o` open URL/search popup (normal mode)
- `r` reload page (normal mode)

## Development Notes

- Terminal panels default to xterm.js WebGL rendering, forced Chromium GPU acceleration, and a performance-oriented scrollback. Useful knobs: set `LIBRO_FORCE_GPU=0` before launch to disable forced GPU mode, `localStorage.setItem('libro.terminal.scrollback', '1000')` to reduce terminal history, or `localStorage.setItem('libro.terminal.cursorBlink', '1')` to restore cursor blinking.
- Electron shortcuts are intercepted in the main process and forwarded to the host page so they still work while a webview has focus.
- Browser guest pages intentionally run with broad web compatibility; this is a desktop shell for arbitrary external sites, not a locked-down Electron app.
- Release builds embed both the Electron app files and a platform-specific Electron runtime zip.
- `./release` bumps the patch version, stages the embedded desktop payload, builds cross-platform binaries into `dist/`, tags the release, and publishes assets through GitHub CLI.
