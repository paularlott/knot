# Scriptling MCP Library

The `mcp` library provides MCP (Model Context Protocol) functionality for scriptling scripts. This library is available in all environments with functions adapted to the context:

1. **MCP Tool Scripts**: All functions including parameter access (get, return_string, return_object, return_error) plus tool access functions
2. **Local/Remote/MCP Environments**: MCP tool access functions for calling MCP tools programmatically

## Available Functions

### For MCP Tool Scripts (Parameter Access)
- `get(name[, default])` - Get MCP parameter value with automatic type conversion
- `return_string(value)` - Return a string result
- `return_object(value)` - Return a structured object as JSON
- `return_error(message)` - Return an error message

### For All Environments (Tool Access)
- `list_tools()` - Get a list of all available MCP tools and their parameters
- `call_tool(name, arguments)` - Call an MCP tool directly
- `tool_search(query[, namespace])` - Search for tools by keyword (helper for discovery pattern)
- `execute_tool(name, arguments[, namespace])` - Execute a discovered tool (helper for discovery pattern)

## Availability

| Function | MCP Tool Scripts | Local | Remote | MCP |
|----------|------------------|-------|--------|-----|
| get | ✓ | ✗ | ✗ | ✗ |
| return_string | ✓ | ✗ | ✗ | ✗ |
| return_object | ✓ | ✗ | ✗ | ✗ |
| return_error | ✓ | ✗ | ✗ | ✗ |
| list_tools | ✓ | ✓ | ✓ | ✓ |
| call_tool | ✓ | ✓ | ✓ | ✓ |
| tool_search | ✓ | ✓ | ✓ | ✓ |
| execute_tool | ✓ | ✓ | ✓ | ✓ |

---

## MCP Tool Scripts

When creating scripts that are exposed as MCP tools, use these functions to access parameters and return results.

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
import mcp

# String parameter
name = mcp.get("name")
name = mcp.get("name", "default")

# Number (auto-converted)
count = mcp.get("count", 0)

# Boolean (auto-converted)
enabled = mcp.get("enabled", False)

# Array (auto-parsed from JSON)
items = mcp.get("items", [])

# Object (auto-parsed from JSON)
config = mcp.get("config", {})
```

---

### return_string(value)

Return a string result from the MCP tool. The script should exit after calling this.

**Parameters:**
- `value` (string): The string value to return

**Example:**
```python
import mcp

result = "Operation completed successfully"
return mcp.return_string(result)
```

---

### return_object(value)

Return a structured object (automatically converted to JSON). The script should exit after calling this.

**Parameters:**
- `value` (dict or list): The object to return

**Example:**
```python
import mcp

result = {
    "status": "success",
    "records_processed": 42,
    "duration_ms": 1234
}
return mcp.return_object(result)
```

---

### return_error(message)

Return an error message. The script should exit after calling this.

**Parameters:**
- `message` (string): The error message

**Example:**
```python
import mcp

if not url:
    return mcp.return_error("URL parameter is required")
```

---

## MCP Tool Access Functions

These functions are available in all scriptling environments (Local, Remote, MCP) for programmatically accessing MCP tools.

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
import mcp

# Get all available tools
tools = mcp.list_tools()

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

**Important:** The MCP server uses a discovery pattern. Only `tool_search` and `execute_tool` are directly callable. Other tools must first be discovered using `tool_search`, then executed using `execute_tool`. Consider using `mcp.tool_search()` and `mcp.execute_tool()` helper functions instead.

**Parameters:**
- `name` (string): Name of the tool to call
- `arguments` (dict): Arguments to pass to the tool

**Returns:**
- `any`: The tool's response content (type depends on the tool)

**Example:**
```python
import mcp

# Search for space-related tools
tool_search_results = mcp.call_tool("tool_search", {
    "query": "list spaces"
})
print("Tool search results:", tool_search_results)

# Execute a tool found through search
space_results = mcp.call_tool("execute_tool", {
    "name": "list_spaces",
    "arguments": {}
})
print("Spaces:", space_results)

# Call a remote tool directly (if configured)
ai_response = mcp.call_tool("ai/generate-text", {
    "prompt": "Write a Python hello world function",
    "max_tokens": 50
})
print(ai_response)
```

---

### tool_search(query[, namespace])

Search for tools by keyword. This is a helper function that wraps `call_tool("tool_search", ...)` for convenience.

**Parameters:**
- `query` (string): Search query to find matching tools
- `namespace` (string, optional): Namespace prefix for the tool_search tool (e.g., "ai" becomes "ai/tool_search")

**Returns:**
- `list`: Array of tool dictionaries (same format as `list_tools`)

**Example:**
```python
import mcp

# Search for space management tools (uses default tool_search)
results = mcp.tool_search("create space")
print("Found tools:", results)

# Search for file operations
file_tools = mcp.tool_search("read write file")
for tool in file_tools:
    print(f"- {tool['name']}: {tool['description']}")

# Search using a specific namespace (calls "ai/tool_search")
ai_results = mcp.tool_search("generate code", "ai")
print("AI tools:", ai_results)
```

---

### execute_tool(name, arguments[, namespace])

Execute a discovered tool. This is a helper function that wraps `call_tool("execute_tool", ...)` for convenience.

**Parameters:**
- `name` (string): Name of the tool to execute
- `arguments` (dict): Arguments to pass to the tool
- `namespace` (string, optional): Namespace prefix for the execute_tool tool (e.g., "ai" becomes "ai/execute_tool")

**Returns:**
- `any`: The tool's response content

**Example:**
```python
import mcp

# List all spaces (uses default execute_tool)
spaces = mcp.execute_tool("list_spaces", {})
print("Spaces:", spaces)

# Start a specific space
result = mcp.execute_tool("start_space", {
    "space_name": "dev-environment"
})
print("Start result:", result)

# Create a new space
new_space = mcp.execute_tool("create_space", {
    "name": "my-new-space",
    "template_name": "python-dev"
})
print("Created:", new_space)

# Execute a tool from a specific namespace (calls "ai/execute_tool")
ai_result = mcp.execute_tool("generate_code", {
    "prompt": "Write a Python function",
    "language": "python"
}, "ai")
print("AI result:", ai_result)
```

---

## Implementation Details

### MCP Tool Scripts
- `mcp.get()`: Reads from `MCP_PARAM_<name>` environment variables
- Automatically parses JSON for arrays and objects
- Converts string numbers to int/float
- Converts string booleans to bool

### Local and Remote Environments
- **mcp.list_tools()**: Uses the `api/chat/tools` endpoint to fetch available tools
- **mcp.call_tool()**: Uses the `api/chat/tools/call` endpoint to execute tools
- Automatically handles authentication with the server

### MCP Environment
- **mcp.list_tools()**: Calls MCP server's ListTools() method directly
- **mcp.call_tool()**: Calls MCP server's CallTool() method directly
- No API calls needed - direct server communication

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

The `mcp.tool_search()` and `mcp.execute_tool()` helper functions simplify this pattern.

When using `ai.completion()`, the AI handles tool discovery and execution automatically.

---

## Error Handling

If the MCP library is not available, it will return an appropriate error message:
- Local/Remote: "MCP tools not available - API client not configured"
- MCP: "MCP tools not available - OpenAI client not configured"

---

## Complete Examples

### Example 1: MCP Tool Script

```python
# A complete MCP tool that greets a user
import mcp

# Get parameters
name = mcp.get("name")
greeting_type = mcp.get("greeting_type", "hello")

# Validate input
if not name:
    return mcp.return_error("name parameter is required")

# Build greeting
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return result
return mcp.return_string(greeting)
```

### Example 2: Discovering and Using Tools

```python
import mcp

def find_and_use_space_tools():
    """Discover space management tools and use them"""

    # Search for space-related tools
    results = mcp.tool_search("manage spaces")
    print("Available tools:")
    for tool in results.get('results', []):
        print(f"  - {tool['name']}: {tool['description']}")

    # List current spaces
    spaces = mcp.execute_tool("list_spaces", {})
    print(f"\nFound {len(spaces)} spaces:")
    for space in spaces:
        print(f"  - {space['name']}: {space['status']}")

find_and_use_space_tools()
```

### Example 3: Using Multiple Tools Together

```python
import mcp

def check_and_start_spaces():
    """Check space status and start stopped ones"""

    # Get all spaces
    spaces = mcp.execute_tool("list_spaces", {})

    stopped_spaces = [s for s in spaces if s['status'] == 'stopped']

    if not stopped_spaces:
        print("All spaces are already running!")
        return

    print(f"Found {len(stopped_spaces)} stopped spaces. Starting...")

    for space in stopped_spaces:
        result = mcp.execute_tool("start_space", {
            "space_name": space['name']
        })
        print(f"Started {space['name']}: {result}")

check_and_start_spaces()
```

### Example 4: Combining with AI Completion

```python
import ai
import mcp

def ai_managed_operations():
    """Let AI manage tools or do it directly"""

    # Option 1: Let AI decide what to do (tools used automatically)
    messages = [
        {"role": "user", "content": "List my spaces and start any that are stopped"}
    ]
    response = ai.completion(messages)
    print("AI handled it:", response)

    # Option 2: Do it programmatically with full control
    spaces = mcp.execute_tool("list_spaces", {})
    for space in spaces:
        if space['status'] == 'stopped':
            mcp.execute_tool("start_space", {"space_name": space['name']})
            print(f"Started: {space['name']}")

ai_managed_operations()
```

---

## Related Libraries

- **ai** - For AI completions with automatic tool usage
- **spaces** - For direct space management functions (alternative to mcp.execute_tool)
