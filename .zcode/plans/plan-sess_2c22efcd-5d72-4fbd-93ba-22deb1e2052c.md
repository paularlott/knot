# Tray-backed desktop mode for knot

Bare `knot` = full server (identical to `knot server`) + system tray ("Open knot UI" → default browser at the advertised URL, "Quit" → graceful shutdown). All existing subcommands unchanged. Tray code excluded from `-tags server` builds. No CGO — cross-compilation of all 6 release targets preserved.

## Code changes

### 1. `command/server.go` — factor Run, extract flags
- Extract `ServerCmd`'s `Flags` slice into `func ServerFlags() []cli.Flag` (llmrouter pattern, proven with this same cli library).
- Extract the `Run` body (currently inline, ~lines 807–1316) into `func RunServer(cmd *cli.Command, quit <-chan struct{}) error`. The blocking wait at line ~1286 changes from `<-c` to:
  ```go
  select {
  case <-c:      // SIGINT/SIGTERM (Ctrl-C still works everywhere)
  case <-quit:   // programmatic shutdown from the tray
  }
  ```
  (nil `quit` channel blocks forever — safe for the normal server path.) All existing cleanup (serverCancel, SSE hub, HTTP shutdown, cluster.Stop, plugins) runs identically for both paths.
- `ServerCmd.Run` becomes a thin wrapper: `return RunServer(cmd, nil)`.

### 2. `main.go` — root command
- Root `Flags`: current global flags + `command.ServerFlags()` so `knot --url https://x` works in desktop mode.
- After the root command struct is built, call `applyDesktopMode(cmd)` before `Execute`.

### 3. New build-tagged hook (main package)
- `desktop_hook.go` (`//go:build !server`): `applyDesktopMode` sets `cmd.Run = desktop.Run`.
- `desktop_hook_server.go` (`//go:build server`): no-op — bare `knot` keeps printing help in server/Docker builds.

### 4. New `internal/desktop/` package (`//go:build !server`)
- `desktop.go` — `Run(ctx, cmd) error`:
  - Starts `command.RunServer(cmd, shutdownCh)` in a goroutine, collects its error on `done`.
  - Reads the URL from `cmd.GetString("url")` (default `http://127.0.0.1:3000`).
  - Runs the tray on the main thread; on Quit → `close(shutdownCh)`, then `return <-done` (waits for the server's full cleanup before exit).
  - **Fallbacks** (desktop mode never hard-crashes; worst case = headless server): tray init fails (e.g. GNOME without SNI), or macOS bare binary outside a `.app` bundle (NSApplication needs a bundle — same guard llmrouter uses): log the tray-unavailable message + URL, block on `<-done` (Ctrl-C still works via the signal channel).
- `tray.go` — the ONLY file importing `github.com/gogpu/systray` (the swap point if the young lib proves unreliable): `Run(icon []byte, tooltip, openLabel string, onOpen, onQuit func()) error`; menu = "Open knot UI" → `onOpen`, separator, "Quit" → `onQuit`.
- `icon.go` — `//go:embed icon.png`.

### 5. Browser helper
- Move `open()` from `command/connect.go:174` to `internal/util/browser.go` as `util.OpenBrowser(url string) error` (behavior identical: `cmd /c start` / `open` / `xdg-open`); `connect.go` calls it.

### 6. Dependencies
- `go get github.com/gogpu/systray` (+ godbus/dbus/v5 indirect). Pure-Go FFI: Shell_NotifyIconW syscalls (Windows), objc-runtime FFI (macOS), D-Bus SNI (Linux).

### 7. Icons
- Taskfile `icons` task: `rsvg-convert` `web/public_html/images/logo.svg` → `internal/desktop/icon.png` (single ~44px PNG; per-platform sizing guidance from the lib). Note: macOS menu-bar icons are ideally monochrome "template" icons — colorful logo is fine for v1, template variant is polish.

### 8. Build integration
- Dockerfile: add `-tags server` to `go build` so container images exclude the tray dependency entirely.
- Taskfile release targets unchanged (no CGO anywhere).

## Verification
- `go build` for all 6 platforms, with and without `-tags server`; `go vet ./...`
- Run bare `knot` on macOS: server starts, tray appears (or documented fallback), "Open knot UI" opens the browser to the URL, "Quit" runs the same cleanup as Ctrl-C; Ctrl-C also still works in both modes
- `knot server` and all other subcommands behave exactly as before

## Out of scope (follow-ups)
- macOS `.app` packaging task (Info.plist with `LSUIElement=true`, `.icns`, ad-hoc codesign) — until then macOS desktop mode uses the headless fallback outside a bundle
- Windows `-H windowsgui` console-less exe variant
- macOS template (monochrome) tray icon
- Single-instance handling (port already bound → just open the browser)

## Risk note
gogpu/systray is ~3 months old with no tagged releases — deliberately contained: it's imported by exactly one wrapper file, and every failure path degrades to a headless server rather than a crash. If it misbehaves, replacing the backend touches only `internal/desktop/tray.go`.