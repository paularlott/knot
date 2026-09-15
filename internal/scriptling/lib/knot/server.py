# knot.server - Server information library for Knot server

import knot.apiclient as api


def info():
    """Get server-wide information.

    Returns a dict with:
    - wildcard_domain - the server's wildcard domain for space web-port URLs
      (e.g. "*.knot.example.com"); empty when none is configured.
    - tunnel_domain - the server's tunnel domain as a dot-prefixed suffix
      (e.g. ".tunnel.example.com") that appends straight after a tunnel
      name; empty when none is configured.
    """
    response = api.get("/api/server-info")
    return {
        "version": response.get("version", ""),
        "wildcard_domain": response.get("wildcard_domain", ""),
        "tunnel_domain": response.get("tunnel_domain", "")
    }
