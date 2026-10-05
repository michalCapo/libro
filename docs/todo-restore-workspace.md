# Todo: Restore Workspace on Start

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
- [ ] Manual desktop restart with live Claude and Codex sessions. The managed application currently reports no URL; its URL/port must be configured before browser validation.

The current project-thread flow creates virtual worktree projects instead of
thread rows. Layout panels therefore also save `SessionID`, `AgentModel`, and
`AgentEffort` for project agents. `WorktreeOrder` preserves the sidebar order.
Normal layout saves only serialize state; shutdown refreshes agent settings
before the final save. Later empty pages do not replace the saved layout owner.
