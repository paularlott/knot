# knot.user - User management library for Knot server
#
# This library provides functions for managing users in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.user
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   me = knot.user.get_me()
#   users = knot.user.list()

import knot.api as api

def get_me():
    """Get current user details.

    Returns:
        A dict containing user details:
        - id: User ID
        - username: Username
        - email: Email address
        - active: Boolean indicating if user is active
        - max_spaces: Maximum number of spaces allowed
        - compute_units: Compute units allocated
        - storage_units: Storage units allocated
        - max_tunnels: Maximum tunnels allowed
        - preferred_shell: Preferred shell
        - timezone: User timezone
        - github_username: GitHub username
        - number_spaces: Current number of spaces
        - number_spaces_deployed: Number of running spaces
        - used_compute_units: Compute units in use
        - used_storage_units: Storage units in use
        - used_tunnels: Tunnels in use
        - roles: List of role names
        - groups: List of group names

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/users/whoami")
    return _parse_user(response)


def get(user_id):
    """Get user by ID, username, or email.

    Args:
        user_id: User ID, username, or email

    Returns:
        A dict containing user details (see get_me() for structure)

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/users/{user_id}")
    return _parse_user(response)


def list(state="", zone=""):
    """List all users with optional filters.

    Args:
        state: Filter by state (e.g., "active", "inactive")
        zone: Filter by zone

    Returns:
        A list of user dicts, each containing:
        - id: User ID
        - username: Username
        - email: Email address
        - active: Boolean indicating if user is active
        - number_spaces: Current number of spaces

    Raises:
        Exception if not configured or on API error
    """
    params = {}
    if state:
        params["state"] = state
    if zone:
        params["zone"] = zone

    response = api.get("/api/users", params if params else None)

    result = []
    for user in response.get("users", []):
        result.append({
            "id": user.get("user_id"),
            "username": user.get("username"),
            "email": user.get("email"),
            "active": user.get("active", False),
            "number_spaces": user.get("number_spaces", 0)
        })

    return result


def create(username, email, password, active=True, max_spaces=0, compute_units=0, storage_units=0, max_tunnels=0):
    """Create a new user.

    Args:
        username: Username
        email: Email address
        password: Password
        active: Whether user is active (default: True)
        max_spaces: Maximum spaces allowed (default: 0 = unlimited)
        compute_units: Compute units allocated (default: 0)
        storage_units: Storage units allocated (default: 0)
        max_tunnels: Maximum tunnels allowed (default: 0)

    Returns:
        The new user ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "username": username,
        "email": email,
        "password": password,
        "active": active,
        "max_spaces": max_spaces,
        "compute_units": compute_units,
        "storage_units": storage_units,
        "max_tunnels": max_tunnels
    }

    response = api.post("/api/users", body)
    return response.get("user_id")


def update(user_id, username=None, email=None, password=None, active=None, max_spaces=None):
    """Update user properties.

    Args:
        user_id: User ID, username, or email
        username: New username (optional)
        email: New email (optional)
        password: New password (optional)
        active: Whether user is active (optional)
        max_spaces: Maximum spaces allowed (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current user data first
    current = api.get(f"/api/users/{user_id}")

    body = {
        "username": username if username is not None else current.get("username"),
        "email": email if email is not None else current.get("email"),
        "service_password": current.get("service_password", ""),
        "roles": current.get("roles", []),
        "groups": current.get("groups", []),
        "active": active if active is not None else current.get("active", True),
        "max_spaces": max_spaces if max_spaces is not None else current.get("max_spaces", 0),
        "compute_units": current.get("compute_units", 0),
        "storage_units": current.get("storage_units", 0),
        "max_tunnels": current.get("max_tunnels", 0),
        "ssh_public_key": current.get("ssh_public_key", ""),
        "github_username": current.get("github_username", ""),
        "preferred_shell": current.get("preferred_shell", ""),
        "timezone": current.get("timezone", ""),
        "totp_secret": current.get("totp_secret", "")
    }

    if password is not None:
        body["password"] = password

    api.put(f"/api/users/{user_id}", body)
    return True


def delete(user_id):
    """Delete a user.

    Args:
        user_id: User ID, username, or email

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/users/{user_id}")
    return True


def get_quota(user_id):
    """Get user quota and usage.

    Args:
        user_id: User ID, username, or email

    Returns:
        A dict containing:
        - max_spaces: Maximum spaces allowed
        - compute_units: Compute units allocated
        - storage_units: Storage units allocated
        - max_tunnels: Maximum tunnels allowed
        - number_spaces: Current number of spaces
        - number_spaces_deployed: Number of running spaces
        - used_compute_units: Compute units in use
        - used_storage_units: Storage units in use
        - used_tunnels: Tunnels in use

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/users/{user_id}/quota")

    return {
        "max_spaces": response.get("max_spaces", 0),
        "compute_units": response.get("compute_units", 0),
        "storage_units": response.get("storage_units", 0),
        "max_tunnels": response.get("max_tunnels", 0),
        "number_spaces": response.get("number_spaces", 0),
        "number_spaces_deployed": response.get("number_spaces_deployed", 0),
        "used_compute_units": response.get("used_compute_units", 0),
        "used_storage_units": response.get("used_storage_units", 0),
        "used_tunnels": response.get("used_tunnels", 0)
    }


def list_permissions(user_id):
    """List all permissions for a user.

    Args:
        user_id: User ID, username, or email

    Returns:
        A list of permission IDs (integers)

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/users/{user_id}/permissions")
    return response.get("permissions", [])


def has_permission(user_id, permission_id):
    """Check if user has a specific permission.

    Args:
        user_id: User ID, username, or email
        permission_id: Permission ID (integer or string)

    Returns:
        True if user has the permission, False otherwise

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/users/{user_id}/has-permission", {"permission": str(permission_id)})
    return response.get("has_permission", False)


def _parse_user(response):
    """Parse a user response into a standardized dict."""
    return {
        "id": response.get("user_id"),
        "username": response.get("username"),
        "email": response.get("email"),
        "active": response.get("active", False),
        "max_spaces": response.get("max_spaces", 0),
        "compute_units": response.get("compute_units", 0),
        "storage_units": response.get("storage_units", 0),
        "max_tunnels": response.get("max_tunnels", 0),
        "preferred_shell": response.get("preferred_shell", ""),
        "timezone": response.get("timezone", ""),
        "github_username": response.get("github_username", ""),
        "number_spaces": response.get("number_spaces", 0),
        "number_spaces_deployed": response.get("number_spaces_deployed", 0),
        "used_compute_units": response.get("used_compute_units", 0),
        "used_storage_units": response.get("used_storage_units", 0),
        "used_tunnels": response.get("used_tunnels", 0),
        "current": response.get("current", False),
        "roles": response.get("roles", []),
        "groups": response.get("groups", [])
    }
