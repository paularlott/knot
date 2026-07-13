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
import urllib.parse

# Capture builtin list type before the module-level def list() below shadows it.
# Used by isinstance(x, _builtin_list) checks elsewhere in this file.
_builtin_list = list


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


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
    response = api.get(f"/api/spaces/{_enc(name)}")
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
    api.put(f"/api/spaces/{_enc(space.get('id'))}", body)
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
    api.delete(f"/api/spaces/{_enc(name)}")
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
    api.post(f"/api/spaces/{_enc(name)}/start")
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
    api.post(f"/api/spaces/{_enc(name)}/stop")
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
    api.post(f"/api/spaces/{_enc(name)}/restart")
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
    return api.get(f"/api/spaces/{_enc(name)}/usage/current")


def usage_history(name, range="1h"):
    """Get historical resource usage for a space.

    Args:
        name: Space name or ID
        range: "1h" for minute samples or "7d" for daily samples
    """
    return api.get(f"/api/spaces/{_enc(name)}/usage/history", {"range": range})


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

    api.put(f"/api/spaces/{_enc(name)}", body)
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
    api.put(f"/api/spaces/{_enc(name)}", body)
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
    api.put(f"/api/spaces/{_enc(name)}", body)
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
    response = api.get(f"/api/spaces/{_enc(name)}/custom-field/{_enc(field)}")
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
    api.put(f"/api/spaces/{_enc(name)}/custom-field", body)
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
    api.post(f"/api/spaces/{_enc(name)}/transfer", body)
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
    api.post(f"/api/spaces/{_enc(name)}/share", {"shares": user_ids})
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
    path = f"/api/spaces/{_enc(name)}/share"
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

    response = api.post(f"/api/spaces/{_enc(name)}/run-command", body)
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

    response = api.post(f"/api/spaces/{_enc(name)}/execute-script", body)
    return {
        "output": response.get("output", ""),
        "exit_code": response.get("exit_code", 0)
    }


def eval(name, code, args=None):
    """Execute inline Scriptling code in a running space.

    Unlike run_script, which looks up a stored script by name, eval sends the
    code directly so no script needs to exist in the database. The code runs in
    the target space's agent with the same permissions, libraries, and argument
    conventions as a named script (argv[0] is "inline").

    Args:
        name: Space name or ID
        code: Scriptling source to evaluate
        args: List of script arguments (optional)

    Returns:
        A dict containing:
        - output: Script output
        - error: Error message (empty string on success)
        - exit_code: Script exit code

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "content": code,
        "arguments": args or []
    }

    response = api.post(f"/api/spaces/{_enc(name)}/execute-script", body)
    return {
        "output": response.get("output", ""),
        "error": response.get("error", ""),
        "exit_code": response.get("exit_code", 0)
    }


def read_file(name, file_path, offset=0, limit=0):
    """Read file contents from a running space, optionally a 1-based line range.

    Args:
        name: Space name or ID
        file_path: Path to the file
        offset: 1-based line number to start at (0 = from the beginning)
        limit: Maximum number of lines to return (0 = no limit / whole file)

    Returns:
        The file contents as a string. When offset/limit are given, only the
        requested line range is returned.

    Raises:
        Exception if not configured or on API error
    """
    body = {"path": file_path}
    if offset > 0:
        body["offset"] = offset
    if limit > 0:
        body["limit"] = limit
    response = api.post(f"/api/spaces/{_enc(name)}/files/read", body)
    return response.get("content", "")


def write_file(name, file_path, content, mode="overwrite"):
    """Write content to a file in a running space.

    Args:
        name: Space name or ID
        file_path: Path to the file
        content: Content to write
        mode: Write mode — "overwrite" (default), "append", or "prepend"

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "path": file_path,
        "content": content
    }
    if mode and mode != "overwrite":
        body["mode"] = mode
    api.post(f"/api/spaces/{_enc(name)}/files/write", body)
    return True


def grep(name, pattern, path, literal=False, recursive=False, ignore_case=False,
         glob="", follow_links=False, max_size=0, workdir=""):
    """Search file contents in a running space using a regex or literal pattern.

    Runs in the space's agent via the scriptling extlibs worker pool — no file
    contents leave the space, only matching lines are returned.

    Args:
        name: Space name or ID
        pattern: Regular expression (or literal string when literal=True)
        path: File or directory to search (relative to workdir if given)
        literal: Treat pattern as a literal string, not a regex (default False)
        recursive: Recurse into subdirectories when path is a directory
        ignore_case: Case-insensitive matching (default False)
        glob: Only search files matching this glob pattern, e.g. "*.py"
        follow_links: Follow symlinks if they resolve within the space
        max_size: Skip files larger than this many bytes; 0 = default 1 MiB,
                  negative = unlimited
        workdir: Resolve relative path against this directory

    Returns:
        A list of match dicts: {"file": str, "line": int, "text": str}
    """
    body = {
        "pattern": pattern,
        "path": path,
        "literal": literal,
        "recursive": recursive,
        "ignore_case": ignore_case,
        "follow_links": follow_links,
        "max_size": max_size,
    }
    if glob:
        body["glob"] = glob
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/grep", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "grep failed"))
    return response.get("matches", [])


def find(name, path=".", recursive=True, type="any", name_glob="", mtime_min=None,
         mtime_max=None, size_min=None, size_max=None, include_hidden=False,
         follow_links=False, max_depth=0, workdir=""):
    """Find files and directories in a running space by name, type, mtime, or size.

    Runs in the space's agent via the scriptling extlibs concurrent walker.

    Args:
        name: Space name or ID
        path: Directory (or file) to search under (relative to workdir if given)
        recursive: Descend into subdirectories (default True)
        type: Restrict to "file", "dir", or "any" (default "any")
        name_glob: Shell-style glob matched against the entry's base name
        mtime_min: Include entries modified at/after this epoch time (float seconds)
        mtime_max: Include entries modified at/before this epoch time
        size_min: Include entries whose size in bytes is >= this value
        size_max: Include entries whose size in bytes is <= this value
        include_hidden: Match entries whose name starts with "." (default False)
        follow_links: Follow symlinks if they resolve within the space
        max_depth: Maximum recursion depth; 0 = unlimited
        workdir: Resolve relative path against this directory

    Returns:
        A list of matching path strings (arbitrary order).
    """
    body = {
        "path": path,
        "recursive": recursive,
        "type": type,
        "include_hidden": include_hidden,
        "follow_links": follow_links,
        "max_depth": max_depth,
    }
    if name_glob:
        body["name"] = name_glob
    if mtime_min is not None:
        body["mtime_min"] = mtime_min
    if mtime_max is not None:
        body["mtime_max"] = mtime_max
    if size_min is not None:
        body["size_min"] = size_min
    if size_max is not None:
        body["size_max"] = size_max
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/find", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "find failed"))
    return response.get("paths", [])


def sed_replace(name, old, new, path, recursive=False, ignore_case=False,
                glob="", follow_links=False, max_size=0, workdir=""):
    """Replace every literal occurrence of old with new in a file (or files under
    a directory) in a running space. Files are modified in place using an atomic
    temp-file + rename. old is matched literally, not as a regular expression.

    Args:
        name: Space name or ID
        old: Literal string to search for
        new: Replacement string
        path: File or directory to modify (relative to workdir if given)
        recursive: Recurse into subdirectories when path is a directory
        ignore_case: Case-insensitive matching (default False)
        glob: Only modify files matching this glob pattern
        follow_links: Follow symlinks if they resolve within the space
        max_size: Skip files larger than this many bytes; 0 = default 1 MiB
        workdir: Resolve relative path against this directory

    Returns:
        The number of files modified.
    """
    body = {
        "mode": "replace",
        "pattern": old,
        "replacement": new,
        "path": path,
        "recursive": recursive,
        "ignore_case": ignore_case,
        "follow_links": follow_links,
        "max_size": max_size,
    }
    if glob:
        body["glob"] = glob
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/sed", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "sed replace failed"))
    return response.get("files_modified", 0)


def sed_replace_pattern(name, pattern, new, path, recursive=False, ignore_case=False,
                        glob="", follow_links=False, max_size=0, workdir=""):
    """Replace every regex match of pattern with new in a file (or files under a
    directory) in a running space. Capture groups may be referenced in new as
    ${1}, ${2}, or ${name}. Files are modified in place using an atomic
    temp-file + rename.

    Args:
        name: Space name or ID
        pattern: Regular expression pattern
        new: Replacement string (may reference capture groups as ${1}, ${name})
        path: File or directory to modify (relative to workdir if given)
        recursive: Recurse into subdirectories when path is a directory
        ignore_case: Case-insensitive matching (default False)
        glob: Only modify files matching this glob pattern
        follow_links: Follow symlinks if they resolve within the space
        max_size: Skip files larger than this many bytes; 0 = default 1 MiB
        workdir: Resolve relative path against this directory

    Returns:
        The number of files modified.
    """
    body = {
        "mode": "replace_pattern",
        "pattern": pattern,
        "replacement": new,
        "path": path,
        "recursive": recursive,
        "ignore_case": ignore_case,
        "follow_links": follow_links,
        "max_size": max_size,
    }
    if glob:
        body["glob"] = glob
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/sed", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "sed replace_pattern failed"))
    return response.get("files_modified", 0)


def sed_extract(name, pattern, path, recursive=False, ignore_case=False,
                glob="", follow_links=False, max_size=0, workdir=""):
    """Extract regex capture groups from a file (or files under a directory) in a
    running space.

    Args:
        name: Space name or ID
        pattern: Regular expression with capture groups
        path: File or directory to search (relative to workdir if given)
        recursive: Recurse into subdirectories when path is a directory
        ignore_case: Case-insensitive matching (default False)
        glob: Only search files matching this glob pattern
        follow_links: Follow symlinks if they resolve within the space
        max_size: Skip files larger than this many bytes; 0 = default 1 MiB
        workdir: Resolve relative path against this directory

    Returns:
        A list of match dicts: {"file": str, "line": int, "text": str,
        "groups": [str, ...]}
    """
    body = {
        "mode": "extract",
        "pattern": pattern,
        "path": path,
        "recursive": recursive,
        "ignore_case": ignore_case,
        "follow_links": follow_links,
        "max_size": max_size,
    }
    if glob:
        body["glob"] = glob
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/sed", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "sed extract failed"))
    return response.get("matches", [])


def edit_file(name, file_path, search, replace, workdir=""):
    """Perform a targeted search-and-replace edit on a single file in a running
    space. The search text must appear exactly once in the file; the operation
    fails if it matches zero or multiple times. The modification is written
    atomically (temp file + rename).

    Unlike sed_replace (which replaces ALL occurrences), edit_file targets ONE
    specific occurrence with uniqueness verification — the gold standard for
    coding-agent edits where "replace all" is dangerous.

    Args:
        name: Space name or ID
        file_path: Path to the file to edit
        search: Exact text to find (may span multiple lines; provide enough
                surrounding context to make the match unique)
        replace: Replacement text
        workdir: Resolve relative path against this directory

    Returns:
        The number of bytes written.

    Raises:
        Exception if the search text is not found, matches multiple times,
        or the API/agent fails.
    """
    body = {
        "path": file_path,
        "search": search,
        "replace": replace,
    }
    if workdir:
        body["workdir"] = workdir
    response = api.post(f"/api/spaces/{_enc(name)}/files/edit", body)
    if not response.get("success", False):
        raise Exception(response.get("error", "edit failed"))
    return response.get("bytes_written", 0)


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
    api.post(f"/space-io/{_enc(source_space)}/port/forward", body)
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
    response = api.get(f"/space-io/{_enc(name)}/port/list")

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
    api.post(f"/space-io/{_enc(name)}/port/stop", body)
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
    return api.post(f"/space-io/{_enc(source_space)}/port/apply", body)


def _space_id(space):
    """Resolve a space name or ID to its UUID.

    The /space-io/{space_id}/* endpoints require a UUID in the path; /api/spaces
    accepts either, so resolve via get() when needed.
    """
    resolved = get(space)
    return resolved.get("id", space) if resolved else space


def tunnel_start(space, protocol, port, name):
    """Start an agent-owned web tunnel in a space.

    The tunnel exposes a port inside the space on the internet as
    <user>--<name>.<domain>. It is owned by the space's agent and runs until the
    agent exits or the tunnel is stopped; it is not persisted.

    Args:
        space: Space name or ID
        protocol: "http" or "https"
        port: The port within the space to tunnel
        name: The tunnel name (forms <user>--<name>.<domain>)

    Returns:
        The public tunnel URL string

    Raises:
        Exception if not configured or on API error
    """
    space_id = _space_id(space)
    response = api.post(f"/space-io/{_enc(space_id)}/tunnel/start", {
        "protocol": protocol,
        "port": port,
        "name": name,
    })
    if response and not response.get("success", True):
        raise Exception(response.get("error", "failed to start tunnel"))
    return response.get("url", "") if response else ""


def tunnel_list(space):
    """List agent-owned web tunnels in a space.

    Args:
        space: Space name or ID

    Returns:
        A list of dicts, each containing:
        - port: Port number within the space
        - protocol: "http" or "https"
        - name: Tunnel name
        - url: Public tunnel URL

    Raises:
        Exception if not configured or on API error
    """
    space_id = _space_id(space)
    response = api.get(f"/space-io/{_enc(space_id)}/tunnel/list")
    return response.get("tunnels", []) if response else []


def tunnel_stop(space, name):
    """Stop an agent-owned web tunnel in a space by name.

    Args:
        space: Space name or ID
        name: The tunnel name

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    space_id = _space_id(space)
    api.post(f"/space-io/{_enc(space_id)}/tunnel/stop", {"name": name})
    return True
