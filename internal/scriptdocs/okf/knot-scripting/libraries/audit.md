---
type: "API Reference"
title: "knot.audit"
description: "Audit log search and filtering functions."
resource: "https://getknot.dev/reference/libraries/audit/"
tags:
  - "security"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/audit/"
---
# knot.audit

The `knot.audit` library provides audit log search and filtering functions.

---

## Functions

| Function | Description |
|----------|-------------|
| `list(...)` | List audit log entries with optional filtering |
| `search(q, ...)` | Search audit logs with a text query |
| `export(format='json', ...)` | Export every matching entry, unpaged, as entry dicts or CSV text |

---

## Usage

```python
import knot.audit as audit

# List recent audit logs
logs = audit.list()
print(f"Total: {logs['count']}")
for entry in logs['items']:
    print(f"{entry['when']} {entry['event']} by {entry['actor']}")

# Search audit logs
logs = audit.search("Login")

# Filter by actor
logs = audit.list(actor="admin@example.com")

# Filter by event type
logs = audit.list(event="Space Create")

# Filter by actor type
logs = audit.list(actor_type="System")

# Filter by date range (RFC3339)
logs = audit.list(from_time="2025-01-01T00:00:00Z", to_time="2025-01-31T23:59:59Z")

# Combine filters
logs = audit.search("template", actor_type="User", event="Template Update")

# Pagination
logs = audit.list(start=20, max_items=10)
```

---

## Function Reference

### list

```python
list(start=0, max_items=10, q="", actor="", actor_type="", event="", from_time="", to_time="")
```

List audit log entries with optional filtering.

**Parameters:**
- `start` - Offset to start at (default: 0)
- `max_items` - Maximum number of items to return (default: 10)
- `q` - Case-insensitive substring search across actor, event, details and the entry's properties
- `actor` - Filter by actor name (exact match, case-insensitive)
- `actor_type` - Filter by actor type: `User`, `System`, or `MCP` (case-insensitive)
- `event` - Filter by event type: a case-insensitive substring, so `"space"` matches every `Space ...` event
- `from_time` - Start of date range (RFC3339 string)
- `to_time` - End of date range (RFC3339 string)

**Returns:** A dict with `count` (int) and `items` (list of entry dicts).

### search

```python
search(q, start=0, max_items=10, actor="", actor_type="", event="", from_time="", to_time="")
```

Search audit logs with a text query. Convenience wrapper around `list()` with `q` as the first positional argument.

### export

```python
export(format="json", q="", actor="", actor_type="", event="", from_time="", to_time="")
```

Export every matching audit log entry, without paging. Needs the **Download Audit Logs** permission.

**Parameters:**
- `format` - `"json"` (default) or `"csv"`
- `q`, `actor`, `actor_type`, `event`, `from_time`, `to_time` - Filters, as for `list()`

**Returns:** For `"json"`, a list of entry dicts; for `"csv"`, the CSV text the web UI downloads (columns `time`, `zone`, `actor`, `actor_type`, `event`, `details`).

---

## Entry Properties

Each audit log entry contains:
- `id` - Audit log entry ID
- `zone` - Server zone name
- `actor` - Who performed the action
- `actor_type` - Type of actor (`User`, `System`, `MCP`)
- `event` - Event type (e.g. `Space Create`, `Login Success`, `Space File Op`, `Token Create`)
- `when` - Timestamp of the event
- `details` - Additional details
- `properties` - Dict of additional properties (may include `source_ip`, `user_agent`)

---

## Event Types

| Event | Description |
|-------|-------------|
| `System Start` | Server started |
| `Login Success` | Successful authentication |
| `Login Failed` | Failed authentication attempt |
| `Login Blocked` | Authentication attempt while the source is blocked |
| `Anomaly Detected` | An anomaly detection rule fired {{< pro-badge >}} |
| `User Create` / `Update` / `Delete` | User management |
| `User Link` / `User Unlink` | Linking accounts for user switching |
| `User Switch` / `User Switch Denied` | Switching into a linked account |
| `Passkey Add` / `Passkey Delete` | A user registered or removed a passkey |
| `Space Create` / `Update` / `Delete` | Space lifecycle |
| `Space Start` / `Space Stop` / `Space Restart` | Space state changes |
| `Space Transfer` / `Space Shared` / `Space Stop Share` | Space sharing |
| `Space Restrict` | A space's restrictions were set or lifted |
| `Space File Op` | File read/write in a space (when `server.audit.file_operations` is enabled; paths and byte counts only) |
| `Space Session Open` | Interactive session opened to a space (when `server.audit.space_sessions` is enabled) |
| `Tunnel Create` / `Tunnel Close` | Public tunnels |
| `Template Create` / `Update` / `Delete` | Template management |
| `Group Create` / `Update` / `Delete` | Group management |
| `Role Create` / `Update` / `Delete` | Role management |
| `Variable Create` / `Update` / `Delete` | Variable management |
| `Volume Create` / `Update` / `Delete` | Volume management |
| `Bucket Create` / `Delete` / `Rename` | File storage bucket management |
| `Bucket Share` / `Bucket Unshare` / `Bucket Transfer` | File storage bucket sharing and ownership |
| `Backup` / `Restore` | Server backup and restore |
| `Script Create` / `Update` / `Delete` / `Execute` | Script management |
| `AI Script Run` | An AI-written script ran through the `run_script` tool |
| `Skill Create` / `Update` / `Delete` | Skill management |
| `Slash Command Create` / `Update` / `Delete` | Slash command management |
| `MCP Server Create` / `Update` / `Delete` | MCP server management |
| `Stack Definition Create` / `Update` / `Delete` | Stack definition management |
| `Stack Start` / `Stack Stop` / `Stack Restart` / `Stack Delete` | Stack lifecycle |
| `Event Sink Create` / `Update` / `Delete` | Event sink management |
| `Event Sink Delivery Failed` / `Event Sink Script Failed` / `Event Sink Dropped` | Event sink delivery problems |
| `Log Sink Register` / `Log Sink Deregister` | A space registered or removed as a log sink {{< pro-badge >}} |
| `Token Create` / `Update` / `Delete` | API token management (OAuth flow tokens are not audited) |
| `Config Update` | Runtime configuration change |
