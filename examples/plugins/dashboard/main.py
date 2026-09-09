# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0.0"
# description = "Your landing dashboard: spaces, live CPU / memory / disk aggregates, fleet history, and per-space usage. All data comes from knot.* over the loopback, as the requesting user."
#
# [[tool.knot.pages]]
# path = "/home"
# handler = "dashboard"
# label = "Dashboard"
# menu_label = "Dashboard"
# default = true
# icon = "assets/icon.svg"
# ///

"""dashboard: a real landing dashboard built only from knot.* libraries.

The layout handler runs as the requesting user on every render; each column
handler is self-contained, reading the page's params directly (state, range)
so filter submissions flow to every column's next fetch. It claims the
post-login landing spot with default = true.
"""

GIB = 1073741824.0


def gib(bytes_value):
    if bytes_value is None:
        return 0.0
    return round(bytes_value / GIB, 1)


def dashboard(request):
    params = request["params"]
    state_filter = params.get("state", "all")
    heading = "Your spaces"
    if state_filter != "all":
        heading = "Your spaces: " + state_filter
    return {
        "rows": [
            {
                "title": heading,
                "style": "card",
                "columns": [
                    {"id": "spaces", "type": "stat", "handler": "stat_spaces", "refresh": 30},
                    {"id": "pending", "type": "stat", "handler": "stat_pending"},
                    {"id": "cpu", "type": "stat", "handler": "stat_cpu"},
                    {"id": "memory", "type": "stat", "handler": "stat_memory"},
                ],
            },
            {
                "columns": [
                    {"id": "series", "type": "chart", "title": "Fleet usage over time", "handler": "col_series", "refresh": 30, "width": 2},
                    {"id": "top", "type": "chart", "title": "Top spaces by memory", "handler": "col_top", "refresh": 30, "width": 1},
                    {"id": "filter", "type": "form", "title": "Filter", "handler": "filter_form", "width": 1},
                ],
            },
            {
                "columns": [
                    {
                        "id": "fleet",
                        "type": "table",
                        "title": "Spaces",
                        "handler": "col_spaces",
                        "refresh": 30,
                        "width": 4,
                        # The fetch-time gate serves an undeclared handler only
                        # if the layout names it: a column-level action with a
                        # handler puts space_edit in that set. Row actions
                        # (from col_spaces) replace this at render time.
                        "actions": [
                            {"action": "edit", "label": "Edit", "icon": "edit", "handler": "space_edit"},
                        ],
                    },
                ],
            },
        ]
    }


def _state_filter(params):
    state = params.get("state", "all")
    if state not in ["all", "running", "stopped", "pending", "deleting"]:
        state = "all"
    return state


def _range_filter(params):
    rng = params.get("range", "1h")
    if rng not in ["1h", "7d"]:
        rng = "1h"
    return rng


def filter_form(request):
    params = request["params"]
    # GET: the definition. POST: acknowledge - the runtime merges the
    # submitted fields into the page params and refreshes the columns.
    if request["method"] == "POST":
        return {"status": "ok", "message": "Filtered.", "refresh": True}
    return {
        "fields": [
            {"type": "select", "name": "state", "label": "State", "value": _state_filter(params), "options": ["all", "running", "stopped", "pending", "deleting"]},
            {"type": "select", "name": "range", "label": "Graph range", "value": _range_filter(params), "options": ["1h", "7d"]},
        ],
        "submit": "Apply",
    }


def _spaces():
    try:
        import knot.space as space_lib

        return space_lib.list()
    except Exception:
        return []


def _visible_spaces(params):
    state = _state_filter(params)
    rows = []
    for s in _spaces():
        if s.get("is_deleting", False):
            s_state = "deleting"
        elif s.get("is_pending", False):
            s_state = "pending"
        elif s.get("is_running", False):
            s_state = "running"
        else:
            s_state = "stopped"
        if state == "all" or state == s_state:
            rows.append((s, s_state))
    return rows


def _fleet_summary():
    total = 0
    running = 0
    pending = 0
    cpu_total = 0.0
    mem_used = 0.0
    for s in _spaces():
        total = total + 1
        if s.get("is_pending", False):
            pending = pending + 1
        if s.get("is_running", False):
            running = running + 1
        usage = s.get("resource_usage")
        if usage is not None:
            try:
                cpu_total = cpu_total + usage.get("cpu_percent", 0.0)
                mem_used = mem_used + usage.get("memory_used_bytes", 0)
            except Exception:
                pass
    return total, running, pending, cpu_total, mem_used


def stat_spaces(request):
    total, running, _pending, _cpu, _mem = _fleet_summary()
    return {"label": "Spaces", "value": total, "delta": str(running) + " running"}


def stat_pending(request):
    _total, _running, pending, _cpu, _mem = _fleet_summary()
    return {"label": "Pending", "value": pending}


def stat_cpu(request):
    _total, _running, _pending, cpu, _mem = _fleet_summary()
    return {"label": "CPU", "value": round(cpu, 1), "unit": "%", "accent": "#3b82f6"}


def stat_memory(request):
    _total, _running, _pending, _cpu, mem = _fleet_summary()
    return {"label": "Memory", "value": round(gib(mem), 1), "unit": "GiB"}


def col_series(request):
    params = request["params"]
    import knot.space as space_lib

    range_filter = _range_filter(params)
    cpu_by_bucket = {}
    mem_by_bucket = {}
    for sp in space_lib.list():
        if not sp.get("is_running", False):
            continue
        space_id = sp.get("id", "")
        if space_id == "":
            continue
        try:
            hist = space_lib.usage_history(space_id, range_filter)
        except Exception:
            continue
        for point in hist.get("points", []):
            ts = str(point.get("bucket_start", ""))
            if ts == "":
                continue
            usage = point.get("resource_usage")
            if usage is None:
                continue
            cpu_by_bucket[ts] = cpu_by_bucket.get(ts, 0.0) + usage.get("cpu_percent", 0.0)
            mem_by_bucket[ts] = mem_by_bucket.get(ts, 0.0) + usage.get("memory_used_bytes", 0)
    labels = []
    cpu_line = []
    mem_line = []
    if len(cpu_by_bucket) > 1:
        buckets = sorted(cpu_by_bucket.keys())
        for ts in buckets:
            stamp = ts
            if len(stamp) >= 16:
                stamp = stamp[11:16]
            if range_filter == "7d":
                stamp = ts[5:10]
            labels.append(stamp)
            cpu_line.append(round(cpu_by_bucket.get(ts, 0.0), 1))
            mem_line.append(gib(mem_by_bucket.get(ts, 0)))
    if len(labels) == 0:
        labels = ["-"]
        cpu_line = [0]
        mem_line = [0]
    return {
        "chart_type": "line",
        "labels": labels,
        "height": 260,
        "datasets": [
            {"name": "CPU %", "data": cpu_line, "color": "#3b82f6"},
            {"name": "Memory GiB", "data": mem_line, "color": "#8b5cf6"},
        ],
    }


def col_top(request):
    params = request["params"]
    by_mem = []
    for s, _state in _visible_spaces(params):
        usage = s.get("resource_usage")
        mem_u = 0.0
        if usage is not None:
            try:
                mem_u = gib(usage.get("memory_used_bytes", 0))
            except Exception:
                mem_u = 0.0
        if mem_u > 0:
            by_mem.append([mem_u, s.get("name", "?")])
    names = []
    mems = []
    for i in range(5):
        if len(by_mem) == 0:
            break
        best = 0
        for j in range(1, len(by_mem)):
            if by_mem[j][0] > by_mem[best][0]:
                best = j
        picked = by_mem[best]
        names.append(picked[1])
        mems.append(picked[0])
        by_mem.pop(best)
    if len(names) == 0:
        names = ["-"]
        mems = [0]
    return {
        "chart_type": "bar",
        "height": 200,
        "labels": names,
        "datasets": [{"name": "Memory (GiB)", "data": mems, "color": "#8b5cf6"}],
    }


def col_spaces(request):
    params = request["params"]
    # POST: row actions arrive with the action name and the row key, and
    # drive the real space lifecycle as the requesting user.
    if request["method"] == "POST":
        action = params.get("action", "")
        key = params.get("key", "")
        if action in ["start", "stop", "restart"]:
            import knot.space as space_lib

            try:
                if action == "start":
                    space_lib.start(key)
                    return {"status": "ok", "message": key + " is starting.", "refresh": ["fleet", "spaces"]}
                if action == "stop":
                    space_lib.stop(key)
                    return {"status": "ok", "message": key + " is stopping.", "refresh": ["fleet", "spaces"]}
                space_lib.restart(key)
                return {"status": "ok", "message": key + " is restarting.", "refresh": ["fleet", "spaces"]}
            except Exception as err:
                message = str(err)
                if "unsupported platform" in message:
                    message = key + " has no container runtime behind it on this server"
                return {"status": "error", "message": "Could not " + action + " " + key + ": " + message}
        return {"status": "error", "message": "Unknown action: " + action}

    rows = []
    for s, state in _visible_spaces(params):
        usage = s.get("resource_usage")
        cpu = 0.0
        mem_u = 0.0
        mem_l = 0.0
        disk_u = 0.0
        disk_l = 0.0
        if usage is not None:
            try:
                cpu = round(usage.get("cpu_percent", 0.0), 1)
                mem_u = gib(usage.get("memory_used_bytes", 0))
                mem_l = gib(usage.get("memory_limit_bytes", 0))
                disk_u = gib(usage.get("disk_used_bytes", 0))
                disk_l = gib(usage.get("disk_limit_bytes", 0))
            except Exception:
                pass
        name = s.get("name", "?")
        actions = [{"action": "edit", "label": "Edit " + name, "icon": "edit", "handler": "space_edit"}]
        if state == "running":
            actions.insert(0, {"action": "stop", "label": "Stop " + name, "icon": "stop", "style": "danger", "confirm": "Stop " + name + "?"})
            actions.append({"action": "restart", "label": "Restart " + name, "icon": "restart", "menu": True, "confirm": "Restart " + name + "?"})
        else:
            actions.insert(0, {"action": "start", "label": "Start " + name, "icon": "play", "style": "success"})
        rows.append(
            {
                "id": name,
                "name": name,
                "state": state,
                "template": s.get("template_name", ""),
                "cpu": str(cpu) + " %",
                "memory": str(mem_u) + " / " + str(mem_l) + " GiB",
                "disk": str(disk_u) + " / " + str(disk_l) + " GiB",
                "node": s.get("node_hostname", ""),
                "actions": actions,
            }
        )
    if len(rows) == 0:
        rows = [{"name": "(no spaces)", "state": "-", "template": "", "cpu": "", "memory": "", "disk": "", "node": ""}]
    return {
        "columns": [
            {"key": "name", "label": "Space"},
            {"key": "state", "label": "State", "badge": True},
            {"key": "template", "label": "Template"},
            {"key": "cpu", "label": "CPU"},
            {"key": "memory", "label": "Memory"},
            {"key": "disk", "label": "Disk"},
            {"key": "node", "label": "Node"},
        ],
        "rows": rows,
    }


def space_edit(request):
    params = request["params"]
    # Popup form: GET returns the definition with current values, POST
    # answers with the envelope. Errors keep the popup open; success
    # closes it and refreshes the table.
    key = params.get("key", "")
    if request["method"] == "POST":
        name = params.get("name", "")
        if name == "":
            return {"status": "error", "message": "The form has errors.", "field_errors": {"name": "Required."}}
        return {"status": "ok", "message": name + " saved (demo).", "refresh": ["fleet"]}
    return {
        "title": "Edit " + key,
        "fields": [
            {"type": "text", "name": "name", "label": "Name", "value": key},
            {"type": "select", "name": "state", "label": "State", "value": "running", "options": ["running", "stopped"]},
        ],
        "submit": "Save changes",
    }
