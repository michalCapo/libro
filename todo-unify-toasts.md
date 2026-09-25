# Todo: Unify toast notifications

## Outcome
Use only g-sui notifications at the top right. The central overlay and its animations are removed.

## Implementation
- [x] Replace the custom renderer in `internal/components.go` with a thin browser adapter.
- [x] Register `app.notify` in `internal/app.go`, using the existing g-sui `r.Notify` API.
- [x] Route Go notifications directly through `r.Notify`.
- [x] Preserve subtitle text as part of the message.
- [x] Replace custom durations with explicit info, success, and error variants across `internal/app.go`, `internal/components.go`, `internal/components/browser.go`, and `internal/workspace.js`.
- [x] Remove custom toast DOM, styling, and animation code.
- [x] Run `make check` (all checks passed).
- [ ] Verify in the Libro-managed application. Blocked: no application command is configured in Libro project settings.

## Tradeoffs
No database or dependency changes. The proposed library release is unnecessary: the browser adapter uses Libro's existing action transport and the public g-sui API.

Browser-originated messages require a server round trip. All notifications use g-sui's standard five-second duration, stacking, and dismiss control. Subtitle content remains, but no longer has separate styling. Framework reload notifications remain unchanged.
