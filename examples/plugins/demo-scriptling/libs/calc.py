# /// script
# [tool.knot.lib]
# version = "1.0"
# ///

"""An in-process scriptling library: the plugin publishes this module for
reuse. This plugin's handlers import it as plugin.calc, and so may OTHER
installed plugins — installed plugins share one trust domain, so a plain
import is ungated cross-plugin composition. (User-created MCP tools cannot
import it: the plugin pool is not attached to their environment; they reach
a plugin only via knot.plugin.call.) Because a composing plugin imports it
ungated, an export that needs a permission carries its own check — see
gated_report. This file is the scriptling twin of demo-go's Go peer:
constant, functions, stateful class.
"""

MAX = 100


def add(a, b):
    """Add two numbers."""
    return a + b


def scale(value, by):
    """Scale a number, clamped to MAX."""
    result = value * by
    if result > MAX:
        return MAX
    return result


class Counter:
    """A stateful counter — the twin of demo-go's Go Counter class."""

    def __init__(self, step):
        self.step = step
        self.n = 0

    def next(self):
        self.n = self.n + self.step
        return self.n

    def value(self):
        return self.n


def gated_report():
    # A library is a module: it never receives a handler's `request`, so the
    # permission decision is made through knot.identity (the authoritative,
    # gated surface). A composing plugin imports this ungated, so the gate
    # has to live here. Note the grant uses the folder name (demo-scriptling)
    # — permissions qualify by folder, while the import namespace sanitises
    # the hyphen to plugin.demo_scriptling.
    import knot.identity

    user = knot.identity.user()
    if not user.has_permission("plugin.demo-scriptling.view_dashboard"):
        raise Exception("view_dashboard not granted")
    return {"for": user.name, "max": MAX}
