# knot.ai - AI client library for Knot server
#
# In embedded contexts (MCP, remote, local), this module is shadowed by the
# Go-registered knot.ai library. Client() returns the pre-configured server
# AI client and get_default_model() returns "".
#
# For standalone use (outside knot), knot.apiclient must be configured
# (explicitly or via env vars). AI config comes from knot.apiclient's
# ai_url, ai_token, ai_model, ai_provider kwargs or the corresponding
# env vars (KNOT_AI_URL, KNOT_AI_TOKEN, KNOT_AI_MODEL, KNOT_AI_PROVIDER).
#
# Usage:
#   import knot.apiclient
#   import knot.ai
#
#   knot.apiclient.configure("https://knot.example.com", "your-token",
#       ai_model="gpt-4o")
#   client = knot.ai.Client()
#   answer = client.ask(knot.ai.get_default_model(), "Hello!")

import knot.apiclient as _apiclient


def get_default_model():
    """Get the configured default AI model name.

    Returns the ai_model from knot.apiclient config, or "" if not set.

    Returns:
        str: The model name, or ""
    """
    client = _apiclient._get_client()
    if not client:
        return ""
    return client.get("ai_model", "")


def Client():
    """Get a configured AI client instance.

    Uses the AI connection details from knot.apiclient config
    (ai_url, ai_token, ai_provider).

    Returns:
        A scriptling.ai client instance

    Raises:
        Exception if knot.apiclient is not configured
    """
    import scriptling.ai as ai

    client = _apiclient._get_client()
    if not client:
        raise Exception("knot.apiclient not configured. Set KNOT_URL and KNOT_TOKEN or call knot.apiclient.configure().")

    return ai.Client(
        client["ai_url"],
        api_key=client["ai_token"],
        provider=client["ai_provider"],
    )
