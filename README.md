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

Flags must come before browser commands: `libro --dev browser list`.
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

### Browser automation for agents

**Settings → Browser control → Allow agents to control the browser** turns this
feature on or off. It is **On by default** and persists across restarts. Turning
it off immediately cancels browser work and managed downloads, stops diagnostics,
and blocks CLI/MCP commands. Enable it in Settings to allow browser control again.

Agents can inspect and operate their own thread's browser panels, using their
current pages and login sessions. The `open` action creates a panel in that thread
when needed. Libro does not launch another browser.
They can inspect accessibility snapshots, click and type, navigate, capture
screenshots, upload files, and manage agent-started downloads.

New Codex, Claude (including Ollama-launched Claude), and OpenCode sessions get
an automatically registered `libro_browser` MCP server with a `browser` tool.
Pi gets startup instructions for the same controls through the Libro CLI.
Restart an existing agent session to load this integration. Custom agents can
register `libro browser-mcp` as a stdio MCP server or use the CLI below.
Agent tool approval and sandbox settings still apply.

Libro panels use `libro_browser`, not Codex's `iab` or shared browser connection.
If an agent reports "Browser is not available: iab" or "No browser is available",
tell it to call the `libro_browser` tool with `{"action":"list"}`, or run
`libro browser list`. Those errors refer to a different browser connection.

The agent first uses `list`, chooses a panel ID, then sends actions with that ID.
If no panel exists, it uses `open` with an optional URL. Each thread has separate
panels, browser storage, and command queues. Agents can work at the same time.
Hidden panels remain controllable: commands do not switch threads or take focus
from your work. `visible` reports layout, not whether automation is available.
Use `select_panel` to reveal a panel when its thread is already shown, and `wait`
to wait for a page load. Missing or closed panels return an error.

Available actions:

- `open`: create a browser in the calling thread; optional `url` defaults to `about:blank`.
- `list`: panel IDs, session IDs, titles, URLs, and visibility.
- `status`, `pause`, `stop`: read control state, cancel queued work, or also cancel
  managed downloads. Only the user can resume via the toolbar.
- `select_panel`: select an existing panel in the current project.
- `snapshot`: accessibility roles, names, states, and stable element references.
  Use `format: "dom"` for DOM structure. References expire on navigation.
- `wait`: wait for `selector`/`ref` with `state` set to `visible`, `hidden`,
  `attached`, or `detached`; or an exact `url` with `interactive`/`complete`.
  `timeoutMs` defaults to 10000 and is limited to 20000.
- `diagnostics`: recent console warnings/errors and failed network requests,
  including HTTP errors. `clear: true` clears the returned history.
- `screenshot`: viewport PNG, returned directly as an MCP image. Use
  `fullPage: true` for the document or `ref`/`selector` for one element.
- `move`, `click`, `down`, `up`: mouse actions with viewport `x`, `y`; optional
  `button` (`left`, `middle`, `right`). Drag with down, move with the same button,
  then up. Mouse actions also accept a snapshot `ref` or unique CSS `selector`.
- `text`: insert `text` into the focused field, including Unicode.
- `key`: press `key`, such as `Tab`, `Enter`, or `Backspace`, with optional
  `modifiers` (`control`, `shift`, `alt`, `meta`).
- `scroll`: wheel at `x`, `y` with `deltaY` and optional `deltaX` (positive is up/left).
- `navigate`, `back`, `forward`, `reload`: navigate that same panel; navigate takes `url`.
- `select_option`: choose `values` (an array of option values) in a select located
  by `ref` or `selector`.
- `check`: set `checked` to true/false on a checkbox or true on a radio button.
- `upload`: set `files` (absolute paths) on a file input identified by `ref` or
  `selector`. An empty array clears the selection.
- `download`: fetch `url` with the panel's session into a unique folder under
  `Downloads/Libro`. An optional `filename` controls its name. Returns its ID
  and path without opening a save dialog.
- `downloads`, `cancel_download`: inspect download progress or cancel by `downloadId`.

Coordinates and screenshots use viewport CSS pixels, including when the page
is zoomed. Mouse actions display a blue **Agent** pointer for five seconds.
It does not move the user's pointer or block page interaction. Commands are
serialized, and keyboard input focuses the target panel.

CLI examples (replace `PANEL_ID` with an ID returned by `list`):

```sh
libro browser list
libro browser '{"action":"click","panel":"PANEL_ID","x":120,"y":80}'
libro browser '{"action":"text","panel":"PANEL_ID","text":"Hello"}'
libro browser '{"action":"key","panel":"PANEL_ID","key":"Enter"}'
libro browser '{"action":"screenshot","panel":"PANEL_ID"}' /tmp/page.png
```

The desktop bridge listens on loopback with a per-launch authentication token
stored in the user's Libro config directory. No browser control is exposed to
web pages. Browser-only (`--no-desktop`) mode cannot provide these controls.
No global agent config or project instruction files are changed.

Use the pause button beside the browser console button to pause all agent browser
control. It changes to a play button for resuming. The browser actions menu also
has **Stop agent browser work**, which cancels managed downloads. Pausing cancels
pending waits and queued commands while leaving normal user input available.
Agents cannot resume a user-paused browser.

Element lookup covers the main document and open shadow roots. Cross-origin
iframe controls are not yet exposed. Snapshots are capped at 1000 nodes and omit
input values. Diagnostics keep the last 200 entries per category, without request
headers or bodies; network capture begins when browser control first connects.
Call diagnostics, then reload, to capture startup failures. Advanced controls
use Electron's debugger connection and may need reconnecting after DevTools opens.
Full-page and element screenshots are limited to 24 megapixels and 16000 pixels
per side. Very large captures should be narrowed to an element.

Examples of the additional controls:

```sh
libro browser '{"action":"snapshot","panel":"PANEL_ID"}'
libro browser '{"action":"wait","panel":"PANEL_ID","selector":"#save","state":"visible"}'
libro browser '{"action":"check","panel":"PANEL_ID","selector":"#agree","checked":true}'
libro browser '{"action":"screenshot","panel":"PANEL_ID","fullPage":true}' /tmp/full-page.png
libro browser '{"action":"download","panel":"PANEL_ID","url":"https://example.com/report.csv"}'
```

### Agent application control

Agents can use the `application` tool on the existing `libro_browser` MCP
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
Only the saved command can be run. This uses the desktop browser bridge and
respects its enable and pause controls.

### Agent issue management

The `issues` tool on the existing `libro_browser` MCP server supports:

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
