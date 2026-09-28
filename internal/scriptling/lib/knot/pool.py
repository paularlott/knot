# knot.pool - Space pool management library for Knot server
#
# Requires knot.apiclient to be configured first.

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def _parse_member(member):
    """Parse a pool member response into a stable dict."""
    return {
        "id": member.get("space_id", member.get("id")),
        "name": member.get("name", ""),
        "state": member.get("state", ""),
        "combined_rps": member.get("combined_rps", 0),
        "method_rps": member.get("method_rps", 0),
        "http_rps": member.get("http_rps", 0),
        "tcp_rps": member.get("tcp_rps", 0),
        "method_inflight": member.get("method_inflight", 0),
        "cpu_percent": member.get("cpu_percent", 0),
        "memory_percent": member.get("memory_percent", 0),
        "healthy": member.get("healthy", True),
        "is_pending": member.get("is_pending", False),
        "is_deleting": member.get("is_deleting", False),
        "is_deployed": member.get("is_deployed", False),
        "lease_state": member.get("lease_state", ""),  # "" | active | draining
        "lease_holder": member.get("lease_holder", ""),
        "lease_expires_at": member.get("lease_expires_at"),
    }


def _parse_pool(pool):
    """Parse a pool response into a stable dict."""
    util = pool.get("utilization", {}) or {}
    return {
        "id": pool.get("pool_id", pool.get("id")),
        "name": pool.get("name", ""),
        "template_id": pool.get("template_id", ""),
        "startup_script_id": pool.get("startup_script_id", ""),
        "desired_count": pool.get("desired_count", 0),
        "alive_members": pool.get("alive_members", 0),
        "active": pool.get("active", False),
        "lease_max_time": pool.get("lease_max_time", 0),  # seconds; 0 off, -1 no timeout
        "lease_max_extensions": pool.get("lease_max_extensions", 0),  # 0 off, -1 unlimited
        "utilization": {
            "combined_rps": util.get("combined_rps", 0),
            "method_rps": util.get("method_rps", 0),
            "http_rps": util.get("http_rps", 0),
            "tcp_rps": util.get("tcp_rps", 0),
            "method_inflight": util.get("method_inflight", 0),
            "avg_cpu_percent": util.get("avg_cpu_percent", 0),
            "avg_memory_percent": util.get("avg_memory_percent", 0),
        },
        "members": [_parse_member(member) for member in pool.get("members", [])],
    }


def list():
    """List visible pools with utilization."""
    response = api.get("/api/pools")
    return [_parse_pool(pool) for pool in response.get("pools", [])]


def get(name):
    """Get pool details and utilization by name or ID."""
    response = api.get(f"/api/pools/{_enc(name)}")
    return _parse_pool(response)


def create(name, template_name, startup_script_id="", desired_count=1, active=True):
    """Create a pool with the given number of spaces and return its ID.

    Accepts a template name (resolved to ID internally).
    If active is True, spaces are started as they are created.
    Bridged KVM templates are rejected — their spaces need an IP address
    chosen at creation, which a pool can't provide; NAT KVM templates work.
    """
    import knot.template
    tmpl = knot.template.get(template_name)
    template_id = tmpl.get("id") if tmpl else None
    if not template_id:
        raise Exception(f"Template not found: {template_name}")

    response = api.post("/api/pools", {
        "name": name,
        "template_id": template_id,
        "startup_script_id": startup_script_id,
        "desired_count": desired_count,
        "active": active,
    })
    return response.get("pool_id")


def update(name, desired_count=None, active=None):
    """Update the pool's desired count or active state.

    Pool name, template, and startup script are immutable after creation.
    """
    current = get(name)
    body = {
        "desired_count": current.get("desired_count", 1),
        "active": current.get("active", True),
    }
    if desired_count is not None:
        body["desired_count"] = desired_count
    if active is not None:
        body["active"] = active

    api.put(f"/api/pools/{_enc(current.get('id'))}", body)
    return True


def delete(name):
    """Delete a stopped pool and all its spaces. Pool must be stopped first."""
    api.delete(f"/api/pools/{_enc(name)}")
    return True


def set_size(name, desired_count):
    """Set the pool's desired space count.

    The sweep loop creates, drains, or deletes spaces asynchronously.
    """
    api.post(f"/api/pools/{_enc(name)}/size", {"desired_count": desired_count})
    return True


def start(name):
    """Start a stopped pool: starts all member spaces and creates any missing ones."""
    api.post(f"/api/pools/{_enc(name)}/start")
    return True


def stop(name):
    """Stop a running pool: stops all member spaces without deleting them."""
    api.post(f"/api/pools/{_enc(name)}/stop")
    return True


# ---------------------------------------------------------------------------
# Exclusive member leases
# ---------------------------------------------------------------------------


def _parse_lease(lease):
    """Parse a lease response into a stable dict."""
    return {
        "lease_id": lease.get("lease_id", ""),
        "pool_id": lease.get("pool_id", ""),
        "pool_name": lease.get("pool_name", ""),
        "space_id": lease.get("space_id", ""),
        "space_name": lease.get("space_name", ""),
        "user_id": lease.get("user_id", ""),
        "username": lease.get("username", ""),
        "expires_at": lease.get("expires_at"),  # None = never expires
        "extensions_used": lease.get("extensions_used", 0),
        "max_extensions": lease.get("max_extensions", 0),  # -1 = unlimited
        "state": lease.get("state", "active"),  # active | draining
    }


def _duration_seconds(time):
    """Convert a lease duration into duration_seconds for the API.

    Accepts None (use the pool's configured maximum), "none" (never
    expire; unlimited pools only), a number of seconds, or a duration
    string like "5m" / "90s" / "2h".
    """
    if time is None:
        return 0
    if isinstance(time, (int, float)):
        return int(time)
    if time == "none" or time == "never":
        return -1
    import re
    match = re.fullmatch(r"(\d+(?:\.\d+)?)\s*([smhd]?)", str(time).strip())
    if not match:
        raise Exception(f"Invalid lease duration: {time!r} (use seconds, '5m', '2h' or 'none')")
    value = float(match.group(1))
    unit = match.group(2) or "s"
    multipliers = {"s": 1, "m": 60, "h": 3600, "d": 86400}
    return int(value * multipliers[unit])


def acquire(name, time=None, wait=None):
    """Acquire a pool member exclusively until the lease ends.

    time: None (pool's max), "none" (never; unlimited pools), seconds or
    "5m"-style string. wait: optionally wait this long for a free member
    ("30s" style or seconds) before raising; the server caps it at 5m.

    Returns the lease dict; the held member is space_name / space_id. Pin
    method calls to it with the space_id while held.
    """
    response = api.post(f"/api/pools/{_enc(name)}/acquire", {
        "duration_seconds": _duration_seconds(time),
        "wait_seconds": _duration_seconds(wait) if wait is not None else 0,
    })
    return _parse_lease(response)


def extend(name, lease_id, time=None):
    """Extend a held lease: the new deadline is now + time (or never on
    unlimited pools). Bounded by the pool's max extension count."""
    response = api.post(f"/api/pools/{_enc(name)}/leases/{_enc(lease_id)}/extend", {
        "duration_seconds": _duration_seconds(time),
    })
    return _parse_lease(response)


def release(name, lease_id):
    """Release a held lease early. The member returns to the pool after
    in-flight work drains (normally within ~15s)."""
    response = api.delete(f"/api/pools/{_enc(name)}/leases/{_enc(lease_id)}")
    return _parse_lease(response)


def leases(name):
    """List the pool's held leases — active plus draining."""
    response = api.get(f"/api/pools/{_enc(name)}/leases")
    return [_parse_lease(lease) for lease in response.get("leases", [])]


class leased:
    """Context manager for exclusive pool member use.

    with knot.pool.leased("browsers", "5m") as member:
        knot.apiclient.post("/api/methods/call", {
            "jsonrpc": "2.0", "id": 1,
            "method": "open_page",
            "params": {"url": "https://example.com"},
            "space_id": member["space_id"],  # pin to the held member
        })

    Releases on exit (including on exception). acquire-time errors
    propagate; release errors on exit are suppressed to not mask the
    original exception.
    """

    def __init__(self, name, time=None, wait=None):
        self._name = name
        self._time = time
        self._wait = wait
        self.lease = None

    def __enter__(self):
        self.lease = acquire(self._name, time=self._time, wait=self._wait)
        return self.lease

    def __exit__(self, exc_type, exc_value, traceback):
        if self.lease is not None:
            try:
                release(self._name, self.lease.get("lease_id"))
            except Exception:
                pass
        return False
