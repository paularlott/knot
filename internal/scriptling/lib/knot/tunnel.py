# knot.tunnel - Tunnel management library for Knot server
#
# Tunnels expose a local port on the public tunnel domain as
# https://<user>--<name><tunnel_domain>. This library lists and closes the
# current user's tunnels; tunnels a space runs are started and stopped with
# knot.space.tunnel_start / tunnel_stop. Needs the use tunnels permission.
# Requires knot.apiclient to be configured first.
#
# Usage:
#   import knot.tunnel
#
#   for t in knot.tunnel.list():
#       print(t["name"], t["address"])

import knot.apiclient as api
import urllib.parse


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def list():
    """List the current user's open tunnels, from every client and space.

    Returns:
        A list of dicts sorted by name, each containing:
        - name: Full tunnel name, "<username>--<tunnel>"
        - address: Public URL, e.g. "https://alice--web.tunnel.example.com"

    Raises:
        Exception if you lack the use tunnels permission, or on API error
    """
    response = api.get("/api/tunnels")
    return [{"name": t.get("name", ""), "address": t.get("address", "")} for t in response or []]


def server_info():
    """Get the tunnel server settings clients connect with.

    Returns:
        A dict containing:
        - domain: Tunnel domain suffix, e.g. ".tunnel.example.com"
        - tunnel_servers: List of tunnel server URLs

    Raises:
        Exception if you lack the use tunnels permission, or on API error
    """
    response = api.get("/api/tunnels/server-info")
    return {
        "domain": response.get("domain", ""),
        "tunnel_servers": response.get("tunnel_servers") or [],
    }


def delete(name):
    """Close one of the current user's tunnels, disconnecting its client.

    Args:
        name: Tunnel name, either the full "<username>--<tunnel>" that list returns
            or the bare tunnel name

    Returns:
        True if successful

    Raises:
        Exception if the tunnel is not found, or on API error
    """
    name = str(name)
    if "--" not in name:
        name = api.get("/api/users/whoami").get("username", "") + "--" + name
    api.delete(f"/api/tunnels/{_enc(name)}")
    return True
