# Scriptling AI Library

The `ai` library provides AI completion functionality for scriptling scripts. This library is available in all three scriptling execution environments (Local, MCP, and Remote), with the implementation automatically adapting to the environment.

## Available Functions

- `completion(messages)` - Get an AI completion from a list of messages
- `response_create(input, model=None, instructions=None, previous_response_id=None)` - Create an async AI response
- `response_get(id)` - Get the status and result of an async response
- `response_wait(id, timeout=300)` - Wait for an async response to complete
- `response_cancel(id)` - Cancel an in-progress async response
- `response_delete(id)` - Delete an async response

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

### response_create(input, model=None, instructions=None, previous_response_id=None)

Create an asynchronous AI response. This is useful for long-running AI operations that you don't want to block on.

**Parameters:**
- `input` (string, dict, or list): The input for the AI response. Can be a simple string, a structured dict, or a list of items
- `model` (string, optional): The AI model to use (if not specified, uses server default)
- `instructions` (string, optional): Additional instructions for the AI
- `previous_response_id` (string, optional): ID of a previous response to continue from

**Returns:**
- `string`: The response ID that can be used to check status, wait for completion, or cancel

**Example:**
```python
import ai

# Create an async response
response_id = ai.response_create(
    input="Analyze all my spaces and provide a detailed report",
    instructions="Include resource usage and recommendations"
)
print(f"Created response: {response_id}")
```

### response_get(id)

Get the current status and result of an async response.

**Parameters:**
- `id` (string): The response ID returned from response_create()

**Returns:**
- `dict`: A dictionary containing:
  - `response_id` (string): The response ID
  - `status` (string): Current status ("pending", "in_progress", "completed", "failed", or "cancelled")
  - `request` (dict): The original request data
  - `response` (dict, optional): The response data (only present when completed)
  - `error` (string, optional): Error message (only present if failed)

**Example:**
```python
import ai

# Check response status
status = ai.response_get(response_id)
print(f"Status: {status['status']}")

if status['status'] == 'completed':
    print(f"Result: {status['response']}")
elif status['status'] == 'failed':
    print(f"Error: {status['error']}")
```

### response_wait(id, timeout=300)

Wait for an async response to complete. This will block until the response is finished or the timeout is reached.

**Parameters:**
- `id` (string): The response ID returned from response_create()
- `timeout` (int, optional): Maximum time to wait in seconds (default: 300)

**Returns:**
- `dict`: Same format as response_get() - a dictionary with the final response status and result

**Example:**
```python
import ai

# Create and wait for response
response_id = ai.response_create(
    input="Generate a comprehensive analysis"
)

# Wait up to 5 minutes for completion
result = ai.response_wait(response_id, timeout=300)

if result['status'] == 'completed':
    print(f"Analysis complete: {result['response']}")
else:
    print(f"Failed with status: {result['status']}")
```

### response_cancel(id)

Cancel an in-progress async response.

**Parameters:**
- `id` (string): The response ID returned from response_create()

**Returns:**
- `bool`: True if successfully cancelled

**Example:**
```python
import ai
import time

# Create a response
response_id = ai.response_create(
    input="Long running task"
)

# Wait a bit then cancel
time.sleep(2)
ai.response_cancel(response_id)
print("Response cancelled")
```

### response_delete(id)

Delete an async response and clean up its data.

**Parameters:**
- `id` (string): The response ID returned from response_create()

**Returns:**
- `bool`: True if successfully deleted

**Example:**
```python
import ai

# Clean up a completed response
ai.response_delete(response_id)
print("Response deleted")

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

### Example 1: Using Async Responses for Long Operations

```python
import ai
import time

def process_with_async_ai():
    """Use async AI responses for long-running operations"""

    # Start multiple async operations
    print("Starting async AI operations...")
    
    response_ids = []
    for i in range(3):
        response_id = ai.response_create(
            input=f"Task {i+1}: Analyze system component {i+1}",
            instructions="Provide detailed analysis with recommendations"
        )
        response_ids.append(response_id)
        print(f"Started task {i+1}: {response_id}")
    
    # Wait for all to complete
    results = []
    for i, response_id in enumerate(response_ids):
        print(f"\nWaiting for task {i+1}...")
        result = ai.response_wait(response_id, timeout=120)
        
        if result['status'] == 'completed':
            print(f"Task {i+1} completed successfully")
            results.append(result['response'])
        else:
            print(f"Task {i+1} failed: {result.get('error', 'Unknown error')}")
        
        # Clean up
        ai.response_delete(response_id)
    
    return results

# Execute
results = process_with_async_ai()
print(f"\nCompleted {len(results)} tasks")
```

### Example 2: AI-Assisted Space Management

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

### Example 3: Using AI for Code Generation

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

### Example 4: Multi-turn Conversation

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
