# /// script
# requires-scriptling = ">=0.34"
#
# dependencies = [
#   "plugin.demolib via demolib >= 1.0.0",
# ]
#
# [tool.knot]
# version = "1.0.0"
# description = "Demo plugin whose dashboard measures its Go binary peer live. Run make in this folder to build the peer; until then this plugin fails its requirements on purpose (that is the failure path demo)."
# permissions = ["view_status"]
# logo_light = "assets/logo-light.svg"
#
# [[tool.knot.pages]]
# path = "/status"
# handler = "status"
# label = "Go Demo Status"
# menu_label = "Go Demo"
# permission = "view_status"
# icon = "assets/icon.svg"
#
# [[tool.knot.handlers]]
# handler = "peer_summary"
# ///

"""demo-go: a knot plugin that uses a Go binary peer.

The peer (peer/main.go, built into bin/ by make) is spawned by knot at plugin
load and handshakes as "demolib". The /status dashboard calls it live: it
measures round-trip latency to the Go process for every sample and charts
the results. The single logo_light declaration demos one-file logos (knot
copies it to the dark slot).
"""


def status():
    import time

    import plugin.demolib as demolib

    info = demolib.status()

    return {
        "rows": [
            {
                "title": "Peer " + info.get("peer", "demolib"),
                "columns": [
                    {"id": "avg", "type": "stat", "handler": "stat_avg", "width": 1},
                    {"id": "lo", "type": "stat", "handler": "stat_lo", "width": 1},
                    {"id": "hi", "type": "stat", "handler": "stat_hi", "width": 1},
                    {"id": "samples", "type": "stat", "handler": "stat_samples", "width": 1},
                ],
            },
            {
                "columns": [
                    {"id": "latency", "type": "chart", "title": "Round trips (ms)", "handler": "col_latency", "width": 3},
                    {"id": "form", "type": "form", "title": "Re-measure", "handler": "col_form", "width": 1},
                ],
            },
            {
                "columns": [
                    {"id": "peer", "type": "table", "title": "Peer", "handler": "col_peer", "width": 2},
                    {"id": "about", "type": "markdown", "title": "About", "handler": "col_about", "width": 2},
                ],
            },
        ]
    }


def _latencies():
    import time

    import plugin.demolib as demolib

    samples = 20
    try:
        samples = int(params.get("samples", "20"))
    except Exception:
        samples = 20
    if samples < 5:
        samples = 5
    if samples > 100:
        samples = 100
    latencies = []
    for i in range(samples):
        t0 = time.perf_counter()
        demolib.greeting("knot")
        latencies.append(round((time.perf_counter() - t0) * 1000.0, 3))
    return samples, latencies


def _stats():
    samples, latencies = _latencies()
    lo = min(latencies)
    hi = max(latencies)
    total = 0.0
    for v in latencies:
        total = total + v
    avg = round(total / len(latencies), 3)
    return samples, latencies, lo, hi, avg


def stat_avg():
    _s, _l, _lo, _hi, avg = _stats()
    return {"label": "Average", "value": avg, "unit": "ms"}


def stat_lo():
    _s, _l, lo, _hi, _avg = _stats()
    return {"label": "Fastest", "value": lo, "unit": "ms"}


def stat_hi():
    _s, _l, _lo, hi, _avg = _stats()
    return {"label": "Slowest", "value": hi, "unit": "ms"}


def stat_samples():
    samples, _l, _lo, _hi, _avg = _stats()
    return {"label": "Samples", "value": samples}


def col_latency():
    samples, latencies = _latencies()
    labels = []
    for i in range(samples):
        labels.append(str(i + 1))
    return {
        "chart_type": "bar",
        "labels": labels,
        "height": 220,
        "datasets": [{"name": "Round trip (ms)", "data": latencies, "color": "#f59e0b"}],
    }


def col_form():
    if request.method == "POST":
        return {"status": "ok", "message": "Re-measured.", "refresh": True}
    return {
        "fields": [{"type": "number", "name": "samples", "label": "Samples", "value": params.get("samples", "20")}],
        "submit": "Re-measure",
    }


def col_peer():
    import time

    import plugin.demolib as demolib

    info = demolib.status()
    return {
        "columns": [
            {"key": "prop", "label": "Property"},
            {"key": "value", "label": "Value"},
        ],
        "rows": [
            {"prop": "peer", "value": info.get("peer", "?")},
            {"prop": "go", "value": info.get("go", "?")},
            {"prop": "os / arch", "value": info.get("os", "?") + " / " + info.get("arch", "?")},
            {"prop": "measured at", "value": time.now()},
        ],
    }


def col_about():
    return {
        "markdown": "Each bar is a live `plugin.demolib.greeting()` round trip measured by the handler. The peer is a Go subprocess spawned by knot at plugin load; the handler talks to it over the plugin protocol."
    }


def peer_summary():
    # One live round trip to the Go peer. Addressed from other plugins'
    # pages too: pluginFetch('peer_summary', { plugin: 'demo-go' }). The
    # [[tool.knot.handlers]] declaration opts it into plugin-root URLs; it
    # declares no permission, so any logged-in user may call it (the demo
    # button works without role grants - add a permission to the
    # declaration to tighten).
    import plugin.demolib as demolib

    info = demolib.status()
    return {
        "summary": info.get("peer", "demolib") + " (" + info.get("go", "go") + ", " + info.get("os", "?") + "/" + info.get("arch", "?") + ")"
    }
