# knot.mcp - MCP tool interaction library for Knot server
#
# In embedded contexts (MCP, remote, local), this module is shadowed by the
# Go-registered knot.mcp library which routes through the server's internal
# MCP endpoint.
#
# For standalone use (outside knot), this module uses scriptling.mcp.Client
# connected to the server's MCP endpoint via knot.apiclient config.
#
# Usage:
#   import knot.mcp as mcp
#   tools = mcp.list_tools()
#   result = mcp.execute_tool("my_tool", {"param": "value"})

import knot.apiclient as _apiclient

_client = None


def _get_client():
    """Get or create the scriptling.mcp.Client instance."""
    global _client
    if _client is None:
        import scriptling.mcp as mcp
        client = _apiclient._get_client()
        if not client:
            raise Exception("knot.apiclient not configured. Set KNOT_URL and KNOT_TOKEN or call knot.apiclient.configure().")
        _client = mcp.Client(
            client["url"] + "/mcp",
            bearer_token=client["token"],
        )
    return _client


def list_tools():
    """Get a list of all available MCP tools.

    Returns:
        list: List of tool dicts with name, description, and parameters
    """
    return _get_client().tools()


def call_tool(name, arguments):
    """Call an MCP tool directly.

    Args:
        name: Tool name
        arguments: Dict of arguments to pass to the tool

    Returns:
        The tool's response
    """
    return _get_client().call_tool(name, arguments)


def tool_search(query, max_results=10):
    """Search for tools by keyword.

    Args:
        query: Search query string
        max_results: Maximum number of results to return (default: 10)

    Returns:
        list: Matching tools
    """
    return _get_client().tool_search(query, max_results=max_results)


def execute_tool(name, arguments):
    """Execute a discovered tool.

    Args:
        name: Tool name
        arguments: Dict of arguments to pass to the tool

    Returns:
        The tool's response
    """
    return _get_client().execute_discovered(name, arguments)
