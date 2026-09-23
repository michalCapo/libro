# Product

<!-- impeccable:product-schema 1 -->

## Platform

desktop (Go + Electron), with a browser fallback

## Product Purpose

Libro is a Go and Electron desktop workspace for CLI coding agents. The main area runs Codex, Pi, Claude, OpenCode, or another CLI in a native terminal. Projects retain their running apps when the user switches context.

## Capabilities and Constraints

Project agents run in project threads, with one agent per thread. Ctrl+N opens the agent picker for a new project thread. Ctrl+Shift+N opens Replace agent, which starts a fresh session in the current thread and keeps its tools open. Project autolaunch creates a project thread. Standalone threads keep their existing behavior.

Collapsible project and thread navigation with one center agent panel per thread, a tabbed right tool dock, and a bottom project-terminal dock. Ctrl+N opens the agent picker for a new project thread. Ctrl+Shift+N opens Replace agent, which starts a fresh session in the current thread and keeps its tools open. Threads in the same project share Issues, one live project start/stop application, and bottom terminal processes without restarting them when switching threads. The selected right tool overlays the agent when the workspace is too narrow to show both. Browser and terminal tools use a shared plugin manifest and host-managed lifecycle. Show running bottom terminal activity on the project row; standalone threads show their own activity. Shortcut numbers trail all row indicators and actions. Keep existing project and worktree workflows.

## Brand Commitments

The user requested the look and feel of Codex desktop or T3 Code. Use a quiet desktop coding interface with the agent as the main content.

## Layout

Projects and agent threads stay on the left. Each thread has one center agent, right-dock tools switch through a shared tab row, and project terminals open below the main row. Build directly from the supplied T3 Code screenshots: near-white content, softly tinted sidebar, subtle dividers, rounded selected rows, muted icons, and compact bordered controls. Keep the existing Go-rendered UI and Electron runtime.
