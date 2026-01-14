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
# Import the MCP helper library
import knot.mcp

# Access parameters (automatically handles env vars and JSON parsing)
name = knot.mcp.get("name")
greeting_type = knot.mcp.get("greeting_type", "hello")  # with default

# Do your work
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return output
return knot.mcp.return_string(greeting)
```

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

The `knot.mcp` library handles all parameter access and type conversion:

```python
import knot.mcp

# String parameter
name = knot.mcp.get("name")

# Number parameter (automatically converted)
count = knot.mcp.get("count", 10)  # default: 10

# Boolean parameter (automatically converted)
enabled = knot.mcp.get("enabled", False)

# Array parameter (automatically parsed from JSON)
headers = knot.mcp.get("headers", [])
for header in headers:
    print(f"Header: {header}")

# Object parameter (automatically parsed from JSON)
config = knot.mcp.get("config", {})
retry = config.get('retry', False)
max_attempts = config.get('max_attempts', 3)
```

## Returning Results

### Using the MCP Library (Recommended)

```python
import knot.mcp

# Return a string (script ends after return)
return knot.mcp.return_string("Operation completed successfully")

# Return structured data (automatically converted to JSON)
result = {
    "status": "success",
    "records_processed": 42,
    "duration_ms": 1234
}
return knot.mcp.return_object(result)

# Return an error
if not url:
    return knot.mcp.return_error("URL parameter is required")
```

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
- **Timeout:** `10`

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
import knot.mcp

# Get parameters using MCP library
name = knot.mcp.get("name")
greeting_type = knot.mcp.get("greeting_type", "hello")

# Build greeting
greeting = f"{greeting_type.capitalize()}, {name}!"

# Return the greeting
return knot.mcp.return_string(greeting)
```

## Best Practices

### 1. Validate Input

Always validate required parameters:

```python
import knot.mcp

url = knot.mcp.get("url")
if not url:
    return knot.mcp.return_error("url parameter is required")
```

### 2. Provide Defaults

Use sensible defaults for optional parameters:

```python
import knot.mcp

timeout = knot.mcp.get("timeout", 30)
method = knot.mcp.get("method", "GET")
```

### 3. Handle Errors Gracefully

Catch exceptions and return helpful messages:

```python
import knot.mcp

try:
    # Your code
    pass
except ValueError as e:
    return knot.mcp.return_error(f"Invalid parameter: {e}")
except Exception as e:
    return knot.mcp.return_error(f"Unexpected error: {e}")
```

### 4. Keep Output Concise

AI context windows are limited. Truncate large outputs:

```python
import knot.mcp

if len(output) > 1000:
    output = output[:1000] + "... (truncated)"
return knot.mcp.return_string(output)
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

### 7. Set Appropriate Timeouts

Consider the operation's expected duration:
- Quick operations: 10-30 seconds
- API calls: 30-60 seconds
- Long-running tasks: 120+ seconds

## Testing Your Tool

MCP tools are designed to be executed by AI assistants through the Model Context Protocol:

1. The AI assistant discovers your tool using `tool_search`
2. The AI executes it using `execute_tool` with the appropriate parameters
3. Your tool receives parameters via `knot.mcp.get()` and returns results

This is the recommended way to test MCP tools as it exercises the full MCP integration.

## MCP Library Reference

### Parameter Access

The `knot.mcp` library provides a simple interface for accessing tool parameters:

- `knot.mcp.get("name")` - Get parameter value with automatic type conversion and JSON parsing
- `knot.mcp.get("name", default)` - Get parameter with default value if not provided

### Return Functions

- `knot.mcp.return_string(value)` - Return a string result
- `knot.mcp.return_object(value)` - Return a structured object (automatically converted to JSON)
- `knot.mcp.return_error(message)` - Return an error message

All return functions should be used with the `return` statement to properly end script execution.

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
3. Add debug output: `print(f"Received: {knot.mcp.get('param_name')}")` to verify parameter values

### Timeout Errors

1. Increase timeout value in script settings
2. Optimize script performance
3. Consider breaking into smaller operations

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

## MCP Library Reference

The `knot.mcp` library is automatically available in all MCP tool scripts.

### Functions

#### `knot.mcp.get(name, default=None)`
Get a parameter value with automatic type conversion.

```python
# String
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

#### `knot.mcp.return_string(text)`
Return a text response.

```python
knot.mcp.return_string("Operation completed")
```

#### `knot.mcp.return_object(obj)`
Return a structured object (automatically converted to JSON).

```python
knot.mcp.return_object({
    "status": "success",
    "count": 42
})
```

#### `knot.mcp.return_error(message)`
Return an error message and exit.

```python
if not valid:
    knot.mcp.return_error("Invalid input")
```

### Implementation Details

The MCP library:
- Reads from `MCP_PARAM_<name>` environment variables
- Automatically parses JSON for arrays and objects
- Converts string numbers to int/float
- Converts string booleans to bool
- Handles missing parameters with defaults
- Formats output appropriately

## Additional Resources

- [MCP Specification](https://modelcontextprotocol.io/)
- [Scriptling Documentation](https://github.com/paularlott/scriptling)
- [Knot API Documentation](https://getknot.dev/docs/api/)
