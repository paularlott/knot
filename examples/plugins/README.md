# Example knot plugins

Three example plugins for the knot plugin system (docs: `knot-website/content/docs/plugins/`).
Run a server pointed at this folder:

```sh
knot server --plugins-path ./examples/plugins
```

(or `server.plugins_path` in `knot.toml`, or `KNOT_PLUGINS_PATH`).

## demo-scriptling: pure script plugin

A folder plugin (`main.py` + `assets/`) declaring:

- **a live showcase page** at `/plugins/demo-scriptling/showcase`: its
  `showcase` handler runs per-request as your user and knot renders the
  returned rows/columns, gated by the plugin permission `view_dashboard`, and
  listed in the sidebar via its `menu_label`;
- **two link menu items**: one public (any logged-in user), one gated by
  `admin_dashboard`;
- **an in-process scriptling library** (`libs/calc.py`): a constant, plain
  functions and a stateful `Counter` class importable as `plugin.calc` —
  exercised by the showcase's *Plugin peers* row, and importable by other
  installed plugins for in-process composition (not by user-created MCP
  tools, which reach a plugin only via `knot.plugin.call`); its self-gating
  `gated_report()` shows an ungated cross-plugin import carrying its own
  `knot.identity.user()` permission check;
- **its own SVG icons**: every item uses `assets/icon.svg`, a
  currentColor-stroked SVG that themes with the UI like knot's built-ins.

**Logos**: demo-scriptling declares a themed `logo_light`/`logo_dark` pair;
demo-go declares only `logo_light`, which knot copies to the dark slot so one
file serves both themes. Both are site-logo claims, and with both plugins
loaded the first by name wins (demo-go); the admin Plugins page shows the
multi-claim warning. Set `server.ui.logo_url` to beat plugins entirely.

Add `default = true` to the dashboard's `[[tool.knot.pages]]` entry to make
it the post-login landing page (the demo ships without the claim so it
doesn't take over your login).

Until a role grants `plugin.demo-scriptling.view_dashboard` (Role editor →
Plugin Permissions, as a role-admin), the gated items are invisible; that is
the security profile working.

## dashboard: a real landing dashboard

A folder plugin that is the post-login landing page (`default = true`), built
entirely from `knot.*` libraries over the loopback, the same
permission-checked path MCP tools use, running as the requesting user:

- **your spaces** with state, template and node, filterable via an AJAX form;
- **live aggregates**: CPU % plus memory and disk progress bars (used of
  limit), summed from each running space's `resource_usage`;
- a **fleet usage time series** (aggregate CPU % and memory across running
  spaces, 1h or 7d, straight from each space's usage history over the
  loopback), a **top-five spaces by memory** chart, and colour-coded state
  badges;
- 30-second auto-refresh.

Every user sees their own data: no permissions to grant, no configuration.
Unlike the demos, this one claims the login landing spot by design.

## demo-scriptlingcli: script plugin with a scriptling CLI peer

A folder plugin whose `bin/kvstore` peer is a **scriptling script that looks
like a binary**: an executable whose shebang (`#!/usr/bin/env -S
scriptling --json-rpc`) hands it to the scriptling CLI, which serves the
plugin protocol with the database drivers compiled in — knot links none of
them. The peer's `impl.py` companion backs a small sqlite key/value store
(`kvstore.db`, persisted beside the executable), and the page calls it
through the same auto-generated stubs a Go peer gets. This plugin keeps a
`main.py` for its scriptling page handlers (addressed as
`plugin.demo_scriptlingcli.<fn>`), which compose the peer's `plugin.kvstore`
surface. Requires the scriptling CLI on the server's PATH; without it the
plugin fails its requirements at load (named on the admin Plugins page).

## demo-go: pure peer plugin (Go binary, no main.py)

The peer-manifest flavour: `peer/main.go` is a scriptling plugin-protocol
server (stdio JSON-RPC) that IS the whole plugin — there is **no companion
`main.py`**. At handshake the peer returns two things knot needs: its
`[tool.knot]` declaration table as static manifest data (`SetMetadata`,
carried in the handshake's custom metadata), and its handler surface as
registered functions. knot parses the manifest exactly like a pure-script
plugin's block, and addresses each handler as `plugin.demolib.<fn>`. Its
page at `/plugins/demo-go/status` and every column/handler it names run in
the Go process. Build the peer first:

```sh
cd examples/plugins/demo-go && make
```

`make` writes `bin/demolib_<goos>_<goarch>` (the universal-bundle naming knot
resolves per host). Until it is built, the folder has no `main.py` and no
loadable peer, so knot ignores it; build it and the plugin appears.

### Peer directions

- **Go exposes, scriptling consumes** (supported): a `bin/` peer handshakes
  a library name and serves functions, classes and whole libraries over
  stdio JSON-RPC; the plugin's handlers import it as `plugin.<name>`
  (demo-go's `import plugin.demolib`, whose `peer_class` handler exercises
  the peer's `Counter` class - `RegisterClass` in `peer/main.go`).
- **Scriptling libraries, in-process** (supported): a `.py` in the plugin's
  `libs/` folder loads via knot's embedded scriptling - no CLI, no
  subprocess - and its public surface is importable as `plugin.<name>`,
  with the `[tool.knot.lib]` version checked by dependency declarations. The metadata dependency
  (`plugin.demolib via demolib >= 1.0.0`) is verified against the live
  handshake at load. Handlers can also drive peers dynamically through the
  `scriptling.plugin` control library (`list`, `describe`, `call_function`).
- **Scriptling exposes, Go consumes** (not part of knot's plugin system):
  peers serve calls; handlers make them. There is no in-process path for a
  Go peer to import a scriptling-side library. (The reverse embedding —
  the scriptling CLI spawning the *knot binary* as its peer — is what
  `knot scriptling-server` autostarts for, a host feature, not a plugin
  one.)
- **Cross-plugin composition, in-process** (supported): installed plugins
  are one trust domain and share a plugin pool, so a handler may import
  another plugin's exposed library, class or peer as `plugin.<name>` and use
  it directly — install several plugins, build your own from their on-disk
  parts. User-authored MCP tools are the untrusted side: the plugin pool is
  simply not attached to their environment, so they reach a plugin only over
  the gated loopback (`knot.plugin.call`), never by import. The handler URL
  below is the other cross-plugin path (browser/ajax and loopback).

## Handler URLs: ajax endpoints, any caller

Every handler is addressable as a URL and answers JSON:

- `/plugins/<name>/<page-path>/<handler>` — runs through that page's gate
  (and, below, the column gates) unless the handler has its own
  declaration;
- `/plugins/<name>/<handler>` — plugin root; serves only handlers with a
  `[[tool.knot.handlers]]` declaration, whose permission is the gate.

A handler does not care who fetches it — the plugin's own pages, another
plugin's pages, or a user with curl. The gate layers, outermost in: the
**page's** permission governs the page and every handler riding its
path; **row** gates are presentation (a gated row vanishes with its
columns, which hides their handlers); **column** gates hold at fetch time
too — knot re-runs the layout as the requesting user and an undeclared
handler is served only if that layout offers it; a **handler declaration**
replaces the inherited gates wherever it applies (page path or plugin
root):

```toml
# [[tool.knot.handlers]]
# handler = "export_all"
# permission = "admin"     # optional; empty = any logged-in user
```

Declaring a handler also opts it into plugin-root addressability, which is
what cross-plugin `pluginFetch` uses — undeclared handlers inherit the
calling page's gate and are reachable only through a page path. Column data
fetches, form POSTs, row actions, popups and dynamic-option fetches all use
these URLs.

From trusted html, `pluginFetch` is the wrapper — same transport, auth and
gate:

```js
await pluginFetch('echo_word', { params: { word: 'hi' } });        // own plugin
await pluginFetch('peer_summary', { plugin: 'demo-go' });          // another plugin's handler
await pluginFetch('save', { method: 'POST', body: { name: 'x' } }); // POST
```

The echo card on demo-scriptling's showcase demonstrates the cross-plugin
call: its **Ask demo-go** button fetches demo-go's `peer_summary` handler
(declared in demo-go's metadata with no gate, so any logged-in user may
call it), which makes a live round trip to the Go peer. With demo-go
unloaded (peer not built) the button shows the error instead — degrade,
don't break.

## What to look at

- **Admin → Plugins**: inventory with declared permissions, menus, pages,
  binary peers with health, and load warnings.
- **Sidebar → More**: plugin menu items appear per the gates above (pinnable
  like built-in items).
- **Roles**: the Plugin Permissions section lists what loaded plugins declare;
  grants are stored as text (`plugin.<plugin>.<permission>`) and survive
  cluster restarts and plugin reinstall.
