# knot.stack - Stack management library for Knot server
#
# This library provides functions for managing stack definitions and stacks in Knot.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.stack
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   defs = knot.stack.list_defs()
#   knot.stack.create("lamp", "myproject")

import knot.apiclient as api


# ── Definition Management ──────────────────────────────────────────────

def list_defs():
    """List all stack definitions visible to the current user.

    Returns:
        A list of dicts, each containing:
        - id: Stack definition ID
        - name: Definition name
        - description: Description
        - user_id: Owner user ID (empty for global)
        - active: Whether the definition is active
        - spaces: List of space dicts

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/stack-definitions")

    result = []
    for defn in response.get("stack_definitions", []):
        result.append({
            "id": defn.get("stack_definition_id"),
            "name": defn.get("name"),
            "description": defn.get("description", ""),
            "user_id": defn.get("user_id", ""),
            "active": defn.get("active", True),
            "spaces": defn.get("spaces", []),
        })

    return result


def get_def(name):
    """Get a stack definition by name or ID.

    Args:
        name: Definition name or ID

    Returns:
        A dict containing definition details:
        - id: Stack definition ID
        - name: Definition name
        - description: Description
        - user_id: Owner user ID (empty for global)
        - icon_url: Icon URL
        - active: Whether the definition is active
        - groups: List of group IDs
        - zones: List of zone restrictions
        - spaces: List of space dicts with templates, dependencies, etc.

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/stack-definitions/{name}")

    return {
        "id": response.get("stack_definition_id"),
        "name": response.get("name"),
        "description": response.get("description", ""),
        "user_id": response.get("user_id", ""),
        "icon_url": response.get("icon_url", ""),
        "active": response.get("active", True),
        "groups": response.get("groups", []),
        "zones": response.get("zones", []),
        "spaces": response.get("spaces", []),
    }


def create_def(name, description="", scope="personal", active=True, groups=None, zones=None, spaces=None, icon_url=""):
    """Create a new stack definition.

    Args:
        name: Unique name for the definition
        description: Optional description
        scope: "personal" or "system" (default: "personal")
        active: Whether the definition is active (default: True)
        groups: List of group names allowed to create instances (system scope only)
        zones: List of zone restrictions
        spaces: List of space dicts, each containing:
            - name: Space identifier (used in prefix-name naming and dependency references)
            - template: Template name
            - description: Space description
            - shell: Override shell
            - startup_script: Startup script name
            - depends_on: List of space names this depends on
            - custom_fields: List of {name, value} dicts
            - port_forwards: List of {to_space, local_port, remote_port} dicts
        icon_url: Optional icon URL

    Returns:
        The new stack definition ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": name,
        "description": description,
        "scope": scope,
        "active": active,
        "groups": groups or [],
        "zones": zones or [],
        "spaces": spaces or [],
        "icon_url": icon_url,
    }

    response = api.post("/api/stack-definitions", body)
    return response.get("stack_definition_id")


def update_def(name, **fields):
    """Update an existing stack definition.

    Fetches the current state, merges the given fields, and saves.
    Only the fields you pass are changed.

    Args:
        name: Definition name or ID
        **fields: Fields to update (name, description, active, groups, zones,
                  spaces, icon_url, scope)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    defn = get_def(name)

    body = {
        "name": fields.get("name", defn.get("name")),
        "description": fields.get("description", defn.get("description", "")),
        "scope": fields.get("scope", "system" if not defn.get("user_id") else "personal"),
        "active": fields.get("active", defn.get("active", True)),
        "groups": fields.get("groups", defn.get("groups", [])),
        "zones": fields.get("zones", defn.get("zones", [])),
        "spaces": fields.get("spaces", defn.get("spaces", [])),
        "icon_url": fields.get("icon_url", defn.get("icon_url", "")),
    }

    api.put(f"/api/stack-definitions/{defn.get('id')}", body)
    return True


def delete_def(name):
    """Delete a stack definition.

    Args:
        name: Definition name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    defn = get_def(name)
    api.delete(f"/api/stack-definitions/{defn.get('id')}")
    return True


# ── Stack Operations ────────────────────────────────────────────────────

def create(definition_name, prefix, stack_name=None):
    """Create spaces from a stack definition.

    Creates each space with the name {prefix}-{name}, sets the stack field,
    resolves dependencies, and applies port forwards.

    Args:
        definition_name: Definition name or ID
        prefix: Prefix for space names (spaces are named prefix-name)
        stack_name: Stack name to group spaces under (defaults to prefix)

    Returns:
        A dict containing:
        - spaces: Dict mapping space names to space IDs

    Raises:
        Exception if not configured or on API error
    """
    if stack_name is None:
        stack_name = prefix

    defn = get_def(definition_name)
    spaces = defn.get("spaces", [])
    space_map = {}

    # Pass 1: Create all spaces
    for comp in spaces:
        comp_name = comp.get("name", "")
        space_name = f"{prefix}-{comp_name}"
        template_id = comp.get("template_id", comp.get("template", ""))

        custom_fields = []
        for cf in comp.get("custom_fields", []):
            custom_fields.append({"name": cf.get("name"), "value": cf.get("value")})

        body = {
            "name": space_name,
            "template_id": template_id,
            "stack": stack_name,
            "description": comp.get("description", ""),
            "custom_fields": custom_fields,
        }

        response = api.post("/api/spaces", body)
        space_id = response.get("space_id", "")
        space_map[comp_name] = space_id

    # Pass 2: Set dependencies
    for comp in spaces:
        comp_name = comp.get("name", "")
        depends_on = comp.get("depends_on", [])
        if not depends_on:
            continue

        dep_ids = []
        for dep_name in depends_on:
            if dep_name in space_map:
                dep_ids.append(space_map[dep_name])

        if dep_ids:
            space_id = space_map.get(comp_name, "")
            space_name = f"{prefix}-{comp_name}"
            update_body = {
                "name": space_name,
                "stack": stack_name,
                "depends_on": dep_ids,
            }
            api.put(f"/api/spaces/{space_id}", update_body)

    # Pass 3: Apply port forwards
    for comp in spaces:
        comp_name = comp.get("name", "")
        port_forwards = comp.get("port_forwards", [])
        if not port_forwards:
            continue

        space_id = space_map.get(comp_name, "")
        forwards = []
        for pf in port_forwards:
            target_name = pf.get("to_space", "")
            if target_name not in space_map:
                continue
            forwards.append({
                "local_port": pf.get("local_port"),
                "space": f"{prefix}-{target_name}",
                "remote_port": pf.get("remote_port"),
                "persistent": True,
            })

        if forwards:
            api.post(f"/space-io/{space_id}/port/apply", {"forwards": forwards})

    return {"spaces": space_map}


def delete(stack_name):
    """Delete all spaces in a stack.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/spaces")
    spaces = response.get("spaces", [])

    for space in spaces:
        if space.get("stack") == stack_name:
            api.delete(f"/api/spaces/{space.get('space_id', space.get('id', ''))}")

    return True


def start(stack_name):
    """Start all spaces in a stack in dependency order.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/stacks/{stack_name}/start", {})
    return True


def stop(stack_name):
    """Stop all spaces in a stack in reverse dependency order.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/stacks/{stack_name}/stop", {})
    return True


def restart(stack_name):
    """Restart all spaces in a stack.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/stacks/{stack_name}/restart", {})
    return True


def list():
    """List all stacks for the current user by grouping spaces.

    Returns:
        A list of dicts, each containing:
        - name: Stack name
        - spaces: List of space dicts belonging to the stack

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/spaces")
    spaces = response.get("spaces", [])

    stacks = {}
    order = []
    for space in spaces:
        stack = space.get("stack", "")
        if not stack:
            continue
        if stack not in stacks:
            order.append(stack)
            stacks[stack] = []
        stacks[stack].append({
            "id": space.get("space_id", space.get("id", "")),
            "name": space.get("name", ""),
            "is_running": space.get("is_deployed", False),
        })

    result = []
    for name in order:
        result.append({"name": name, "spaces": stacks[name]})
    return result
