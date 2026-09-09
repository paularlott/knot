"""demo-scriptling's exported MCP client modules ([tool.knot] export).

Materialized in user tool environments as plugin.demo_scriptling (this
module, the first declared) and plugin.demo_scriptling.format (the second):
user tools `import plugin.demo_scriptling as demo` and get this surface.
It is a CLIENT — the classes run in the caller's environment with the
caller's authority, and every method is a thin knot.plugin.call over the
gated loopback, exactly like a hand-written call. Only handlers the plugin
DECLARED ([[tool.knot.handlers]]) are reachable this way; the gates apply
per call.

Exported modules import each other by their full user-side name (see the
format import below) and must otherwise be self-contained: sibling plugin
modules and peers do not exist on this side.
"""

import knot.plugin

import plugin.demo_scriptling.format as fmt

PLUGIN = "demo-scriptling"


class Widgets:
    """The plugin's declared handler surface as a friendly object."""

    def echo(self, word):
        """echo(word) - upper-case a word and count its letters."""
        return knot.plugin.call(PLUGIN, "echo_word", {"word": word})

    def shout(self, word):
        """shout(word) - echo, client-side formatted via the second module."""
        reply = knot.plugin.call(PLUGIN, "echo_word", {"word": fmt.titled(word)})
        return reply

    def notes(self, key):
        """notes(key) - the information-popup markdown for a widget."""
        return knot.plugin.call(PLUGIN, "widget_notes", {"key": key})

    def rename(self, key, name):
        """rename(key, name) - POST the edit form for a widget."""
        return knot.plugin.call(PLUGIN, "widget_edit", {"key": key, "name": name}, method="POST")


def echo(word):
    """Module-level convenience mirroring Widgets.echo."""
    return knot.plugin.call(PLUGIN, "echo_word", {"word": word})
