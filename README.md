# Libro

Libro is a Go + Electron desktop workspace for CLI coding agents. Run Codex, Pi, Claude, or OpenCode as side-by-side agent panels, with browser, editor, and repository tools in a right sidebar and one shell terminal per project at the bottom. Switch projects on the left.

Open **New agent session** to launch an agent, or **Apps & plugins** to open a tool. Open **Commands** for the command palette. Panel widths are fixed, so a new panel never resizes the others — scroll the strip horizontally to reach panels outside the viewport.

Tools and agents use a small local plugin manifest. See [Creating plugins](docs/plugins.md) for the API and examples.

![Libro empty state with agent launcher](demo/libro-home.png)

## What It Does

- Runs agent panels (Codex, Pi, Claude, OpenCode) side by side in one desktop window.
- Hosts tools in the right sidebar: Terminal, Browser, Files, Nvim, Git, Database.
- Keeps per-project panel state in memory while you switch between projects.
- Saves reusable app definitions in SQLite, either globally or per project.
- Integrates Git worktrees into the project picker.
- Tracks agent state: project icons spin while any agent works and turn green when all agents finish.
- Remappable shortcuts for every tool in **Settings → Keyboard shortcuts**.

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

Native window-close requests are intentionally ignored. Quitting is routed through the `quit` command. Libro asks before closing running panels, stops terminal processes and their child jobs, then closes the desktop window. Open panels are not saved or restored; launching or reloading Libro starts with an empty workspace. Projects, settings, and browser cookies are still saved.

## Data Locations

- Linux: `~/.local/share/libro/libro.db`
- macOS: `~/Library/Application Support/libro/libro.db`
- Windows: `%APPDATA%/libro/libro.db`

Bundled Electron runtimes extracted from release binaries are stored under the user cache directory, in a versioned `libro/desktop/...` path.

## Features

### Panels

- Center panels run CLI agents; right sidebar tools are Terminal, Browser, Files, Nvim, Git, and Database; the bottom panel is one shell terminal per project.
- Panels sit side by side at fixed widths. New panels default to MD (640px) without resizing existing panels. Change this in **Settings → Default panel width**; the preference is saved for all projects.
- Choose XS (320px), SM (480px), MD (640px), LG (960px), XL (1280px), or 2XL (1920px) from the panel toolbar. Existing 3XL and full-width options remain available.
- Scroll horizontally or use the previous/next panel buttons to reach panels outside the viewport.
- Tabs and hidden panels keep their sessions alive. Closing a terminal stops its PTY.

### Search And Commands

- `Ctrl + N` opens the agent launcher.
- `Ctrl + ;` opens the command palette for workspace, project, and panel commands.
- `` Ctrl + ` `` toggles the bottom terminal.
- `Ctrl + A` focuses the current agent, or opens the agent launcher when none is open.

### Projects And Worktrees

- `home` is the default project.
- Projects are tied to directories.
- Each project keeps its own in-memory running strip while inactive projects stay hidden.
- Git repositories expose worktrees in the project picker.
- `Ctrl + P` opens the project/worktree picker.
- **Commands → New worktree from current branch** creates a worktree.
- Project and worktree icons turn blue and spin while any agent is working, then show a green check when all active agents finish. Codex, Pi, Claude, and OpenCode use lifecycle signals, not terminal inactivity. Launch commands must start with the agent executable or `ollama launch claude` (with optional `--model`/`--yes` flags and Claude arguments after `--`); shell aliases and other wrapper scripts are not automatically instrumented. Codex requires terminal-title `run-state` support. Pi and OpenCode must allow the launch-local extension/plugin, and Claude must allow session hooks.

### Browser Workflow

- Plain-key browser navigation is supported in normal mode (outside input fields, not in insert mode):
  - `o` open browser address popup
  - `r` reload page
  - `m` cycle viewport width (normal → SM → MD → XL); `M` rotate portrait/landscape
  - `g` top, `Shift + G` bottom
  - `j / k` scroll down / up
  - `h / l` scroll left / right
  - `i` enter insert mode; `Esc` exits insert mode

## Keyboard Shortcuts

Tool shortcuts are remappable in **Settings → Keyboard shortcuts**. Use Ctrl, Alt, or Meta with a letter, number, `,`, `.`, `=`, or `-`.

### Tools

- `Ctrl + T` toggle Terminal
- `Ctrl + B` toggle Browser
- `Ctrl + F` toggle Files
- `Ctrl + E` toggle Nvim
- `Ctrl + G` toggle Git
- `Ctrl + D` toggle Database
- `Ctrl + Q` close the selected panel
- `Ctrl + ,` / `Ctrl + .` decrease / increase selected panel size
- `Ctrl + P` open the project/worktree picker
- `Ctrl + N` open the agent launcher
- `Ctrl + H` / `Ctrl + L` select the previous / next panel in the active project, including the visible tool when it fits beside the agents (wraps at either end)
- `Ctrl + Shift + P` toggle the project sidebar
- `Ctrl + =` / `Ctrl + -` / `Ctrl + 0` zoom in / out / reset

### Workspace

- `` Ctrl + ` `` toggle the bottom terminal
- `Ctrl + A` focus the current agent, or open the agent launcher when none is open
- `Ctrl + ;` open command palette
- **Commands → Quit Libro** quits cleanly

## Development Notes

- Terminal panels default to xterm.js WebGL rendering, forced Chromium GPU acceleration, and a performance-oriented scrollback. Useful knobs: set `LIBRO_FORCE_GPU=0` before launch to disable forced GPU mode, `localStorage.setItem('libro.terminal.scrollback', '1000')` to reduce terminal history, or `localStorage.setItem('libro.terminal.cursorBlink', '1')` to restore cursor blinking.
- Electron shortcuts are intercepted in the main process and forwarded to the host page so they still work while a webview has focus.
- Browser guest pages intentionally run with broad web compatibility; this is a desktop shell for arbitrary external sites, not a locked-down Electron app.
- Release builds embed both the Electron app files and a platform-specific Electron runtime zip.
- `./release` bumps the patch version, stages the embedded desktop payload, builds cross-platform binaries into `dist/`, tags the release, and publishes assets through GitHub CLI.