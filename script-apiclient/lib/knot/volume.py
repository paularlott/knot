# knot.volume - Volume management library for Knot server
#
# This library provides functions for managing volumes in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.volume
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   volumes = knot.volume.list()

from . import api

def list():
    """List all volumes.

    Returns:
        A list of volume dicts, each containing:
        - id: Volume ID
        - name: Volume name
        - active: Boolean indicating if volume is active
        - zone: Zone name where volume is running
        - platform: Platform type

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/volumes")

    result = []
    for vol in response.get("volumes", []):
        result.append({
            "id": vol.get("volume_id"),
            "name": vol.get("name"),
            "active": vol.get("active", False),
            "zone": vol.get("zone", ""),
            "platform": vol.get("platform", "")
        })

    return result


def get(volume_id):
    """Get detailed information about a volume.

    Args:
        volume_id: Volume name or ID

    Returns:
        A dict containing volume details:
        - id: Volume ID
        - name: Volume name
        - definition: Volume definition YAML
        - active: Boolean indicating if volume is active
        - zone: Zone name
        - platform: Platform type

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/volumes/{volume_id}")

    return {
        "id": response.get("volume_id"),
        "name": response.get("name"),
        "definition": response.get("definition", ""),
        "active": response.get("active", False),
        "zone": response.get("zone", ""),
        "platform": response.get("platform", "")
    }


def create(name, definition, platform=""):
    """Create a new volume.

    Args:
        name: Volume name
        definition: Volume definition YAML
        platform: Platform type (e.g., "docker", "podman", "nomad")

    Returns:
        The new volume ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "definition": definition,
        "platform": platform
    }

    response = api.post("/api/volumes", body)
    return response.get("volume_id")


def update(volume_id, name=None, definition=None, platform=None):
    """Update volume properties.

    Args:
        volume_id: Volume name or ID
        name: New name (optional)
        definition: New definition YAML (optional)
        platform: New platform type (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    current = api.get(f"/api/volumes/{volume_id}")

    body = {
        "name": name if name is not None else current.get("name"),
        "definition": definition if definition is not None else current.get("definition", ""),
        "platform": platform if platform is not None else current.get("platform", "")
    }

    api.put(f"/api/volumes/{volume_id}", body)
    return True


def delete(volume_id):
    """Delete a volume.

    Args:
        volume_id: Volume name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/volumes/{volume_id}")
    return True


def start(volume_id):
    """Start a volume.

    Args:
        volume_id: Volume name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/volumes/{volume_id}/start")
    return True


def stop(volume_id):
    """Stop a volume.

    Args:
        volume_id: Volume name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/volumes/{volume_id}/stop")
    return True


def is_running(volume_id):
    """Check if a volume is running.

    Args:
        volume_id: Volume name or ID

    Returns:
        True if the volume is active, False otherwise

    Raises:
        Exception if not configured or on API error
    """
    vol = get(volume_id)
    return vol.get("active", False)
