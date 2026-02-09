# Scriptling AI Library (knot.ai)

The `knot.ai` library provides access to the server's AI client for scriptling scripts. It returns a pre-configured client instance connected to the upstream AI provider, ready for use with `scriptling.ai.agent` or direct completion calls.

The server can be configured to use different LLM providers (OpenAI, Claude, Gemini, Ollama, etc.) via the `--chat-provider` flag or `KNOT_CHAT_PROVIDER` environment variable. Scripts automatically connect to whichever provider the server is configured to use.

## Available Functions

| Function            | Description                                      |
| ------------------- | ------------------------------------------------ |
| `Client()`          | Get a pre-configured AI client instance          |
| `get_default_model()` | Get the server-configured default model name   |

## Functions

### Client()

Returns a pre-configured AI client instance. In MCP context, the client connects directly to the upstream AI provider with the MCP server attached for per-user tool discovery. In desktop/agent context, it connects to the server's OpenAI-compatible endpoint which handles tools server-side.

**Returns:**

- `Client`: An AI client instance with `completion()`, `stream_completion()`, and other methods.

**Example:**

```python
import knot.ai as ai

client = ai.Client()

# Use directly for a simple completion
response = client.completion("gpt-4o", [
    {"role": "user", "content": "What is the capital of France?"}
])
print(response.choices[0].message.content)
```

### get_default_model()

Get the name of the server-configured default model. This returns the model set in `[server.chat] model` in `.knot.toml`.

**Returns:**

- `str`: The model name (e.g. `"gpt-4o"`, `"claude-sonnet-4-20250514"`), or an empty string if the model is not configured.

**Example:**

```python
import knot.ai as ai

model = ai.get_default_model()
print(f"Server is using model: {model}")
```

## Usage with scriptling.ai.agent

The primary use case for `knot.ai.Client()` is with the `scriptling.ai.agent` library for agentic AI workflows:

```python
import knot.ai as ai
import scriptling.ai as sai
import scriptling.ai.agent as agent

# Get the pre-configured client
client = ai.Client()
model = ai.get_default_model()

# Create an agent with tools
tools = sai.ToolRegistry()
tools.add("greet", "Greet someone", {"name": "string"}, lambda args: f"Hello, {args['name']}!")

bot = agent.Agent(
    client=client,
    model=model,
    tools=tools,
    system_prompt="You are a helpful assistant. Use tools when needed."
)

response = bot.trigger("Please greet Paul", max_iterations=5)
print(response.content)
```

### Interactive Mode

```python
import knot.ai as ai
import scriptling.ai as sai
import scriptling.ai.agent.interact as interact

client = ai.Client()
model = ai.get_default_model()

tools = sai.ToolRegistry()
tools.add("search", "Search for information", {"query": "string"}, lambda args: f"Results for: {args['query']}")

bot = interact.Agent(
    client=client,
    model=model,
    tools=tools,
    system_prompt="You are a helpful search assistant."
)

# Start interactive CLI session
bot.interact()
```

## Direct Completion

For simple completions without the agent framework:

```python
import knot.ai as ai

client = ai.Client()
model = ai.get_default_model()

# Simple completion
response = client.completion(model, [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is Python?"}
])
print(response.choices[0].message.content)

# String shorthand
response = client.completion(model, "What is the capital of France?")
print(response.choices[0].message.content)

# With system prompt shorthand
response = client.completion(model, "Hello",
    system_prompt="You are a friendly assistant.")
print(response.choices[0].message.content)
```

## Multi-turn Conversation

```python
import knot.ai as ai

client = ai.Client()
model = ai.get_default_model()

messages = [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is Python?"}
]

response = client.completion(model, messages)
print("AI:", response.choices[0].message.content)

# Continue the conversation
messages.append({"role": "assistant", "content": response.choices[0].message.content})
messages.append({"role": "user", "content": "What are its main use cases?"})

response = client.completion(model, messages)
print("AI:", response.choices[0].message.content)
```

## Server Configuration

The AI provider is configured server-side in the `.knot.toml` configuration file:

```toml
[server.chat]
    enabled = true
    openai_api_key = "your-api-key"
    openai_base_url = "https://api.openai.com/v1"
    model = "gpt-4o"
    system_prompt = "You are a helpful assistant."
```

## Error Handling

If the AI client is not configured on the server, `Client()` will return an error:

```python
import knot.ai as ai

try:
    client = ai.Client()
except Exception as e:
    print(f"AI not available: {e}")
```

## Related Libraries

- **scriptling.ai** - Core AI library with ToolRegistry and `new_client()` for creating clients from scratch
- **scriptling.ai.agent** - Agentic AI loop with automatic tool execution
- **scriptling.ai.agent.interact** - Interactive CLI agent with colored output
- **knot.mcp** - For direct MCP tool access (list_tools, call_tool, tool_search, execute_tool)
- **knot.space** - For space management functions
