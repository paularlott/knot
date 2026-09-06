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

## demo-go: script plugin with a Go binary peer

Same shape, plus a **binary peer**: `peer/main.go` is a scriptling
plugin-protocol server (stdio JSON-RPC) that `main.py` declares as a metadata
dependency. Its page at `/plugins/demo-go/status` calls the peer; the
handler's `import plugin.demolib` reaches the Go process through the plugin
environment. Build the peer first:

```sh
cd examples/plugins/demo-go && make
```

`make` writes `bin/demolib_<goos>_<goarch>` (the universal-bundle naming knot
resolves per host). Until it is built, the plugin shows on the admin Plugins
page as **failed to load: required plugin "demolib" not loaded**, which is
also a demo of the failure path.

## What to look at

- **Admin → Plugins**: inventory with declared permissions, menus, pages,
  binary peers with health, and load warnings.
- **Sidebar → More**: plugin menu items appear per the gates above (pinnable
  like built-in items).
- **Roles**: the Plugin Permissions section lists what loaded plugins declare;
  grants are stored as text (`plugin.<plugin>.<permission>`) and survive
  cluster restarts and plugin reinstall.
