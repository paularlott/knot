# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0.0"
# requires_knot = ">=0.34"
# description = "Demo script plugin: a live dashboard, a block showcase, permission-gated menus, an SVG icon asset, and a themed logo pair."
# permissions = ["view_dashboard", "admin_dashboard"]
# logo_light = "assets/logo-light.svg"
# logo_dark = "assets/logo-dark.svg"
#
# [[tool.knot.field_handlers]]
# label = "Environments (demo plugin)"
# handler = "field_environment"
#
# [[tool.knot.handlers]]
# handler = "echo_word"
# permission = "view_dashboard"
#
# [[tool.knot.mcp_tools]]
# name = "echo_word"
# description = "Echo a word back in upper case, with its length."
# handler = "echo_word"
#
# [[tool.knot.mcp_tools.parameters]]
# name = "word"
# type = "string"
# description = "The word to echo back."
# required = true
#
# [[tool.knot.pages]]
# path = "/showcase"
# handler = "showcase"
# label = "Block Showcase"
# menu_label = "Block Showcase"
# permission = "view_dashboard"
# icon = "assets/icon.svg"
#
# [[tool.knot.menus]]
# label = "Scriptling"
# url = "https://github.com/paularlott/scriptling"
# icon = "assets/icon.svg"
#
# [[tool.knot.menus]]
# label = "Demo Admin"
# url = "https://example.com/demo-admin"
# permission = "admin_dashboard"
# icon = "assets/icon.svg"
# ///

"""demo-scriptling: a knot plugin written in scriptling.

The showcase page demonstrates the rows/columns page contract: KPI stat
columns, charts (with refresh), a table with live knot.space data and the
full row-action set (icon buttons, kebab menu, inline and modal confirms,
popup forms, markdown popups, success dialogs), a one-handler form (GET
definition / POST envelope) with a dynamic autocompleter, markdown and
trusted html columns styled with the kp-* helpers. Everything is
data-bound: the layout is fetched once, each column talks directly to its
handler, and form envelopes drive notifications and column refreshes.
"""

import math
import time


def _space_rows():
    """Real spaces the requesting user can see (knot.* runs as them)."""
    rows = []
    count = 0
    running = 0
    try:
        import knot.space as space

        for s in space.list():
            count = count + 1
            if s.get("is_deleting", False):
                state = "deleting"
            elif s.get("is_pending", False):
                state = "pending"
            elif s.get("is_running", False):
                state = "running"
                running = running + 1
            else:
                state = "stopped"
            if len(rows) < 8:
                rows.append(
                    {
                        "name": s.get("name", "?"),
                        "state": state,
                        "template": s.get("template_name", ""),
                    }
                )
    except Exception:
        rows = []
    return count, running, rows


def field_environment():
    """Field handler: suggestion source for template custom fields."""
    return {"options": [{"key": "dev", "text": "development"}, {"key": "stage", "text": "staging"}, {"key": "prod", "text": "production"}, {"key": "qa", "text": "qa"}, {"key": "demo", "text": "demo"}]}


def showcase():
    # The layout: rows of columns. knot enforces the permission
    # gates and never sends a row left with no columns.
    return {
        "rows": [
            {
                "title": "Fleet overview",
                "style": "card",
                "columns": [
                    {"id": "spaces", "type": "stat", "handler": "stat_spaces", "refresh": 30},
                    {"id": "widgets", "type": "stat", "handler": "stat_widgets"},
                    {"id": "uptime", "type": "stat", "handler": "stat_uptime"},
                    {"id": "queue", "type": "stat", "handler": "stat_queue"},
                ],
            },
            {
                "columns": [
                    {"id": "cpu", "type": "chart", "title": "Fleet CPU / memory", "handler": "col_chart", "refresh": 30, "width": 3},
                    {"id": "cap", "type": "chart", "title": "Capacity", "handler": "col_doughnut", "width": 1},
                ],
            },
            {
                "title": "Spaces",
                "columns": [
                    # The column's actions are the fallback; rows that carry
                    # their own list (see _space_actions) replace them.
                    {
                        "id": "fleet",
                        "type": "table",
                        "title": "Spaces",
                        "handler": "col_spaces",
                        "refresh": 30,
                        "width": 3,
                        "actions": [
                            {"action": "notes", "label": "Notes", "icon": "document", "menu": True, "handler": "widget_notes"},
                            {"action": "report", "label": "Run report", "icon": "info", "menu": True},
                        ],
                    },
                    {"id": "widget", "type": "form", "title": "New widget", "handler": "col_widget_form", "width": 1},
                ],
            },
            {
                "columns": [
                    {"id": "notes", "type": "markdown", "title": "Markdown", "handler": "col_notes", "width": 2},
                    {"id": "clock", "type": "html", "title": "Trusted html (kp-* helpers)", "handler": "col_clock", "refresh": 60, "width": 1},
                    {"id": "echo", "type": "html", "title": "Trusted html (Alpine calling the plugin)", "handler": "col_echo", "width": 1},
                ],
            },
            {
                "title": "Text and bars",
                "columns": [
                    {"id": "blurb", "type": "text", "title": "Text", "handler": "col_text", "width": 2},
                    {"id": "bars", "type": "bar", "title": "Bar", "handler": "col_bars", "width": 2},
                ],
            },
        ]
    }


def col_text():
    return {"text": "Plain text column: whitespace preserved, escaped, theme-aware. Good for captions, generated-at stamps, and anything that needs no markup."}



def col_bars():
    # The bar payload is a single bar; the column stacks a pair via a list.
    return [
        {"label": "Memory", "value": 4.7, "max": 8.0, "unit": "GiB", "color": "#8b5cf6"},
        {"label": "Disk", "value": 15.0, "max": 40.0, "unit": "GiB", "color": "#10b981"},
    ]


def col_kpi():
    space_count, running, _rows = _space_rows()
    return [
        {"label": "Spaces", "value": space_count, "delta": str(running) + " running"},
        {"label": "Widgets", "value": 3, "delta": "1 pending"},
        {"label": "Uptime", "value": 97.2, "unit": "%", "accent": "#10b981"},
        {"label": "Queue", "value": 0},
    ]


def _jitter(value, scale):
    # A small random walk that keeps values positive and rounded - enough
    # movement for refreshes to visibly animate, not enough to look noisy.
    import random

    return round(max(0.0, value + random.uniform(-scale, scale)), 1)


def _kpi():
    space_count, running, _rows = _space_rows()
    return [
        {"label": "Spaces", "value": space_count, "delta": str(running) + " running"},
        {"label": "Widgets", "value": 3, "delta": "1 pending"},
        {"label": "Uptime", "value": 97.2, "unit": "%", "accent": "#10b981"},
        {"label": "Queue", "value": 0},
    ]


def stat_spaces():
    return _kpi()[0]


def stat_widgets():
    return _kpi()[1]


def stat_uptime():
    return _kpi()[2]


def stat_queue():
    return _kpi()[3]


def col_chart():
    import math

    # The last few points drift a little on each fetch so the line chart
    # animates on refresh; the rest of the series stays put.
    cpu = []
    mem = []
    for i in range(12):
        c = 40 + 20 * math.sin(i / 2.0)
        m = 30 + 10 * math.cos(i / 3.0)
        if i >= 9:
            c = _jitter(c, 2.5)
            m = _jitter(m, 1.5)
        cpu.append(round(c, 1))
        mem.append(round(m, 1))
    return {
        "chart_type": "line",
        "labels": [str(i) for i in range(12)],
        "height": 240,
        "datasets": [
            {"name": "CPU %", "data": cpu, "color": "#3b82f6"},
            {"name": "Memory GB", "data": mem, "color": "#8b5cf6"},
        ],
    }


def col_doughnut():
    # "In use" wobbles slightly; "Free" moves to keep the total constant.
    used = _jitter(15.0, 1.2)
    return {
        "chart_type": "doughnut",
        "height": 200,
        "labels": ["In use", "Free"],
        "datasets": [{"name": "Capacity", "data": [used, round(40.0 - used, 1)], "color": "#8b5cf6"}],
    }


def col_spaces():
    # POST: row actions arrive here with the action name and the row key.
    if request.method == "POST":
        action = params.get("action", "")
        key = params.get("key", "")
        if action == "report":
            # A success envelope may carry a dialog: markdown the client
            # renders in a popup after the toast.
            return {
                "status": "ok",
                "message": "Report for " + key + " generated.",
                "dialog": {
                    "title": "Report: " + key,
                    "markdown": f"""Generated {time.now()} for **{key}**.

- actions POST to the column's own handler
- the envelope carries a dialog with markdown
- knot renders it server-side, the client just places it

```python
def col_spaces():
    return {{"status": "ok", "dialog": {{"title": ..., "markdown": ...}}}}
```""",
                },
            }
        if action == "archive":
            return {"status": "ok", "message": key + " archived (demo).", "refresh": ["fleet"]}
        if action in ["start", "stop", "restart"]:
            # Real lifecycle, running as the requesting user.
            import knot.space as space_lib

            try:
                if action == "start":
                    space_lib.start(key)
                elif action == "stop":
                    space_lib.stop(key)
                else:
                    space_lib.restart(key)
            except Exception as err:
                message = str(err)
                if "unsupported platform" in message:
                    message = key + " has no container runtime behind it on this server"
                return {"status": "error", "message": "Could not " + action + " " + key + ": " + message}
            return {"status": "ok", "message": key + " " + action + " requested.", "refresh": ["fleet", "spaces"]}
        return {"status": "error", "message": "Unknown action: " + action}

    _count, _running, rows = _space_rows()
    if len(rows) == 0:
        # No per-row actions here, so these rows fall back to the column's
        # action set below.
        rows = [{"name": "(no spaces visible)", "state": "-", "template": "-"}]
    else:
        for row in rows:
            row["id"] = row["name"]
            row["actions"] = _space_actions(row["name"], row["state"])
    return {
        "columns": [
            {"key": "name", "label": "Space"},
            {"key": "state", "label": "State", "badge": True},
            {"key": "template", "label": "Template"},
        ],
        "rows": rows,
    }


def _space_actions(name, state):
    # Per-row actions replace the column's set: the handler decides, per
    # row, what is offered. Icon buttons render inline; menu: True items
    # collect into the kebab dropdown.
    actions = []
    if state == "running":
        actions.append({"action": "stop", "label": "Stop " + name, "icon": "stop", "style": "danger", "confirm": "Stop " + name + "?"})
    else:
        actions.append({"action": "start", "label": "Start " + name, "icon": "play", "style": "success"})
    actions.append({"action": "edit", "label": "Edit " + name, "icon": "edit", "handler": "widget_edit"})
    actions.append({"action": "notes", "label": "Notes", "icon": "document", "menu": True, "handler": "widget_notes"})
    actions.append({"action": "restart", "label": "Restart " + name, "icon": "restart", "menu": True, "confirm": "Restart " + name + "?"})
    actions.append({"action": "report", "label": "Run report", "icon": "info", "menu": True})
    actions.append({"action": "archive", "label": "Archive " + name, "icon": "trash", "style": "danger", "menu": True, "confirm": "Archive " + name + "? This only hides it in the demo."})
    return actions


def widget_edit():
    # Popup form handler: GET returns the form (fields carry their values),
    # POST validates and answers with an envelope. Errors keep the popup
    # open with per-field messages; success closes it.
    key = params.get("key", "")
    if request.method == "POST":
        errors = {}
        if params.get("name", "") == "":
            errors["name"] = "Required."
        if params.get("owner", "") == "":
            errors["owner"] = "Pick an owner from the suggestions."
        if len(errors) > 0:
            return {"status": "error", "message": "The form has errors; the popup stays open.", "field_errors": errors}
        return {"status": "ok", "message": params.get("name", "") + " saved (demo).", "refresh": ["fleet"]}

    if params.get("_data", "") == "owner":
        owners = []
        for owner in ["alice", "bob", "carol", "dave", "erin"]:
            owners.append({"key": owner, "text": owner})
        return {"options": owners}

    state = "running"
    return {
        "title": "Edit " + key,
        "fields": [
            {"type": "text", "name": "name", "label": "Name", "value": key},
            {"type": "autocomplete", "name": "owner", "label": "Owner", "value": "alice", "dynamic_options": True},
            {"type": "select", "name": "state", "label": "State", "options": ["running", "stopped"], "value": state},
        ],
        "submit": "Save changes",
    }


def widget_notes():
    # Information popup: an action names this handler, the client GETs it
    # with the row key, and the markdown response opens read-only.
    key = params.get("key", "")
    return {
        "title": "Notes: " + key,
        "markdown": """Information popups are just handlers: an action names one, the client
GETs it with the row key, and a markdown response opens read-only.

- fetched fresh on every open
- rendered server-side, so the client never runs plugin markdown
- the popup is draggable and resizable like every knot dialog""",
    }


def col_widget_form():
    # Dynamic options for the autocompleter: the client asks this handler
    # with _data=<field name> and it answers with key/text suggestions.
    if params.get("_data", "") == "owner":
        return {"options": [
            {"key": "alice", "text": "alice"},
            {"key": "bob", "text": "bob"},
            {"key": "carol", "text": "carol"},
            {"key": "dave", "text": "dave"},
        ]}

    # One handler, two faces: GET returns the definition, POST processes
    # the submitted fields and returns the action envelope.
    if request.method == "POST":
        name = params.get("widget_name", "")
        if name == "":
            return {
                "status": "error",
                "message": "A widget needs a name.",
                "field_errors": {"widget_name": "Required."},
            }
        return {
            "status": "ok",
            "message": "Widget " + name + " created for " + (params.get("owner", "") or "nobody") + ".",
            "refresh": True,
        }
    return {
        "fields": [
            {"type": "hidden", "name": "action", "value": "create_widget"},
            {"type": "text", "name": "widget_name", "label": "Widget name", "placeholder": "e.g. cache-warmer"},
            {"type": "autocomplete", "name": "owner", "label": "Owner", "dynamic_options": True},
            {"type": "select", "name": "size", "label": "Size", "options": ["small", "medium", "large"]},
        ],
        "submit": "Create widget",
    }


def col_notes():
    return {
        "markdown": """Panels are **data-bound**: each column fetches its own data from its
handler URL (the page path plus `/<handler>`), shows a loader meanwhile, and
refreshes on its own timer.

- rows and columns carry permission gates
- forms POST to their column's handler URL and answer with an envelope
- table rows carry action buttons
- handlers are addressable from any page, including other plugins' pages

> The handler returns data; knot owns every pixel.""",
    }


def col_echo():
    # Static markup: the widget calls back into the plugin from the browser
    # with pluginFetch('echo_word', ...) - same transport, auth and gates
    # as every column fetch. The second button demos a cross-plugin call:
    # demo-go's handler is addressed by plugin name, gated by that plugin's
    # default page for the requesting user. No refresh key: an interactive
    # column must not have its content replaced under the user.
    return {
        "html": """
<div class="kp-card" x-data="{ word: '', busy: false, reply: '', peer: '', peerBusy: false }">
  <div class="kp-label">Echo service</div>
  <div class="kp-flex" style="margin-top:0.5rem">
    <input class="kp-input"
           x-model="word" placeholder="type a word" aria-label="Word to echo">
    <button class="kp-button"
            :disabled="busy"
            @click="busy = true; try { reply = (await pluginFetch('echo_word', { params: { word: word } })).reply } finally { busy = false }"
            x-text="busy ? '...' : 'Send'"></button>
    <button class="kp-button"
            :disabled="peerBusy"
            @click="peerBusy = true; try { const p = await pluginFetch('peer_summary', { plugin: 'demo-go' }); const c = await pluginFetch('peer_class', { plugin: 'demo-go' }); peer = 'demo-go peer: ' + p.summary + ' (class demo: ' + c.first + ', ' + c.second + ')' } catch (e) { peer = 'demo-go not available: ' + e.message } finally { peerBusy = false }"
            x-text="peerBusy ? '...' : 'Ask demo-go'"></button>
  </div>
  <div class="kp-muted" style="margin-top:0.5rem; min-height:1.2rem" x-show="reply" x-text="reply"></div>
  <div class="kp-muted" style="margin-top:0.25rem; min-height:1.2rem" x-show="peer" x-text="peer"></div>
</div>
"""
    }


def echo_word():
    # Called by the echo widget's pluginFetch; params arrive like any
    # handler's (query merged over POST body). The [[tool.knot.handlers]]
    # declaration is required because no layout column references it - a
    # handler reachable only from trusted html declares its own gate.
    word = params.get("word", "")
    if word == "":
        return {"reply": "type something first"}
    return {"reply": "echo: " + word.upper() + " (" + str(len(word)) + " chars)"}


def col_clock():
    now = time.now()
    return {
        "html": f"""
<div class="kp-card kp-flex">
  <svg width="20" height="20" style="color:#3b82f6; flex-shrink:0" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"/></svg>
  <div>
    <div class="kp-label">Server time</div>
    <div class="kp-title kp-mono">{now}</div>
  </div>
</div>
"""
    }
