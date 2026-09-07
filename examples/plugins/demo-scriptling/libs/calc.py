# /// script
# [tool.knot.lib]
# version = "1.0"
# ///

"""An in-process scriptling peer: the plugin publishes this module for
reuse. Handlers import it as plugin.calc; the same import works in
user-created MCP tools, where no metadata gate applies — so the exported
code that needs a gate carries it (see gated_report). This file is the
scriptling twin of demo-go's Go peer: constant, functions, stateful class.
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
    # Module code can't see the `user` global — its scope is the calling
    # program — so the identity library carries the same instance, and the
    # permission decision is the plugin's own.
    import knot.identity

    user = knot.identity.user()
    if not user.has_permission("plugin.demo-scriptling.view_dashboard"):
        raise Exception("view_dashboard not granted")
    return {"for": user.name, "max": MAX}
