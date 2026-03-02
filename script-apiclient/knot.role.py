# knot.role - Role management library for Knot server
#
# This library provides functions for managing roles in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.role
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   roles = knot.role.list()

import knot.api as api

def list():
    """List all roles.

    Returns:
        A list of role dicts, each containing:
        - id: Role ID
        - name: Role name

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/roles")

    result = []
    for role in response.get("roles", []):
        result.append({
            "id": role.get("role_id"),
            "name": role.get("name")
        })

    return result


def get(role_id):
    """Get role by ID.

    Args:
        role_id: Role ID

    Returns:
        A dict containing role details:
        - id: Role ID
        - name: Role name
        - permissions: List of permission IDs (integers)

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/roles/{role_id}")

    return {
        "id": response.get("role_id"),
        "name": response.get("name"),
        "permissions": response.get("permissions", [])
    }


def create(name, permissions=None):
    """Create a new role.

    Args:
        name: Role name
        permissions: List of permission IDs (optional)

    Returns:
        The new role ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "permissions": permissions or []
    }

    response = api.post("/api/roles", body)
    return response.get("role_id")


def update(role_id, name=None, permissions=None):
    """Update role properties.

    Args:
        role_id: Role ID
        name: Role name (optional)
        permissions: List of permission IDs (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current role data first
    current = api.get(f"/api/roles/{role_id}")

    body = {
        "name": name if name is not None else current.get("name"),
        "permissions": permissions if permissions is not None else current.get("permissions", [])
    }

    api.put(f"/api/roles/{role_id}", body)
    return True


def delete(role_id):
    """Delete a role.

    Args:
        role_id: Role ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/roles/{role_id}")
    return True
