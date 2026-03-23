# knot.api - Core API client library for Knot server
#
# This library provides the base client for connecting to a Knot server.
# All other knot.* libraries depend on this library being configured first.
#
# Usage:
#   import knot.api
#   knot.api.configure("https://knot.example.com", "your-access-token")
#
#   # Or with insecure SSL (for development):
#   knot.api.configure("https://localhost:8443", "your-token", insecure=True)

# Global client state
_client = None

def configure(url, token, insecure=False):
    """Configure the Knot API client with server URL and access token.

    Args:
        url: The base URL of the Knot server (e.g., "https://knot.example.com")
        token: The access token for authentication
        insecure: If True, skip SSL certificate verification (default: False)

    Returns:
        True if configuration was successful

    Example:
        knot.api.configure("https://knot.example.com", "my-token")
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


def get_url():
    """Get the configured server URL.

    Returns:
        The server URL string

    Raises:
        Exception if not configured
    """
    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")
    return _client["url"]


def get_token():
    """Get the configured access token.

    Returns:
        The access token string

    Raises:
        Exception if not configured
    """
    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")
    return _client["token"]


def _get_headers():
    """Get the HTTP headers for API requests."""
    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

    return {
        "Authorization": "Bearer " + _client["token"],
        "Content-Type": "application/msgpack",
        "Accept": "application/msgpack"
    }


def _get_headers_json():
    """Get the HTTP headers for JSON API requests."""
    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

    return {
        "Authorization": "Bearer " + _client["token"],
        "Content-Type": "application/json",
        "Accept": "application/json"
    }


def get(path, params=None):
    """Make a GET request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces")
        params: Optional dict of query parameters

    Returns:
        The response data as a dict or list

    Raises:
        Exception on HTTP error
    """
    import requests as req

    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

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
        headers=_get_headers_json(),
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

    Raises:
        Exception on HTTP error
    """
    import requests as req
    import json

    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

    url = _client["url"] + path

    resp = req.post(
        url,
        headers=_get_headers_json(),
        data=json.dumps(body) if body else None,
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    # Some endpoints return empty response
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

    Raises:
        Exception on HTTP error
    """
    import requests as req
    import json

    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

    url = _client["url"] + path

    resp = req.put(
        url,
        headers=_get_headers_json(),
        data=json.dumps(body) if body else None,
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    # Some endpoints return empty response
    if resp.text:
        return resp.json()
    return None


def delete(path):
    """Make a DELETE request to the Knot API.

    Args:
        path: The API path (e.g., "/api/spaces/123")

    Returns:
        The response data as a dict or list

    Raises:
        Exception on HTTP error
    """
    import requests as req

    if not _client:
        raise Exception("Knot client not configured. Call knot.api.configure() first.")

    url = _client["url"] + path

    resp = req.delete(
        url,
        headers=_get_headers_json(),
        verify=not _client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    # Some endpoints return empty response
    if resp.text:
        return resp.json()
    return None
