# knot.var - Template variable management library for Knot server
#
# This library provides functions for managing template variables in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.var
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   vars = knot.var.list()

import knot.api as api

def list():
    """List all template variables.

    Returns:
        A list of variable dicts, each containing:
        - id: Variable ID
        - name: Variable name
        - local: Boolean indicating if variable is local
        - protected: Boolean indicating if variable is protected
        - restricted: Boolean indicating if variable is restricted

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/template-vars")

    result = []
    for v in response.get("template_var", []):
        result.append({
            "id": v.get("template_var_id"),
            "name": v.get("name"),
            "local": v.get("local", False),
            "protected": v.get("protected", False),
            "restricted": v.get("restricted", False)
        })

    return result


def get(var_id):
    """Get variable value.

    Args:
        var_id: Variable ID or name

    Returns:
        A dict containing variable details:
        - id: Variable ID
        - name: Variable name
        - value: Variable value
        - local: Boolean indicating if variable is local
        - protected: Boolean indicating if variable is protected
        - restricted: Boolean indicating if variable is restricted

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/template-vars/{var_id}")

    return {
        "id": response.get("template_var_id"),
        "name": response.get("name"),
        "value": response.get("value", ""),
        "local": response.get("local", False),
        "protected": response.get("protected", False),
        "restricted": response.get("restricted", False)
    }


def create(name, value, local=False, protected=False):
    """Create a new variable.

    Args:
        name: Variable name
        value: Variable value
        local: Whether variable is local (default: False)
        protected: Whether variable is protected (default: False)

    Returns:
        The new variable ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "value": value,
        "local": local,
        "protected": protected,
        "zones": []
    }

    response = api.post("/api/template-vars", body)
    return response.get("template_var_id")


def set(var_id, value):
    """Set variable value (updates existing variable).

    Args:
        var_id: Variable ID or name
        value: New value

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current variable data first
    current = api.get(f"/api/template-vars/{var_id}")

    body = {
        "id": current.get("template_var_id"),
        "name": current.get("name"),
        "value": value,
        "zones": current.get("zones", []),
        "local": current.get("local", False),
        "protected": current.get("protected", False),
        "restricted": current.get("restricted", False),
        "is_managed": current.get("is_managed", False)
    }

    api.put(f"/api/template-vars/{var_id}", body)
    return True


def delete(var_id):
    """Delete a variable.

    Args:
        var_id: Variable ID or name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/template-vars/{var_id}")
    return True
