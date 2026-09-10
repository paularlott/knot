# knot.token - API token management library for the Knot server
#
# Tokens carry the creating user's permissions, optionally narrowed by
# scopes: a token scoped to "methods", "mcp" and/or "tunnels" can reach
# only those endpoint groups; no scopes means full access.

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def list():
    """List the current user's API tokens.

    Returns:
        A list of dicts, each containing:
        - id: The token value (the bearer key itself)
        - name: The token's name
        - expires_after: Expiry timestamp — any use resets the two week clock
        - scopes: The token's scope list, empty for full access

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/tokens")

    result = []
    for token in response or []:
        result.append({
            "id": token.get("token_id"),
            "name": token.get("name"),
            "expires_after": token.get("expires_after"),
            "scopes": token.get("scopes") or [],
        })

    return result


def create(name, scopes=None):
    """Create an API token for the current user and return its value.

    Args:
        name: Name identifying the token
        scopes: Optional list narrowing the token to endpoint groups —
            "methods" (/api/methods*), "mcp" (/mcp) and "tunnels"
            (/tunnel/* and /api/tunnels*: create, list and delete tunnels
            only). None or empty means full access. A tunnels-only key is
            what a machine that should do nothing but expose a port wants.

    Returns:
        The new token's value (the bearer key) — pass it as the token to
        clients, e.g. knot.space.tunnel_start(..., token=...) or a
        pipeline's knot tunnel --token.

    Raises:
        Exception if not configured or on API error
    """
    body = {"name": name}
    if scopes:
        body["scopes"] = scopes

    response = api.post("/api/tokens", body)
    return response.get("token_id")


def delete(token_id):
    """Delete an API token by id (its value), revoking it immediately.

    Args:
        token_id: The token's value, from create or the listing above

    Returns:
        True if successful

    Raises:
        Exception if not configured or on API error
    """
    api.delete(f"/api/tokens/{_enc(token_id)}")
    return True
