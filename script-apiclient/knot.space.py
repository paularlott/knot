# knot.space - Space management library for Knot server
#
# This library provides functions for managing spaces in Knot.
# Requires knot.api to be configured first.
#
# Usage:
#   import knot.api
#   import knot.space
#
#   knot.api.configure("https://knot.example.com", "your-token")
#   spaces = knot.space.list()
#   knot.space.start("my-space")

import knot.api as api

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
            "is_running": space.get("is_deployed", False)
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
        - shared_user_id: Shared user ID (if shared)
        - shared_username: Shared username (if shared)
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
        "shared_user_id": response.get("shared_user_id"),
        "shared_username": response.get("shared_username"),
        "shell": response.get("shell"),
        "platform": response.get("platform"),
        "zone": response.get("zone"),
        "is_running": response.get("is_deployed", False),
        "is_pending": response.get("is_pending", False),
        "is_deleting": response.get("is_deleting", False),
        "node_hostname": response.get("node_hostname", ""),
        "created_at": response.get("created_at", "")
    }


def create(name, template_name, description="", shell="bash"):
    """Create a new space.

    Args:
        name: Space name
        template_name: Name of the template to use
        description: Optional description
        shell: Shell to use (default: "bash")

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
        "shell": shell
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

    body = {
        "name": space.get("name"),
        "description": description,
        "template_id": space.get("template_id"),
        "shell": space.get("shell")
    }

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


def share(name, user_id):
    """Share a space with another user.

    Args:
        name: Space name or ID
        user_id: User ID, username, or email to share with

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {"user_id": user_id}
    api.post(f"/api/spaces/{name}/share", body)
    return True


def unshare(name):
    """Remove a space share.

    Args:
        name: Space name or ID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/spaces/{name}/share")
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


def port_forward(source_space, local_port, remote_space, remote_port):
    """Forward a local port to a remote space port.

    Args:
        source_space: Source space name or ID
        local_port: Local port number
        remote_space: Remote space name or ID
        remote_port: Remote port number

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "local_port": local_port,
        "space": remote_space,
        "remote_port": remote_port
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
