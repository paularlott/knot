"""The peer's implementation: a tiny sqlite-backed key/value store.

The database file lives beside the executable, so the plugin's state
travels with its folder. The scriptling CLI carries the sqlite driver —
knot never links it.
"""

import os
import os.path
import sys

import scriptling.sqlite as sqlite


def _db():
    # sys.argv[0] is the executable's path once the server is running
    # (it is NULL at import time, so resolve lazily) — abspath because
    # knot may spawn peers through a relative plugins path.
    return os.path.join(os.path.dirname(os.path.abspath(sys.argv[0])), "store.db")


def add(a, b):
    return a + b


def remember(key, value):
    conn = sqlite.connect(_db())
    conn.execute("create table if not exists kv (k text primary key, v text)")
    conn.execute("insert or replace into kv (k, v) values (?, ?)", key, value)
    conn.close()
    return True


def recall(key):
    conn = sqlite.connect(_db())
    rows = conn.query("select v from kv where k = ?", key)
    conn.close()
    if len(rows) > 0:
        return rows[0].get("v")
    return None
