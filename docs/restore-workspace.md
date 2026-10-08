# Restore Workspace on Start

Status: implemented. This was the design; see "Implementation status" at the end for what changed during review.

## Overview

Quitting Libro still kills every terminal, agent, and project command. That is fine.

Goal: when Libro starts again, the user sees the same layout they left:
- the same open projects and threads, and the same active workspace,
- the same panels in each workspace (dock, order, width, name, selection),
- agent panels started again with `--resume <session>`, the same agent, model, and reasoning level.

The user types "continue" in the agent to go on with the work.
Project commands (the application) are **not** started again. The user starts them when needed.

No PTY host, no detached processes, no output replay.

## What already exists

- Threads, their `session_id`, `agent_id`, and `agent_command` are in the DB (`internal/threads.go`, `internal/db.go:174`).
- `agentLaunch` (`internal/app.go:225`) builds a resume command with `components.ResumeAgentCommand` (`internal/components/agent_activity.go:532`).
- `projectAutolaunchJS` (`internal/app.go:255`) starts the thread agent when a thread has no agent panel.

What is lost on quit: everything in `sm.states` (`internal/state.go:56`): open workspaces, snapshots, panels, active project, transient projects.
Each page also starts with an empty workspace (`internal/app.go:1521`).

## Design

Save a layout snapshot to the `settings` table under the key `layout`. Restore it when a page opens.

```
sm.states[sid]  ──save (debounced)──▶  settings["layout"] (JSON)
                ◀──restore on page──
```

- One layout per Libro instance. The last saved page wins (matches "one window" use).
- Save is debounced (1 s) after any state change, plus once in `CleanupRuntime` before `tm.StopAll()`.
- Restore only puts panels into state. Terminals start lazily, the same way they do today when a workspace is shown.

## Database Changes

None. Use the existing `settings` table (`key = 'layout'`).

Thread rows get two more columns in the existing migration loop (`internal/db.go:175`):
- `agent_model TEXT NOT NULL DEFAULT ''`
- `agent_effort TEXT NOT NULL DEFAULT ''`

## Layout format (v1)

```go
type savedLayout struct {
    V             int
    ActiveProject string
    Open          []savedWorkspace // open workspaces, in sidebar order
    Transient     []Project        // session-only folder projects
}

type savedWorkspace struct {
    Name          string // project name or thread ID
    SelectedIndex int
    Apps          []savedApp
}

type savedApp struct {
    ID, PluginID, Dock, Name, Command, URL, IconURL string
    Type          AppType
    Width, PreviousWidth Width
    Writable      bool
    ApplicationPath string
    ApplicationPerThread bool
}
```

Not saved: `TerminalReady`, `TerminalID`, `ApplicationPort`. They are runtime state.

Rules:
- Unknown `V` -> ignore the layout, start empty.
- Only add fields.

## Code Changes

### New file `internal/layout_restore.go`

- `saveLayout(sid)`: read `sm.states[sid]` under `sm.mu.RLock`, build `savedLayout`, write JSON to `settings`.
  - Include the active workspace (`state.Apps`) and every `snapshots[name]` that is not in `closedWorkspaces`.
- `scheduleLayoutSave(sid)`: debounce timer (1 s) that calls `saveLayout`.
- `restoreLayout(sid)`: read `settings["layout"]`, then:
  - Re-add transient projects whose path still exists.
  - Drop workspaces whose project is gone, whose thread is missing or archived, or whose path no longer exists.
  - Put the active workspace into `state.Apps`, others into `snapshots`. Set `renderedProjects` for the active one.
  - Drop `project-command` panels. The user starts the application again (keeps port logic untouched).
  - Set `TerminalReady=false` on every terminal panel.
  - Raise `sm.nextID` above the highest restored `app-N`, so new panel IDs do not collide.

### `internal/state.go`

- Call `scheduleLayoutSave(sid)` from the mutating methods: `InsertApp`, `InsertTerminalPlaceholder`, remove/close app, `SwitchProject`, close workspace, move app, width change, `SetAppPlugin`, add/remove transient project.
  - If one central place exists (e.g. after each action), use that instead of many call sites.

### `internal/app.go`

- Page handler (`internal/app.go:1521`): after `sm.NewSession()`, call `restoreLayout(sid)` before `renderPage`.
- `CleanupRuntime` (`internal/app.go:493`): `saveLayout` for the last active session, then `tm.StopAll()` as today.
- Terminal start on show: when a workspace becomes visible (initial render and `SwitchProject`), start every terminal panel with `TerminalReady=false`:
  - agent panel -> `agentLaunch` (resume), the same path as today's restart (`internal/app.go:958`),
  - other terminals (shell, editor, plugins) -> run their `Command` fresh in the workspace path,
  - `project-command` -> never auto-started (already dropped on restore).
- Check how the terminal placeholder starts today (`InsertTerminalPlaceholder` + its start action) and reuse it. Do not add a second start path.
- `projectAutolaunchJS` stays. It only runs when a thread has no agent panel, so it does not double-start a restored agent.

### Agent model and reasoning level

`agent_command` keeps the flags from the plugin command, but not a model or level changed inside the agent (`/model` in Claude, `/model` in Codex).

- On each session report (`saveThreadSession`, `internal/threads.go:409`), also read the current model and level from the agent session files:
  - Codex: last `turn_context` in the rollout file under `CODEX_HOME/sessions` has `model` and `effort`.
  - Claude: last assistant message in `~/.claude/projects/<dir>/<session>.jsonl` has `model`. Level: read it if present, else leave empty.
  - Others (pi, opencode, ollama): leave empty.
- Save them to `agent_model` and `agent_effort`.
- `ResumeAgentCommand(command, sessionID, model, effort)` adds the flags only when the base command does not already set them:
  - Claude: `--model <m>`, `--effort <e>`.
  - Codex: `-c model="<m>" -c model_reasoning_effort="<e>"`.
- Put the file parsing in `internal/components/agent_activity.go` next to `RecoverCodexTitle`.

### Frontend

No change expected. Panels render from state as today. Verify that `renderPage` with a filled state shows the restored workspaces and sidebar open state.

### Docs

- `README.md`: one short line: "Libro reopens your workspaces and resumes agents on start. Applications start when you start them."

## Implementation Steps

1. `savedLayout` types, `saveLayout`, `restoreLayout` (no wiring). Run `make check`.
2. Wire save: debounce on state changes, save in `CleanupRuntime`.
3. Wire restore in the page handler, with the drop rules and the `nextID` bump.
4. Lazy terminal start for restored panels on show (reuse the existing start path).
5. Model and level: DB columns, read from session files, pass to `ResumeAgentCommand`.
6. Tests (real behavior, no string asserts):
   - save + restore round trip keeps workspaces, panel order, dock, width, selection.
   - archived thread, missing project, and missing path are dropped.
   - `project-command` panels are dropped.
   - `nextID` is above restored IDs.
   - Codex rollout and Claude jsonl fixtures -> correct model and level.
   - `ResumeAgentCommand` does not duplicate a `--model` already in the command.
7. Manual test on the dev instance:
   - Open two threads with Claude and Codex (change model and level inside each), a shell panel, a browser panel, and start the application.
   - Quit and reopen: same workspaces and panels. Agents resume the same session with the same model and level. The application is stopped until started.
   - Type "continue" in each agent: the conversation goes on.
8. `make check`, update README.

## Risks and edge cases

- **Multiple windows:** each page restores the same layout, and terminals with the same panel ID could clash.
  Restore only for the first page after backend start. Later pages start empty, as today.
- **Session file format changes:** model/level parsing is best effort. On failure, resume with the saved command only.
- **Agent with no session yet** (quit before the first prompt): the panel starts the agent fresh.
- **Shell panels** start a new shell. Running commands and scrollback are not kept.
- **Crash:** the debounced save means at most ~1 s of layout changes is lost.

## Implementation status

- [x] Layout persistence, debounce, shutdown flush, and first-page restoration.
- [x] Drop unavailable workspaces and project-command panels; reserve restored panel IDs.
- [x] Lazy terminal startup through the existing hydrate action.
- [x] Agent model/effort migration, session parsing, and resume flags.
- [x] Round-trip, drop-rule, shutdown, multi-window, worktree, and real PTY tests.
- [x] README startup behavior documented.
- [x] Manual restart on the dev instance (port 8101) with live Claude and Codex sessions:
  layout, browser URL, and panels restored; Claude resumed with `--model`/`--effort`,
  Codex with `-c model=… -c model_reasoning_effort=…`; the conversation continued.
- [x] Fix: readiness probes (Electron `isServerRunning`, curl) also `GET /` and took the
  one-time restore. Only requests that accept `text/html` restore now.

- [x] Fixes after review:
  - Only the page that restored the layout saves it. A second window cannot overwrite it.
  - A reload keeps its session (`?sid=` in the URL): same panels, same running terminals
    (old output is not replayed). Session IDs carry a random part, so a page from an earlier
    server run cannot take another window's session.
  - A closed workspace that gets a new panel is open again and saved.
  - Shutdown stops terminals first (their final session report lands), then saves.
  - Quit without a desktop process (the server keeps running) lets the next page restore again.
  - Concurrent first pages wait until restored panel IDs are reserved.
  - Managed child agent panels (no command) are not restored as shells.
  - Resume reads the model/effort from the agent session file at launch, so a model changed
    inside the agent survives a crash. The shutdown refresh is gone.
- [x] Fixes after the second review:
  - Terminal launches fail after shutdown starts, so no agent outlives quit. A panel whose
    start fails this way stays in the final layout.
  - Workspaces save their path. A thread whose worktree path changed is dropped on restore.
  - Existing `--model`/`--effort`/`-c` flags are found by splitting the command into shell
    words, so a prompt that mentions `--model` does not hide the saved model.
  - One page per session: opening a copied `?sid=` URL moves the older page to a new, empty
    session (no hash, so it does not start the same thread agent again).
- [x] Fixes after the third review:
  - Agent launches capture the terminal generation before slow work. A launch asked for
    before quit fails even if quit lets launches run again.
  - Not fixed: two tabs opening the same `?sid=` URL at the same moment both keep the session.
- [x] Fixes after the fourth review:
  - Child agent launches capture the generation too.
  - A hydrate that lost its panel stops only the terminal it started, not a newer one
    with the same panel ID.

Notes from the manual test:
- An archived thread that is still open is dropped on restore (spec rule).
- Flags already in the agent command win over the saved model/effort (spec rule).

The current project-thread flow creates virtual worktree projects instead of
thread rows. Layout panels therefore also save `SessionID`, `AgentModel`, and
`AgentEffort` for project agents. `WorktreeOrder` preserves the sidebar order.
Layout saves only serialize state. Later pages do not replace the saved layout owner.
