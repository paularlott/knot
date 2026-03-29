# knot.vars - Variable management library for Knot server
#
# This library provides functions for managing template variables in Knot.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.vars
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   variables = knot.vars.list()

import knot.apiclient as api

def list():
    """List all template variables.

    Returns:
        A list of variable dicts, each containing:
        - id: Variable ID
        - name: Variable name
        - local: Boolean indicating if variable is local
        - protected: Boolean indicating if variable is protected
        - restricted: Boolean indicating if variable is restricted
        - is_managed: Boolean indicating if variable is managed

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/templatevars")

    result = []
    for var in response.get("variables", []):
        result.append({
            "id": var.get("templatevar_id"),
            "name": var.get("name"),
            "local": var.get("local", False),
            "protected": var.get("protected", False),
            "restricted": var.get("restricted", False),
            "is_managed": var.get("is_managed", False)
        })

    return result


def get(var_id):
    """Get detailed information about a variable.

    Args:
        var_id: Variable name or ID

    Returns:
        A dict containing variable details:
        - id: Variable ID
        - name: Variable name
        - value: Variable value (empty if protected)
        - zones: List of zones
        - local: Boolean indicating if variable is local
        - protected: Boolean indicating if variable is protected
        - restricted: Boolean indicating if variable is restricted
        - is_managed: Boolean indicating if variable is managed

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/templatevars/{var_id}")

    return {
        "id": response.get("templatevar_id"),
        "name": response.get("name"),
        "value": response.get("value", ""),
        "zones": response.get("zones", []),
        "local": response.get("local", False),
        "protected": response.get("protected", False),
        "restricted": response.get("restricted", False),
        "is_managed": response.get("is_managed", False)
    }


def create(name, value, zones=None, local=False, protected=False, restricted=False):
    """Create a new template variable.

    Args:
        name: Variable name
        value: Variable value
        zones: List of zones (optional)
        local: Whether variable is local (default: False)
        protected: Whether variable is protected (default: False)
        restricted: Whether variable is restricted (default: False)

    Returns:
        The new variable ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "value": value,
        "zones": zones or [],
        "local": local,
        "protected": protected,
        "restricted": restricted
    }

    response = api.post("/api/templatevars", body)
    return response.get("templatevar_id")


def set_value(var_id, value=None):
    """Set variable value.

    Args:
        var_id: Variable name or ID
        value: New value

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    return update(var_id, value=value)


def update(var_id, value=None, zones=None, local=None, protected=None, restricted=None):
    """Update variable properties.

    Args:
        var_id: Variable name or ID
        value: New value (optional)
        zones: New zones list (optional)
        local: New local flag (optional)
        protected: New protected flag (optional)
        restricted: New restricted flag (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    current = api.get(f"/api/templatevars/{var_id}")

    body = {
        "name": current.get("name"),
        "value": value if value is not None else current.get("value", ""),
        "zones": zones if zones is not None else current.get("zones", []),
        "local": local if local is not None else current.get("local", False),
        "protected": protected if protected is not None else current.get("protected", False),
        "restricted": restricted if restricted is not None else current.get("restricted", False)
    }

    api.put(f"/api/templatevars/{var_id}", body)
    return True


def delete(var_id):
    """Delete a variable.

    Args:
        var_id: Variable name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/templatevars/{var_id}")
    return True
