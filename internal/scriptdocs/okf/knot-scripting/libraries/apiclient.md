---
type: "API Reference"
title: "knot.apiclient"
description: "Transport layer used by all knot.* libraries for HTTP and AI connectivity."
resource: "https://getknot.dev/reference/libraries/apiclient/"
tags:
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/apiclient/"
---
# knot.apiclient

The `knot.apiclient` library is the transport layer used by all `knot.*` libraries. In embedded contexts (MCP, remote, local scripts) it is provided by the Go runtime — `configure()` is a no-op and tokens are never exposed to scripts.

For standalone use outside knot, configure it explicitly or set environment variables.

---

## Functions

| Function | Description |
|----------|-------------|
| `configure(url, token, ...)` | Configure the client (no-op in embedded contexts) |
| `is_configured()` | Returns True if configured |
| `get(path, params=None, timeout=DEFAULT_TIMEOUT)` | Make a GET request |
| `post(path, body=None, timeout=DEFAULT_TIMEOUT, headers=None)` | Make a POST request, with any extra request headers (a `Content-Type` given replaces `application/json`) |
| `put(path, body=None, timeout=DEFAULT_TIMEOUT)` | Make a PUT request |
| `delete(path, headers=None, timeout=DEFAULT_TIMEOUT)` | Make a DELETE request, with any extra request headers (such as `If-Match`) |
| `get_bytes(path, timeout=CONTENT_TIMEOUT)` | Make a GET request, returning the response body as bytes (file content) |
| `put_bytes(path, data, content_type="", timeout=CONTENT_TIMEOUT)` | Make a PUT request with a string or bytes body (file content) |

`get`, `post`, `put`, `delete` and `put_bytes` return the decoded JSON response — a dict or a list — or `None` when the response has no body.

| Constant | Value | Description |
|----------|-------|-------------|
| `DEFAULT_TIMEOUT` | `30` | Default seconds `get`, `post`, `put` and `delete` wait in the standalone Python implementation |
| `CONTENT_TIMEOUT` | `600` | Default seconds `get_bytes` and `put_bytes` wait in the standalone Python implementation |

`timeout` is how many seconds a request waits for the server's answer. Standalone scripts default to `DEFAULT_TIMEOUT` (30 seconds), or `CONTENT_TIMEOUT` (600) for `get_bytes` and `put_bytes`; `timeout=None` there means no limit. Embedded scripts given no timeout (or `None`) keep the limit of the client they run with: none in the server, six minutes in a space. The `knot.*` libraries give their own timeouts to calls that wait on a space: `knot.space.run` waits for the command's timeout plus a minute, running a script in a space waits up to 7 minutes, and reading, writing and searching files in a space up to 11 minutes. Pass a `timeout` to your own calls that do the same.

---

## Configuration

### Explicit

```python
import knot.apiclient

knot.apiclient.configure(
    "https://knot.example.com",
    "your-access-token",
    insecure=False,         # optional: skip SSL verification
    ai_model="gpt-4o",      # optional: default AI model for knot.ai
    ai_provider="openai",   # optional: AI provider (default: "openai")
    ai_url="",              # optional: AI endpoint URL (default: url + "/v1")
    ai_token="",            # optional: AI token (default: same as token)
)
```

### Environment Variables

Read on first use if `configure()` has not been called. Inside a space, the agent's `/connect` endpoint (on `127.0.0.1:$KNOT_API_PORT`) is tried first; the environment variables are used only when that is unavailable:

| Variable | Description | Default |
|----------|-------------|---------|
| `KNOT_URL` | Knot server URL | required |
| `KNOT_TOKEN` | Access token | required |
| `KNOT_INSECURE` | Skip SSL verification (`true`/`1`/`yes`) | `false` |
| `KNOT_AI_URL` | AI endpoint URL | `KNOT_URL + /v1` |
| `KNOT_AI_TOKEN` | AI access token | `KNOT_TOKEN` |
| `KNOT_AI_MODEL` | Default AI model name | `""` |
| `KNOT_AI_PROVIDER` | AI provider type | `openai` |

```bash
export KNOT_URL=https://knot.example.com
export KNOT_TOKEN=your-access-token
export KNOT_AI_MODEL=gpt-4o
```

```python
import knot.space as space

# Auto-configured from env vars on first use
spaces = space.list()
```

---

## configure() Parameters

### configure(url, token, insecure=False, ai_url="", ai_token="", ai_model="", ai_provider="openai")

| Parameter | Type | Description |
|-----------|------|-------------|
| `url` | str | Knot server base URL |
| `token` | str | Access token |
| `insecure` | bool | Skip SSL certificate verification |
| `ai_url` | str | AI endpoint URL (defaults to `url + "/v1"`) |
| `ai_token` | str | AI access token (defaults to `token`) |
| `ai_model` | str | Default model for `knot.ai.get_default_model()` |
| `ai_provider` | str | Provider: `openai`, `claude`, `gemini`, `ollama`, `mistral`, `zai`, `grok` |

---

## Supported AI Providers

| Value | Provider |
|-------|----------|
| `openai` | OpenAI / OpenAI-compatible (default) |
| `claude` | Anthropic Claude |
| `gemini` | Google Gemini |
| `ollama` | Ollama (local) |
| `mistral` | Mistral AI |
| `zai` | Z.AI |
| `grok` | xAI Grok |
