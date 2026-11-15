# knot.group - Group management library for Knot server

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')

def _parse_group(group):
    """Parse a group response into a stable dict."""
    return {
        "id": group.get("group_id"),
        "name": group.get("name"),
        "max_spaces": group.get("max_spaces", 0),
        "compute_units": group.get("compute_units", 0),
        "storage_units": group.get("storage_units", 0),
        "max_tunnels": group.get("max_tunnels", 0),
        "file_storage_mb": group.get("file_storage_mb", 0),
        "max_buckets": group.get("max_buckets", 0)
    }


def list():
    """List all groups."""
    response = api.get("/api/groups")
    return [_parse_group(group) for group in (response.get("groups") or [])]


def get(group_id):
    """Get group by ID."""
    return _parse_group(api.get(f"/api/groups/{_enc(group_id)}"))


def create(name, max_spaces=0, compute_units=0, storage_units=0, max_tunnels=0, file_storage_mb=0, max_buckets=0):
    """Create a new group."""
    body = {
        "name": name,
        "max_spaces": max_spaces,
        "compute_units": compute_units,
        "storage_units": storage_units,
        "max_tunnels": max_tunnels,
        "file_storage_mb": file_storage_mb,
        "max_buckets": max_buckets
    }

    response = api.post("/api/groups", body)
    return response.get("group_id")


def update(group_id, name=None, max_spaces=None, compute_units=None, storage_units=None, max_tunnels=None, file_storage_mb=None, max_buckets=None):
    """Update group properties; fields not given keep their current value."""
    current = api.get(f"/api/groups/{_enc(group_id)}")

    def pick(value, key):
        return value if value is not None else current.get(key, 0)

    body = {
        "name": name if name is not None else current.get("name"),
        "max_spaces": pick(max_spaces, "max_spaces"),
        "compute_units": pick(compute_units, "compute_units"),
        "storage_units": pick(storage_units, "storage_units"),
        "max_tunnels": pick(max_tunnels, "max_tunnels"),
        "file_storage_mb": pick(file_storage_mb, "file_storage_mb"),
        "max_buckets": pick(max_buckets, "max_buckets")
    }

    api.put(f"/api/groups/{_enc(group_id)}", body)
    return True


def delete(group_id):
    """Delete a group."""
    api.delete(f"/api/groups/{_enc(group_id)}")
    return True
