# /// script
# requires-scriptling = ">=0.24"
# dependencies = [
#   "plugin.kvstore via kvstore >= 1.0",
# ]
#
# [tool.knot]
# version = "1.0.0"
# description = "Demo plugin whose bin/ peer is the standalone scriptling CLI: knot spawns the script like a binary, the shebang hands it to the scriptling CLI (--plugin), and the CLI's sqlite driver backs a small persistent key/value store. This plugin keeps a main.py for its scriptling page handlers, which orchestrate the peer as plugin.kvstore. Requires the scriptling CLI on the server's PATH."
# permissions = ["use_store"]
#
# [[tool.knot.pages]]
# path = "/store"
# handler = "store_page"
# label = "Scriptling CLI KVStore"
# menu_label = "KVStore"
# permission = "use_store"
# icon = "assets/icon.svg"
# ///
"""demo-scriptlingcli: a plugin whose bin/ peer is the standalone scriptling CLI.

It shows the second peer flavour: instead of a compiled Go binary, the peer
is a scriptling script run by the scriptling CLI (via the shebang). knot
spawns and handshakes it exactly like any peer, and it registers as
plugin.kvstore. This plugin also keeps a main.py — its page handlers are
scriptling, addressed as plugin.demo_scriptlingcli.<fn>, and they compose
the peer's plugin.kvstore surface. Handlers receive the request argument;
knot.identity is the authoritative identity surface.
"""

import time


def store_page(request):
    return {"rows": [{"columns": [{"id": "kv", "type": "table", "title": "Key/value store (kvstore.db)", "handler": "col_store"}]}]}


def col_store(request):
    import plugin.kvstore as kvstore

    kvstore.remember("last_visit", str(time.now()))
    value = kvstore.recall("last_visit")
    return {
        "columns": [
            {"key": "prop", "label": "Property"},
            {"key": "value", "label": "Value"},
        ],
        "rows": [
            {"prop": "peer", "value": "plugin.kvstore (scriptling CLI, sqlite)"},
            {"prop": "add(2, 3)", "value": str(kvstore.add(2, 3))},
            {"prop": "remembered at", "value": value},
        ],
    }
