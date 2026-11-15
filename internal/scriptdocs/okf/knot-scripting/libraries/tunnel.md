---
type: "API Reference"
title: "knot.tunnel"
description: "List and close your tunnels, and read the tunnel server settings."
resource: "https://getknot.dev/reference/libraries/tunnel/"
tags:
  - "tunnels"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/tunnel/"
---
# knot.tunnel

The `knot.tunnel` library lists and closes the current user's [tunnels](https://getknot.dev/docs/tunnels/) — every tunnel open under your name, whichever client or space runs it — and reads the tunnel server settings. Tunnels a space runs are started and stopped with [`knot.space.tunnel_start`](../space/#tunnel_startspace-protocol-port-name-server-token) and `tunnel_stop`. Every function needs the **Use Tunnels** permission.

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List your open tunnels |
| `server_info()` | Get the tunnel server settings |
| `delete(name)` | Close one of your tunnels |

---

### list()

List your open tunnels from every client and space.

**Returns:** `list` of dicts sorted by name, each containing:
- `name` (string): Full tunnel name, `<username>--<tunnel>`
- `address` (string): Public URL, e.g. `https://alice--web.tunnel.example.com`

---

### server_info()

**Returns:** `dict` containing:
- `domain` (string): Tunnel domain suffix, e.g. `.tunnel.example.com`
- `tunnel_servers` (list): Tunnel server URLs

---

### delete(name)

Close one of your tunnels, disconnecting the client that runs it.

**Parameters:**
- `name` (string): The full `<username>--<tunnel>` name, or the bare tunnel name

**Returns:** `bool` - `True` on success. Raises if the tunnel is not found.

---

## Examples

```python
import knot.tunnel as tunnel

for t in tunnel.list():
    print(t["name"], t["address"])

tunnel.delete("web")
```
