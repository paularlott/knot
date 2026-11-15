# knot.mcp_server - Remote MCP server management library for Knot server
#
# Remote MCP servers are other MCP servers whose tools knot exposes to the
# user who added them, under the server's namespace. A server is reached over
# HTTP(S) (url) or started as a stdio command. Calling the tools themselves is
# knot.mcp's job; this library manages the server records. Requires
# knot.apiclient to be configured first.
#
# Usage:
#   import knot.mcp_server
#
#   knot.mcp_server.create("github", url="https://api.githubcopilot.com/mcp",
#       auth_type="bearer", token="ghp_...")
#   for tool in knot.mcp_server.tools("github"):
#       print(tool["name"], tool["enabled"])

import knot.apiclient as api
import re
import urllib.parse

_UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def _parse_server(server):
    """Parse an MCP server response into a stable dict, without secrets."""
    return {
        "id": server.get("mcp_server_id", server.get("id")),
        "user_id": server.get("user_id", ""),
        "namespace": server.get("namespace", ""),
        "url": server.get("url", ""),
        "command": server.get("command", ""),
        "args": server.get("args") or [],
        "env": server.get("env") or [],
        "enabled": server.get("enabled", False),
        "tool_visibility": server.get("tool_visibility", "") or "native",
        "disabled_tools": server.get("disabled_tools") or [],
        "remote_search": server.get("remote_search", False),
    }


def _parse_details(server):
    """Parse MCP server details: the listing's fields plus the auth settings,
    with the token values replaced by flags saying whether they are set."""
    out = _parse_server(server)
    out["auth_type"] = server.get("auth_type", "")
    out["has_token"] = bool(server.get("token", ""))
    out["oauth_client_id"] = server.get("oauth_client_id", "")
    out["oauth_token_url"] = server.get("oauth_token_url", "")
    out["has_oauth_tokens"] = bool(server.get("oauth_access_token", "") or server.get("oauth_refresh_token", ""))
    return out


def _resolve_id(namespace_or_id):
    """Return the ID of the current user's server with this namespace, or the ID given."""
    if _UUID_RE.match(str(namespace_or_id)):
        return str(namespace_or_id)
    for server in list():
        if server["namespace"] == namespace_or_id:
            return server["id"]
    raise Exception(f"MCP server not found: {namespace_or_id}")


def list(user_id=None):
    """List remote MCP servers.

    Args:
        user_id: Optional user ID whose servers to list; needs the manage MCP
            servers permission. Default: the current user's servers.

    Returns:
        A list of dicts sorted by namespace, each containing:
        - id: MCP server ID
        - user_id: Owner user ID
        - namespace: Prefix the server's tools are exposed under
        - url: HTTP(S) endpoint, "" for a stdio server
        - command: stdio command, "" for an HTTP server
        - args: List of command arguments (stdio)
        - env: List of "KEY=value" environment entries (stdio)
        - enabled: Whether knot connects to the server
        - tool_visibility: "native" (tools listed in tools/list) or
          "discoverable" (tools found only through tool_search)
        - disabled_tools: Tool names switched off
        - remote_search: Whether tool_search queries are forwarded to the server

    Raises:
        Exception if not configured or on API error
    """
    params = {"user_id": user_id} if user_id else None
    response = api.get("/api/mcp-servers", params)
    return [_parse_server(s) for s in response.get("servers", [])]


def get(namespace_or_id):
    """Get a remote MCP server by namespace (your own servers) or ID.

    Token values are never returned; flags say whether they are set.

    Args:
        namespace_or_id: The server's namespace or UUID

    Returns:
        A dict with the fields list() returns plus:
        - auth_type: "" (none), "bearer" or "oauth2"
        - has_token: Whether a bearer token is set
        - oauth_client_id: OAuth2 client ID
        - oauth_token_url: OAuth2 token endpoint
        - has_oauth_tokens: Whether OAuth2 access/refresh tokens are held

    Raises:
        Exception if the server is not found, not visible to you, or on API error
    """
    return _parse_details(api.get(f"/api/mcp-servers/{_enc(_resolve_id(namespace_or_id))}"))


def create(namespace, url="", command="", args=None, env=None, auth_type="", token="",
           oauth_client_id="", oauth_token_url="", oauth_access_token="",
           oauth_refresh_token="", enabled=True, tool_visibility="native",
           remote_search=False):
    """Add a remote MCP server for the current user and return its ID.

    Give url for an HTTP(S) server or command (with args and env) for a stdio
    server. Needs the manage MCP servers permission.

    Args:
        namespace: Prefix for the server's tools; unique among your servers
            and must not contain "__"
        url: HTTP(S) endpoint of the MCP server
        command: Command to start a stdio MCP server
        args: List of command arguments
        env: List of "KEY=value" environment entries for the command
        auth_type: "" (none, the default), "bearer" or "oauth2"
        token: Bearer token (auth_type "bearer")
        oauth_client_id: OAuth2 client ID (auth_type "oauth2")
        oauth_token_url: OAuth2 token endpoint (auth_type "oauth2")
        oauth_access_token: OAuth2 access token (auth_type "oauth2")
        oauth_refresh_token: OAuth2 refresh token (auth_type "oauth2")
        enabled: Whether knot connects to the server (default True)
        tool_visibility: "native" (default) or "discoverable"
        remote_search: Forward tool_search queries to the server

    Returns:
        The new server's ID

    Raises:
        Exception if neither url nor command is given, the namespace is taken
        or invalid, you lack the permission, or on API error
    """
    response = api.post("/api/mcp-servers", {
        "namespace": namespace,
        "url": url,
        "command": command,
        "args": args or [],
        "env": env or [],
        "auth_type": auth_type,
        "token": token,
        "oauth_client_id": oauth_client_id,
        "oauth_token_url": oauth_token_url,
        "oauth_access_token": oauth_access_token,
        "oauth_refresh_token": oauth_refresh_token,
        "enabled": enabled,
        "tool_visibility": tool_visibility,
        "remote_search": remote_search,
    })
    return response.get("mcp_server_id")


def update(namespace_or_id, namespace=None, url=None, command=None, args=None, env=None,
           auth_type=None, token=None, oauth_client_id=None, oauth_token_url=None,
           oauth_access_token=None, oauth_refresh_token=None, enabled=None,
           tool_visibility=None, remote_search=None):
    """Update a remote MCP server; only the arguments given change.

    Args are as for create(); namespace renames the server. Stored tokens
    are kept unless new values are given.

    Args:
        namespace_or_id: The server's current namespace or UUID

    Returns:
        True if successful

    Raises:
        Exception if the server is not found, the new namespace is taken, you
        lack the permission, or on API error
    """
    server_id = _resolve_id(namespace_or_id)
    current = api.get(f"/api/mcp-servers/{_enc(server_id)}")
    changes = {
        "namespace": namespace, "url": url, "command": command, "args": args,
        "env": env, "auth_type": auth_type, "token": token,
        "oauth_client_id": oauth_client_id, "oauth_token_url": oauth_token_url,
        "oauth_access_token": oauth_access_token,
        "oauth_refresh_token": oauth_refresh_token, "enabled": enabled,
        "tool_visibility": tool_visibility, "remote_search": remote_search,
    }
    body = {
        "namespace": current.get("namespace", ""),
        "url": current.get("url", ""),
        "command": current.get("command", ""),
        "args": current.get("args") or [],
        "env": current.get("env") or [],
        "auth_type": current.get("auth_type", ""),
        "token": current.get("token", ""),
        "oauth_client_id": current.get("oauth_client_id", ""),
        "oauth_token_url": current.get("oauth_token_url", ""),
        "oauth_access_token": current.get("oauth_access_token", ""),
        "oauth_refresh_token": current.get("oauth_refresh_token", ""),
        "enabled": current.get("enabled", False),
        "tool_visibility": current.get("tool_visibility", "") or "native",
        "disabled_tools": current.get("disabled_tools") or [],
        "remote_search": current.get("remote_search", False),
    }
    for key in changes:
        if changes[key] is not None:
            body[key] = changes[key]
    api.put(f"/api/mcp-servers/{_enc(server_id)}", body)
    return True


def delete(namespace_or_id):
    """Delete a remote MCP server.

    Args:
        namespace_or_id: The server's namespace or UUID

    Returns:
        True if successful

    Raises:
        Exception if the server is not found, you lack the permission, or on API error
    """
    api.delete(f"/api/mcp-servers/{_enc(_resolve_id(namespace_or_id))}")
    return True


def tools(namespace_or_id):
    """List the tools a remote MCP server offers, connecting to it now.

    Args:
        namespace_or_id: The server's namespace or UUID

    Returns:
        A list of dicts, each containing:
        - name: Tool name, without the namespace prefix
        - description: Tool description
        - enabled: False when switched off with set_tool_enabled()
        - is_app: True for an MCP Apps tool (it has a linked ui:// resource)

    Raises:
        Exception if the server cannot be reached (the message says why), is
        not found, or on API error
    """
    response = api.get(f"/api/mcp-servers/{_enc(_resolve_id(namespace_or_id))}/tools", timeout=60)
    if response.get("error"):
        raise Exception("MCP server tools unavailable: " + response.get("error"))
    return [{
        "name": t.get("name", ""),
        "description": t.get("description", ""),
        "enabled": t.get("enabled", True),
        "is_app": t.get("is_app", False),
    } for t in response.get("tools", [])]


def set_tool_enabled(namespace_or_id, tool_name, enabled):
    """Switch one of a remote MCP server's tools on or off.

    Args:
        namespace_or_id: The server's namespace or UUID
        tool_name: Tool name without the namespace prefix
        enabled: True to offer the tool, False to hide it

    Returns:
        True if successful

    Raises:
        Exception if the server is not found, you lack the permission, or on API error
    """
    api.post(f"/api/mcp-servers/{_enc(_resolve_id(namespace_or_id))}/toggle-tool", {
        "tool_name": tool_name,
        "enabled": bool(enabled),
    })
    return True


def protocol_version(namespace_or_id):
    """Get the MCP protocol version negotiated with a remote server, e.g.
    "2025-06-18".

    Args:
        namespace_or_id: The server's namespace or UUID

    Returns:
        The protocol version string, "" when the server cannot be reached

    Raises:
        Exception if the server is not found, or on API error
    """
    response = api.get(f"/api/mcp-servers/{_enc(_resolve_id(namespace_or_id))}/protocol", timeout=60)
    return response.get("protocol_version", "")
