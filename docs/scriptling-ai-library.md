# Scriptling AI Library

The `ai` library provides AI completion functionality for scriptling scripts. This library is available in all three scriptling execution environments (Local, MCP, and Remote), with the implementation automatically adapting to the environment.

## Available Functions

- `completion(messages)` - Get an AI completion from a list of messages

## Availability

| Environment | Available | Implementation |
|-------------|-----------|----------------|
| Local       | ✓         | API Client     |
| MCP         | ✓         | Direct OpenAI  |
| Remote      | ✓         | API Client     |

## Usage

```python
import ai

# Use AI completion with automatic tool usage
messages = [
    {"role": "user", "content": "What spaces do I have and what's their status?"}
]
response = ai.completion(messages)
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

## Implementation Details

### Local and Remote Environments
- **ai.completion()**: Uses the `api/chat/completion` endpoint via REST API
- Automatically handles authentication with the server
- Returns complete response without streaming

### MCP Environment
- **ai.completion()**: Uses the MCP server's direct OpenAI client integration
- No API calls needed - direct server communication

## Error Handling

If the AI library is not available, it will return an appropriate error message:
- Local/Remote: "AI completion not available - API client not configured"
- MCP: "AI completion not available - OpenAI client not configured"

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

### Example 2: Using AI for Code Generation

```python
import ai

def generate_code():
    """Use AI to generate code"""

    messages = [
        {"role": "system", "content": "You are a helpful programming assistant. Write clean, well-documented code."},
        {"role": "user", "content": "Write a Python function to calculate the factorial of a number"}
    ]

    response = ai.completion(messages)
    print(response)

generate_code()
```

### Example 3: Multi-turn Conversation

```python
import ai

def chat_conversation():
    """Have a multi-turn conversation with the AI"""

    messages = [
        {"role": "system", "content": "You are a helpful assistant."}
    ]

    # First turn
    messages.append({"role": "user", "content": "What is Python?"})
    response = ai.completion(messages)
    print("AI:", response)
    messages.append({"role": "assistant", "content": response})

    # Second turn (context is preserved)
    messages.append({"role": "user", "content": "What are its main use cases?"})
    response = ai.completion(messages)
    print("AI:", response)

chat_conversation()
```

## Related Libraries

- **mcp** - For direct MCP tool access (list_tools, call_tool, tool_search, execute_tool)
- **spaces** - For space management functions
