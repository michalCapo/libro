# Libro

Libro is a desktop app for working with AI coding assistants. Run Codex, Claude, Pi, OpenCode, or any other agent in separate threads, with a browser, your files, and terminals right next to each one.

Think of it as one desk for all your AI helpers. Each one gets its own thread and tool state.

![Libro home screen with the agent launcher open](demo/libro-home.png)

## What It Does

- Runs several AI coding assistants at the same time in separate threads.
- Puts a browser, Issues, file manager, terminal, and more right next to them.
- Keeps all your projects in one place. Switch between them with one click.
- Shows when an assistant is busy (its icon spins) or finished (green check).
- Plays a sound when an assistant finishes its work.
- Strong keyboard support — nearly every action has a shortcut.

## Install

1. Download the file for your system below.
2. Run it. Libro sets itself up on first start. Nothing else to install.

[![Download Linux amd64](https://img.shields.io/badge/Linux-amd64-1f6feb?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-linux-amd64)
[![Download Linux arm64](https://img.shields.io/badge/Linux-arm64-1f6feb?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-linux-arm64)
[![Download macOS amd64](https://img.shields.io/badge/macOS-amd64-111827?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-darwin-amd64)
[![Download macOS arm64](https://img.shields.io/badge/macOS-arm64-111827?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-darwin-arm64)
[![Download Windows amd64](https://img.shields.io/badge/Windows-amd64-0ea5e9?style=for-the-badge)](https://github.com/michalCapo/libro/releases/latest/download/libro-windows-amd64.exe)

Works on Linux, macOS, and Windows.

Note: Libro starts the assistants for you, but the assistants themselves (Codex, Claude, Pi, OpenCode) must be installed on your computer first.

## Development instances

Run `make dev` (or `go run . --dev`) while your installed Libro stays open.
The development instance uses port 8101 and starts with its own empty database,
settings, notes, and browser profile. Its data persists across restarts.
The regular instance keeps port 8100 and its existing data.

For additional instances, choose a unique name and unused port:

```sh
go run . --instance feature-a --port 8102
go run . --dev --no-desktop
```

Named instance data lives under `libro/instances/<name>` in the usual data and
Electron profile directories. Project folders remain shared if you add the same
folder to both instances; edits and commands still affect those files.

Flags must come before commands: `libro --dev application status`.
`LIBRO_INSTANCE` and `LIBRO_PORT` also select an instance and are inherited by
agents and terminals launched inside Libro. An occupied port fails before the
app opens a window or database.

The notes editor is bundled locally into the Go binary. After changing
`internal/note-editor.js`, run `npm ci` and `npm run build:notes`, and include the
updated `internal/note-editor.bundle.js`. `npm run check:notes` checks that the
bundle matches its source. Run the editor integration tests with
`xvfb-run -a node_modules/.bin/electron --no-sandbox --ozone-platform=x11 electron/notes.integration.cjs`.

## Features

### AI assistants

- Each project thread has one agent and its own browser and tool state. Starting another agent creates a new thread under the same project. Issues and the running application are shared across that project. Standalone threads are unchanged.
- Use whatever agent you like. Name it, add its start command, and it shows up next to the built-in ones.

![Agent commands in Settings: named agents with their start commands](demo/agents-settings.png)
- Choose the width of each panel, from small to full width. Press `Ctrl + M` to make a panel full width and back.
- Hidden panels keep running. Switch away and come back without losing anything.

### Tools

- Open the tool you need next to your assistants: Terminal, Browser, Issues, Files, Nvim, Git, Database, or any tool of your own.
- Use whatever tool you like. Name it, add its command or web address, and it shows up next to the built-in tools.

![Tools in Settings: named tools with their commands and shortcuts](demo/tools-settings.png)
- The terminal at the bottom runs your project's start command. Start it again with `Ctrl + Shift + R`, stop it with `Ctrl + Shift + T`.
- In Files, press `Backspace` to go up one folder.

### Projects

- A project is a folder on your computer. Add as many as you like.
- Switch projects with `Ctrl + P`, or with `Ctrl + 1` to `Ctrl + 9`.
- Project-backed threads keep the project folder as their working directory.
- Each thread remembers its own browser, files, terminal, and other thread-local tools. Threads in the same project share Issues and the running project start/stop application.

![Project sidebar: projects with their running assistants listed underneath](demo/projects-sidebar.png)

### Browser

- Open a browser panel with `Ctrl + B`, or a new separate one with `Ctrl + Shift + B`.
- Browse with the keyboard: `o` opens a page, `r` reloads, `j` and `k` scroll, `i` lets you type in a page, `Esc` goes back.
- Ask the agent about part of a page: press `a` and click an element, or `d` and draw a box around it. Type a short note and it is pasted into the agent, together with that part of the page.

![Browser panel open next to other tools](demo/browser-panel.png)

### Issues

- Open Issues with `Ctrl + I`. Issues are stored separately for each project.
- Start with the Open list. Switch the filter to see Archived notes or all notes.
- Edit formatted text in one area using Markdown shortcuts or the formatting toolbar.
- Paste a PNG, JPEG, or GIF screenshot at the cursor, or use Insert image. Images stay in place between paragraphs when saved. Select an image and press Backspace or Delete to remove it.
- Save keeps the editor open. Cancel discards unsaved changes. Send a saved note to the selected agent to execute it, with local paths to any attached images.

### Settings

- Light, dark, or follow your system theme.
- Play a sound when an assistant finishes.
- Set the default panel size for new panels.
- Choose one agent to start on its own when you open a project that has no agents open.
- Choose if browser prompts run right away, or are only pasted in.
- Change how assistants and tools start, rename them, add your own.
- Change any keyboard shortcut and restore the defaults anytime.

![Settings page: theme and notification options](demo/settings.png)

## Keyboard Shortcuts

### Agents and projects

| Shortcut | Action |
| --- | --- |
| `Ctrl + N` | New assistant |
| `Ctrl + A` | Jump to the current assistant |
| `Ctrl + P` | Pick a project |
| `Ctrl + 1` – `Ctrl + 9` | Go to a project |
| `Ctrl + Shift + P` | Show or hide the project sidebar |

### Panels and tools

| Shortcut | Action |
| --- | --- |
| `Ctrl + T` | Terminal |
| `Ctrl + B` | Browser |
| `Ctrl + Shift + B` | New browser panel |
| `Ctrl + F` | Files |
| `Ctrl + I` | Issues |
| `Ctrl + E` | Nvim |
| `Ctrl + G` | Git |
| `Ctrl + D` | Database |
| `Ctrl + Q` | Close the selected panel |
| `Ctrl + ,` / `Ctrl + Shift + .` | Bigger / smaller panel |
| `Ctrl + M` | Full width on or off |
| `Ctrl + H` / `Ctrl + L` | Previous / next panel |
| `` Ctrl + ` `` | Bottom terminal |
| `Ctrl + ;` | Search all commands |
| `Ctrl + .` | Settings |
| `Ctrl + =` / `Ctrl + -` / `Ctrl + 0` | Zoom in / out / reset |

Every shortcut can be changed in **Settings → Keyboard shortcuts**.

### Agent tools

New Codex, Claude (including Ollama-launched Claude), and OpenCode sessions
register the `libro` MCP server automatically. It provides `application` and
`issues` tools. Pi receives instructions for the equivalent CLI commands.
Custom agents can register `libro mcp` as a stdio MCP server.
Agents also receive instructions to use the separately installed `agent-browser`
CLI for browser testing, with a unique session per agent/thread. Libro does not
install agent-browser or share its browser panel logins with it.
Restart existing agent sessions to load the updated tools and instructions.

### Agent application control

Agents can use the `application` tool on the `libro` MCP
server to start, restart, stop, or check the project application. Set the
**Start command** in project settings first. The tool uses the same bottom
terminal as Libro's application shortcuts and leaves agent terminals running.

For agents without MCP:

```sh
libro application status
libro application start
libro application restart
libro application stop
```

The project path defaults to the agent's working directory. Pass an explicit
path as the second argument (or `project` in MCP) if needed. Libro resolves the
closest registered project root, including subdirectories and symlinked paths.
Status checks work without changing the visible project. Lifecycle actions select
the matching project when needed. `start` preserves a running or starting application;
`restart` replaces it. A `starting` response confirms a launch request, while
`status` reports whether the process is running; it does not test server readiness.
Only the saved command can be run. Commands use the local desktop bridge.

### Agent issue management

The `issues` tool on the `libro` MCP server supports:

- `list`: issue summaries, with optional `status`, `limit` (default 100, max 200), and `offset`.
- `read`: the full issue, including Markdown body and saved images, by `id`.
- `create`: requires `title`; accepts `body` and `status`.
- `set_status`: requires `id` and `status`; preserves the description and images.
- `delete`: permanently removes the issue by `id`.

Statuses are `new` (Open) and `archived`. Create defaults to `new`.
Use full issue IDs from `list` or `create`. Read/create results use `state`
for the saved status, matching Libro's issue storage.

The project defaults to the agent's working directory, or accepts an explicit
`project` path. That project must be active in Libro. No Issues or browser
panel needs to be open. This uses the same desktop bridge and enable/pause
controls as application control. Restart existing agent sessions to discover
the new tool. Open issue lists refresh after agent changes; unsaved editor
text is preserved.

CLI fallback:

```sh
libro issues '{"action":"list"}'
libro issues '{"action":"create","title":"Fix login","body":"Steps to reproduce"}'
libro issues '{"action":"read","id":"ISSUE_ID"}'
libro issues '{"action":"set_status","id":"ISSUE_ID","status":"archived"}'
libro issues '{"action":"delete","id":"ISSUE_ID"}'
```
