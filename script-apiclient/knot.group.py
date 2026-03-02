# knot.group - Group management library for Knot server
#
# This library provides functions for managing groups in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.group
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   groups = knot.group.list()

import knot.api as api

def list():
    """List all groups.

    Returns:
        A list of group dicts, each containing:
        - id: Group ID
        - name: Group name
        - max_spaces: Maximum spaces for group members
        - compute_units: Compute units for group members
        - storage_units: Storage units for group members

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/groups")

    result = []
    for group in response.get("groups", []):
        result.append({
            "id": group.get("group_id"),
            "name": group.get("name"),
            "max_spaces": group.get("max_spaces", 0),
            "compute_units": group.get("compute_units", 0),
            "storage_units": group.get("storage_units", 0)
        })

    return result


def get(group_id):
    """Get group by ID.

    Args:
        group_id: Group ID

    Returns:
        A dict containing group details:
        - id: Group ID
        - name: Group name
        - max_spaces: Maximum spaces for group members
        - compute_units: Compute units for group members
        - storage_units: Storage units for group members
        - max_tunnels: Maximum tunnels for group members

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/groups/{group_id}")

    return {
        "id": response.get("group_id"),
        "name": response.get("name"),
        "max_spaces": response.get("max_spaces", 0),
        "compute_units": response.get("compute_units", 0),
        "storage_units": response.get("storage_units", 0),
        "max_tunnels": response.get("max_tunnels", 0)
    }


def create(name, max_spaces=0, compute_units=0, storage_units=0, max_tunnels=0):
    """Create a new group.

    Args:
        name: Group name
        max_spaces: Maximum spaces for group members (default: 0 = unlimited)
        compute_units: Compute units for group members (default: 0)
        storage_units: Storage units for group members (default: 0)
        max_tunnels: Maximum tunnels for group members (default: 0)

    Returns:
        The new group ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "max_spaces": max_spaces,
        "compute_units": compute_units,
        "storage_units": storage_units,
        "max_tunnels": max_tunnels
    }

    response = api.post("/api/groups", body)
    return response.get("group_id")


def update(group_id, name=None, max_spaces=None, compute_units=None, storage_units=None):
    """Update group properties.

    Args:
        group_id: Group ID
        name: Group name (optional)
        max_spaces: Maximum spaces (optional)
        compute_units: Compute units (optional)
        storage_units: Storage units (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current group data first
    current = api.get(f"/api/groups/{group_id}")

    body = {
        "name": name if name is not None else current.get("name"),
        "max_spaces": max_spaces if max_spaces is not None else current.get("max_spaces", 0),
        "compute_units": compute_units if compute_units is not None else current.get("compute_units", 0),
        "storage_units": storage_units if storage_units is not None else current.get("storage_units", 0),
        "max_tunnels": current.get("max_tunnels", 0)
    }

    api.put(f"/api/groups/{group_id}", body)
    return True


def delete(group_id):
    """Delete a group.

    Args:
        group_id: Group ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/groups/{group_id}")
    return True
