"""A second exported module: exports may be several files, and they import
each other by their full user-side name (plugin.demo_scriptling.format)."""


def titled(word):
    """Capitalize a word — a leaf helper client.py composes."""
    return str(word).capitalize()
