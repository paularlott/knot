# knot.event_sink - Event sink management library for Knot server
#
# Event sinks are listeners for space lifecycle and custom events: a webhook
# sink POSTs each matching event to a URL, a script sink runs a scriptling
# script, and a json-rpc sink is a named formatter that shapes event data for
# methods subscribed through knot.methods. A sink is owned by the current user
# or, with the manage global events permission, global (it sees every
# user's events). Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.event_sink
#
#   sink_id = knot.event_sink.create("deploy-hook", ["custom.myapp.*"],
#       url="https://hooks.example.com/knot")
#   for sink in knot.event_sink.list():
#       print(sink["name"], sink["sink_type"], sink["events"])

import knot.apiclient as api
import re
import urllib.parse

_UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def _parse_sink(sink):
    """Parse an event sink response into a stable dict."""
    webhook = sink.get("webhook")
    parsed_webhook = None
    if webhook:
        parsed_webhook = {
            "url": webhook.get("url", ""),
            "secret": webhook.get("secret", ""),
            "headers": webhook.get("headers") or {},
            "body_template": webhook.get("body_template", ""),
            "skip_tls_verify": webhook.get("skip_tls_verify", False),
        }
    return {
        "id": sink.get("event_sink_id", sink.get("id")),
        "user_id": sink.get("user_id", ""),
        "is_global": not sink.get("user_id", ""),
        "name": sink.get("name", ""),
        "description": sink.get("description", ""),
        "events": sink.get("events") or [],
        "sink_type": sink.get("sink_type", "") or "webhook",
        "webhook": parsed_webhook,
        "script_id": sink.get("script_id", ""),
        "active": sink.get("active", False),
    }


def _resolve_id(name_or_id):
    """Return the ID of the sink with this name or ID."""
    if _UUID_RE.match(str(name_or_id)):
        return str(name_or_id)
    matches = [s for s in list() if s["name"] == name_or_id]
    if not matches:
        raise Exception(f"Event sink not found: {name_or_id}")
    if len(matches) > 1:
        raise Exception(f"More than one event sink is named {name_or_id} (a global sink and your own); give its ID")
    return matches[0]["id"]


def _resolve_script(script):
    """Return the ID of the script with this name or ID."""
    if not script or _UUID_RE.match(str(script)):
        return script or ""
    import knot.script
    return knot.script.get_by_name(script)["id"]


def list():
    """List the event sinks the current user may manage.

    Returns the user's own sinks (with the manage events permission) and
    global sinks (with the manage global events permission); an empty list
    without either. Webhook secrets are masked in this listing — use get()
    for the plain secret.

    Returns:
        A list of dicts, each containing:
        - id: Event sink ID
        - name: Sink name
        - description: Description
        - user_id: Owner user ID, empty for a global sink
        - is_global: True for a global sink (receives every user's events)
        - events: List of event type patterns, e.g. ["space.started",
          "custom.myapp.*", "*"]
        - sink_type: "webhook", "script" or "json-rpc"
        - webhook: None, or a dict with url, secret, headers (dict),
          body_template and skip_tls_verify; for a json-rpc sink only
          body_template is used
        - script_id: The script a script sink runs, else ""
        - active: Whether the sink receives events

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/event-sinks")
    return [_parse_sink(s) for s in response.get("event_sinks", [])]


def get(name_or_id):
    """Get an event sink by name or ID, with its webhook secret in plain text.

    Args:
        name_or_id: Sink name or UUID. A name shared by a global sink and one
            of your own is ambiguous; give the ID.

    Returns:
        A dict as list() returns for each sink, the webhook secret unmasked

    Raises:
        Exception if the sink is not found or not visible to you, or on API error
    """
    return _parse_sink(api.get(f"/api/event-sinks/{_enc(_resolve_id(name_or_id))}"))


def create(name, events, sink_type="webhook", description="", url="", secret="",
           headers=None, body_template="", skip_tls_verify=False, script="",
           active=True, is_global=False):
    """Create an event sink and return its ID.

    Args:
        name: Sink name: 2-64 letters, digits and _, starting with a letter
        events: List of event type patterns to receive, e.g.
            ["space.created", "space.started"], ["custom.myapp.*"] or ["*"].
            Custom events always carry the "custom." prefix.
        sink_type: "webhook" (POST each event to url, the default), "script"
            (run a scriptling script) or "json-rpc" (a named formatter used
            by methods that list it in event_sinks)
        description: Optional description
        url: Webhook URL (webhook sinks)
        secret: HMAC signing secret for webhook deliveries; generated when
            empty (webhook sinks)
        headers: Optional dict of extra HTTP headers sent with each webhook
        body_template: Optional template shaping the request body (webhook)
            or the method params (json-rpc); empty sends the default payload
        skip_tls_verify: Skip TLS certificate checks for the webhook URL
        script: Script name or ID to run (script sinks)
        active: Whether the sink receives events (default True)
        is_global: Create a global sink that receives every user's events
            (needs the manage global events permission). Default False: the
            sink is the current user's own (needs the manage events
            permission).

    Returns:
        The new sink's ID

    Raises:
        Exception if the name is invalid, you lack the permission, or on API error
    """
    body = {
        "user_id": "" if is_global else "current",
        "name": name,
        "description": description,
        "events": events or [],
        "sink_type": sink_type,
        "script_id": _resolve_script(script),
        "active": active,
    }
    if sink_type in ("", "webhook"):
        body["webhook"] = {
            "url": url,
            "secret": secret,
            "headers": headers or {},
            "body_template": body_template,
            "skip_tls_verify": skip_tls_verify,
        }
    elif sink_type == "json-rpc":
        body["webhook"] = {"body_template": body_template}
    response = api.post("/api/event-sinks", body)
    return response.get("event_sink_id")


def update(name_or_id, name=None, description=None, events=None, sink_type=None,
           url=None, secret=None, headers=None, body_template=None,
           skip_tls_verify=None, script=None, active=None):
    """Update an event sink; only the arguments given change.

    Args:
        name_or_id: Sink name or UUID
        name: New name
        description: New description
        events: New list of event type patterns (replaces the old list)
        sink_type: "webhook", "script" or "json-rpc"
        url: Webhook URL
        secret: New webhook signing secret; omit to keep the current one
        headers: Dict of extra webhook headers (replaces the old headers)
        body_template: Webhook body / json-rpc params template
        skip_tls_verify: Skip TLS certificate checks for the webhook URL
        script: Script name or ID (script sinks)
        active: Enable or disable the sink

    Returns:
        True if successful

    Raises:
        Exception if the sink is not found, you lack the permission, or on API error
    """
    sink_id = _resolve_id(name_or_id)
    current = api.get(f"/api/event-sinks/{_enc(sink_id)}")
    webhook = current.get("webhook") or {}

    new_type = sink_type if sink_type is not None else (current.get("sink_type", "") or "webhook")
    body = {
        "name": name if name is not None else current.get("name", ""),
        "description": description if description is not None else current.get("description", ""),
        "events": events if events is not None else (current.get("events") or []),
        "sink_type": new_type,
        "script_id": _resolve_script(script) if script is not None else current.get("script_id", ""),
        "active": active if active is not None else current.get("active", False),
    }
    if new_type in ("", "webhook"):
        body["webhook"] = {
            "url": url if url is not None else webhook.get("url", ""),
            "secret": secret if secret is not None else "",  # "" keeps the stored secret
            "headers": headers if headers is not None else (webhook.get("headers") or {}),
            "body_template": body_template if body_template is not None else webhook.get("body_template", ""),
            "skip_tls_verify": skip_tls_verify if skip_tls_verify is not None else webhook.get("skip_tls_verify", False),
        }
    elif new_type == "json-rpc":
        body["webhook"] = {
            "body_template": body_template if body_template is not None else webhook.get("body_template", ""),
        }
    api.put(f"/api/event-sinks/{_enc(sink_id)}", body)
    return True


def delete(name_or_id):
    """Delete an event sink.

    Args:
        name_or_id: Sink name or UUID

    Returns:
        True if successful

    Raises:
        Exception if the sink is not found, you lack the permission, or on API error
    """
    api.delete(f"/api/event-sinks/{_enc(_resolve_id(name_or_id))}")
    return True
