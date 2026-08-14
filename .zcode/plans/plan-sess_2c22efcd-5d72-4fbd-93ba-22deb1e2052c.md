# Desktop setup wizard + host-IP agent endpoint

## Behavior

Bare `knot` (desktop mode) with **no config file found OR no database backend configured** → instead of silently starting, serve the existing config wizard at the advertised address; the tray menu reads "Open knot setup". The wizard is pre-filled from any existing config, validates required fields, writes `~/.knot/knot.toml`, and its success page tells the user to quit and relaunch the app (app stays running until Quit, per decision). Second launch boots the full server from the written config.

The `agent-endpoint` gains a `${{ host_ip }}` token resolved at every startup to the host's primary non-loopback IPv4 — required because agents live in containers and cannot reach 127.0.0.1. Users may use the token in their own knot.toml too.

## Changes

### 1. Host-IP token — `internal/util/hostip.go` (new)
- `HostIP() string`: UDP-dial trick (`net.Dial("udp","8.8.8.8:80")`, no packet sent) → fallback first non-loopback interface IPv4 → fallback `127.0.0.1`.
- `ResolveHostIP(s string) string`: replaces every `${{ host_ip }}` occurrence.
- Applied in `buildServerConfig` to `AgentEndpoint` for **all** modes (so the token works in server configs too).

### 2. Desktop fallback rework — `command/server.go` (desktop block)
- `AgentEndpoint` empty → default to `${{ host_ip }}:` + the port of `listen-agent` (parsed from the flag, default 3010), then resolve the token. Replaces the hardcoded `127.0.0.1:3001`; endpoint port and agent listener can never disagree. (To use 3001, set `listen_agent = "…:3001"`.)
- BadgerDB `~/.knot/data` and plain-HTTP-loopback fallbacks stay as gap-fillers (wizard normally pre-empts them).

### 3. Wizard gate — `internal/desktop/desktop.go`
- `needsSetup := cmd.ConfigFile.FileUsed() == "" || (!mysql && !badger && !redis enabled)` (flag resolution already folds in config-file values).
- If `needsSetup`: derive wizard address from the `url` flag (default `127.0.0.1:3000`), run `configwizard.Serve` with desktop options (below); tray shows "Open knot setup". When the wizard returns (saved or shut down): log/notify "Config saved — quit knot from the tray and reopen to apply", keep running until Quit, then exit. The full server is **not** started in this phase.

### 4. configwizard upgrades — `internal/configwizard/`
- `Serve(ctx, addr, opts)` with options: `TargetPath` (desktop forces `~/.knot/knot.toml`), `AllowOverwrite` (skips the existing 409 refuse-to-overwrite; CLI path keeps current behavior), `PrefillFrom`, `DesktopPreset`.
- **Prefill**: new `FormFromConfig(path)` mapping an existing knot.toml onto the wizard Form (url, listens, agent_endpoint — token rendered verbatim, timezone, db fields, nomad/docker/podman, dns, totp, chat, mcp, tunnel where present).
- **Validation** in `saveHandler`: parse TOML, then require non-empty `server.url`, `server.agent_endpoint`, `server.listen`, `server.listen_agent`, exactly one db backend with its own fields (badger `path`; mysql host/user/password/database; redis hosts). 400 + field list on failure; wizard JS surfaces it. Client-side `required` attributes on the same fields.
- **Desktop deployment card** (4th preset in step 1, preselected for desktop): badger at `~/.knot/data` (real path injected server-side), `url = http://127.0.0.1:3000`, `listen = 127.0.0.1:3000`, `listen_agent = 127.0.0.1:3010`, `agent_endpoint = ${{ host_ip }}:3010`, no nomad, local docker/podman sockets, emits `[server.tls] use_tls = false`.
- Success page: desktop variant message — "Quit knot from the tray menu and reopen it to apply the new configuration."
- Add `$HOME/.knot` to its `configSearchPaths` (parity with main.go).

### 5. Docs
- Website Desktop Mode section: first-run wizard behavior + restart step + `~/.knot/knot.toml` location; regenerate okf. In-app Clients page unchanged.

## Verification
- Empty `$HOME` double-click sim: wizard serves at 127.0.0.1:3000, tray label correct; POST incomplete TOML → 400 listing missing fields; POST complete desktop TOML → file at `$HOME/.knot/knot.toml`; process stays alive post-save with restart message; quit/relaunch → full server boots on new config, health 200, `${{ host_ip }}` resolved to real IP in advertised agent endpoint (checked via log/cluster metadata).
- Config-without-DB (partial knot.toml) → wizard returns prefilled, overwrite allowed.
- Complete config → no wizard, straight to server. `knot server` behavior unchanged (still fatals on missing config; token resolution still applies).
- Cross-compile all 6 platforms ± `-tags server`; vet clean.

## Follow-ups (out of scope)
- Auto-restart in place after wizard save (re-exec)
- Single-instance guard (second launch while wizard/server already running)
- Notarization