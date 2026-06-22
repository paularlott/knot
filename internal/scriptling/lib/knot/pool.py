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


def create(name, template_id, startup_script_id="", desired_count=1, active=True):
    """Create a pool with the given number of spaces and return its ID.

    If active is True, spaces are started as they are created. If quota is
    exhausted before all spaces are created, the pool is created with fewer
    spaces and the response message field describes the shortfall.
    """
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
