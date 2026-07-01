# knot.event - Event library for Knot
#
# emit(): available in space-side scripts, MCP tool execution, and external
#         standalone scripts. In a space (KNOT_API_PORT set) it delivers via
#         the local agent and is space-aware; elsewhere it sends via
#         knot.apiclient to /api/events/emit as a user-scoped event.
# Server-side sink scripts: provides get_* accessors and metadata functions
#                            (registered as the Go knot.event library there).
#
# Usage (emit):
#   import knot.event
#   knot.event.emit("myapp.deployed", {"version": "1.0"})
#
# Usage (server-side, sink scripts):
#   import knot.event
#   version = knot.event.get_string("version", "")

import os
import json
import requests


def emit(event_type, payload=None):
    """Emit a custom event.

    The 'custom.' prefix is prepended automatically.

    When running inside a space (KNOT_API_PORT is set) the event is delivered
    via the local agent and associated with the originating space. Everywhere
    else (MCP tool execution, external standalone scripts) it is sent to the
    server's /api/events/emit endpoint via knot.apiclient as a user-scoped
    event (no space).

    Args:
        event_type: The event type string (without 'custom.' prefix).
        payload: Optional dict payload to include with the event.

    Returns:
        True if the event was accepted.

    Raises:
        Exception if delivery fails.
    """
    if payload is None:
        payload = {}

    # Inside a space: deliver via the local agent (space-aware).
    if "KNOT_API_PORT" in os.environ:
        url = "http://127.0.0.1:" + os.environ["KNOT_API_PORT"] + "/event"

        resp = requests.post(
            url,
            headers={"Content-Type": "application/json"},
            data=json.dumps({"type": event_type, "payload": payload}),
        )

        if resp.status_code != 202 and resp.status_code != 200:
            raise Exception("failed to emit event: agent returned HTTP " + str(resp.status_code))

        return True

    # MCP tool execution or external standalone: deliver via knot.apiclient
    # (the loopback transport in embedded contexts, real HTTP externally).
    # The server sets a nil space id for these user-scoped events.
    import knot.apiclient as api

    api.post("/api/events/emit", {"type": event_type, "payload": payload})
    return True
