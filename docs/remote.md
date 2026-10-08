# Remote Backend

Goal: the Libro backend runs on a server and is controlled from Libro on a laptop. Same UI as today. One frontend can work with many backends.

Status: proposal. None of the remote features below exist yet.

Design: TLS pairing. Everything is inside Libro. No Vemari, no relay, no frp, no SSH install.

## Current Baseline

- `libro` starts one local HTTP/WebSocket server on port `8100` and opens the Electron shell.
- `libro --no-desktop` starts the server only. `libro --version` prints the version.
- `--dev` and named instances default to port `8101` (`electron/main.js`); `--port` picks another.
- Terminals no longer use `ttyd`. Libro serves `xterm.js` panels over `/terminal/ws/<id>`, backed by Go PTYs.

| Area | Current state |
|---|---|
| Terminal backend | `internal/components/terminal.go` owns `TerminalManager`, PTY startup, resize/input, and `/terminal/ws/<id>`. |
| Terminal UI | DOM containers with bundled `xterm.js` and fit addon from `assets/xterm/`. No iframe. |
| State model | `Application` uses `TerminalID` and `TerminalReady`, not a ttyd URL/port. |
| Removed dependency | No `ttyd` process, proxy route, or per-terminal TCP port. |

Known gaps that matter for remote:

- Main HTTP listener binds `":"+Port()` (`internal/app.go`), not `127.0.0.1`.
- Data dir is created with `0755` (`internal/db.go`).
- Session IDs are `session-N-<random>` (`internal/state.go`). They are hard to guess but are not a security identity.
- The terminal endpoint lets a client attach to a live PTY without an ownership check (`internal/components/terminal.go`).
- Closing the window (`app.close.all`, `internal/app.go`) stops all terminals. The layout is saved and restored on the next start, but processes are not kept.
- After an outage longer than ~15 s, g-sui reloads the page. The reload keeps its session through `?sid=` in the URL (`internal/app.go`).
- One global Electron window, one `serverURL`, global IPC. Closing the window quits the app.
- Webview partition comes from the project name (`internal/browser_profile.go`).

## Main Idea

No custom protocol, no multiplexing. Each TCP connection from the laptop = one TLS connection to the server. The server's TLS listener serves **the same `http.Handler`** as today, wrapped in an authorization layer.

```
Electron ──http + local token──▶ 127.0.0.1:82xx (Libro on laptop, pipe)
         ══ TLS 1.3 + device certificate ══▶ server (Libro backend)
                                               └─ authorization → same handler as :8100
                                                  └─ app-3000.localhost → 127.0.0.1:3000
```

- WebSockets (UI, terminals, HMR) go through the pipe unchanged.
- The pipe keeps `Host` and `Origin`. g-sui compares their host part, so the UI works.
- No new dependency: `crypto/tls`, `crypto/x509`, `io.Copy`.
- The browser keeps connections open (keep-alive, WebSocket), so a handshake per connection is fine.
- Local ports use the `8200+` range.

## Trust Boundaries

| Boundary | Who crosses it | Check |
|---|---|---|
| Network → server | anyone on the network | TLS 1.3, client certificate required, device pin |
| Unpaired device → server | new laptop | only exact `POST /pair` with a valid token |
| Local process → laptop pipe | any program on the laptop | local token, exact `Host`/`Origin` |
| Electron window → backend | that backend's window | token belongs only to that backend's window |
| Session → terminal | UI session | terminal ownership, not just "PTY is running" |
| Backend → app (proxy) | paired device | registry of apps started by Libro |

Request order on the server:

1. TLS handshake. Reject without a client certificate.
2. `VerifyConnection`: certificate fingerprint. Mark unknown certificates as unpaired. This check also runs on TLS resumption; `VerifyPeerCertificate` does not.
3. The authorization handler wraps the **whole** mux (pages, assets, `/__ws`, terminals, notes, voice, proxy). g-sui page middleware is not enough.
4. Unpaired certificate: only `POST /pair`, otherwise 403.
5. By `Host`: `app-N.localhost` goes to the proxy, everything else to the Libro UI. Unknown `app-*` host returns 404 and must not fall through to the Libro UI.

## Server (Backend)

- Start: `libro --no-desktop --listen <address:port>`. Port is optional.
- Main HTTP listener changes to `127.0.0.1` (`internal/app.go`). Fix this even without remote mode.
- First start generates an ECDSA key and a self-signed certificate.
- Keys live in a private `0700` dir, files `0600`. On Windows, restrict ACL to the user.
- Limits: handshake timeout, max `/pair` body size, max concurrent connections, connections per IP.
- Connections are tracked by device ID, also after WebSocket hijack, so they can be closed on revoke.
- Remote headless never listens on `0.0.0.0` unless the user opts in explicitly.
- Logs never print tokens.

## Pairing

1. On the server, `libro pair` prints a code:
   ```
   libro://server.example:8443#fp=<sha256 of server certificate>&t=<token>
   ```
   - token: at least 256 random bits, only a hash in DB, expires after 10 minutes on the server;
   - the code moves over a trusted path (terminal over SSH, in person). Whoever replaces the whole code points the laptop at themselves.
2. On the laptop: Settings → Backends → Add → paste the code. The token is never logged or sent in a navigation URL.
3. The laptop generates a device key and certificate, connects, and **first** verifies the server `fp`. Only then it sends `POST /pair` with the token and device name.
4. The server checks and consumes the token and stores the device in one DB transaction. Two concurrent uses of one token cannot both succeed. The device fingerprint comes from the TLS connection, not the request body.
5. Rate limit `/pair`: global and per IP. Per-certificate limits are not enough; an attacker can make a new certificate any time.

A changed server pin is never accepted automatically. The laptop shows an error and requires new pairing.

## Devices and Revocation

- A device has a stable ID. The name is only a label and may repeat.
- `libro devices` lists ID, name, pairing date, and last use.
- `libro devices remove <id>`:
  - deletes the device from DB;
  - tells the running server through a local authenticated endpoint;
  - the server closes all connections of that device, including WebSockets.
- Authorization is checked on every HTTP request, not only at handshake.
- First version disables TLS session resumption. Add it later together with the `VerifyConnection` check.
- Lost laptop key: revoke the device, pair again. Server certificate rotation: new `fp`, all laptops pair again.

## Laptop (Frontend)

- The laptop owns the backend list and device keys. The server owns devices and pair tokens.
- The Libro Go process opens one listener per backend on `127.0.0.1` in the `8200+` range. The port is saved with the backend so the origin stays stable (localStorage, cookies).
- **Local auth:** `127.0.0.1` is not protection. The pipe accepts only requests with the local token and exact `Host`/`Origin` (`localhost:82xx` or `app-N.localhost:82xx`). A trusted part of Electron (main process, `session.webRequest`) adds the token, only for that backend's window and webviews. The token is never sent to the server or to an app.
- Per connection: TLS dial with the device certificate, verify `fp`, then `io.Copy` both ways. The pipe never retries in-flight requests automatically.
- When the server is down: local "disconnected" page (also before the remote UI ever loaded), retry with backoff.
- Local Settings (backend list) work without a reachable backend.

## Electron

Needed changes:

- map backend → window;
- route IPC by sender (`event.sender`), not to the global window;
- closing a backend window only disconnects, it does not quit the app;
- window, webview, and popup partitions contain the stable backend ID. Today two backends with the same project name would share cookies;
- set the partition before the first navigation;
- check backend version compatibility with the Electron preload. If versions differ, attach first and warn only if the API compatibility check fails.

## Sessions and Terminals

- Binding: device → session → terminal. Actions with a foreign SID are rejected.
- Always check terminal ownership on the terminal endpoint.
- First version: one active GUI client per backend. Another client gets "busy" with an option to take over the session. No session merging.
- Split "disconnect window" from "stop runtime".
- After an outage, restore the device's existing session instead of creating a new one.

## Apps in Browser Panels

The backend runs a reverse proxy by subdomain:

```
http://app-3000.localhost:82xx  →  pipe  →  backend  →  http://127.0.0.1:3000
```

- Chromium resolves `*.localhost` to loopback (`127.0.0.1` and `::1`). The local listener must listen on both, or verify behavior in the shipped Electron.
- Each app has its own origin, separate from Libro.
- `httputil.ReverseProxy` supports WebSocket upgrade.
- External URLs like `https://github.com` stay plain local Electron webviews. Only loopback URLs are treated as remote apps.

**Allowed targets:**

- only the registry of apps Libro started that are still running. An assigned port is not enough; the command may ignore it (`internal/application_control.go`);
- target is always `127.0.0.1`, never another address;
- Libro ports and local control ports are forbidden;
- in remote mode, port takeover is off (`internal/application_settings.go`). Busy port = error.

**Headers:**

- `Host` is rewritten to `localhost:N`. `Origin` stays original, which may break app CSRF or WebSocket checks. The app gets `X-Forwarded-Host` and `X-Forwarded-Proto: http` (Electron uses HTTP; internal TLS does not change the scheme).
- `Location` is rewritten only from `localhost:N` / `127.0.0.1:N` to `app-N.localhost:82xx`. Foreign addresses (OAuth) stay unchanged.
- The Libro local token and Libro cookies are never sent to the app.

**Cookies and HMR:**

- host-only cookies work. `Domain`, `Secure`, and `SameSite` need testing;
- HMR must use the public proxy host and port. For Vite set `server.hmr.clientPort` / `host`, otherwise the client connects to the app port directly.

**App URL:**

- the server does not know the laptop's local port, and different laptops use different ports;
- the backend returns only the app ID and port. The frontend builds the URL from its own `location`;
- public URLs are not stored in the server DB (`internal/application_settings.go`).

Does not work for apps with hardcoded `localhost:N` in HTML/JS, CORS, or OAuth callbacks.

## CLI and MCP on the Server

The bridge (`internal/agent_control.go`) reads a local descriptor and calls Electron on the laptop. It does not work on a server without Electron. Remote agents need:

- `notes`, `application`, `children`, and MCP handled directly in the backend;
- a local authenticated endpoint on the server (also used by `libro devices remove`);
- the same workspace restrictions as today.

This is part of the first version, not a later add-on.

## Data Ownership

| Data | Stored where | Reason |
|---|---|---|
| Runtime DB | one DB per backend machine/user | one daemon per machine, no DB per workspace |
| Projects, apps, worktrees, run history | backend DB | belong to the machine that runs them |
| Saved browser/terminal apps | backend DB | scoped to backend projects for v1, no global catalog |
| Devices, pair tokens | backend DB (`remote_devices`, `pair_tokens`) | server decides who may connect |
| Backend list, device keys | laptop DB (`backends`) | connection manager config |
| Browser URL history | laptop | browser is local UI |

## Libro Changes

| File | Change |
|---|---|
| `main.go` | `--listen`, `pair` and `devices` commands |
| `internal/app.go` | bind `127.0.0.1`, TLS listener, authorization layer, proxy, disconnect vs. stop |
| `internal/remote_server.go` (new) | certificate, `VerifyConnection`, `/pair`, connection registry, revocation |
| `internal/remote_client.go` (new) | local listeners, local token, TLS pipe, reconnect |
| `internal/remote_proxy.go` (new) | `app-N.localhost` proxy, allowed app registry |
| `internal/db.go` | `0700` data dir; `remote_devices`, `pair_tokens` (server), `backends` (laptop) tables |
| `internal/state.go` | device → session binding, session restore |
| `internal/components/terminal.go` | always check terminal ownership |
| `internal/browser_profile.go` | backend ID in partition |
| `internal/application_settings.go`, `application_port.go` | port takeover off in remote mode, frontend builds URL |
| `internal/agent_control.go` | CLI/MCP directly in backend |
| `internal/desktop.go`, `electron/main.js`, `electron/preload.js` | windows per backend, IPC by sender, local token, partitions, version check |
| Settings UI | backend list, add, remove, connection status |

## Order

1. Security base: bind `127.0.0.1`, terminal ownership check, port takeover off, `0700` data dir.
2. CLI/MCP in backend, split "disconnect window" from "stop runtime", session restore.
3. TLS listener, authorization layer, pairing, revocation.
4. Laptop: pipe, local token, Electron windows and partitions per backend.
5. `app-N.localhost` proxy with app registry.
6. Settings UI for backends.

## Acceptance Scenarios

- Two concurrent `POST /pair` with the same token: exactly one succeeds.
- Expired or used token: 403.
- Unpaired certificate: 403 on every route except `/pair` (pages, assets, `/__ws`, terminals, notes, voice, proxy).
- No certificate: handshake fails.
- Changed server `fp`: laptop rejects the connection.
- Device revoke: open WebSocket and terminal close within 1 s. New connection fails.
- Local process without token on `127.0.0.1:82xx`: rejected.
- Terminal of another session: 403, even if the PTY runs.
- Two backends with the same project name: separate cookies and localStorage.
- Network outage over 15 s: same session after return, terminals keep running.
- Closing a backend window: terminals on the server keep running.
- `libro application` and MCP on a server without Electron: work.
- Proxy: unknown port, Libro port, and port of an app that already exited return 404.
- Proxy: redirect to `localhost:N` rewritten, redirect to a foreign OAuth server unchanged.
- Proxy: Vite HMR works through `app-N.localhost`.
- Proxy: Libro token and cookies never reach the app.

## Not Covered

- Terminal output during a disconnect is not replayed after reconnect (only a 64 KiB log).
- Page-tool attachments are saved on the laptop; a remote agent cannot see them. Needs upload to the server.
- Agent CLIs and their auto-update (today in Electron, `electron/agent-updates.js`) must run on the server.
- Multiple users on one backend.
- The server must be reachable from the laptop (public IP/DNS, LAN, VPN, or `ssh -L` tunnel). NAT on both sides without a VPN is not solved.
- SSH install and auto-start of the backend. The backend is started manually.
- `doctor` report of server tools.
