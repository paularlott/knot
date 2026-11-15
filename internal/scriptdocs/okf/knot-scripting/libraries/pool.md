---
type: "API Reference"
title: "knot.pool"
description: "Manage space pools that keep a desired count of identical spaces running."
resource: "https://getknot.dev/reference/libraries/pool/"
tags:
  - "spaces"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/pool/"
---
# knot.pool

The `knot.pool` library manages space pools. A pool keeps a desired count of identical spaces (created from the same template) running and ready, so the server can hand out method, HTTP, and TCP traffic across healthy members. Pools are useful for scaling stateless services and for method backends that need more capacity than a single space. Lease-enabled pools additionally support exclusive member checkout — `acquire()`, `extend()`, `release()`, `leases()` and the `leased()` context manager — for callers that need a member to themselves.

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List visible pools with current utilization |
| `get(name)` | Get pool details and utilization by name or ID |
| `create(name, template_name, startup_script_id='', desired_count=1, active=True, lease_max_time=0, lease_max_extensions=0, custom_fields=None)` | Create a pool from a template (name or ID) and return its ID. Set `lease_max_time` to enable member leases (`-1` = no timeout, seconds = maximum lease length) and `lease_max_extensions` to allow extending them (`-1` = unlimited). `custom_fields` (a list of `{"name": ..., "value": ...}`) is given to every member; fields left out get the template default. Bridged KVM templates are rejected — their spaces need an IP address chosen at creation, which a pool can't provide; NAT KVM templates work |
| `update(name, desired_count=None, active=None, lease_max_time=None, lease_max_extensions=None, custom_fields=None)` | Update the pool's desired count, active state, lease settings or custom fields; omitted values are kept. `custom_fields` replaces the members' values (`[]` clears them); members pick them up when next created or started |
| `delete(name)` | Delete a stopped pool and all its spaces |
| `set_size(name, desired_count)` | Set the pool's desired space count |
| `start(name)` | Start a stopped pool (starts all members, creates any missing) |
| `stop(name)` | Stop a running pool (stops all members without deleting them) |
| `acquire(name, time=None, wait=None)` | Acquire a member exclusively until the lease ends (lease-enabled pools). `time`: `None` = the pool's maximum, `"none"` = never expire (no-timeout pools only), seconds or a `"5m"`-style string. `wait`: optionally wait this long for a free member before raising |
| `extend(space, time=None)` | Renew the lease held on a member (space name or id) — the new deadline is now + `time` (or never, on no-timeout pools). Bounded by the pool's extension count |
| `release(space, destroy=False)` | Release the lease held on a member (space name or id — what acquire returned); the member returns to the pool after in-flight work drains (~15s). With `destroy=True` the member is deleted and a fresh replacement is created, so the next acquire gets a clean space |
| `leases(name)` | List the pool's held leases — active plus draining |
| `leased(name, time=None, wait=None, destroy=False)` | Context manager: acquire on entry, release (or destroy) on exit |

---

## Usage

```python
import knot.pool as pool

# List pools
for p in pool.list():
    print(f"{p['name']}: {p['alive_members']}/{p['desired_count']} alive")

# Create a pool of 3 spaces from a template
pool_id = pool.create(
    "api-pool",
    "my-service",
    desired_count=3,
    active=True,
)

# Scale the pool up
pool.set_size("api-pool", 5)

# Stop and later start the pool
pool.stop("api-pool")
pool.start("api-pool")

# Delete the pool (must be stopped first)
pool.delete("api-pool")
```

Exclusive member leases (requires a lease-enabled pool — set `lease_max_time`
with `create()` or `update()`, in the web UI, or through the pools REST API):

```python
import knot.apiclient
import knot.pool as pool

# Check a member out for 5 minutes; the held instance is
# member["space_name"] / member["space_id"]
member = pool.acquire("build-workers", time="5m", wait="30s")

# Pin method calls to it while held
knot.apiclient.post("/api/methods/call", {
    "jsonrpc": "2.0", "id": 1,
    "method": "run_build",
    "params": {},
    "space_id": member["space_id"],
})

pool.extend(member["space_name"], time="5m")
pool.release(member["space_name"])

# Or let the context manager release on exit (including on exception)
with pool.leased("build-workers", "5m") as member:
    ...
```

---

## Pool Properties

`get()` and `list()` return pool dicts containing:

- `id` - Pool ID
- `name` - Pool name
- `template_id` - Template the pool's spaces are created from
- `startup_script_id` - Startup script applied to members
- `desired_count` - Target number of spaces
- `alive_members` - Number of currently healthy members
- `active` - Whether the pool is active (members are started as they are created)
- `lease_max_time` - Lease time budget in seconds: `0` = leases disabled, `-1` = no timeout
- `lease_max_extensions` - Max extensions per lease: `0` = extending forbidden, `-1` = unlimited
- `custom_fields` - Custom field values given to every member, as `{"name": ..., "value": ...}` (template defaults not included)
- `utilization` - Aggregate utilization across members:
  - `combined_rps` - Total requests per second (method + HTTP + TCP)
  - `method_rps` - Method requests per second
  - `http_rps` - HTTP requests per second
  - `tcp_rps` - TCP requests per second
  - `method_inflight` - In-flight method requests
  - `avg_cpu_percent` - Average CPU usage across members
  - `avg_memory_percent` - Average memory usage across members
- `members` - List of member space dicts (see below)

---

## Member Properties

Each member in `members` contains:

- `id` - Space ID
- `name` - Space name
- `state` - Member state: `"alive"` (agent connected and healthy, receiving traffic), `"starting"` (pending), `"stopping"` (being deleted) or `"dead"` (not running, or unhealthy)
- `combined_rps`, `method_rps`, `http_rps`, `tcp_rps` - Per-member request rates
- `method_inflight` - In-flight method requests
- `cpu_percent` - CPU usage
- `memory_percent` - Memory usage
- `healthy` - Whether the member is healthy
- `is_pending` - Whether the member is pending creation
- `is_deleting` - Whether the member is being deleted
- `is_deployed` - Whether the member is deployed (running)
- `lease_state` - Lease state: `""` free, `"active"` exclusively leased, `"draining"` lease ended and waiting for in-flight work to finish
- `lease_holder` - Username of the lease holder (when leased)
- `lease_expires_at` - Lease expiry (`None` = never expires)

---

## Lease Properties

`acquire()`, `extend()` and `release()` return lease dicts, and `leases()`
returns a list of them:

- `pool_name` - The pool the lease was granted from
- `space_id`, `space_name` - The held member; extend/release take these, and method calls can be pinned with `space_id`
- `username` - The lease holder
- `expires_at` - When the lease ends (`None` = never expires)
- `extensions_used`, `max_extensions` - Extension counter and cap (`-1` = unlimited)
- `state` - `"active"`, `"draining"` (ended, waiting for in-flight work) or `"destroying"` (released with `destroy=True`)

---

## Lifecycle Notes

- `create()` accepts a template **name** or ID (resolved to an ID internally). The template, startup script, and pool name are immutable after creation; only `desired_count`, `active`, `lease_max_time`, `lease_max_extensions` and `custom_fields` are mutable via `update()`.
- `delete()` requires the pool to be stopped first.
- `set_size()`, `start()`, and `stop()` are asynchronous: the server's sweep loop creates, drains, or deletes member spaces to reach the desired state.
- Lease functions require the pool to be lease-enabled (`lease_max_time != 0`) and active. The simplest mode is a no-timeout pool (`lease_max_time = -1`): acquire, use, release — `time` can stay `None` everywhere. While a lease is held, shared method routing and pool-name port routing skip the member; on expiry or release the member returns to the pool after in-flight method work drains, normally within one 15-second sweep. `acquire()` raises when no member is free (after `wait`, if given); `extend()` raises once the pool's extension count is exhausted.
