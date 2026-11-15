# knot.server - Server information library for Knot server

import knot.apiclient as api
import urllib.parse


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


def ping():
    """Check the server is reachable and the credentials work.

    Returns:
        A dict containing:
        - status: True
        - version: The server's knot version
        - zone: The zone of the server that answered

    Raises:
        Exception if the server cannot be reached or refuses the credentials
    """
    response = api.get("/api/ping")
    return {
        "status": response.get("status", False),
        "version": response.get("version", ""),
        "zone": response.get("zone", ""),
    }


def cluster_info():
    """List the servers of the cluster. Needs the view cluster info permission.

    Returns:
        A list of dicts sorted by address, one per server node, each containing:
        - id: Node ID
        - address: Advertised cluster address
        - state: Gossip state, e.g. "alive", "suspect", "dead"
        - zone: The node's zone
        - hostname: The node's hostname
        - allocated_spaces: Number of spaces assigned to the node
        - running_spaces: Number of those spaces deployed
        - runtimes: Container runtimes the node offers, e.g. ["docker"]
          (only known for the node that answered)
        - metadata: All gossip metadata of the node, as strings

    Raises:
        Exception if you lack the permission, or on API error
    """
    response = api.get("/api/cluster-info")
    result = []
    for node in response or []:
        meta = node.get("metadata") or {}
        runtimes = meta.get("runtimes", "")
        result.append({
            "id": node.get("id", ""),
            "address": node.get("address", ""),
            "state": node.get("state", ""),
            "zone": meta.get("zone", ""),
            "hostname": meta.get("hostname", ""),
            "allocated_spaces": int(meta.get("allocated_spaces", "0") or 0),
            "running_spaces": int(meta.get("running_spaces", "0") or 0),
            "runtimes": runtimes.split(",") if runtimes else [],
            "metadata": meta,
        })
    return result


def search(query):
    """Search everything the current user may see by name, as the web UI's
    command palette does.

    Matching is a case-insensitive substring match on each entity's name;
    each group holds at most 5 hits. Admin entities appear only with the
    matching manage permission; personal ones (scripts, skills, commands,
    MCP servers, tokens) are your own plus global ones.

    Args:
        query: Text to look for

    Returns:
        A dict containing the query and one list per entity type: pages,
        spaces, templates, variables, volumes, stacks, scripts, skills,
        commands, mcp_servers, event_sinks, users, groups, roles and tokens.
        Each list holds dicts with id, name and description (description
        may be absent); a type with no hits has an empty list.

    Raises:
        Exception if not configured or on API error
    """
    response = api.get("/api/search?q=" + urllib.parse.quote(str(query), safe=''))
    result = {"query": response.get("query", query)}
    for group in ("pages", "spaces", "templates", "variables", "volumes", "stacks",
                  "scripts", "skills", "commands", "mcp_servers", "event_sinks",
                  "users", "groups", "roles", "tokens"):
        result[group] = response.get(group) or []
    return result


def plugins():
    """List the loaded server plugins and what each provides. Needs the
    manage roles permission; raises when no plugins are loaded (the endpoint
    exists only while plugins are present).

    Returns:
        A dict containing:
        - count: Number of loaded plugins
        - plugins: List of dicts, each with name, version, description,
          permissions (list of {id, label}), menus, pages, peers, mcp_tools
          (list of {name, description, handler, permission, parameters}),
          handlers (list of {handler, permission}), field_handlers (list of
          {id, label, permission}) and libs (list of {name, version})
        - failed: Plugins that did not load, each {name, path, reason}
        - warnings: Load warnings, as strings

    Raises:
        Exception if no plugins are loaded, you lack the permission, or on API error
    """
    response = api.get("/api/plugins")
    return {
        "count": response.get("count", 0),
        "plugins": response.get("plugins") or [],
        "failed": response.get("failed") or [],
        "warnings": response.get("warnings") or [],
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
