# Scriptling AI Library

The AI library provides scriptling scripts with access to AI completion functionality through the server's chat API. It works seamlessly across local, remote, and MCP environments.

## Availability

The AI library is available in all scriptling environments when the server has AI chat enabled (when OpenAI API credentials are configured).

- **Local scripts**: Available when run via agents or desktop CLI - uses REST API
- **Remote scripts**: Available when run in spaces - uses REST API
- **MCP scripts**: Available when run as MCP tools - uses MCP server directly

## API Endpoints

The server provides the following chat-related endpoints:

- `/api/chat/stream` - Streaming chat completion (used by web UI and agent chat command)
- `/api/chat/completion` - Non-streaming chat completion with automatic tool calling (used by scriptling AI library)
- `/api/chat/tools` - Lists available MCP tools (used by `ai.list_tools()`)
- `/api/chat/tools/call` - Executes a tool directly (used by `ai.call_tool()` - advanced usage)

## Functions

### `ai.completion(messages)`

Get an AI completion from a list of messages.

**Parameters:**
- `messages` (list): List of message objects, each containing:
  - `role` (string): Message role ("system", "user", "assistant", or "tool")
  - `content` (string): Message content

**Returns:**
- `string`: The AI's response content

### `ai.list_tools()`

Get a list of all available MCP tools and their parameters.

**Parameters:** None

**Returns:**
- `list`: List of tool objects, each containing:
  - `name` (string): The tool's name
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
```

### `ai.call_tool(name, arguments)`

Call a tool directly without AI intervention.

**Parameters:**
- `name` (string): Name of the tool to call
- `arguments` (dict): Arguments to pass to the tool

**Returns:**
- `any`: The tool's response content (type depends on the tool)

**Note:** For most use cases, it's better to use the dedicated libraries (like `spaces`, `commands`) or let the AI automatically use tools during completion rather than calling tools directly.

**Important:** The MCP server uses a discovery pattern. Only `tool_search` and `execute_tool` are directly callable. Other tools must first be discovered using `tool_search`, then executed using `execute_tool`.

**Examples:**
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
        "space_id": "your-space-id"
    }
})
print("Start result:", start_result)
```

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

## Tool Calling

When using AI completion through the server, the AI automatically has access to all MCP tools and can use them during conversations:

- **Automatic Tool Discovery**: The AI knows what tools are available and their capabilities
- **Contextual Tool Usage**: The AI will use appropriate tools based on your requests without you needing to specify them
- **Tool Results in Response**: Tool outputs are incorporated into the AI's response
- **Transparent Execution**: The AI explains what tools it used and why

### MCP Tool Discovery Pattern

The MCP server uses a discovery pattern where:
1. **tool_search**: Search for tools based on keywords and descriptions
2. **execute_tool**: Execute a specific tool by name with arguments

When using `ai.call_tool()` directly, you must follow this pattern:
- First call `tool_search` to find available tools
- Then call `execute_tool` with the tool name and arguments

When using `ai.completion()`, the AI handles this discovery automatically.

### Available Tool Categories
- **Space Management**: List, start, stop, create, and delete spaces
- **File Operations**: Read, write, and manage files
- **Command Execution**: Run commands in spaces
- **System Information**: Get system status and information
- **Template Management**: Create, update, and manage deployment templates
- **User and Group Management**: Manage users and access control

### Example of Automatic Tool Usage

```python
import ai

# The AI will automatically use tools to gather information
messages = [
    {"role": "user", "content": "Check what spaces I have and start any that are stopped"}
]

response = ai.completion(messages)
print(response)
# AI might respond: "I found 3 spaces. 'dev-space' was stopped so I started it for you.
# The other two ('web-space' and 'test-space') are already running."
```

This automatic tool calling happens seamlessly - you just ask what you want to accomplish, and the AI figures out which tools to use.

## Error Handling

If the AI library is not available, it will return an appropriate error message:
- Local/Remote: "AI completion not available - API client not configured"
- MCP: "AI completion in MCP environment should be handled through MCP server tools"

## Complete Example

### Using AI with Automatic Tool Calling

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

### Using Libraries Directly

```python
import spaces

def manage_spaces_programmatically():
    """Direct programmatic space management using the spaces library"""

    # List all spaces
    spaces_list = spaces.list()
    print("Available spaces:")
    for space in spaces_list:
        print(f"  - {space['name']}: {space['state']}")

    # Find stopped spaces
    stopped_spaces = [s for s in spaces_list if s['state'] == 'stopped']

    if stopped_spaces:
        # Start the first stopped space
        space_name = stopped_spaces[0]['name']
        print(f"\nStarting space: {space_name}")
        result = spaces.start(space_name)
        print("Start result:", result)
    else:
        print("\nNo stopped spaces found")

# Execute the programmatic management
manage_spaces_programmatically()
```

### Combined AI and Direct Control

```python
import ai
import spaces

def intelligent_space_management():
    """Combine AI guidance with direct control"""

    # Get space information directly
    spaces_list = spaces.list()
    stopped_spaces = [s for s in spaces_list if s['state'] == 'stopped']

    if not stopped_spaces:
        print("All spaces are running!")
        return

    # Use AI to decide what to do
    messages = [
        {"role": "system", "content": "You are a space management expert. Analyze the situation and provide recommendations."},
        {"role": "user", "content": f"I have {len(stopped_spaces)} stopped spaces: {[s['name'] for s in stopped_spaces]}. Should I start them? Why or why not?"}
    ]

    advice = ai.completion(messages)
    print("AI Advice:", advice)

    # Based on AI advice, take action
    if "start" in advice.lower() and "yes" in advice.lower():
        for space in stopped_spaces[:1]:  # Start just the first one as example
            print(f"\nStarting space: {space['name']}")
            result = spaces.start(space['name'])
            print("Result:", result)
    else:
        print("\nFollowing AI advice - not starting spaces")

# Execute the intelligent management
intelligent_space_management()
```

In these examples, the AI will:
1. Automatically use MCP tools during completion when appropriate
2. Provide intelligent analysis and recommendations
3. Guide programmatic actions based on context

The AI has access to all MCP tools through the server, so it can take actions based on your requests while keeping you informed of what it's doing.