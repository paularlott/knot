# MCP Tool Authoring Guide

## Overview

Knot allows you to create scripts that are exposed as MCP (Model Context Protocol) tools to AI assistants. This enables AI to discover and execute your custom automation directly.

## Script Types

When creating a script, set the `script_type` field:

- **`script`** - Standard executable script (default)
- **`lib`** - Library/reusable code for import by other scripts
- **`tool`** - MCP tool exposed to AI assistants

## Creating an MCP Tool

### 1. Basic Structure

```python
# Import the standard MCP tool helpers (recommended)
import scriptling.mcp.tool as tool

# Access parameters with type-safe functions
name = tool.get_string("name")
greeting_type = tool.get_string("greeting_type", "hello")  # with default

# Do your work
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return output
tool.return_string(greeting)
```

**Note:** The `scriptling.mcp.tool` library provides portable, standardized MCP tool helpers. For Knot-specific features like calling other tools, use `knot.mcp` (see [Advanced Features](#knot-mcp-advanced-features)).

### 2. Define Input Schema (TOML)

Use TOML to define your tool's parameters. This tells the AI what parameters to send.

**Simple parameters:**
```toml
[name]
type = "string"
description = "The name to greet"
required = true

[greeting_type]
type = "string"
description = "Type of greeting (hello, hi, hey, etc.)"
required = false
```

**Parameter types:**
- `string` - Text value
- `number` - Integer or float
- `boolean` - true/false
- `array` - List of values (currently string arrays)

**Array parameters:**
```toml
[headers]
type = "array"
description = "HTTP headers to include"
required = false
```

### 3. Add Keywords

Keywords help AI assistants discover your tool. Add relevant search terms:

```json
["http", "api", "request", "web", "rest", "curl"]
```

### 4. Set Active Status

Only scripts marked as `active = true` are registered as MCP tools.

### 5. Group Access Control

Assign groups to control which users can access the tool:
- Empty groups array = available to all users
- Specific groups = only users in those groups see the tool

## Accessing Parameters

The `scriptling.mcp.tool` library (recommended) provides portable, type-safe functions for accessing tool parameters:

```python
import scriptling.mcp.tool as tool

# String parameter
name = tool.get_string("name")

# Integer parameter
count = tool.get_int("count", 10)  # default: 10

# Float parameter
rate = tool.get_float("rate", 1.5)  # default: 1.5

# Boolean parameter (handles various string representations)
enabled = tool.get_bool("enabled", False)

# List parameter (handles comma-separated strings or JSON arrays)
headers = tool.get_list("headers", [])
for header in headers:
    print(f"Header: {header}")
```

**Alternative:** You can also use `knot.mcp` which provides the same functions plus additional Knot-specific features.

## Returning Results

### Using the Standard MCP Tool Library (Recommended)

```python
import scriptling.mcp.tool as tool

# Return a string (script ends immediately after this call)
tool.return_string("Operation completed successfully")

# Return structured data (automatically converted to JSON)
result = {
    "status": "success",
    "records_processed": 42,
    "duration_ms": 1234
}
tool.return_object(result)

# Return TOON format (compact format for LLMs)
tool.return_toon(result)

# Return an error (exits with error code)
if not url:
    tool.return_error("URL parameter is required")
```

**Note:** All `tool.return_*` functions stop execution immediately. Code after them will not run.

### Direct Output (Alternative)

You can also use standard print:

```python
import json

# Text output
print("Operation completed successfully")

# Structured data
result = {"status": "success"}
print(json.dumps(result, indent=2))

# Errors
import sys
if not url:
    print("Error: URL parameter is required", file=sys.stderr)
    sys.exit(1)
```

## Complete Example

### Script Metadata

- **Name:** `greeting_tool`
- **Description:** `Generate personalized greetings`
- **Script Type:** `tool`
- **Active:** `true`
- **Groups:** `["all"]`
- **Keywords:** `["greeting", "hello", "hi", "welcome"]`

### Input Schema (TOML)

```toml
[name]
type = "string"
description = "The name to greet"
required = true

[greeting_type]
type = "string"
description = "Type of greeting (hello, hi, hey, etc.)"
required = false
```

### Script Content

```python
import scriptling.mcp.tool as tool

# Get parameters using standard MCP tool helpers
name = tool.get_string("name")
greeting_type = tool.get_string("greeting_type", "hello")

# Build greeting
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return the greeting (script ends here)
tool.return_string(greeting)
```

## Best Practices

### 1. Validate Input

Always validate required parameters:

```python
import scriptling.mcp.tool as tool

url = tool.get_string("url")
if not url:
    tool.return_error("url parameter is required")
```

### 2. Provide Defaults

Use sensible defaults for optional parameters:

```python
import scriptling.mcp.tool as tool

timeout = tool.get_int("timeout", 30)
method = tool.get_string("method", "GET")
```

### 3. Handle Errors Gracefully

Catch exceptions and return helpful messages:

```python
import scriptling.mcp.tool as tool

try:
    # Your code
    pass
except ValueError as e:
    tool.return_error(f"Invalid parameter: {e}")
except Exception as e:
    tool.return_error(f"Unexpected error: {e}")
```

### 4. Keep Output Concise

AI context windows are limited. Truncate large outputs:

```python
import scriptling.mcp.tool as tool

if len(output) > 1000:
    output = output[:1000] + "... (truncated)"
tool.return_string(output)
```

### 5. Use Descriptive Names

- Tool name: Clear, action-oriented (`send_email`, not `email_tool`)
- Parameters: Self-explanatory (`recipient_email`, not `to`)
- Description: Explain what the tool does and when to use it

### 6. Add Comprehensive Keywords

Include synonyms and related terms:

```python
# Good
["email", "mail", "send", "smtp", "message", "notification"]

# Too narrow
["email"]
```

## Testing Your Tool

MCP tools are designed to be executed by AI assistants through the Model Context Protocol:

1. The AI assistant discovers your tool using `tool_search`
2. The AI executes it using `execute_tool` with the appropriate parameters
3. Your tool receives parameters via `tool.get_string()`, `tool.get_int()`, etc. and returns results

This is the recommended way to test MCP tools as it exercises the full MCP integration.

## MCP Tool Helper Library Reference

### Standard Library: scriptling.mcp.tool (Recommended)

The `scriptling.mcp.tool` library provides portable, standardized MCP tool helpers that work across different environments:

**Parameter Access Functions:**

- `tool.get_string(name, default="")` - Get parameter as a trimmed string
- `tool.get_int(name, default=0)` - Get parameter as an integer
- `tool.get_float(name, default=0.0)` - Get parameter as a float
- `tool.get_bool(name, default=False)` - Get parameter as a boolean
- `tool.get_list(name, default=[])` - Get parameter as a list

**Return Functions:**

- `tool.return_string(value)` - Return a string result and stop execution
- `tool.return_object(value)` - Return a structured object (JSON) and stop execution
- `tool.return_toon(value)` - Return TOON-encoded object and stop execution
- `tool.return_error(message)` - Return an error message and exit with code 1

For complete documentation, see the [scriptling.mcp documentation](https://github.com/paularlott/scriptling/blob/main/docs/libraries/scriptling/mcp.md).

### Knot-Specific Library: knot.mcp (Advanced Features) {#knot-mcp-advanced-features}

The `knot.mcp` library provides all the same parameter access and return functions as `scriptling.mcp.tool`, plus additional Knot-specific features:

**Tool Discovery and Execution:**

- `knot.mcp.list_tools()` - List all available MCP tools
- `knot.mcp.call_tool(name, arguments)` - Call another MCP tool directly
- `knot.mcp.tool_search(query, max_results=10)` - Search for tools by keyword
- `knot.mcp.execute_tool(name, arguments)` - Execute a discovered tool

**Example:**

```python
import knot.mcp
import scriptling.mcp.tool as tool

# Get parameter
query = tool.get_string("query")

# Search for relevant tools using knot.mcp
tools = knot.mcp.tool_search(query)

if not tools:
    tool.return_error(f"No tools found for: {query}")
else:
    # Execute the first matching tool
    result = knot.mcp.execute_tool(tools[0]["name"], {"input": query})
    tool.return_object(result)
```

For full knot.mcp documentation, see [scriptling-mcp-library.md](./scriptling-mcp-library.md).

### Future Enhancements

Additional context may be provided in future versions:

- User executing the tool
- Space context (if applicable)
- Execution environment details

## TOML Schema Reference

### Basic Structure

```toml
[parameter_name]
type = "string|number|boolean|array"
description = "Human-readable description"
required = true|false
```

### Supported Types

- **string** - Text values
- **number** - Integers or floats
- **boolean** - true/false values
- **array** - Lists of strings (object arrays coming soon)

### Array Schema

```toml
[parameter_name]
type = "array"
description = "List of items"
required = false
```

Currently supports string arrays. Object arrays will be added in a future update.ype = "array"
description = "List of items"
items = "string|number|boolean|object"
required = false
```

### Object Schema

```toml
[parameter_name]
type = "object"
description = "Complex structure"
required = false

[parameter_name.properties.field1]
type = "string"
description = "First field"

[parameter_name.properties.field2]
type = "number"
description = "Second field"
```

### Nested Objects

```toml
[config]
type = "object"
description = "Configuration"

[config.properties.database]
type = "object"
description = "Database settings"

[config.properties.database.properties.host]
type = "string"
description = "Database host"

[config.properties.database.properties.port]
type = "number"
description = "Database port"
```

## Troubleshooting

### Tool Not Appearing

1. Check `active = true`
2. Verify user is in allowed groups
3. Confirm `script_type = "tool"`
4. Check MCP server logs

### Parameters Not Received

1. Verify TOML schema is valid
2. Check parameter names match (case-sensitive)
3. Add debug output: `print(f"Received: {knot.mcp.get_string('param_name')}")` to verify parameter values

### JSON Parse Errors

```python
import json

try:
    data = json.loads(os.getenv('MCP_PARAM_config', '{}'))
except json.JSONDecodeError as e:
    print(f"Invalid JSON in config parameter: {e}", file=sys.stderr)
    sys.exit(1)
```

## Examples

See the Web UI for example MCP tools:
- `generate_calendar` - Date/time operations
- `generate_password` - Security utilities
- `greeting_tool` - Personalized greetings
- `database_backup` - System operations

## Additional Resources

- [Scriptling MCP Tool Helpers Documentation](https://github.com/paularlott/scriptling/blob/main/docs/libraries/scriptling/mcp.md)
- [Knot MCP Library Documentation](./scriptling-mcp-library.md)
- [MCP Specification](https://modelcontextprotocol.io/)
- [Scriptling Documentation](https://github.com/paularlott/scriptling)
- [Knot API Documentation](https://getknot.dev/docs/api/)
