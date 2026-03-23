# knot.role - Role management library for Knot server

from . import api

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
    response = api.get(f"/api/roles/{role_id}")

    return {
        "id": response.get("role_id"),
        "name": response.get("name"),
        "permissions": response.get("permissions", [])
    }


def create(name, permissions=None):
    """Create a new role."""
    body = {
        "name": name,
        "permissions": permissions or []
    }

    response = api.post("/api/roles", body)
    return response.get("role_id")


def update(role_id, name=None, permissions=None):
    """Update role properties."""
    current = api.get(f"/api/roles/{role_id}")

    body = {
        "name": name if name is not None else current.get("name"),
        "permissions": permissions if permissions is not None else current.get("permissions", [])
    }

    api.put(f"/api/roles/{role_id}", body)
    return True


def delete(role_id):
    """Delete a role."""
    api.delete(f"/api/roles/{role_id}")
    return True
