# Product

<!-- impeccable:product-schema 1 -->

## Platform

desktop (Go + Electron), with a browser fallback

## Product Purpose

Libro is a Go and Electron desktop workspace for CLI coding agents. The main area runs Codex, Pi, Claude, OpenCode, or another CLI in a native terminal. Projects retain their running apps when the user switches context.

## Capabilities and Constraints

Collapsible project navigation with fixed-width center agent panels, a tabbed right tool dock, and a bottom project-terminal dock. Panels retain their width when another panel opens. The selected right tool overlays the agents when the workspace is too narrow to show both. Browser and terminal tools use a shared plugin manifest and host-managed lifecycle. Keep existing project and worktree workflows.

## Brand Commitments

The user requested the look and feel of Codex desktop or T3 Code. Use a quiet desktop coding interface with the agent as the main content.

## Layout

Projects stay on the left. Center agents remain side by side, right-dock tools switch through a shared tab row, and project terminals open below the main row. Build directly from the supplied T3 Code screenshots: near-white content, softly tinted sidebar, subtle dividers, rounded selected rows, muted icons, and compact bordered controls. Keep the existing Go-rendered UI and Electron runtime.
