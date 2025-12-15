# Scriptling AI Library

The AI library provides scriptling scripts with access to AI completion functionality through the server's chat API. It works seamlessly across local, remote, and MCP environments.

## Availability

The AI library is available in all scriptling environments when the server has AI chat enabled (when OpenAI API credentials are configured).

- **Local scripts**: Available when run via agents or desktop CLI - uses REST API
- **Remote scripts**: Available when run in spaces - uses REST API
- **MCP scripts**: Available when run as MCP tools - uses MCP server directly

## API Endpoints

The server now provides two chat endpoints:

- `/api/chat/stream` - Streaming chat completion (used by web UI and agent chat command)
- `/api/chat/completion` - Non-streaming chat completion (used by scriptling AI library)

## Functions

### `ai.completion(messages)`

Get an AI completion from a list of messages.

**Parameters:**
- `messages` (list): List of message objects, each containing:
  - `role` (string): Message role ("system", "user", "assistant", or "tool")
  - `content` (string): Message content

**Returns:**
- `string`: The AI's response content

**Example:**
```python
import ai

# Simple completion
messages = [
    {"role": "user", "content": "What is the capital of France?"}
]
response = ai.completion(messages)
print(response)  # "The capital of France is Paris."
```

## Implementation Details

### Local and Remote Environments
- Uses the `api/chat/completion` endpoint via REST API
- Automatically handles authentication with the server
- Returns complete response without streaming

### MCP Environment
- For MCP tools, the AI completion should be handled through the MCP server's built-in AI capabilities
- The AI library currently returns an error directing to use MCP server tools

## Tool Calling

When using AI completion through the server, the AI automatically has access to all MCP tools:
- List, start, stop, and manage spaces
- Execute commands in spaces
- Read and write files
- Access all other MCP tools

This happens automatically - you don't need to configure anything. The AI will use tools when appropriate based on your prompts.

## Error Handling

If the AI library is not available, it will return an appropriate error message:
- Local/Remote: "AI completion not available - API client not configured"
- MCP: "AI completion in MCP environment should be handled through MCP server tools"

## Complete Example

```python
import ai
import spaces

# Use AI to help manage spaces
def analyze_space_status():
    # Get list of spaces
    space_list = spaces.list()

    # Ask AI to analyze the status
    messages = [
        {"role": "system", "content": "You are a helpful assistant for managing development spaces."},
        {"role": "user", "content": f"Here are my spaces: {space_list}. Please summarize which ones are running and which are stopped."}
    ]

    analysis = ai.completion(messages)
    print("Space Analysis:", analysis)

    # AI might suggest actions
    messages.append({"role": "assistant", "content": analysis})
    messages.append({"role": "user", "content": "Should I start any of the stopped spaces? Please start the development space if it exists."})

    response = ai.completion(messages)
    print("AI Recommendation:", response)

# Execute the analysis
analyze_space_status()
```

In this example, the AI will:
1. Analyze the space list you provide
2. Give a summary of running/stopped spaces
3. Potentially use MCP tools to start a space if appropriate

The AI has access to all MCP tools through the server, so it can take actions based on your requests while keeping you informed of what it's doing.