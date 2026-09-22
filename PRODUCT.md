# Product

<!-- impeccable:product-schema 1 -->

## Platform

desktop (Go + Electron), with a browser fallback

## Product Purpose

Libro is a Go and Electron desktop workspace for CLI coding agents. The main area runs Codex, Pi, Claude, OpenCode, or another CLI in a native terminal. Projects retain their running apps when the user switches context.

## Capabilities and Constraints

Projects support multiple agent panels in the same workspace. The one-agent limit and automatic new-thread behavior apply only when launching from a standalone thread. Project autolaunch starts a panel directly in the project.

Collapsible project and thread navigation with one center agent panel per thread, a tabbed right tool dock, and a bottom project-terminal dock. Starting another agent creates another thread so every agent has isolated browser and thread-local tool state. Threads in the same project share Issues and one live project start/stop application. The selected right tool overlays the agent when the workspace is too narrow to show both. Browser and terminal tools use a shared plugin manifest and host-managed lifecycle. Keep existing project and worktree workflows.

## Brand Commitments

The user requested the look and feel of Codex desktop or T3 Code. Use a quiet desktop coding interface with the agent as the main content.

## Layout

Projects and agent threads stay on the left. Each thread has one center agent, right-dock tools switch through a shared tab row, and project terminals open below the main row. Build directly from the supplied T3 Code screenshots: near-white content, softly tinted sidebar, subtle dividers, rounded selected rows, muted icons, and compact bordered controls. Keep the existing Go-rendered UI and Electron runtime.
