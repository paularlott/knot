# knot.space - Space management library for Knot server
#
# This library provides functions for managing spaces in Knot.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.space
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   spaces = knot.space.list()
#   knot.space.start("my-space")

import knot.apiclient as api

# Capture builtin list type before the module-level def list() below shadows it.
# Used by isinstance(x, _builtin_list) checks elsewhere in this file.
_builtin_list = list


def _parse_space(space):
    """Parse a space response into a stable dict."""
    return {
        "id": space.get("space_id", space.get("id")),
        "name": space.get("name"),
        "description": space.get("description", ""),
        "note": space.get("note", ""),
        "template_id": space.get("template_id", ""),
        "template_name": space.get("template_name", ""),
        "user_id": space.get("user_id", ""),
        "username": space.get("username", ""),
        "shares": space.get("shares", []),
        "depends_on": space.get("depends_on", []),
        "shell": space.get("shell", ""),
        "platform": space.get("platform", ""),
        "zone": space.get("zone", ""),
        "is_running": space.get("is_deployed", space.get("is_running", False)),
        "is_pending": space.get("is_pending", False),
        "is_deleting": space.get("is_deleting", False),
        "has_code_server": space.get("has_code_server", False),
        "has_ssh": space.get("has_ssh", False),
        "has_terminal": space.get("has_terminal", False),
        "has_http_vnc": space.get("has_http_vnc", False),
        "has_vscode_tunnel": space.get("has_vscode_tunnel", False),
        "vscode_tunnel_name": space.get("vscode_tunnel_name", ""),
        "tcp_ports": space.get("tcp_ports", {}),
        "http_ports": space.get("http_ports", {}),
        "node_id": space.get("node_id", ""),
        "node_hostname": space.get("node_hostname", ""),
        "created_at": space.get("created_at", ""),
        "started_at": space.get("started_at", ""),
        "alt_names": space.get("alt_names", []),
        "icon_url": space.get("icon_url", ""),
        "custom_fields": space.get("custom_fields", []),
        "startup_script_id": space.get("startup_script_id", ""),
        "stack": space.get("stack", ""),
        "healthy": space.get("healthy", True),
        "resource_usage": space.get("resource_usage"),
    }


def _resolve_template_id(template_name):
    """Resolve a template name or ID to a template ID."""
    import knot.template

    template = knot.template.get(template_name)
    template_id = template.get("id")
    if not template_id:
        raise Exception(f"Template not found: {template_name}")
    return template_id


def _current_user_id():
    """Return the current API user's ID."""
    user = api.get("/api/users/whoami")
    user_id = user.get("user_id", user.get("id", ""))
    if not user_id:
        raise Exception("Current user ID not available")
    return user_id


def _resolve_dependency_ids(depends_on):
    """Resolve dependency names or IDs to space IDs."""
    resolved = []
    for dependency in depends_on or []:
        space = get(dependency)
        dependency_id = space.get("id")
        if not dependency_id:
            raise Exception(f"Space not found: {dependency}")
        resolved.append(dependency_id)
    return resolved


def _build_space_update_body(space, **overrides):
    """Build a full update body preserving existing mutable fields."""
    body = {
        "name": space.get("name"),
        "description": space.get("description", ""),
        "template_id": space.get("template_id"),
        "shell": space.get("shell"),
        "alt_names": space.get("alt_names", []),
        "icon_url": space.get("icon_url", ""),
        "custom_fields": space.get("custom_fields", []),
        "selected_node_id": space.get("node_id", ""),
        "startup_script_id": space.get("startup_script_id", ""),
        "depends_on": space.get("depends_on", []),
        "stack": space.get("stack", ""),
    }
    body.update(overrides)
    return body


def list(all_zones=False):
    """List spaces visible to the current user.

    Args:
        all_zones: If True, include spaces from all zones. Default False
            (only spaces in the current server's zone are returned).
    """
    params = {"user_id": _current_user_id()}
    if all_zones:
        params["all_zones"] = "true"

    response = api.get("/api/spaces", params)

    result = []
    for space in response.get("spaces", []):
        result.append(_parse_space(space))
    return result


def get(name):
    """Get detailed information for a space by name or ID."""
    response = api.get(f"/api/spaces/{name}")
    return _parse_space(response)


def create(name, template_name, description="", shell="bash", depends_on=None,
           stack="", selected_node_id="", alt_names=None, icon_url="",
           custom_fields=None, startup_script_id="", start_on_create=False):
    """Create a new space and return its ID."""
    body = {
        "name": name,
        "description": description,
        "template_id": _resolve_template_id(template_name),
        "shell": shell,
        "alt_names": alt_names or [],
        "icon_url": icon_url,
        "custom_fields": custom_fields or [],
        "selected_node_id": selected_node_id,
        "startup_script_id": startup_script_id,
        "depends_on": _resolve_dependency_ids(depends_on),
        "stack": stack,
    }

    response = api.post("/api/spaces", body)
    space_id = response.get("space_id")
    if start_on_create:
        start(space_id)
    return space_id


def update(name, new_name=None, description=None, shell=None, template_name=None,
           depends_on=None, stack=None, selected_node_id=None, alt_names=None,
           icon_url=None, custom_fields=None, startup_script_id=None):
    """Update space properties while preserving fields not explicitly changed."""
    space = get(name)
    overrides = {}

    if new_name is not None:
        overrides["name"] = new_name
    if description is not None:
        overrides["description"] = description
    if shell is not None:
        overrides["shell"] = shell
    if template_name is not None:
        overrides["template_id"] = _resolve_template_id(template_name)
    if depends_on is not None:
        overrides["depends_on"] = _resolve_dependency_ids(depends_on)
    if stack is not None:
        overrides["stack"] = stack
    if selected_node_id is not None:
        overrides["selected_node_id"] = selected_node_id
    if alt_names is not None:
        overrides["alt_names"] = alt_names
    if icon_url is not None:
        overrides["icon_url"] = icon_url
    if custom_fields is not None:
        overrides["custom_fields"] = custom_fields
    if startup_script_id is not None:
        overrides["startup_script_id"] = startup_script_id

    body = _build_space_update_body(space, **overrides)
    api.put(f"/api/spaces/{space.get('id')}", body)
    return True


def delete(name):
    """Delete a space.

    Args:
        name: Space name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/spaces/{name}")
    return True


def start(name):
    """Start a space.

    Args:
        name: Space name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/{name}/start")
    return True


def stop(name):
    """Stop a space.

    Args:
        name: Space name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/{name}/stop")
    return True


def restart(name):
    """Restart a space.

    Args:
        name: Space name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.post(f"/api/spaces/{name}/restart")
    return True


def is_running(name):
    """Check if a space is running.

    Args:
        name: Space name or ID

    Returns:
        True if the space is running, False otherwise

    Raises:
        Exception if not configured or on API error
    """
    space = get(name)
    return space.get("is_running", False)


def usage_current(name):
    """Get the current resource usage point for a space."""
    return api.get(f"/api/spaces/{name}/usage/current")


def usage_history(name, range="1h"):
    """Get historical resource usage for a space.

    Args:
        name: Space name or ID
        range: "1h" for minute samples or "7d" for daily samples
    """
    return api.get(f"/api/spaces/{name}/usage/history", {"range": range})


def set_description(name, description):
    """Set the description of a space.

    Args:
        name: Space name or ID
        description: New description

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    # Get current space data
    space = get(name)

    body = _build_space_update_body(space, description=description)

    api.put(f"/api/spaces/{name}", body)
    return True


def get_description(name):
    """Get the description of a space.

    Args:
        name: Space name or ID

    Returns:
        The space description string

    Raises:
        Exception if not configured or on API error
    """
    space = get(name)
    return space.get("description", "")


def get_dependencies(name):
    """Get the dependency space IDs for a space."""
    space = get(name)
    return space.get("depends_on", [])


def set_dependencies(name, depends_on):
    """Set the dependency spaces for a space.

    Args:
        name: Space name or ID
        depends_on: List of dependency space names or IDs
    """
    space = get(name)
    body = _build_space_update_body(
        space,
        depends_on=_resolve_dependency_ids(depends_on),
    )
    api.put(f"/api/spaces/{name}", body)
    return True


def get_stack(name):
    """Get the stack name for a space.

    Args:
        name: Space name or ID

    Returns:
        The stack name string (empty string if unstacked)
    """
    space = get(name)
    return space.get("stack", "")


def set_stack(name, stack):
    """Set the stack name for a space.

    Args:
        name: Space name or ID
        stack: Stack name (empty string to unstack)

    Returns:
        True if successful
    """
    space = get(name)
    body = _build_space_update_body(space, stack=stack)
    api.put(f"/api/spaces/{name}", body)
    return True


def get_field(name, field):
    """Get a custom field value from a space.

    Args:
        name: Space name or ID
        field: Custom field name

    Returns:
        The custom field value string

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/spaces/{name}/custom-field/{field}")
    return response.get("value", "")


def set_field(name, field, value):
    """Set a custom field value on a space.

    Args:
        name: Space name or ID
        field: Custom field name
        value: Custom field value

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "name": field,
        "value": value
    }
    api.put(f"/api/spaces/{name}/custom-field", body)
    return True


def transfer(name, user_id):
    """Transfer a space to another user.

    Args:
        name: Space name or ID
        user_id: User ID, username, or email of the new owner

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {"user_id": user_id}
    api.post(f"/api/spaces/{name}/transfer", body)
    return True


def share(name, user_ids):
    """Share a space with one or more users.

    Args:
        name: Space name or ID
        user_ids: User ID, username, email, or a list of those values

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    if not isinstance(user_ids, _builtin_list):
        user_ids = [user_ids]
    api.post(f"/api/spaces/{name}/share", {"shares": user_ids})
    return True


def unshare(name, user_id=None):
    """Remove a space share.

    Args:
        name: Space name or ID
        user_id: Optional user ID, username, or email to remove sharing for.
                 If omitted, owners stop all sharing and recipients leave.

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    path = f"/api/spaces/{name}/share"
    if user_id:
        path += f"?user_id={user_id}"
    api.delete(path)
    return True


def run(name, command, args=None, timeout=30, workdir=""):
    """Execute a command in a space.

    Args:
        name: Space name or ID
        command: Command to execute
        args: List of command arguments (optional)
        timeout: Timeout in seconds (default: 30)
        workdir: Working directory (optional)

    Returns:
        The command output string

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "command": command,
        "args": args or [],
        "timeout": timeout,
        "workdir": workdir
    }

    response = api.post(f"/api/spaces/{name}/run-command", body)
    return response.get("output", "")


def run_script(name, script_name, args=None):
    """Execute a script in a space.

    Args:
        name: Space name or ID
        script_name: Name of the script to execute
        args: List of script arguments (optional)

    Returns:
        A dict containing:
        - output: Script output
        - exit_code: Script exit code

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "script_name": script_name,
        "arguments": args or []
    }

    response = api.post(f"/api/spaces/{name}/execute-script", body)
    return {
        "output": response.get("output", ""),
        "exit_code": response.get("exit_code", 0)
    }


def read_file(name, file_path):
    """Read file contents from a running space.

    Args:
        name: Space name or ID
        file_path: Path to the file

    Returns:
        The file contents as a string

    Raises:
        Exception if not configured or on API error
    """
    body = {"path": file_path}
    response = api.post(f"/api/spaces/{name}/files/read", body)
    return response.get("content", "")


def write_file(name, file_path, content):
    """Write content to a file in a running space.

    Args:
        name: Space name or ID
        file_path: Path to the file
        content: Content to write

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "path": file_path,
        "content": content
    }
    api.post(f"/api/spaces/{name}/files/write", body)
    return True


def port_forward(source_space, local_port, remote_space, remote_port, persistent=False, force=False):
    """Forward a local port to a remote space port.

    Args:
        source_space: Source space name or ID
        local_port: Local port number
        remote_space: Remote space name or ID
        remote_port: Remote port number
        persistent: Persist the forward across agent restarts (default False)
        force: Create the forward even if the target space is not running (default False)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "local_port": local_port,
        "space": remote_space,
        "remote_port": remote_port,
        "persistent": persistent,
        "force": force,
    }
    api.post(f"/space-io/{source_space}/port/forward", body)
    return True


def port_list(name):
    """List active port forwards for a space.

    Args:
        name: Space name or ID

    Returns:
        A list of dicts, each containing:
        - local_port: Local port number
        - space: Remote space name
        - remote_port: Remote port number

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/space-io/{name}/port/list")

    result = []
    for fwd in response.get("forwards", []):
        result.append({
            "local_port": fwd.get("local_port"),
            "space": fwd.get("space"),
            "remote_port": fwd.get("remote_port")
        })

    return result


def port_stop(name, local_port):
    """Stop a port forward.

    Args:
        name: Space name or ID
        local_port: Local port number

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {"local_port": local_port}
    api.post(f"/space-io/{name}/port/stop", body)
    return True


def port_apply(source_space, forwards):
    """Replace all port forwards with the given list.

    Stops any existing forwards not in the list and starts any new ones.
    Forwards that already exist with the same local_port, space, and
    remote_port are left unchanged.

    Args:
        source_space: Source space name or ID
        forwards: List of dicts, each containing:
            - local_port: Local port number
            - space: Remote space name or ID
            - remote_port: Remote port number
            Optional keys:
            - persistent: bool (default False)
            - force: bool (default False)

    Returns:
        A dict containing:
        - applied: List of forwards that were started
        - stopped: List of forwards that were stopped
        - errors: List of error messages (if any)

    Raises:
        Exception if not configured or on API error
    """
    body = {"forwards": forwards}
    return api.post(f"/space-io/{source_space}/port/apply", body)
