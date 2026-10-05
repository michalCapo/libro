# Todo: Persistent Terminals (PTY host)

## Overview

Today quitting Libro kills every terminal, agent, and project command.
Goal: the user can quit Libro, start it again, and find all running terminals
in their workspaces with recent output.

The quit dialog gets two choices:
- **Quit, keep running** (default button). Terminals keep running.
- **Quit and stop all**. Same as today: everything is killed.

Terminals that no longer belong to any workspace are killed automatically.

Unix (Linux, macOS) only. Windows keeps today's in-process behavior.

### Why the current design can't do it

- Electron starts `libro --no-desktop` (`electron/main.js:472`) and stops it with SIGTERM (`electron/main.js:491`).
- SIGTERM runs `CleanupRuntime` -> `tm.StopAll()` (`internal/app.go:493`, `internal/app.go:501`).
- Even without `StopAll`, the backend holds every PTY master (`internal/components/terminal.go:216`).
  When the backend exits, the kernel hangs up the PTY and the shells get SIGHUP.

So PTYs must be owned by a separate long-lived process: the **PTY host**.

### Design

```
Electron ──http/ws──▶ libro backend (restarts) ──unix socket──▶ libro pty-host (long-lived)
                         agent activity, UI state                 PTYs, ring buffer, metadata
```

- `libro pty-host` is a hidden subcommand of the same binary.
- The backend starts it on demand, detached with `Setsid`, so Electron, Ctrl+C, and `air` restarts don't reach it.
- The host is kept simple. It owns the PTY, the process, an output ring buffer, opaque metadata, and the managed log file.
- Everything smart stays in the backend: agent status, titles, session IDs, process-status, and UI state.
  After a restart, the backend rebuilds that state from the metadata and from the agent activity files.
- The host exits when it has no sessions and no backend connection for 30 s.

## Database Changes

None. Panel metadata lives in the host, next to the process it describes.
That means the restore list can never disagree with the set of live processes.

## Host protocol (v1)

Socket: `$XDG_RUNTIME_DIR/libro/<instance|default>/pty-host.sock`.
Fallback: `<libroDataDir>/pty-host.sock`.
- The directory is `0700` and the socket is `0600`.
- On Linux, also check `SO_PEERCRED` uid. On macOS, use `getpeereid`.
- Instances (`--dev`, `LIBRO_INSTANCE`) get separate hosts.

Each connection starts with one JSON request line and gets one JSON response line.

| op | request | response |
|---|---|---|
| `hello` | `{v}` | `{v, pid}` |
| `list` | – | `[{id, pid, meta, cols, rows, started}]` |
| `spawn` | `{id, argv, env, cwd, cols, rows, meta, cleanup[], managed?{logPath, statePath, secrets[]}}` | `{pid}` |
| `meta` | `{id, meta}` | `{}` |
| `kill` | `{id}` | `{code}` after the process is reaped |
| `killall` | – | `{}`; the host exits afterwards |
| `attach` | `{id, replay}` | `{}`, then the connection switches to frames |

Frames after `attach`: `[type byte][len uint32 BE][payload]`.
- Host -> backend: `o` output, `x` exit (`{"code":n}`).
- Backend -> host: `i` input, `r` resize (`cols uint16, rows uint16`).
- Only one attachment per session. A new `attach` closes the old one (this covers a crashed backend).
- With `replay`, the host first sends the ring buffer (1 MiB per session), cut at the first `\n` so a partial escape sequence is not replayed.
- Writes to the backend use a 5 s deadline. On error the host drops the attachment and keeps reading the PTY into the ring.

Rules:
- Only add fields. Never change the meaning of a field.
- A breaking change needs a new socket name (`pty-host-v2.sock`). Sessions on an old host are then orphans.
- The host spawns with the env sent by the backend, never its own env.
- On process exit, the host removes the `cleanup` paths (agent activity temp dirs). This prevents leaks while the backend is down.
- For managed attempts, the host writes the redacted log. If no backend is attached at exit, it also writes the final `statePath` JSON with the exit code.

## Code Changes

### Backend: PTY host (new files)

- `internal/components/ptyhost_unix.go`
  - Host server: `RunPTYHost(socketPath string) error` with the ops above.
  - Moves `killTerminalProcess` usage here for `kill` and `killall`.
  - Idle exit timer.
- `internal/components/ptyhost_client_unix.go`
  - `hostClient`: `ensure()` connects or starts the host and waits for the socket (2 s), plus `list`, `spawn`, `attach`, `kill`, `killall`, `setMeta`.
  - `hostProcess` implements `ptyProcess` (below) over an attached connection.
- `internal/components/ptyhost_windows.go` – `newPTYProcess` always returns the local implementation. No host.
- `internal/components/ptyhost_test.go` – real tests only (see Steps).

### Backend: terminal manager refactor

`internal/components/terminal.go`
- Add a small interface so the session code doesn't care where the PTY lives:
  ```go
  type ptyProcess interface {
      io.ReadWriter          // output / input
      Resize(cols, rows uint16) error
      Pid() int
      Wait() int             // exit code
      Kill()                 // kill tree, wait
      Detach()               // drop connection, process keeps running
  }
  ```
  - `localProcess` wraps today's `exec.Cmd` + `*os.File`. It is used on Windows and as a fallback when the host can't start (log a warning).
  - `hostProcess` is the Unix default.
- `TerminalSession`: replace `cmd *exec.Cmd` and `ptyFile *os.File` with `proc ptyProcess`.
  - Line 216 `pty.StartWithSize` -> `newPTYProcess(...)`.
  - Lines 333 and 637-669: `ptyFile.Write` -> `proc.Write`, `pty.Setsize` -> `proc.Resize`.
  - Line 290: `s.cmd.Process.Pid` -> `s.proc.Pid()`.
  - `waitLoop` (line 425): `code := s.proc.Wait()`.
- `close(killProcess bool)` (line 501) becomes `close(mode)` with `kill | exited | detach`.
  - `detach` must **not** call `s.activity.cleanup()`. The running agent still uses its settings and hooks dir.
- New `(tm) DetachAll()`: detach every session and keep its log. Used by "keep running" and by signals.
- `StopAll()` (line 469): stop local sessions, then `host.killall` so unattached and unclaimed sessions die too.
- New `(tm) Restore() []RestoredTerminal`:
  - Calls `list`, then attaches each session with `replay`.
  - Rebuilds `agentActivity` from meta (`kind`, `dir`, `path`, `codexHome`) and restarts `watchAgentActivity` and `watchProcessActivity`.
  - Puts the replay into `log` and `pendingOutput`. Raise the `pendingOutput` cap to the replay size for restored sessions only.
  - After the first client connects, nudges a redraw: resize to `rows-1`, then back to `rows` (SIGWINCH).
- New `(tm) SetMeta(id string, meta []byte)` and `(tm) Kill(id)` for orphans.
- `startTerminal` accepts a `meta []byte`. Add it to `StartWithSessionReporter` and `StartWithEnvironment`.

`internal/components/agent_activity.go`
- Export a serializable `agentActivityMeta{Kind, Dir, Path, CodexHome}` and `restoreAgentActivity(meta)`.
- Pass `a.dir` to the host as a `cleanup` path.

`internal/components/managed_terminal.go`
- `StartManaged` sends `managed{logPath, statePath, secrets}` to the host. The host writes the redacted log. The backend no longer writes `m.file` when the host is used.
- `finish` and `activity` still write `statePath` from the backend while attached.
- Restore: rebuild `managedTerminal` from `statePath` JSON.

`internal/components/terminal_logs.go` – no API change. After a restore, the log is seeded from the replay.

`internal/components/process_unix.go` – keep `terminalHasChildren`. `killTerminalProcess` is called by the host.

### Backend: app wiring

`main.go:51` – add the `pty-host` case -> `libro.RunPTYHost()`. It is hidden: not in help or flags.

`internal/app.go`
- `CleanupRuntime` (line 493): call `tm.DetachAll()` instead of `tm.StopAll()`.
  SIGTERM and SIGINT (Electron quit, `air` reload, Ctrl+C) now keep terminals.
  Killing happens only through the explicit "stop all" action.
- `app.close.all` (line 1432): kill, as today. Rename the label to "Quit and stop all".
- New action `app.close.keep`: `syncTerminalMeta()`, `tm.DetachAll()`, clear `sm.states`, then the same `forceClose` JS.
- `app.close.check` (line 1412): no change in logic. It shows the dialog only when apps are running.
- Page handler (line 1522): after `sm.NewSession()`, call `restoreTerminals(sid)` (once per process, see below).
- Start sites pass meta:
  - line 866 and 961: `StartWithSessionReporter(..., panelMeta(sid, term))`
  - `internal/application_control.go:281`: `StartWithEnvironment(..., panelMeta(...))`
  - `internal/child_control.go:370`: `StartManaged`; meta includes the child ID.
- Start a background `sweepOrphans()` (every 30 s).

New file `internal/terminal_restore.go`
- `type panelMeta struct { V int; Workspace string; ProjectPath string; Transient bool; App Application; Activity *components.AgentActivityMeta; Child string }`.
  `App` is the `Application` record: ID, Type, Command, PluginID, Dock, Width, Name, Writable, IconURL, ApplicationPort/PerThread/Path.
- `syncTerminalMeta()`: walk every `sm.states` workspace (active + snapshots) and `tm.SetMeta` each terminal panel.
  It runs on "keep running" and after workspace moves (`MoveSelectedAppToProject`, `MoveSharedProjectApps`), so the saved workspace is current.
- `restoreTerminals(sid)`:
  - Runs once: the first page after backend start claims all host sessions.
  - For each restored terminal, run `isOrphan(meta)`. Kill orphans.
  - Otherwise insert the panel into the right workspace (`state.Apps` if active, else `snapshots[workspace]`) with `TerminalReady=true`.
  - Raise `sm.nextID` above the highest restored `app-N`. Today's counter restarts at 1 and would collide.
  - Re-add a transient project from `ProjectPath` when its path still exists.
  - Project-command panels keep their port. `controlApplication` status then reports "running" again.
- `isOrphan(meta)` returns true when:
  - meta is missing, can't be parsed, or `V` is unknown,
  - the workspace is a thread and the thread is missing or archived (DB),
  - the workspace is a project that is not in the DB and is not a transient project with an existing path,
  - the workspace path (worktree) no longer exists,
  - it is a child attempt and the child record is gone, already cleaned, or `child.Agent != id`.
- `sweepOrphans()` handles runtime orphans: a host session that is not referenced by any `sm.states` panel (active or snapshot) and not waiting to be claimed.
  It is killed only after **two** sweeps in a row (60 s grace), to avoid racing a start.
  This also catches PTYs leaked by page reloads (each page starts a new empty session, `internal/app.go:1521`).

`internal/child_control.go:190` `refreshChild`
- When `tm.ManagedStatus` has no live session, ask the host (`tm.IsRunning`) before marking the attempt "failed".
- The comment "Never trust a saved PID after Libro restarts" stays true: the host is the source, not a PID.

### Frontend

`internal/components/close_dialog.go`
- Footer buttons: `Cancel`, `Quit and stop all` (secondary, danger), `Quit, keep running` (primary, `autofocus`).
- Description: "These applications are still running. Keep them running in the background, or stop them all."

`internal/close_dialog.go` `showCloseDialogJS`
- For `app.close.all`, replace both buttons. Keep → `app.close.keep`, stop → `app.close.all`.
- The `project.close` dialog keeps one button (no change).

`internal/components.go:3325` terminal `ws.onclose`
- No change needed. After a restart the page reloads and connects fresh.
- Restored terminals get their replay through `pendingOutput` on first connect.

Electron (`electron/main.js`): no change.
- `isServerRunning` already reuses a running backend.
- Stopping the backend with SIGTERM now detaches.

### Docs

- `README.md`: a short "Persistent terminals" section. Explain both quit options. On Unix, `kill`/SIGTERM keeps terminals running.

## Implementation Steps

1. **Interface first, no behavior change.** Add `ptyProcess` and `localProcess`, and switch `TerminalSession` to `proc`. Run `make check`.
2. **Host server + client** (`ptyhost_unix.go`, `ptyhost_client_unix.go`): socket, auth, ops, frames, ring buffer, cleanup paths, idle exit. Wire `main.go` `pty-host`.
3. **Use the host on Unix** in `startTerminal`, with a fallback to `localProcess` if `ensure()` fails.
4. **Detach vs kill**: `close(mode)`, `DetachAll`, `StopAll` -> `killall`. Change `CleanupRuntime` to detach.
5. **Managed attempts** through the host (log file, redaction, final state).
6. **Metadata**: `panelMeta`, pass it at every start site, `syncTerminalMeta` on keep and on workspace moves.
7. **Restore**: `tm.Restore`, `restoreTerminals(sid)`, the `nextID` bump, agent activity rebuild, replay + redraw nudge.
8. **Orphans**: `isOrphan` at restore, `sweepOrphans` loop, `refreshChild` fix.
9. **Quit dialog**: two buttons, `app.close.keep` action.
10. **Tests** (real behavior, no string asserts):
    - host: spawn `sh -c 'echo a; sleep 30'`, detach, re-attach with replay -> sees `a`, process still alive.
    - host: `kill` kills the process group (child `sleep` is gone).
    - host: client exits without detach (simulated crash) -> process survives. A new attach takes over.
    - host: exit while detached -> `cleanup` dir removed, managed `statePath` has the exit code.
    - `isOrphan` table test: archived thread, missing project, missing worktree path, stale child attempt, valid thread.
    - restore: `nextID` is above restored IDs.
11. **Manual test** on the dev instance:
    - Run Claude in a thread, a shell running `top`, and a project command.
    - Quit -> keep -> reopen: all three are back with output, agent status/title is correct, the app port still serves.
    - Quit -> stop all -> no `libro pty-host` process and no children left.
    - Archive a thread's worktree path while Libro is closed -> its terminal is killed on start.
12. `make check`, update README.

## Risks and edge cases

- **Libro tools while closed:** MCP and CLI calls (`libro application`, `notes`, `children`) fail while the backend is down.
  Running agents keep working but get errors from these tools until Libro is reopened. Add one line to `AgentInstructions` ("if Libro is unavailable, wait and retry").
- **Host crash** kills all PTYs, the same as today's backend crash. Keep the host code small. Recover panics per connection.
- **Replay fidelity:** a raw ring buffer can't rebuild every screen state. The SIGWINCH nudge makes Claude, Codex, and other TUIs redraw.
  If this isn't enough, add a headless VT emulator in the host later.
- **Binary upgrade:** after `make install`, the old host keeps running the old code (Linux keeps the inode), so the protocol must stay additive.
  A new host starts only after the old one exits (it does once it has no sessions).
- **Multiple windows/pages:** only the first page after a backend start claims restored terminals. This matches today's "each page starts empty".
- **Logout/reboot:** processes die with the user session. Nothing is restored. That is expected.
- **Never reopened:** terminals run until the user reopens Libro and quits with "stop all", or the processes exit on their own. No timeout for now.
  Optional later: a setting for the max detached time.
- **Windows:** unchanged (in-process PTYs, killed on quit). The `ptyProcess` interface leaves room for a ConPTY host later.
