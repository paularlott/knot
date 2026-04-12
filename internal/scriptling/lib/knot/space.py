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
        "startup_script_id": space.get("startup_script_id", ""),
        "depends_on": space.get("depends_on", []),
        "stack": space.get("stack", ""),
    }
    body.update(overrides)
    return body

def list():
    """List all spaces for the current user.

    Returns:
        A list of space dicts, each containing:
        - id: Space ID
        - name: Space name
        - description: Space description
        - is_running: Boolean indicating if space is running

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/spaces")

    result = []
    for space in response.get("spaces", []):
        result.append({
            "id": space.get("space_id"),
            "name": space.get("name"),
            "description": space.get("description", ""),
            "is_running": space.get("is_deployed", False),
            "depends_on": space.get("depends_on", []),
            "stack": space.get("stack", ""),
        })

    return result


def get(name):
    """Get detailed information about a space.

    Args:
        name: Space name or ID

    Returns:
        A dict containing space details:
        - id: Space ID
        - name: Space name
        - description: Space description
        - template_id: Template ID
        - template_name: Template name
        - user_id: Owner user ID
        - username: Owner username
        - shares: List of shared user IDs
        - shell: Default shell
        - platform: Platform (e.g., "linux/amd64")
        - zone: Zone name
        - is_running: Boolean indicating if space is running
        - is_pending: Boolean indicating if space is pending
        - is_deleting: Boolean indicating if space is being deleted
        - node_hostname: Hostname of the node running the space
        - created_at: Creation timestamp

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/spaces/{name}")

    return {
        "id": response.get("space_id"),
        "name": response.get("name"),
        "description": response.get("description", ""),
        "template_id": response.get("template_id"),
        "template_name": response.get("template_name"),
        "user_id": response.get("user_id"),
        "username": response.get("username"),
        "shares": response.get("shares", []),
        "depends_on": response.get("depends_on", []),
        "shell": response.get("shell"),
        "platform": response.get("platform"),
        "zone": response.get("zone"),
        "is_running": response.get("is_deployed", False),
        "is_pending": response.get("is_pending", False),
        "is_deleting": response.get("is_deleting", False),
        "node_hostname": response.get("node_hostname", ""),
        "created_at": response.get("created_at", ""),
        "alt_names": response.get("alt_names", []),
        "icon_url": response.get("icon_url", ""),
        "custom_fields": response.get("custom_fields", []),
        "startup_script_id": response.get("startup_script_id", ""),
        "stack": response.get("stack", ""),
    }


def create(name, template_name, description="", shell="bash", depends_on=None, stack=""):
    """Create a new space.

    Args:
        name: Space name
        template_name: Name of the template to use
        description: Optional description
        shell: Shell to use (default: "bash")
        depends_on: Optional list of dependency space names or IDs
        stack: Optional stack name to group this space under

    Returns:
        The new space ID

    Raises:
        Exception if not configured or on API error
    """
    # First, get the template ID from the template name
    templates = api.get("/api/templates")
    template_id = None

    for tmpl in templates.get("templates", []):
        if tmpl.get("name") == template_name:
            template_id = tmpl.get("template_id")
            break

    if not template_id:
        raise Exception(f"Template not found: {template_name}")

    body = {
        "name": name,
        "template_id": template_id,
        "description": description,
        "shell": shell,
        "depends_on": _resolve_dependency_ids(depends_on),
        "stack": stack,
    }

    response = api.post("/api/spaces", body)
    return response.get("space_id")


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


def start_stack(stack_name):
    """Start all spaces in a stack.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured, on API error, or if not licensed for
        stack lifecycle operations
    """
    api.post(f"/api/spaces/stacks/{stack_name}/start")
    return True


def stop_stack(stack_name):
    """Stop all spaces in a stack.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured, on API error, or if not licensed for
        stack lifecycle operations
    """
    api.post(f"/api/spaces/stacks/{stack_name}/stop")
    return True


def restart_stack(stack_name):
    """Restart all spaces in a stack.

    Args:
        stack_name: Stack name

    Returns:
        True if successful

    Raises:
        Exception if not configured, on API error, or if not licensed for
        stack lifecycle operations
    """
    api.post(f"/api/spaces/stacks/{stack_name}/restart")
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
    if not isinstance(user_ids, list):
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
