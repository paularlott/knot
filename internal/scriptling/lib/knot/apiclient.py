# knot.apiclient - Core API transport library for Knot server
#
# In embedded contexts (MCP, remote, local), this module is shadowed by the
# Go-registered knot.apiclient library. configure() is a no-op and tokens
# are never exposed to scripts.
#
# For standalone use (outside knot), this module uses the requests library.
# Call configure(url, token) before using any knot.* library.
#
# Usage:
#   import knot.apiclient
#   knot.apiclient.configure("https://knot.example.com", "your-access-token")

_client = None


def configure(url, token, insecure=False):
    """Configure the Knot API client with server URL and access token.

    Args:
        url: The base URL of the Knot server (e.g., "https://knot.example.com")
        token: The access token for authentication
        insecure: If True, skip SSL certificate verification (default: False)

    Returns:
        True if configuration was successful
    """
    global _client

    if not url:
        raise Exception("URL is required")

    if not token:
        raise Exception("Token is required")

    _client = {
        "url": url.rstrip("/"),
        "token": token,
        "insecure": insecure
    }

    return True


def is_configured():
    """Check if the client has been configured.

    Returns:
        True if configured, False otherwise
    """
    return _client is not None


def get(path, params=None):
    """Make a GET request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces")
        params: Optional dict of query parameters

    Returns:
        The response data as a dict or list
    """
    import requests as req

    if not _client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() first.")

    url = _client["url"] + path

    if params:
        query_parts = []
        for key, value in params.items():
            if value is not None:
                query_parts.append(f"{key}={value}")
        if query_parts:
            url += "?" + "&".join(query_parts)

    resp = req.get(
        url,
        headers={"Authorization": "Bearer " + _client["token"], "Content-Type": "application/json", "Accept": "application/json"},
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    return resp.json()


def post(path, body=None):
    """Make a POST request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces")
        body: Optional request body (will be serialized to JSON)

    Returns:
        The response data as a dict or list
    """
    import requests as req
    import json

    if not _client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() first.")

    url = _client["url"] + path
    headers = {"Authorization": "Bearer " + _client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.post(
        url,
        headers=headers,
        data=json.dumps(body) if body else None,
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None


def put(path, body=None):
    """Make a PUT request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces/123")
        body: Optional request body (will be serialized to JSON)

    Returns:
        The response data as a dict or list
    """
    import requests as req
    import json

    if not _client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() first.")

    url = _client["url"] + path
    headers = {"Authorization": "Bearer " + _client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.put(
        url,
        headers=headers,
        data=json.dumps(body) if body else None,
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None


def delete(path):
    """Make a DELETE request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces/123")

    Returns:
        The response data as a dict or list
    """
    import requests as req

    if not _client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() first.")

    url = _client["url"] + path
    headers = {"Authorization": "Bearer " + _client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.delete(
        url,
        headers=headers,
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None
