# Scriptling AI Library

The `ai` library provides AI completion functionality with access to MCP tools for scriptling scripts. This library is available in all three scriptling execution environments (Local, MCP, and Remote), with the implementation automatically adapting to the environment.

## Available Functions

- `completion(messages)` - Get an AI completion from a list of messages
- `list_tools()` - Get a list of all available MCP tools and their parameters
- `call_tool(name, arguments)` - Call a tool directly without AI intervention

## Availability

| Environment | Available | Implementation |
|-------------|-----------|----------------|
| Local       | ✓         | API Client     |
| MCP         | ✓         | Direct MCP     |
| Remote      | ✓         | API Client     |

## Usage

```python
import ai

# Get all available tools
tools = ai.list_tools()
for tool in tools:
    print(f"Tool: {tool['name']} - {tool['description']}")

# Use AI completion with automatic tool usage
messages = [
    {"role": "user", "content": "What spaces do I have and what's their status?"}
]
response = ai.completion(messages)
print(response)

# Call a tool directly
response = ai.call_tool("execute_tool", {
    "name": "list_spaces",
    "arguments": {}
})
print(response)
```

## Functions

### completion(messages)

Get an AI completion from a list of messages. The AI will automatically have access to all MCP tools and can use them during the conversation.

**Parameters:**
- `messages` (list): List of message objects, each containing:
  - `role` (string): Message role ("system", "user", "assistant", or "tool")
  - `content` (string): Message content

**Returns:**
- `string`: The AI's response content

**System Messages:**
- If you include a `system` role message in your messages, it will be used as the system prompt
- If no `system` message is provided, the server's configured system prompt will be used automatically

**Example:**
```python
import ai

# Simple completion
messages = [
    {"role": "user", "content": "What is the capital of France?"}
]
response = ai.completion(messages)
print(response)  # "The capital of France is Paris."

# With custom system prompt
messages = [
    {"role": "system", "content": "You are a helpful geography expert."},
    {"role": "user", "content": "What is the capital of France?"}
]
response = ai.completion(messages)
print(response)  # Uses your custom system prompt

# Example with automatic tool usage
messages = [
    {"role": "user", "content": "Check what spaces I have and start any that are stopped"}
]
response = ai.completion(messages)
print(response)
# AI might respond: "I found 3 spaces. 'dev-space' was stopped so I started it for you.
# The other two ('web-space' and 'test-space') are already running."
```

---

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
import ai

# Get all available tools
tools = ai.list_tools()

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

Call a tool directly without AI intervention. For most use cases, it's better to use the dedicated libraries (like `spaces`, `commands`) or let the AI automatically use tools during completion.

**Important:** The MCP server uses a discovery pattern. Only `tool_search` and `execute_tool` are directly callable. Other tools must first be discovered using `tool_search`, then executed using `execute_tool`.

**Parameters:**
- `name` (string): Name of the tool to call
- `arguments` (dict): Arguments to pass to the tool

**Returns:**
- `any`: The tool's response content (type depends on the tool)

**Example:**
```python
import ai

# Get all available tools first (this shows you what you can execute)
tools = ai.list_tools()
print("Available tools:", [t['name'] for t in tools])

# Search for space-related tools
tool_search_results = ai.call_tool("tool_search", {
    "query": "list spaces"
})
print("Tool search results:", tool_search_results)

# Execute a tool found through search
space_results = ai.call_tool("execute_tool", {
    "name": "list_spaces",
    "arguments": {}
})
print("Spaces:", space_results)

# Start a space (if you know it exists from the search)
start_result = ai.call_tool("execute_tool", {
    "name": "start_space",
    "arguments": {
        "space_name": "your-space-name"
    }
})
print("Start result:", start_result)

# Call a remote tool directly (if configured)
ai_response = ai.call_tool("ai/generate-text", {
    "prompt": "Write a Python hello world function",
    "max_tokens": 50
})
print(ai_response)
```

## Implementation Details

### Local and Remote Environments
- **ai.completion()**: Uses the `api/chat/completion` endpoint via REST API
- **ai.list_tools()**: Uses the `api/chat/tools` endpoint to fetch available tools
- **ai.call_tool()**: Uses the `api/chat/tools/call` endpoint to execute tools
- Automatically handles authentication with the server
- Returns complete response without streaming

### MCP Environment
- **ai.completion()**: Uses the MCP server's direct OpenAI client integration
- **ai.list_tools()**: Calls MCP server's ListTools() method directly
- **ai.call_tool()**: Calls MCP server's CallTool() method directly
- No API calls needed - direct server communication

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

## MCP Tool Discovery Pattern

The MCP server uses a discovery pattern where:
1. **tool_search**: Search for tools based on keywords and descriptions
2. **execute_tool**: Execute a specific tool by name with arguments

When using `ai.call_tool()` directly, you must follow this pattern:
- First call `tool_search` to find available tools
- Then call `execute_tool` with the tool name and arguments

When using `ai.completion()`, the AI handles this discovery automatically.

## Error Handling

If the AI library is not available, it will return an appropriate error message:
- Local/Remote: "AI completion not available - API client not configured"
- MCP: "AI completion in MCP environment should be handled through MCP server tools"

## Complete Examples

### Example 1: AI-Assisted Space Management

```python
import ai
import spaces

def manage_spaces_with_ai():
    """Use AI to help manage spaces - AI will automatically use tools when needed"""

    # Ask AI to help manage spaces - it will automatically use tools
    messages = [
        {"role": "system", "content": "You are a helpful assistant for managing development spaces. Use the available tools to help the user."},
        {"role": "user", "content": "What spaces do I have available? Please list them and tell me their current status."}
    ]

    response = ai.completion(messages)
    print("AI Response:", response)

    # AI might have used tools automatically to get space information
    # Continue the conversation
    messages.append({"role": "assistant", "content": response})
    messages.append({"role": "user", "content": "If any spaces are stopped, please start the first one you find."})

    response2 = ai.completion(messages)
    print("\nAI Response:", response2)

# Execute the AI-assisted management
manage_spaces_with_ai()
```

### Example 2: Using Remote Tools with Local Tools

```python
import ai

def generate_and_deploy_code():
    """Generate code using remote AI tools and deploy using local tools"""

    messages = [
        {"role": "user", "content": "Generate a Python web server script and save it to my web-dev space"}
    ]

    response = ai.completion(messages)
    print(response)
    # AI might:
    # 1. Use ai/generate-text (remote tool) to create the Python script
    # 2. Use write_file (local tool) to save it to the specified space
```

### Example 3: Direct Tool Discovery and Usage

```python
import ai

def explore_and_use_tools():
    """Discover available tools and use them directly"""

    # List all tools
    tools = ai.list_tools()
    print(f"Found {len(tools)} tools")

    # Search for specific functionality
    search_result = ai.call_tool("tool_search", {
        "query": "create new space"
    })

    # Extract tool name from search results
    if 'results' in search_result and search_result['results']:
        tool_name = search_result['results'][0]['name']

        # Use the discovered tool
        result = ai.call_tool("execute_tool", {
            "name": tool_name,
            "arguments": {
                "name": "my-new-space",
                "template_name": "python-dev"
            }
        })
        print("Created space:", result)
```