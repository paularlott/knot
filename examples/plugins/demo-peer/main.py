# /// script
# requires-scriptling = ">=0.24"
# dependencies = [
#   "plugin.store via store >= 1.0",
# ]
#
# [tool.knot]
# version = "1.0.0"
# description = "Demo plugin whose bin/ peer is a scriptling script: knot spawns it like a binary, the shebang hands it to the scriptling CLI, and the CLI's sqlite driver backs a small persistent store. Requires the scriptling CLI on the server's PATH."
# permissions = ["use_store"]
#
# [[tool.knot.pages]]
# path = "/store"
# handler = "store_page"
# label = "Scriptling Peer Store"
# menu_label = "Peer Store"
# permission = "use_store"
# icon = "assets/icon.svg"
# ///
"""demo-peer: a scriptling-authored binary peer."""

import time


def store_page():
    return {"rows": [{"columns": [{"id": "kv", "type": "table", "title": "Key/value store (peer.db)", "handler": "col_store"}]}]}


def col_store():
    import plugin.store as store

    stamp = str(time.time())
    store.remember("last_visit", stamp)
    value = store.recall("last_visit")
    return {
        "columns": [
            {"key": "prop", "label": "Property"},
            {"key": "value", "label": "Value"},
        ],
        "rows": [
            {"prop": "peer", "value": "plugin.store (scriptling CLI, sqlite)"},
            {"prop": "add(2, 3)", "value": str(store.add(2, 3))},
            {"prop": "remembered at", "value": value},
        ],
    }
