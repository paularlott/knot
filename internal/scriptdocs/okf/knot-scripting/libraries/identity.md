---
type: "API Reference"
title: "knot.identity"
description: "The authoritative requesting-user surface for plugin and tool code - a real User object with permission checks, over the gated loopback."
resource: "https://getknot.dev/reference/libraries/identity/"
tags:
  - "api"
  - "scripting"
  - "plugins"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/identity/"
---
# knot.identity

The `knot.identity` library returns the requesting user as a real `User` object — with `has_permission` / `in_group` — re-bound on every dispatch. It is the **authoritative** identity surface: use it for any permission decision.

It matters most for **module code** — a plugin's exported scriptling libraries (`libs/*.py`) and loaded lib scripts. A plugin handler receives the caller as data in its `request["user"]` argument, but a library is a module: it never sees the handler's `request`, so it reads `knot.identity` directly. (`request["user"]` is an inert snapshot for branching; `knot.identity` is the authority that rides the gated loopback, where the admin role passes every check.) In the MCP-tool environment the same library is how a user tool asks who it runs as.

```python
# libs/report.py — a plugin's exported library, imported by another plugin
import knot.identity

def export_report():
    # A composing plugin imports this ungated, so the gate lives here.
    if not knot.identity.user().has_permission("plugin.metrics.export"):
        raise Exception("plugin.metrics.export not granted")
    return {"rows": []}
```

---

## Functions

| Function | Description |
|----------|-------------|
| `user()` | The requesting `User`: `id`, `name`, `is_admin`, `groups`, `permissions` (stable keys), `plugin_permissions` (qualified grants), with `has_permission(key)` and `in_group(group_id)` methods. `groups` holds group **IDs**, and `in_group` compares its argument against them — pass a group ID, not a group name. `has_permission`'s argument picks the check: an integer is a built-in permission id (the `knot.permission` constants), a `"plugin."`-prefixed string a qualified grant, any other string a built-in's stable key; the admin role passes every check. |

---

## See also

- [The request argument](https://getknot.dev/docs/plugins/writing-plugins/scriptling/#the-request-argument) — how a handler receives the caller as `request["user"]` data.
- [The trust boundary: import vs call](https://getknot.dev/docs/plugins/writing-plugins/mcp-tools/#the-trust-boundary-import-vs-call) — why exported code self-gates for cross-plugin composition.
