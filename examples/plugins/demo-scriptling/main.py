# /// script
# requires-scriptling = ">=0.34"
#
# [tool.knot]
# version = "1.0.0"
# description = "Demo script plugin: a live dashboard, a block showcase, permission-gated menus, an SVG icon asset, and a themed logo pair."
# permissions = ["view_dashboard", "admin_dashboard"]
# logo_light = "assets/logo-light.svg"
# logo_dark = "assets/logo-dark.svg"
#
# [[tool.knot.field_handlers]]
# label = "Environments (demo plugin)"
# handler = "field_environment"
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
columns, charts (with refresh), a table with live knot.space data, a
one-handler form (GET definition / POST envelope) with a dynamic
autocompleter, markdown and trusted html columns styled with the kp-*
helpers. Everything is data-bound: the layout is fetched once, each column
talks directly to its handler, and form envelopes drive notifications and
column refreshes.
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
    # The layout: rows of columns. knot enforces the permission/group
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
                    {"id": "spaces", "type": "table", "title": "Spaces", "handler": "col_spaces", "refresh": 30, "width": 3},
                    {"id": "widget", "type": "form", "title": "New widget", "handler": "col_widget_form", "width": 1},
                ],
            },
            {
                "columns": [
                    {"id": "notes", "type": "markdown", "title": "Markdown", "handler": "col_notes", "width": 2},
                    {"id": "clock", "type": "html", "title": "Trusted html (kp-* helpers)", "handler": "col_clock", "refresh": 60, "width": 2},
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
    _count, _running, rows = _space_rows()
    if len(rows) == 0:
        rows = [{"name": "(no spaces visible)", "state": "-", "template": "-"}]
    return {
        "columns": [
            {"key": "name", "label": "Space"},
            {"key": "state", "label": "State", "badge": True},
            {"key": "template", "label": "Template"},
        ],
        "rows": rows,
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
        "markdown": 'Panels are **data-bound**: each column fetches its own data with `?_col=<id>`, shows a loader meanwhile, and refreshes on its own timer.\n\n- rows and columns carry permission/group gates\n- forms POST to their column and answer with an envelope\n- table rows carry action buttons\n\n> The handler returns data; knot owns every pixel.'
    }


def col_clock():
    return {
        "html": '<div class="kp-card kp-flex"><svg width="20" height="20" style="color:#3b82f6; flex-shrink:0" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"/></svg>'
        + '<div><div class="kp-label">Server time</div>'
        + '<div class="kp-title kp-mono">' + time.now() + '</div></div></div>'
    }
