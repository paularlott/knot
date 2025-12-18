# MCP Remote Servers

Knot's MCP server can connect to external MCP servers to expose their tools alongside Knot's native tools. This provides a unified interface for accessing tools from multiple MCP servers.

## Configuration

Remote MCP servers are configured in the `knot.toml` configuration file under the `[server.mcp]` section:

```toml
[server.mcp]
  enabled = true

  [[server.mcp.remote_servers]]
    namespace = "ai"
    url = "https://ai.example.com/mcp"
    token = "your-bearer-token"
    hidden = false  # Set to true to hide tools from ListTools

  [[server.mcp.remote_servers]]
    namespace = "data"
    url = "https://data.example.com/mcp"
    token = "your-bearer-token"
    hidden = false

  [[server.mcp.remote_servers]]
    namespace = "internal"
    url = "https://internal.example.com/mcp"
    token = "your-internal-bearer-token"
    hidden = true  # Tools callable but not listed
```

### Configuration Fields

- **namespace**: The namespace prefix for tools from this server (e.g., tools will appear as `ai/generate-text`)
- **url**: The full URL of the remote MCP server endpoint
- **token**: Bearer token for authentication
- **hidden**: Optional boolean (default: false). When true, tools are callable but not listed in `ListTools` responses. Useful for internal tools that should only be used from scripts.

## How It Works

When the Knot server starts, it:

1. Reads the remote server configuration
2. Creates a Bearer token authenticator for each remote server
3. Registers each remote server with the local MCP server
4. Exposes all tools (local + remote) through a unified interface

### Tool Namespacing

Tools from remote servers are prefixed with their namespace to avoid conflicts:

- Local tools: `list_spaces`, `create_template`, etc.
- Remote tools: `ai/generate-text`, `data/query`, etc.

### Hidden Tools

Remote servers can be configured with `hidden = true` to make their tools callable but not visible in tool listings. This is useful for:

- **Internal/utility tools**: Tools that should only be called from scripts, not directly by AI
- **Reducing context**: Keeping tool lists concise while still allowing script access
- **Security**: Hiding sensitive internal APIs from external visibility

Hidden tools can still be called using `mcp.call_tool()` in scripts, but won't appear in `mcp.list_tools()` responses.

### Authentication

Remote servers use Bearer token authentication. The token is configured in the TOML file and sent with each request to the remote server.

## Usage Examples

### In Scriptling Scripts

```python
import mcp

# List all available tools (including remote ones)
tools = mcp.list_tools()
for tool in tools:
    print(f"Tool: {tool['name']}")
    # Tools will include both local (list_spaces) and remote (ai/generate-text)

# Call a remote tool directly
response = mcp.call_tool("ai/generate-text", {
    "prompt": "Write a Python function",
    "max_tokens": 100
})
print(response)

# Call a hidden tool (not listed but callable)
response = mcp.call_tool("internal/process-data", {
    "data_id": "12345"
})
print(response)

# Or let AI discover and use tools automatically
messages = [
    {"role": "user", "content": "Generate a Python function and save it to a file in my dev space"}
]
response = ai.completion(messages)
# AI will automatically use both remote (ai/generate-text) and local (write_file) tools
```

### In MCP Clients

When connecting to Knot's MCP server, all tools (local and remote) are available through the standard ListTools and CallTool methods. The MCP server handles routing requests to the appropriate server.

## Security Considerations

1. **Token Security**: Store bearer tokens securely in the configuration file with appropriate file permissions
2. **Network Security**: Ensure remote servers use HTTPS to protect tokens in transit
3. **Access Control**: The Knot server doesn't enforce permissions on remote tools - the remote server is responsible for authorization

## Troubleshooting

### Common Issues

1. **Connection Failed**: Check that the remote server URL is accessible and correct
2. **Authentication Failed**: Verify the bearer token is valid and not expired
3. **Tools Not Appearing**: Check the remote server is running and properly configured

### Debug Logs

Enable debug logging to see information about remote server connections:

```bash
knot server --log-level debug
```

You'll see logs like:
- `Registering remote MCP server: ai-tools (namespace: ai)`
- `Successfully connected to remote MCP server: ai-tools`
- `Failed to register remote MCP server: data-services - authentication failed`