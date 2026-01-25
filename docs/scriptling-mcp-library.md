# Scriptling MCP Library

The `knot.mcp` library provides MCP (Model Context Protocol) functionality for scriptling scripts. This library uses a unified API across all environments.

## Available Functions

| Function | Description |
|----------|-------------|
| `get(name[, default])` | Get MCP parameter value with automatic type conversion |
| `return_string(value)` | Return a string result |
| `return_object(value)` | Return a structured object as JSON |
| `return_toon(value)` | Return a value encoded as toon |
| `return_error(message)` | Return an error message |
| `list_tools()` | Get a list of all available MCP tools and their parameters |
| `call_tool(name, arguments)` | Call an MCP tool directly |
| `tool_search(query, max_results=10)` | Search for tools by keyword |
| `execute_tool(name, arguments)` | Execute a discovered tool |

---

## Parameter Access Functions

These functions are only available when a script is executed as an MCP tool (when mcpParams is provided).

### get(name[, default])

Get a parameter value with automatic type conversion.

**Parameters:**

- `name` (string): The parameter name
- `default` (any, optional): Default value if parameter is not provided

**Returns:**

- The parameter value with automatic type conversion (string, number, boolean, list, or dict)
- Returns `default` if provided and parameter is missing
- Returns `None` if parameter is missing and no default provided

**Example:**

```python
import knot.mcp

# String parameter
name = knot.mcp.get("name")
name = knot.mcp.get("name", "default")

# Number (auto-converted)
count = knot.mcp.get("count", 0)

# Boolean (auto-converted)
enabled = knot.mcp.get("enabled", False)

# Array (auto-parsed from JSON)
items = knot.mcp.get("items", [])

# Object (auto-parsed from JSON)
config = knot.mcp.get("config", {})
```

---

### return_string(value)

Return a string result from the MCP tool. The script should exit after calling this.

**Parameters:**

- `value` (string): The string value to return

**Example:**

```python
import knot.mcp

result = "Operation completed successfully"
return knot.mcp.return_string(result)
```

---

### return_object(value)

Return a structured object (automatically converted to JSON). The script should exit after calling this.

**Parameters:**

- `value` (dict or list): The object to return

**Example:**

```python
import knot.mcp

result = {
    "status": "success",
    "records_processed": 42,
    "duration_ms": 1234
}
return knot.mcp.return_object(result)
```

---

### return_toon(value)

Return a value encoded as toon (a compact serialization format). The script should exit after calling this.

**Parameters:**

- `value` (any): The value to encode and return

**Example:**

```python
import knot.mcp

result = {
    "status": "success",
    "data": [1, 2, 3, 4, 5]
}
return knot.mcp.return_toon(result)
```

---

### return_error(message)

Return an error message. The script should exit after calling this.

**Parameters:**

- `message` (string): The error message

**Example:**

```python
import knot.mcp

if not url:
    return knot.mcp.return_error("URL parameter is required")
```

---

## Tool Access Functions

These functions are available in all scriptling environments for programmatically accessing MCP tools.

### list_tools()

Get a list of all available MCP tools and their parameters, including tools from remote MCP servers if configured.

**Parameters:** None

**Returns:**

- `list`: List of tool objects, each containing:
  - `name` (string): The tool's name (remote tools have namespace prefix like `ai/generate-text`)
  - `description` (string): Description of what the tool does
  - `parameters` (object): JSON Schema describing the tool's parameters

**Example:**

```python
import knot.mcp

# Get all available tools
tools = knot.mcp.list_tools()

# Print tool information
for tool in tools:
    print(f"Tool: {tool['name']}")
    print(f"Description: {tool['description']}")
    print(f"Parameters: {tool['parameters']}")
    print("---")

# Separate local and remote tools
local_tools = []
remote_tools = []

for tool in tools:
    if '/' in tool['name']:
        parts = tool['name'].split('/', 1)
        remote_tools.append((parts[0], parts[1], tool['description']))
    else:
        local_tools.append((tool['name'], tool['description']))

print(f"Local tools: {len(local_tools)}")
print(f"Remote tools: {len(remote_tools)}")
```

---

### call_tool(name, arguments)

Call an MCP tool directly. This is the low-level function for tool execution.

**Important:** The MCP server uses a discovery pattern. Only `tool_search` and `execute_tool` are directly callable. Other tools must first be discovered using `tool_search`, then executed using `execute_tool`. Consider using `knot.mcp.tool_search()` and `knot.mcp.execute_tool()` helper functions instead.

**Parameters:**

- `name` (string): Name of the tool to call
- `arguments` (dict): Arguments to pass to the tool

**Returns:**

- `any`: The tool's response content, automatically decoded:
  - **Single text response**: Returns as a string
  - **JSON in text**: Automatically parsed and returned as objects (dict/list)
  - **Multiple content blocks**: Returns as a list of decoded blocks
  - **Image/Resource blocks**: Returns as a dict with `Type`, `Data`, `MimeType`, etc.

**Example:**

```python
import knot.mcp

# Search for space-related tools
tool_search_results = knot.mcp.call_tool("tool_search", {
    "query": "list spaces"
})
print("Tool search results:", tool_search_results)

# Execute a tool found through search
space_results = knot.mcp.call_tool("execute_tool", {
    "name": "list_spaces",
    "arguments": {}
})
print("Spaces:", space_results)

# Call a remote tool directly (if configured)
ai_response = knot.mcp.call_tool("ai/generate-text", {
    "prompt": "Write a Python hello world function",
    "max_tokens": 50
})
print(ai_response)

# Response decoding examples:
# 1. Text responses are returned as strings
text_result = knot.mcp.call_tool("some_tool", {})
print(text_result)  # "Hello World" (not [{"Type": "text", "Text": "Hello World"}])

# 2. JSON in text is automatically parsed
json_result = knot.mcp.call_tool("json_tool", {})
print(json_result)  # {"status": "ok", "count": 5} - already a dict!
print(json_result["status"])  # "ok"

# 3. Multiple content blocks are returned as a list
multi_result = knot.mcp.call_tool("multi_tool", {})
for block in multi_result:
    print(block)  # Each decoded block
```

---

### tool_search(query, max_results=10)

Search for tools by keyword. This is a helper function that wraps `call_tool("tool_search", ...)` for convenience.

**Parameters:**

- `query` (string): Search query to find matching tools
- `max_results` (int, optional): Maximum number of results to return (default: 10)

**Returns:**

- `list`: Array of tool dictionaries (same format as `list_tools`)

**Example:**

```python
import knot.mcp

# Search for space management tools (default 10 results)
results = knot.mcp.tool_search("create space")
print("Found tools:", results)

# Search for file operations with custom limit
file_tools = knot.mcp.tool_search("read write file", max_results=5)
for tool in file_tools:
    print(f"- {tool['name']}: {tool['description']}")

# Get more results
all_results = knot.mcp.tool_search("space", max_results=50)
print(f"Found {len(all_results)} tools")
```

---

### execute_tool(name, arguments)

Execute a discovered tool. This is a helper function that wraps `call_tool("execute_tool", ...)` for convenience.

**Parameters:**

- `name` (string): Name of the tool to execute
- `arguments` (dict): Arguments to pass to the tool

**Returns:**

- `any`: The tool's response content, automatically decoded:
  - **Single text response**: Returns as a string
  - **JSON in text**: Automatically parsed and returned as objects (dict/list)
  - **Multiple content blocks**: Returns as a list of decoded blocks
  - **Image/Resource blocks**: Returns as a dict with `Type`, `Data`, `MimeType`, etc.

**Example:**

```python
import knot.mcp

# List all spaces
spaces = knot.mcp.execute_tool("list_spaces", {})
print("Spaces:", spaces)

# Start a specific space
result = knot.mcp.execute_tool("start_space", {
    "space_name": "dev-environment"
})
print("Start result:", result)

# Create a new space
new_space = knot.mcp.execute_tool("create_space", {
    "name": "my-new-space",
    "template_name": "python-dev"
})
print("Created:", new_space)
```

---

## Implementation Details

### MCP Tool Scripts

- Parameter access functions (get, return\_\*) are only available when mcpParams is provided
- `knot.mcp.get()`: Reads from mcpParams map passed to the script
- Automatically parses JSON for arrays and objects
- Converts string numbers to int/float
- Converts string booleans to bool

### Local and Remote Environments

- **knot.mcp.list_tools()**: Uses the `api/chat/tools` endpoint via API client
- **knot.mcp.call_tool()**: Uses the `api/chat/tools/call` endpoint via API client
- Automatically handles authentication with the server
- Uses HTTP client for external calls (agent, CLI)

### MCP Tool Execution (Internal)

- Uses MuxClient for direct mux calls (no HTTP overhead)
- Calls API handlers directly via mux
- User context passed through middleware
- Same code paths as HTTP requests

---

## Tool Categories

### Local Tools

- **Space Management**: List, start, stop, create, and delete spaces
- **File Operations**: Read, write, and manage files
- **Command Execution**: Run commands in spaces
- **System Information**: Get system status and information
- **Template Management**: Create, update, and manage deployment templates
- **User and Group Management**: Manage users and access control

### Remote Tools (if configured)

- Tools from external MCP servers with namespace prefixes (e.g., `ai/generate-text`, `data/query`)
- These are automatically discovered and available alongside local tools

---

## MCP Tool Discovery Pattern

The MCP server uses a discovery pattern where:

1. **tool_search**: Search for tools based on keywords and descriptions
2. **execute_tool**: Execute a specific tool by name with arguments

The `knot.mcp.tool_search()` and `knot.mcp.execute_tool()` helper functions simplify this pattern.

When using `knot.ai.completion()`, the AI handles tool discovery and execution automatically.

---

## Error Handling

If the MCP library is not available, it will return an appropriate error message:

- "MCP tools not available - API client not configured"

---

## Complete Examples

### Example 1: MCP Tool Script

```python
# A complete MCP tool that greets a user
import knot.mcp

# Get parameters
name = knot.mcp.get("name")
greeting_type = knot.mcp.get("greeting_type", "hello")

# Validate input
if not name:
    return knot.mcp.return_error("name parameter is required")

# Build greeting
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return result
return knot.mcp.return_string(greeting)
```

### Example 2: Discovering and Using Tools

```python
import knot.mcp

def find_and_use_space_tools():
    """Discover space management tools and use them"""

    # Search for space-related tools
    results = knot.mcp.tool_search("manage spaces")
    print("Available tools:")
    for tool in results.get('results', []):
        print(f"  - {tool['name']}: {tool['description']}")

    # List current spaces
    spaces = knot.mcp.execute_tool("list_spaces", {})
    print(f"\nFound {len(spaces)} spaces:")
    for space in spaces:
        print(f"  - {space['name']}: {space['status']}")

find_and_use_space_tools()
```

### Example 3: Using Multiple Tools Together

```python
import knot.mcp

def check_and_start_spaces():
    """Check space status and start stopped ones"""

    # Get all spaces
    spaces = knot.mcp.execute_tool("list_spaces", {})

    stopped_spaces = [s for s in spaces if s['status'] == 'stopped']

    if not stopped_spaces:
        print("All spaces are already running!")
        return

    print(f"Found {len(stopped_spaces)} stopped spaces. Starting...")

    for space in stopped_spaces:
        result = knot.mcp.execute_tool("start_space", {
            "space_name": space['name']
        })
        print(f"Started {space['name']}: {result}")

check_and_start_spaces()
```

### Example 4: Combining with AI Completion

```python
import knot.ai
import knot.mcp

def ai_managed_operations():
    """Let AI manage tools or do it directly"""

    # Option 1: Let AI decide what to do (tools used automatically)
    messages = [
        {"role": "user", "content": "List my spaces and start any that are stopped"}
    ]
    response = knot.ai.completion(messages)
    print("AI handled it:", response)

    # Option 2: Do it programmatically with full control
    spaces = knot.mcp.execute_tool("list_spaces", {})
    for space in spaces:
        if space['status'] == 'stopped':
            knot.mcp.execute_tool("start_space", {"space_name": space['name']})
            print(f"Started: {space['name']}")

ai_managed_operations()
```

---

## Related Libraries

- **knot.ai** - For AI completions with automatic tool usage
- **knot.space** - For direct space management functions (alternative to mcp.execute_tool)
