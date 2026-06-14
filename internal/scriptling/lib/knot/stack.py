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
import knot.space as space_lib
import knot.template as template_lib


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
        "active": response.get("active", True),
        "groups": response.get("groups", []),
        "zones": response.get("zones", []),
        "spaces": response.get("spaces", []),
    }


def create_def(name, description="", scope="user", active=True, groups=None, zones=None, spaces=None):
    """Create a new stack definition.

    Args:
        name: Unique name for the definition
        description: Optional description
        scope: "user" or "global" (default: "user")
        active: Whether the definition is active (default: True)
        groups: List of group names allowed to create instances (global scope only)
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
                  spaces, scope)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    defn = get_def(name)

    body = {
        "name": fields.get("name", defn.get("name")),
        "description": fields.get("description", defn.get("description", "")),
        "scope": fields.get("scope", "global" if not defn.get("user_id") else "user"),
        "active": fields.get("active", defn.get("active", True)),
        "groups": fields.get("groups", defn.get("groups", [])),
        "zones": fields.get("zones", defn.get("zones", [])),
        "spaces": fields.get("spaces", defn.get("spaces", [])),
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


def validate_def(spaces, name="", description="", scope="user", active=True,
                 groups=None, zones=None):
    """Validate a stack definition without creating it.

    Checks for required fields, circular dependencies, invalid references,
    duplicate space names, and port number ranges.

    Args:
        spaces: List of space dicts, each containing:
            - name: Space identifier
            - template_id: Template ID
            - depends_on: List of space names this depends on
            - port_forwards: List of {to_space, local_port, remote_port} dicts
            - custom_fields: List of {name, value} dicts
        name: Definition name (recommended, checked if provided)
        description: Optional description
        scope: "user" or "global" (default: "user")
        active: Whether the definition would be active
        groups: List of group IDs
        zones: List of zone restrictions

    Returns:
        A dict containing:
        - valid: True if no errors were found
        - errors: List of error dicts (each with field, message, and optionally space)

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
    }

    return api.post("/api/stack-definitions/validate", body)


def add_component(stack_definition, template, name, description="", shell="",
                  startup_script="", depends_on=None):
    """Add a component (template binding) to an existing stack definition.

    Resolves the template name to an ID, then appends a new component to the
    definition. Fails if a component with the same name already exists.

    Args:
        stack_definition: Definition name or ID
        template: Template name or ID to use for this component
        name: Component name within the stack (must be unique within the definition)
        description: Optional component description
        shell: Optional shell override (bash/zsh/fish/sh)
        startup_script: Optional startup script ID
        depends_on: Optional list of other component names this component depends on

    Returns:
        True if successful

    Raises:
        Exception if the template cannot be resolved, the component name is a
        duplicate, or the API rejects the update
    """
    defn = get_def(stack_definition)
    spaces = defn.get("spaces", [])

    for s in spaces:
        if s.get("name") == name:
            raise Exception(
                "Component '{}' already exists in stack definition '{}'".format(
                    name, stack_definition)
            )

    template_obj = template_lib.get(template)
    template_id = template_obj.get("id") or template

    new_component = {
        "name": name,
        "template_id": template_id,
        "description": description,
        "shell": shell,
        "startup_script_id": startup_script,
        "depends_on": depends_on or [],
        "custom_fields": [],
        "port_forwards": [],
    }

    spaces.append(new_component)
    update_def(stack_definition, spaces=spaces)
    return True


def remove_component(stack_definition, name):
    """Remove a component from a stack definition by name.

    Also removes the component from any depends_on lists and port_forward
    targets of other components in the same definition.

    Args:
        stack_definition: Definition name or ID
        name: Component name to remove

    Returns:
        True if successful

    Raises:
        Exception if the component is not found or the API rejects the update
    """
    defn = get_def(stack_definition)
    spaces = defn.get("spaces", [])

    new_spaces = [s for s in spaces if s.get("name") != name]

    if len(new_spaces) == len(spaces):
        raise Exception(
            "Component '{}' not found in stack definition '{}'".format(
                name, stack_definition)
        )

    for s in new_spaces:
        s["depends_on"] = [d for d in s.get("depends_on", []) if d != name]
        s["port_forwards"] = [pf for pf in s.get("port_forwards", []) if pf.get("to_space") != name]

    update_def(stack_definition, spaces=new_spaces)
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

        space_id = space_lib.create(
            space_name,
            template_id,
            description=comp.get("description", ""),
            shell=comp.get("shell", ""),
            stack=stack_name,
            custom_fields=custom_fields,
            startup_script_id=comp.get("startup_script_id", comp.get("startup_script", "")),
        )
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
            current = space_lib.get(space_id)
            update_body = space_lib._build_space_update_body(current, depends_on=dep_ids)
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
            target_id = space_map.get(target_name, "")
            if not target_id:
                continue
            forwards.append({
                "local_port": pf.get("local_port"),
                "space": target_id,
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
    for space in space_lib.list():
        if space.get("stack") == stack_name:
            api.delete(f"/api/spaces/{space.get('id', '')}")

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
    stacks = {}
    order = []
    for space in space_lib.list():
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
