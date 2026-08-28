# knot.apiclient - Plugin transport variant
#
# This module replaces the HTTP-based knot.apiclient when the knot plugin
# serves it. API calls route through the plugin process (which holds the
# token), so scripts never see credentials. The public surface is identical
# to the HTTP version, so every other knot.* module works unchanged.

import scriptling.plugin as _plugin

_client = None


def _get_client():
    """Return the connection info from the plugin process."""
    global _client
    if _client is None:
        _client = _plugin.call_function("plugin.knot", "connection_info")
    return _client


def configure(url, token, insecure=False, ai_url="", ai_token="", ai_model="", ai_provider="openai"):
    """No-op: the plugin process owns the connection."""
    return True


def is_configured():
    """Always true: the plugin process holds a valid connection."""
    return True


def get(path, params=None):
    """Make a GET request to the Knot API via the plugin."""
    return _plugin.call_function("plugin.knot", "api_get", path, params=params)


def post(path, body=None):
    """Make a POST request to the Knot API via the plugin."""
    return _plugin.call_function("plugin.knot", "api_post", path, body=body)


def put(path, body=None):
    """Make a PUT request to the Knot API via the plugin."""
    return _plugin.call_function("plugin.knot", "api_put", path, body=body)


def delete(path):
    """Make a DELETE request to the Knot API via the plugin."""
    return _plugin.call_function("plugin.knot", "api_delete", path)
