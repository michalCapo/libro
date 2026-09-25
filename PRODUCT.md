# Product

<!-- impeccable:product-schema 1 -->

## Platform

desktop (Go + Electron), with a browser fallback

## Product Purpose

Libro is a Go and Electron desktop workspace for CLI coding agents. The main area runs Codex, Pi, Claude, OpenCode, or another CLI in a native terminal. Projects retain their running apps when the user switches context.

## Capabilities and Constraints

Opening a project from the sidebar or project search creates its empty Base thread in the original directory. Unopened projects have no Base row or shortcut number. New project threads create Git worktrees from the original directory’s current committed branch; non-Git directories only support Base. Base and worktree rows share Ctrl+1–9 navigation, whether or not an agent is open. Project agents run in these workspaces, with one agent per workspace. New thread creates a thread directly without opening the agent picker; its configurable shortcut is empty by default. Ctrl+Shift+A opens Replace agent, which starts a fresh session in the current thread and keeps its tools open. Base and worktree workspaces start empty; users choose which agent or tools to open. Standalone threads keep their existing behavior.

Collapsible project and thread navigation with one center agent panel per thread, a tabbed right tool dock, and a bottom project-terminal dock. New thread creates a thread directly without opening the agent picker; its configurable shortcut is empty by default. Ctrl+Shift+A opens Replace agent, which starts a fresh session in the current thread and keeps its tools open. Threads in the same project share Issues and bottom terminal processes without restarting them when switching threads. Application processes can be shared or per-thread. The selected right tool overlays the agent when the workspace is too narrow to show both. Browser and terminal tools use a shared plugin manifest and host-managed lifecycle. Show running bottom terminal activity on the project row; standalone threads show their own activity. Shortcut numbers trail all row indicators and actions. Keep existing project and worktree workflows.

## Brand Commitments

The user requested the look and feel of Codex desktop or T3 Code. Use a quiet desktop coding interface with the agent as the main content.

## Layout

Projects and agent threads stay on the left. Each thread has one center agent, right-dock tools switch through a shared tab row, and project terminals open below the main row. Build directly from the supplied T3 Code screenshots: near-white content, softly tinted sidebar, subtle dividers, rounded selected rows, muted icons, and compact bordered controls. Keep the existing Go-rendered UI and Electron runtime.

## Application instances

Project settings choose Application per thread (default) or Shared application. Per-thread applications inherit the project start command, run in their worktree, and receive an automatic PORT with an optional per-thread override. Settings and application status expose the localhost URL. Agent MCP accepts only an action and binds to its launch workspace; agent CLI uses LIBRO_APPLICATION_PATH and rejects other workspace paths. Stop and restart affect only the assigned application scope.

## Finish threads

Worktree threads have a right-click / overflow menu with Merge thread, Squash thread, Create draft PR, Thread settings, and a separate Discard thread action. Finish thread is also in the command palette and supports an optional configurable shortcut. Merge, squash, and draft GitHub PR each have a dedicated review dialog showing the destination and changes. Successful local integration automatically removes the worktree, thread, and branch. PR creation keeps the thread. New worktrees record their source branch. Dirty checkouts, stale previews, and conflicts block cleanup. Discard removes the thread, branch, and worktree together, without a typed confirmation.

## Voice typing

Hold Tab or the microphone beside the agent tab to dictate. Release to insert the transcript into the selected terminal (or the agent when a browser is selected) without submitting it. Escape, window blur, or switching threads cancels dictation. Libro automatically installs the local sherpa-onnx engine and multilingual Whisper tiny model during `make install` or first launch. After the download, transcription runs locally and works offline.
