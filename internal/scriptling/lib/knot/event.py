# knot.event - Event library for Knot
#
# Space-side: provides emit() to raise custom events from inside a space.
# Server-side (sink scripts): provides get_* accessors and metadata functions.
#
# Usage (space-side):
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
    """Emit a custom event from inside a space.

    The 'custom.' prefix is prepended automatically.

    Args:
        event_type: The event type string (without 'custom.' prefix).
        payload: Optional dict payload to include with the event.

    Returns:
        True if the event was accepted by the agent.

    Raises:
        Exception if the agent API is unreachable or rejects the event.
    """
    if payload is None:
        payload = {}

    api_port = os.environ.get("KNOT_API_PORT", "12201")
    url = "http://127.0.0.1:" + api_port + "/event"

    resp = requests.post(
        url,
        headers={"Content-Type": "application/json"},
        data=json.dumps({"type": event_type, "payload": payload}),
    )

    if resp.status_code != 202 and resp.status_code != 200:
        raise Exception("failed to emit event: agent returned HTTP " + str(resp.status_code))

    return True
