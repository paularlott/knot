---
type: "API Reference"
title: "knot.server"
description: "Server-wide information (version, wildcard and tunnel domains, space web port hosts), cluster nodes, loaded plugins and a search across everything you may see."
resource: "https://getknot.dev/reference/libraries/server/"
tags:
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/server/"
---
# knot.server

The `knot.server` library exposes server-wide information: the server's settings, the cluster's nodes, the loaded plugins, and a search across everything the current user may see.

---

## Functions

| Function | Description |
|----------|-------------|
| `info()` | Get server-wide information |
| `host_label(user, name, port)` | The first label of a space's web port host |
| `ping()` | Check the server is reachable and the credentials work |
| `cluster_info()` | List the cluster's server nodes |
| `search(query)` | Search everything you may see by name |
| `plugins()` | List the loaded server plugins and what each provides |

---

## Usage

```python
import knot.server as server

info = server.info()
print(info['wildcard_domain'])
```

---

## Info Properties

`info()` returns:
- `version` - The knot server version string.
- `wildcard_domain` - The server's wildcard domain for space web-port URLs (e.g. `*.knot.example.com`); empty when none is configured.
- `tunnel_domain` - The server's tunnel domain as a dot-prefixed suffix (e.g. `.tunnel.example.com`) that appends straight after a tunnel name; empty when none is configured.

---

## host_label(user, name, port)

The first label of the host a space's web port is reached at, before the wildcard domain: `<user>--<name>--<port>`, lowercased. DNS allows at most 63 characters there, so when the label would be longer, the name is cut short and ends with a hyphen and an 8-character hash of the full name, which knot routes by. The web interface and events build the same host.

**Parameters:**
- `user` (string): The username of the space's owner
- `name` (string): The space's name, one of its alt names, or a pool's name
- `port` (int or string): The port number, or `"vnc"` for web VNC

**Returns:** `string` - The label, such as `alice--web-app--8080`

```python
import knot.server as server

domain = server.info()["wildcard_domain"].lstrip("*")
url = "https://" + server.host_label("alice", "web-app", 8080) + domain
```

---

## ping()

Check the server is reachable and the credentials work.

**Returns:** `dict` containing `status` (`True`), `version` (the server's knot version) and `zone` (the zone of the server that answered).

---

## cluster_info()

List the servers of the cluster, as the Cluster Info page shows them. Needs the **View Cluster Info** permission.

**Returns:** `list` of dicts sorted by address, each containing:
- `id` (string): Node ID
- `address` (string): Advertised cluster address
- `state` (string): Gossip state, e.g. `alive`, `suspect`, `dead`
- `zone` (string), `hostname` (string)
- `allocated_spaces` (int): Spaces assigned to the node
- `running_spaces` (int): Those of them deployed
- `runtimes` (list): Container runtimes the node offers, e.g. `["docker"]` — known only for the node that answered
- `metadata` (dict): All the node's gossip metadata, as strings

---

## search(query)

Search everything the current user may see by name, as the web UI's command palette does. Matching is a case-insensitive substring match on each entity's name, with at most 5 hits per type. Admin entities appear only with the matching manage permission; personal ones (scripts, skills, commands, MCP servers, tokens) are your own plus global ones.

**Parameters:**
- `query` (string): Text to look for

**Returns:** `dict` containing `query` and one list per type — `pages`, `spaces`, `templates`, `variables`, `volumes`, `stacks`, `scripts`, `skills`, `commands`, `mcp_servers`, `event_sinks`, `users`, `groups`, `roles` and `tokens` — of dicts with `id`, `name` and, where there is one, `description`. A type with no hits has an empty list.

```python
import knot.server as server

hits = server.search("web")
for s in hits["spaces"]:
    print(s["id"], s["name"])
```

---

## plugins()

List the loaded [server plugins](https://getknot.dev/docs/plugins/) and what each provides. Needs the **Manage Roles** permission. The endpoint exists only while plugins are loaded, so with none the call raises.

**Returns:** `dict` containing:
- `count` (int): Number of loaded plugins
- `plugins` (list): Dicts with `name`, `version`, `description`, `permissions` (`{id, label}`), `menus`, `pages`, `peers`, `mcp_tools` (`{name, description, handler, permission, parameters}`), `handlers` (`{handler, permission}`), `field_handlers` (`{id, label, permission}`) and `libs` (`{name, version}`)
- `failed` (list): Plugins that did not load, each `{name, path, reason}`
- `warnings` (list): Load warnings
