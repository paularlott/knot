# knot.audit - Audit log library for Knot server
#
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.apiclient
#   import knot.audit
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   logs = knot.audit.list()
#   logs = knot.audit.search("Login", actor="admin@example.com")

import knot.apiclient as api


def list(start=0, max_items=10, q="", actor="", actor_type="", event="", from_time="", to_time=""):
    """List audit log entries with optional filtering.

    Args:
        start: Offset to start at (default: 0)
        max_items: Maximum number of items to return (default: 10)
        q: Full-text search across actor, event, and details
        actor: Filter by actor name (exact match)
        actor_type: Filter by actor type (User, System, MCP)
        event: Filter by event type (exact match)
        from_time: Start of date range (RFC3339 string)
        to_time: End of date range (RFC3339 string)

    Returns:
        A dict containing:
        - count: Total number of matching audit logs
        - items: List of audit log entry dicts

    Raises:
        Exception if not configured or on API error
    """
    params = {"start": str(start), "max-items": str(max_items)}
    if q:
        params["q"] = q
    if actor:
        params["actor"] = actor
    if actor_type:
        params["actor_type"] = actor_type
    if event:
        params["event"] = event
    if from_time:
        params["from"] = from_time
    if to_time:
        params["to"] = to_time

    response = api.get("/api/audit-logs", params)

    return {
        "count": response.get("count", 0),
        "items": [_parse_entry(e) for e in response.get("items", [])],
    }


def search(q, start=0, max_items=10, actor="", actor_type="", event="", from_time="", to_time=""):
    """Search audit logs with a text query.

    Args:
        q: Search query (searches actor, event, details)
        start: Offset to start at (default: 0)
        max_items: Maximum number of items to return (default: 10)
        actor: Filter by actor name (exact match)
        actor_type: Filter by actor type (User, System, MCP)
        event: Filter by event type (exact match)
        from_time: Start of date range (RFC3339 string)
        to_time: End of date range (RFC3339 string)

    Returns:
        A dict containing:
        - count: Total number of matching audit logs
        - items: List of audit log entry dicts

    Raises:
        Exception if not configured or on API error
    """
    return list(start=start, max_items=max_items, q=q, actor=actor,
                actor_type=actor_type, event=event, from_time=from_time, to_time=to_time)


def _parse_entry(entry):
    """Parse an audit log entry into a standardized dict."""
    return {
        "id": entry.get("audit_log_id"),
        "zone": entry.get("zone", ""),
        "actor": entry.get("actor", ""),
        "actor_type": entry.get("actor_type", ""),
        "event": entry.get("event", ""),
        "when": entry.get("when", ""),
        "details": entry.get("details", ""),
        "properties": entry.get("properties", {}),
    }
