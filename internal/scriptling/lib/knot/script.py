# knot.script - Script management library for Knot server
#
# This library provides functions for managing scripts and executing scripts in
# running spaces. Requires knot.apiclient to be configured first.

import knot.apiclient as api


def _parse_script(script):
    """Parse a script response into a stable dict."""
    return {
        "id": script.get("script_id", script.get("id")),
        "user_id": script.get("user_id", ""),
        "name": script.get("name", ""),
        "description": script.get("description", ""),
        "content": script.get("content", ""),
        "groups": script.get("groups", []),
        "zones": script.get("zones", []),
        "active": script.get("active", False),
        "script_type": script.get("script_type", "script"),
        "mcp_input_schema_toml": script.get("mcp_input_schema_toml", ""),
        "mcp_keywords": script.get("mcp_keywords", []),
        "discoverable": script.get("discoverable", False),
        "is_managed": script.get("is_managed", False),
    }


def list(owner=None, all_zones=False):
    """List scripts visible to the current user.

    Args:
        owner: Optional user ID. Use "current" for the current user's scripts.
        all_zones: If True, include scripts from all zones.
    """
    params = {}
    if owner:
        if owner == "current":
            owner = api.get("/api/users/whoami").get("user_id", "")
        params["user_id"] = owner
    if all_zones:
        params["all_zones"] = "true"

    response = api.get("/api/scripts", params if params else None)
    return [_parse_script(script) for script in response.get("scripts", [])]


def list_global(all_zones=False):
    """List global scripts available for template editing."""
    params = {"all_zones": "true"} if all_zones else None
    response = api.get("/api/scripts/global", params)
    return [_parse_script(script) for script in response.get("scripts", [])]


def get(script_id):
    """Get script details by UUID."""
    return _parse_script(api.get(f"/api/scripts/{script_id}"))


def get_by_name(name):
    """Get script details by name, respecting user script shadowing."""
    return _parse_script(api.get(f"/api/scripts/name/{name}"))


def get_content(name, script_type="script"):
    """Get script content by name and type."""
    return api.get(f"/api/scripts/name/{name}/{script_type}")


def create(name, content, description="", owner=None, groups=None, zones=None,
           active=True, script_type="script", mcp_input_schema_toml="",
           mcp_keywords=None, discoverable=False):
    """Create a script and return its ID.

    Args:
        owner: Optional user ID. Use "current" to create an own script;
               omit or pass None to create a global script.
    """
    body = {
        "user_id": owner or "",
        "name": name,
        "description": description,
        "content": content,
        "groups": groups or [],
        "zones": zones or [],
        "active": active,
        "script_type": script_type,
        "mcp_input_schema_toml": mcp_input_schema_toml,
        "mcp_keywords": mcp_keywords or [],
        "discoverable": discoverable,
    }
    response = api.post("/api/scripts", body)
    return response.get("script_id")


def update(script_id, name=None, content=None, description=None, groups=None,
           zones=None, active=None, script_type=None, mcp_input_schema_toml=None,
           mcp_keywords=None, discoverable=None):
    """Update a script while preserving fields you do not pass."""
    current = get(script_id)
    body = {
        "name": name if name is not None else current.get("name", ""),
        "description": description if description is not None else current.get("description", ""),
        "content": content if content is not None else current.get("content", ""),
        "groups": groups if groups is not None else current.get("groups", []),
        "zones": zones if zones is not None else current.get("zones", []),
        "active": active if active is not None else current.get("active", False),
        "script_type": script_type if script_type is not None else current.get("script_type", "script"),
        "mcp_input_schema_toml": mcp_input_schema_toml if mcp_input_schema_toml is not None else current.get("mcp_input_schema_toml", ""),
        "mcp_keywords": mcp_keywords if mcp_keywords is not None else current.get("mcp_keywords", []),
        "discoverable": discoverable if discoverable is not None else current.get("discoverable", False),
    }
    api.put(f"/api/scripts/{script_id}", body)
    return True


def delete(script_id):
    """Delete a script by UUID."""
    api.delete(f"/api/scripts/{script_id}")
    return True


def execute(space_name, script_name=None, script_id=None, content=None, args=None):
    """Execute a named, ID-based, or inline script in a running space."""
    body = {
        "arguments": args or [],
    }
    if script_id:
        body["script_id"] = script_id
    elif script_name:
        body["script_name"] = script_name
    elif content:
        body["content"] = content
    else:
        raise Exception("script_name, script_id, or content is required")

    response = api.post(f"/api/spaces/{space_name}/execute-script", body)
    return {
        "output": response.get("output", ""),
        "error": response.get("error", ""),
        "exit_code": response.get("exit_code", 0),
    }


def execute_content(space_name, content, args=None):
    """Execute inline script content in a running space."""
    return execute(space_name, content=content, args=args)

