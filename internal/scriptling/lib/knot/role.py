# knot.role - Role management library for Knot server

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')

def list():
    """List all roles."""
    response = api.get("/api/roles")

    result = []
    for role in response.get("roles", []):
        result.append({
            "id": role.get("role_id"),
            "name": role.get("name")
        })

    return result


def get(role_id):
    """Get role by ID."""
    response = api.get(f"/api/roles/{_enc(role_id)}")

    return {
        "id": response.get("role_id"),
        "name": response.get("name"),
        "permissions": response.get("permissions", [])
    }


def create(name, permissions=None, plugin_permissions=None):
    """Create a new role.

    permissions are built-in permission IDs (integers, e.g. the
    knot.permission constants); plugin_permissions are qualified grant
    strings (e.g. "plugin.metrics.read" — see knot.permission.list_plugin()).
    """
    body = {
        "name": name,
        "permissions": permissions or [],
        "plugin_permissions": plugin_permissions or []
    }

    response = api.post("/api/roles", body)
    return response.get("role_id")


def update(role_id, name=None, permissions=None, plugin_permissions=None):
    """Update role properties.

    permissions are built-in permission IDs; plugin_permissions are
    qualified grant strings. Omitted lists keep their current values.
    """
    current = api.get(f"/api/roles/{_enc(role_id)}")

    body = {
        "name": name if name is not None else current.get("name"),
        "permissions": permissions if permissions is not None else current.get("permissions", []),
        "plugin_permissions": plugin_permissions if plugin_permissions is not None else current.get("plugin_permissions", [])
    }

    api.put(f"/api/roles/{_enc(role_id)}", body)
    return True


def delete(role_id):
    """Delete a role."""
    api.delete(f"/api/roles/{_enc(role_id)}")
    return True
