# knot.apiclient - Core API transport library for Knot server
#
# In embedded contexts (MCP, remote, local), this module is shadowed by the
# Go-registered knot.apiclient library. configure() is a no-op and tokens
# are never exposed to scripts.
#
# For standalone use (outside knot), either call configure() explicitly or
# set environment variables which are read on first use:
#
#   KNOT_URL        - Knot server URL
#   KNOT_TOKEN      - Access token
#   KNOT_INSECURE   - Set to "true" to skip SSL verification (optional)
#   KNOT_AI_URL     - AI endpoint URL (optional, defaults to KNOT_URL + /v1)
#   KNOT_AI_TOKEN   - AI access token (optional, defaults to KNOT_TOKEN)
#   KNOT_AI_MODEL   - Default AI model name (optional)
#   KNOT_AI_PROVIDER - AI provider: openai, claude, gemini, ollama, mistral (default: openai)
#
# Usage:
#   import knot.apiclient
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#
#   # With AI config:
#   knot.apiclient.configure("https://knot.example.com", "your-token",
#       ai_model="gpt-4o", ai_provider="openai")

_client = None


def _load_from_env():
    """Try to configure from environment variables. Returns True if successful."""
    import os
    url = os.environ.get("KNOT_URL", "")
    token = os.environ.get("KNOT_TOKEN", "")
    if url and token:
        insecure = os.environ.get("KNOT_INSECURE", "").lower() in ("true", "1", "yes")
        configure(
            url, token, insecure=insecure,
            ai_url=os.environ.get("KNOT_AI_URL", ""),
            ai_token=os.environ.get("KNOT_AI_TOKEN", ""),
            ai_model=os.environ.get("KNOT_AI_MODEL", ""),
            ai_provider=os.environ.get("KNOT_AI_PROVIDER", "openai"),
        )
        return True
    return False


def _get_client():
    """Get the client config, loading from env vars if not yet configured."""
    global _client
    if _client is None:
        _load_from_env()
    return _client


def configure(url, token, insecure=False, ai_url="", ai_token="", ai_model="", ai_provider="openai"):
    """Configure the Knot API client.

    Args:
        url: The base URL of the Knot server (e.g., "https://knot.example.com")
        token: The access token for authentication
        insecure: If True, skip SSL certificate verification (default: False)
        ai_url: AI endpoint URL (default: url + "/v1")
        ai_token: AI access token (default: same as token)
        ai_model: Default AI model name (default: "")
        ai_provider: AI provider type - openai, claude, gemini, ollama, mistral (default: "openai")

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
        "insecure": insecure,
        "ai_url": ai_url.rstrip("/") if ai_url else url.rstrip("/") + "/v1",
        "ai_token": ai_token if ai_token else token,
        "ai_model": ai_model,
        "ai_provider": ai_provider,
    }

    return True


def is_configured():
    """Check if the client has been configured (explicitly or via env vars).

    Returns:
        True if configured, False otherwise
    """
    return _get_client() is not None


def get(path, params=None):
    """Make a GET request to the Knot API."""
    import requests as req

    client = _get_client()
    if not client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() or set KNOT_URL and KNOT_TOKEN.")

    url = client["url"] + path

    if params:
        query_parts = []
        for key, value in params.items():
            if value is not None:
                query_parts.append(f"{key}={value}")
        if query_parts:
            url += "?" + "&".join(query_parts)

    resp = req.get(
        url,
        headers={"Authorization": "Bearer " + client["token"], "Content-Type": "application/json", "Accept": "application/json"},
        verify=not client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    return resp.json()


def post(path, body=None):
    """Make a POST request to the Knot API."""
    import requests as req
    import json

    client = _get_client()
    if not client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() or set KNOT_URL and KNOT_TOKEN.")

    url = client["url"] + path
    headers = {"Authorization": "Bearer " + client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.post(
        url,
        headers=headers,
        data=json.dumps(body) if body else None,
        verify=not client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None


def put(path, body=None):
    """Make a PUT request to the Knot API."""
    import requests as req
    import json

    client = _get_client()
    if not client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() or set KNOT_URL and KNOT_TOKEN.")

    url = client["url"] + path
    headers = {"Authorization": "Bearer " + client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.put(
        url,
        headers=headers,
        data=json.dumps(body) if body else None,
        verify=not client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None


def delete(path):
    """Make a DELETE request to the Knot API."""
    import requests as req

    client = _get_client()
    if not client:
        raise Exception("Knot client not configured. Call knot.apiclient.configure() or set KNOT_URL and KNOT_TOKEN.")

    url = client["url"] + path
    headers = {"Authorization": "Bearer " + client["token"], "Content-Type": "application/json", "Accept": "application/json"}

    resp = req.delete(
        url,
        headers=headers,
        verify=not client["insecure"]
    )

    if resp.status_code >= 400:
        raise Exception(f"API error (HTTP {resp.status_code}): {resp.text}")

    if resp.text:
        return resp.json()
    return None
