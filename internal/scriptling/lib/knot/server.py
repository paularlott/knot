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


# The first label of a space's web host, <user>--<name>--<port>, is kept
# within the 63 characters DNS allows a label, as the server does.
_MAX_LABEL = 63
_HASH_LEN = 8


def _name_hash(name):
    """FNV-1a, 32 bits, of the name lowercased, in hex."""
    h = 0x811c9dc5
    for c in name.lower():
        h = h ^ ord(c)
        h = (h * 0x01000193) & 0xffffffff
    return "%08x" % h


def host_label(user, name, port):
    """The first label of the host a space's web port is reached at, before
    the wildcard domain: "<user>--<name>--<port>", lowercased.

    A label longer than the 63 characters DNS allows keeps as much of the
    name as fits, then a hyphen and a hash of the whole name, which the
    server routes by.

    Args:
        user: The username of the space's owner
        name: The space's name, one of its alt names, or a pool's name
        port: The port number, or "vnc" for web VNC

    Returns:
        The label, e.g. "alice--web-app--8080"; with the wildcard domain,
        https://alice--web-app--8080.knot.example.com
    """
    user, name, port = str(user).lower(), str(name).lower(), str(port).lower()
    full = user + "--" + name + "--" + port
    if len(full) <= _MAX_LABEL:
        return full
    keep = min(_MAX_LABEL - len(user) - len(port) - 4 - _HASH_LEN - 1, len(name))
    if keep < 1:
        keep = 1
    return user + "--" + name[:keep].rstrip("-") + "-" + _name_hash(name) + "--" + port
