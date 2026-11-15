---
type: "API Reference"
title: "knot.event_sink"
description: "Manage event sinks — the webhook, script and JSON-RPC listeners for space lifecycle and custom events."
resource: "https://getknot.dev/reference/libraries/event_sink/"
tags:
  - "events"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/event_sink/"
---
# knot.event_sink

The `knot.event_sink` library manages [event sinks](https://getknot.dev/reference/events/#event-sinks): a **webhook** sink POSTs each matching event to a URL, a **script** sink runs a scriptling script, and a **json-rpc** sink is a named formatter that shapes event data for [methods](https://getknot.dev/reference/events/json-rpc-sinks/) that subscribe to events. A sink is the current user's own (needs the **Manage Events** permission) or global, receiving every user's events (needs **Manage Global Events**). To raise events, use [`knot.event.emit`](../event/) or [`knot.space.emit_event`](../space/#emit_eventname-event_type-payloadnone).

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List the event sinks you may manage |
| `get(name_or_id)` | Get an event sink, with its webhook secret |
| `create(name, events, sink_type="webhook", ...)` | Create an event sink; returns its ID |
| `update(name_or_id, ...)` | Update an event sink; only the arguments given change |
| `delete(name_or_id)` | Delete an event sink |

Sinks are found by name or ID. A name shared by a global sink and one of your own is ambiguous: give the ID.

---

### list()

List the event sinks you may manage: your own with **Manage Events**, global ones with **Manage Global Events**; an empty list with neither. Webhook secrets are masked here.

**Returns:** `list` of [sink dicts](#sink-properties).

---

### get(name_or_id)

Get an event sink, with its webhook secret in plain text.

**Returns:** `dict` - The [sink](#sink-properties). Raises if it is not found or not visible to you.

---

### create(name, events, sink_type="webhook", description="", url="", secret="", headers=None, body_template="", skip_tls_verify=False, script="", active=True, is_global=False)

Create an event sink.

**Parameters:**
- `name` (string): 2–64 letters, digits and `_`, starting with a letter
- `events` (list): Event type patterns to receive, such as `["space.created", "space.started"]`, `["custom.myapp.*"]` or `["*"]`. Custom events always carry the `custom.` prefix.
- `sink_type` (string, optional): `"webhook"` (default), `"script"` or `"json-rpc"`
- `description` (string, optional): Description
- `url` (string, optional): Webhook URL (webhook sinks)
- `secret` (string, optional): HMAC signing secret for webhook deliveries; generated when empty
- `headers` (dict, optional): Extra HTTP headers sent with each webhook
- `body_template` (string, optional): Template shaping the webhook body, or the method params of a json-rpc sink; empty sends the default payload
- `skip_tls_verify` (bool, optional): Skip TLS certificate checks for the webhook URL
- `script` (string, optional): Script name or ID to run (script sinks)
- `active` (bool, optional): Whether the sink receives events (default `True`)
- `is_global` (bool, optional): Create a global sink (needs **Manage Global Events**); default is a sink of your own

**Returns:** `string` - The new sink's ID

---

### update(name_or_id, name=None, description=None, events=None, sink_type=None, url=None, secret=None, headers=None, body_template=None, skip_tls_verify=None, script=None, active=None)

Update an event sink. Only the arguments given change; `events` and `headers` replace the old values, and the webhook secret is kept unless a new one is given.

**Returns:** `bool` - `True` on success

---

### delete(name_or_id)

Delete an event sink.

**Returns:** `bool` - `True` on success

---

## Sink Properties

- `id` (string): Event sink ID
- `name` (string), `description` (string)
- `user_id` (string): Owner user ID, empty for a global sink
- `is_global` (bool): Whether the sink receives every user's events
- `events` (list): Event type patterns
- `sink_type` (string): `"webhook"`, `"script"` or `"json-rpc"`
- `webhook` (dict or None): `url`, `secret` (masked by `list()`), `headers`, `body_template` and `skip_tls_verify`; a json-rpc sink uses only `body_template`
- `script_id` (string): The script a script sink runs
- `active` (bool): Whether the sink receives events

---

## Examples

```python
import knot.event_sink as event_sink

# POST every deploy event to a webhook
sink_id = event_sink.create("deploy_hook", ["custom.myapp.deployed"],
                            url="https://hooks.example.com/knot")
print(event_sink.get(sink_id)["webhook"]["secret"])  # verify signatures with this

# Run a script when any space starts
event_sink.create("on_start", ["space.started"], sink_type="script", script="notify-start")

# Pause the webhook
event_sink.update("deploy_hook", active=False)
```
