# knot.apiclient - Plugin transport variant
#
# This module replaces the HTTP-based knot.apiclient when the knot plugin
# serves it. API calls route through the plugin process (which holds the
# token), so scripts never see credentials. The public surface is identical
# to the HTTP version, so every other knot.* module works unchanged.

import base64

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


def get_bytes(path):
    """Make a GET request via the plugin, returning the response body as bytes.

    Bytes do not cross the plugin wire, so the body travels as Base64 and is
    decoded here.
    """
    encoded = _plugin.call_function("plugin.knot", "api_get_bytes", path)
    return base64.b64decode(encoded)


def put_bytes(path, data, content_type=""):
    """Make a PUT request with a str or bytes body via the plugin.

    The body is UTF-8 encoded and travels the plugin wire as Base64, which
    has no bytes value: it is passed as a string (b64encode returns bytes
    on newer scriptling).
    """
    if isinstance(data, str):
        data = data.encode("utf-8")
    encoded = base64.b64encode(data)
    if not isinstance(encoded, str):
        encoded = encoded.decode()
    return _plugin.call_function("plugin.knot", "api_put_bytes", path, encoded, content_type=content_type)
