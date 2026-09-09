"""The peer's implementation: a small sqlite-backed notes store.

The database file lives beside the executable, so the plugin's state travels
with its folder. The scriptling CLI carries the sqlite driver — knot never
links it. Every function here is served over the plugin protocol and
addressed by knot as plugin.notes.<fn>."""

import os
import os.path
import sys
import time

import scriptling.sqlite as sqlite


def _db():
    # sys.argv[0] is the executable's path once the server is running
    # (it is NULL at import time, so resolve lazily) — abspath because
    # knot may spawn peers through a relative plugins path.
    return os.path.join(os.path.dirname(os.path.abspath(sys.argv[0])), "notes.db")


def _connect():
    conn = sqlite.connect(_db())
    conn.execute("create table if not exists notes (id integer primary key autoincrement, text text, created_at text)")
    return conn


def notes_page(request):
    return {
        "rows": [
            {
                "title": "Notes",
                "columns": [
                    {"id": "add", "type": "form", "title": "Add a note", "handler": "col_add", "width": 1},
                    {
                        "id": "notes",
                        "type": "table",
                        "title": "Saved notes (notes.db, beside the peer)",
                        "handler": "col_notes",
                        "width": 3,
                        # The fetch-time gate serves an undeclared handler only
                        # if the layout names it: a column-level action with a
                        # handler puts note_view in that set. The rows' own
                        # actions lists replace the column's at render time.
                        # Action icons are the plugin's own SVG assets,
                        # declared in [tool.knot] icons.
                        "actions": [
                            {"action": "view", "label": "View note", "icon": "assets/view.svg", "handler": "note_view"},
                        ],
                    },
                ],
            }
        ]
    }


def col_add(request):
    if request["method"] == "POST":
        text = str(request["params"].get("text", "")).strip()
        if text == "":
            return {"status": "error", "message": "A note needs some text."}
        conn = _connect()
        conn.execute("insert into notes (text, created_at) values (?, ?)", text, str(time.now()))
        conn.close()
        return {"status": "ok", "message": "Note added.", "refresh": ["notes"]}

    return {
        "fields": [
            {"type": "textarea", "name": "text", "label": "Note", "language": "markdown", "rows": 6, "placeholder": "something to remember"},
        ],
        "submit": "Add",
    }


def note_view(request):
    # Popup handler (an action with a handler GETs it with the row key):
    # markdown is rendered server-side, so the popup is an information view.
    key = request["params"].get("key", "")
    conn = _connect()
    rows = conn.query("select text, created_at from notes where id = ?", key)
    conn.close()
    if len(rows) == 0:
        return {"title": "Note", "markdown": "_this note no longer exists_"}
    note = rows[0]
    return {
        "title": "Note",
        "markdown": "> added " + str(note.get("created_at", "")) + "\n\n" + str(note.get("text", "")),
    }


def col_notes(request):
    if request["method"] == "POST":
        # Row actions arrive with the action name and the row key.
        if request["params"].get("action", "") == "delete":
            key = request["params"].get("key", "")
            conn = _connect()
            conn.execute("delete from notes where id = ?", key)
            conn.close()
            return {"status": "ok", "message": "Note deleted.", "refresh": ["notes"]}
        return {"status": "error", "message": "Unknown action."}

    conn = _connect()
    stored = conn.query("select id, text, created_at from notes order by id desc")
    conn.close()

    rows = []
    for note in stored:
        rows.append(
            {
                "id": str(note.get("id", "")),
                "note": note.get("text", ""),
                "added": note.get("created_at", ""),
                "actions": [
                    {"action": "view", "label": "View note", "icon": "assets/view.svg", "handler": "note_view"},
                    {"action": "delete", "label": "Delete", "icon": "assets/delete.svg", "style": "danger", "confirm": "Delete this note?"},
                ],
            }
        )
    if len(rows) == 0:
        # An explicit empty actions list: a row without one falls back to the
        # column-level set (the gate entry), which would paint a view button
        # on a row that has nothing to view.
        rows = [{"note": "(no notes yet)", "added": "", "actions": []}]

    return {
        "columns": [
            {"key": "note", "label": "Note"},
            {"key": "added", "label": "Added"},
        ],
        "rows": rows,
    }
