# Libro

Libro is a Go + Electron desktop workspace for CLI coding agents. Run Codex, Pi, Claude, or OpenCode as side-by-side agent panels, with browser, editor, and repository tools in the right dock and project terminals in the bottom dock. Switch projects on the left.

Open **New agent session** to launch an agent, or **Apps & plugins** to open a tool. Open **Commands** for the command palette. Panel widths are fixed, so a new panel never resizes the others — scroll the strip horizontally to reach panels outside the viewport.

Tools and agents use a small local plugin manifest. See [Creating plugins](docs/plugins.md) for the API and examples.

![Libro empty state with agent launcher](demo/libro-home.png)

## What It Does

- Runs agent panels (Codex, Pi, Claude, OpenCode) side by side in one desktop window.
- Hosts tools in the right dock: Terminal, Browser, Files, Nvim, Git, Database.
- Keeps per-project panel state in memory while you switch between projects.
- Saves projects, panel defaults, agent and tool configuration, shortcuts, and project start commands in SQLite.
- Integrates Git worktrees into the project picker.
- Tracks agent state: project icons spin while any agent works and turn green when all agents finish.
- Remappable tool shortcuts in **Settings → Tools** and workspace shortcuts in **Settings → Keyboard shortcuts**.

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

Close the desktop window or use the operating system’s quit action to quit Libro. Libro asks before closing running panels, stops terminal processes and their child jobs, then closes the desktop window. Open panels are not saved or restored; launching or reloading Libro starts with an empty workspace. Projects, settings, and browser cookies are still saved.

## Data Locations

- Linux: `~/.local/share/libro/libro.db`
- macOS: `~/Library/Application Support/libro/libro.db`
- Windows: `%APPDATA%/libro/libro.db`

Bundled Electron runtimes extracted from release binaries are stored under the user cache directory, in a versioned `libro/desktop/...` path.

## Features

### Panels

- Center panels run CLI agents; the right dock hosts Terminal, Browser, Files, Nvim, Git, Database, and custom tools; the bottom dock holds the project shell and its start command.
- A shared tab row lists running agents and tools. All center agents remain visible side by side. One right-dock tool is visible at a time; selecting another tool switches the visible tab without stopping its session. On a narrow workspace, the selected tool overlays the agents.
- Panels sit side by side at fixed widths. New agents default to MD (640px) and new tools to LG (960px). Change these separately in **Settings → Panels → Agent panel width / Tool panel width**. Both preferences apply across projects; existing panels keep their widths.
- Choose XS (320px), SM (480px), MD (640px), LG (960px), XL (1280px), or 2XL (1920px) from the panel toolbar. 3XL (2560px) and MAX (full strip width) remain available as steps beyond 2XL. `Ctrl + M` toggles the selected panel between its current size and MAX.
- Scroll horizontally or use the previous/next panel buttons to reach panels outside the viewport.
- Tabs and hidden panels keep their sessions alive. Closing a terminal stops its PTY.

### Search And Commands

- `Ctrl + N` opens the agent launcher.
- `Ctrl + ;` opens the command palette for workspace, project, and panel commands.
- `` Ctrl + ` `` toggles the bottom terminal.
- The gear beside a project's remove button saves its start command, such as `air` or `bun src/dev.ts`. Commands run in that project folder. `Ctrl + Shift + R` starts or restarts the command; `Ctrl + Shift + T` stops it and closes its terminal. Output and interactive input appear in the bottom terminal, where `Ctrl + C` can also interrupt the command. An existing shell stays running in a separate tab.
- `Ctrl + A` focuses the current agent, or opens the agent launcher when none is open.

### Projects And Worktrees

- Libro opens the first saved project. A fresh install starts with no project until you add or open one.
- Projects are tied to directories.
- Each project keeps its own in-memory running strip while inactive projects stay hidden.
- Running agent sessions appear beneath their project in the sidebar. Select one to switch directly to that project and agent.
- Git repositories expose worktrees in the project picker.
- `Ctrl + P` opens the project/worktree picker.
- **Commands → New worktree from current branch** creates a worktree.
- Project and worktree icons turn blue and spin while any agent is working, then show a green check when all active agents finish. Codex, Pi, Claude, and OpenCode use lifecycle signals, not terminal inactivity. Launch commands must start with the agent executable or `ollama launch claude` (with optional `--model`/`--yes` flags and Claude arguments after `--`); shell aliases and other wrapper scripts are not automatically instrumented. Codex requires terminal-title `run-state` support. Pi and OpenCode must allow the launch-local extension/plugin, and Claude must allow session hooks.

### Browser Workflow

- `Ctrl + Shift + B` opens a new independent browser panel. `Ctrl + [` and `Ctrl + ]` select the previous or next browser panel and wrap at either end. `Ctrl + B` toggles the current browser.
- The address popup keeps up to 200 recently visited addresses for 30 days in local browser storage.
- Plain-key browser navigation is supported in normal mode (outside input fields, not in insert mode):
  - `o` open browser address popup
  - `r` reload page
  - `m` cycle viewport width (normal → SM → MD → XL); `M` rotate portrait/landscape
  - `g` top, `Shift + G` bottom
  - `j / k` scroll down / up
  - `h / l` scroll left / right
  - `i` enter insert mode; `Esc` exits insert mode

### Settings

**Settings** in the project sidebar opens a workspace settings page:

- **Theme**: Auto (follows the OS), Light, or Dark. Changes apply immediately.
- **Agent done sound**: plays a short notification whenever an agent finishes, in any project.
- **Panels**: default agent and tool panel widths for newly opened panels.
- **Agent commands**: edit launch commands, rename agents, disable or remove them from the launcher, and add custom agents. One enabled agent can be set to autolaunch when a project opens with no agent panels; leave all unchecked to choose manually. Running sessions are unchanged.
- **Tools**: edit commands for Nvim, Git, and Database; disable or remove tools; and add custom CLI tools. Changes apply to new sessions.
- **Keyboard shortcuts**: remap workspace shortcuts and restore defaults.

## Keyboard Shortcuts

Nvim, Git, Database, custom tool, and website shortcuts are remappable in **Settings → Tools**. Use Ctrl, Alt, or Meta with a letter, number, `[`, `]`, `,`, `.`, `=`, or `-`.

### Tools

- `Ctrl + T` toggle Terminal
- `Ctrl + B` toggle Browser
- `Ctrl + Shift + B` open a new browser panel
- `Ctrl + [` / `Ctrl + ]` select the previous / next browser panel
- `Ctrl + F` toggle Files
- `Ctrl + E` toggle Nvim
- `Ctrl + G` toggle Git
- `Ctrl + D` toggle Database
- `Ctrl + Q` close the selected panel
- `Ctrl + Shift + Q` close the current project and stop its running panels and terminals
- `Ctrl + .` decreases the selected panel size by one step; `Ctrl + ,` increases it. At the smallest or largest allowed size, the panel stays at that size; it never wraps around. Remap these under **Settings → Keyboard shortcuts → Decrease panel size / Increase panel size**, then click **Save shortcuts**.
- `Ctrl + M` toggle the selected panel between its current size and MAX (full strip width)
- `Ctrl + 1` – `Ctrl + 9` switch between the first nine projects that have open panels
- `Ctrl + P` open the project/worktree picker
- `Ctrl + N` open the agent launcher
- `Ctrl + H` / `Ctrl + L` select the previous / next panel in the active project, including the visible tool when it fits beside the agents (wraps at either end)
- `Ctrl + Shift + P` toggle the project sidebar
- `Ctrl + =` / `Ctrl + -` / `Ctrl + 0` zoom in / out / reset

### Workspace

- `` Ctrl + ` `` toggle the bottom terminal
- `Ctrl + A` focus the current agent, or open the agent launcher when none is open
- `Ctrl + ;` open command palette
- **Close window / system Quit** shows open panels by project and quits cleanly after confirmation

## Development Notes

- Terminal panels default to xterm.js WebGL rendering, forced Chromium GPU acceleration, and a performance-oriented scrollback. Useful knobs: set `LIBRO_FORCE_GPU=0` before launch to disable forced GPU mode, `localStorage.setItem('libro.terminal.scrollback', '1000')` to reduce terminal history, or `localStorage.setItem('libro.terminal.cursorBlink', '1')` to restore cursor blinking.
- Electron shortcuts are intercepted in the main process and forwarded to the host page so they still work while a webview has focus.
- Browser guest pages intentionally run with broad web compatibility; this is a desktop shell for arbitrary external sites, not a locked-down Electron app.
- Release builds embed both the Electron app files and a platform-specific Electron runtime zip.
- `./release` bumps the patch version, stages the embedded desktop payload, builds cross-platform binaries into `dist/`, tags the release, and publishes assets through GitHub CLI.
