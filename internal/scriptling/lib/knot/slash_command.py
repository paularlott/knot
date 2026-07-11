# knot.command - Slash command management library for Knot server
#
# This library provides functions for managing slash commands in Knot.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.command
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   commands = knot.command.list()

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def list(owner=None, all_zones=False):
    """List slash commands the user has access to.

    Args:
        owner: Filter by owner user ID (optional). Pass the current
            user's ID to list only their own commands.
        all_zones: Include commands from all zones (default: False)

    Returns:
        A list of command dicts, each containing:
        - id: Command ID
        - name: Command name
        - description: Command description
        - argument_hint: Argument hint string
        - allowed_tools: List of auto-allowed tool names
        - user_id: Owner user ID (empty for global commands)
        - groups: List of group names
        - zones: List of zone names
        - active: Boolean
        - is_managed: Boolean

    Raises:
        Exception if not configured or on API error
    """
    params = []
    if owner:
        params.append(f"user_id={owner}")
    if all_zones:
        params.append("all_zones=true")
    qs = "&".join(params)
    url = "/api/command" + (f"?{qs}" if qs else "")

    response = api.get(url)

    result = []
    for cmd in response.get("commands", []):
        result.append({
            "id": cmd.get("command_id"),
            "name": cmd.get("name"),
            "description": cmd.get("description", ""),
            "argument_hint": cmd.get("argument_hint", ""),
            "allowed_tools": cmd.get("allowed_tools", []),
            "user_id": cmd.get("user_id", ""),
            "groups": cmd.get("groups", []),
            "zones": cmd.get("zones", []),
            "active": cmd.get("active", True),
            "is_managed": cmd.get("is_managed", False),
        })

    return result


def get(name_or_id):
    """Get a slash command by name or UUID.

    Args:
        name_or_id: Command name or UUID

    Returns:
        A dict containing command details:
        - id: Command ID
        - name: Command name
        - description: Command description
        - argument_hint: Argument hint string
        - allowed_tools: List of auto-allowed tool names
        - body: Command body (markdown with optional $ARGUMENTS)
        - user_id: Owner user ID (empty for global)
        - groups: List of group names
        - zones: List of zone names
        - active: Boolean
        - is_managed: Boolean

    Raises:
        Exception if not configured or on API error
    """
    response = api.get(f"/api/command/{_enc(name_or_id)}")

    return {
        "id": response.get("command_id"),
        "name": response.get("name"),
        "description": response.get("description", ""),
        "argument_hint": response.get("argument_hint", ""),
        "allowed_tools": response.get("allowed_tools", []),
        "body": response.get("body", ""),
        "user_id": response.get("user_id", ""),
        "groups": response.get("groups", []),
        "zones": response.get("zones", []),
        "active": response.get("active", True),
        "is_managed": response.get("is_managed", False),
    }


def create(content, is_global=False, groups=None, zones=None, active=True):
    """Create a new slash command.

    The content must include YAML frontmatter with at least name and
    description fields, followed by the command body:

    ---
    name: "my-command"
    description: "Brief description"
    argument-hint: "<optional-hint>"
    allowed-tools: "tool1, tool2"
    ---

    Command body. Use $ARGUMENTS for the user's input.

    Args:
        content: Full command content including frontmatter
        is_global: If True, create as a global command (default: False,
            creates as the current user's command)
        groups: List of group names (global commands only)
        zones: List of zone restrictions
        active: Whether the command is active (default: True)

    Returns:
        The new command ID

    Raises:
        Exception if not configured or on API error
    """
    body = {
        "content": content,
        "groups": groups or [],
        "zones": zones or [],
        "active": active,
    }

    if is_global:
        body["user_id"] = ""
    else:
        body["user_id"] = "current"

    response = api.post("/api/command", body)
    return response.get("command_id")


def update(name_or_id, content=None, groups=None, zones=None, active=None):
    """Update a slash command.

    Args:
        name_or_id: Command name or UUID
        content: New command content including frontmatter (optional)
        groups: List of group names (optional)
        zones: List of zone restrictions (optional)
        active: Whether the command is active (optional)

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    current = api.get(f"/api/command/{_enc(name_or_id)}")

    body = {
        "content": content if content is not None else _reconstruct_content(current),
        "groups": groups if groups is not None else current.get("groups", []),
        "zones": zones if zones is not None else current.get("zones", []),
        "active": active if active is not None else current.get("active", True),
    }

    api.put(f"/api/command/{_enc(current.get('command_id'))}", body)
    return True


def delete(name_or_id):
    """Delete a slash command.

    Args:
        name_or_id: Command name or UUID

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/command/{_enc(name_or_id)}")
    return True


def _reconstruct_content(cmd):
    """Rebuild frontmatter + body from a command dict."""
    fm = [
        "---",
        f'name: "{cmd.get("name", "")}"',
        f'description: "{cmd.get("description", "")}"',
    ]
    if cmd.get("argument_hint"):
        fm.append(f'argument-hint: "{cmd["argument_hint"]}"')
    tools = cmd.get("allowed_tools", [])
    if tools:
        fm.append(f'allowed-tools: "{", ".join(tools)}"')
    fm.append("---")
    fm.append("")
    fm.append(cmd.get("body", ""))
    return "\n".join(fm)
