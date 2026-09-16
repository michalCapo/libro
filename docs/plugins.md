# Libro plugins

Plugins describe a terminal command or browser app. Libro owns the PTY, webview, tabs, resizing, and project lifecycle. No build step or JavaScript is required.

Create `<Libro data directory>/plugins/<plugin-id>/plugin.json`, then restart Libro.

Data directories:

- Linux: `~/.local/share/libro` (or `$XDG_DATA_HOME/libro`).
- macOS: `~/Library/Application Support/libro`.
- Windows: `%APPDATA%/libro`.

For a CLI agent:

```json
{
  "id": "my-agent",
  "name": "My agent",
  "type": "terminal",
  "command": "my-agent --interactive",
  "dock": "center",
  "description": "My local coding agent"
}
```

For a local browser tool:

```json
{
  "id": "preview",
  "name": "Local preview",
  "type": "url",
  "url": "http://localhost:3000",
  "dock": "right",
  "description": "Preview the running app"
}
```

`id`, `name`, `type`, and `dock` are required. IDs use lowercase letters, numbers, dots, underscores, and hyphens. `type` is `terminal` or `url`. `dock` is `center`, `right`, or `bottom`. An empty terminal command starts your default shell. An empty browser URL opens a blank browser tab with an address field.

Terminal commands run in the active project's directory. Install their executables separately and make them available on Libro's PATH. Commands run with your local user permissions; install manifests you trust. Plugin installation does not run commands. Opening the plugin does.

Open **Apps & plugins** in the project sidebar to launch an installed plugin. The plus button in a panel opens it in that panel. Use a tab group's position selector to move its active app without restarting it. Each launch creates an independent instance.

Built-ins: Codex, Pi, Claude, OpenCode, Terminal, Browser, Files, Nvim, Git (`lazyrepo`), and Database (`lazydata`). `center` dock plugins are CLI agents and must define a command; `right` and `bottom` plugins are tools. Duplicate built-in IDs and invalid manifests are skipped with a server log message. Plugins are read at startup.

This manifest API covers CLI and web apps. It does not load arbitrary renderer scripts or compiled Go extensions. Add functionality through a CLI or a web app and let Libro host it.

Tabs and collapsed panels keep their sessions alive. Project switching keeps all project sessions mounted. Closing a terminal stops its PTY and child processes. Quitting asks for confirmation when panels are open and finishes terminal cleanup before closing. Open panels are never saved or restored across launches or page reloads. Browser cookies use Electron's persistent `persist:libro` partition.

In web mode (`--no-desktop`), browser plugins use iframes. Sites that block embedding cannot load there. Use **Open in new tab**, or run Libro in desktop mode (`go run .`) to browse inside the panel. Local previews still work when their server allows embedding.
