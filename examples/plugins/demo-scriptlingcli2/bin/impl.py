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




# The plugin's declared assets, served from the fetcher (bin/notes
# registers impl.fetch_read): the scriptling equivalent of a Go peer's
# go:embed — the peer carries its own icon and action icons, so the plugin
# folder ships no assets/ at all. Paths are exactly what the manifest
# declares; None answers a miss and knot falls back to disk.
ASSETS = {
    'assets/icon.svg': '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 0 1 3 19.875v-6.75ZM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V8.625ZM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V4.125Z"/></svg>',
    'assets/view.svg': '<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M2.036 12.322a1.012 1.012 0 0 1 0-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178Z" /><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" /></svg>',
    'assets/delete.svg': '<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0" /></svg>',
}


def fetch_read(source, path):
    return ASSETS.get(path)

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
