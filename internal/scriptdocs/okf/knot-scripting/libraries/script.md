---
type: "API Reference"
title: "knot.script"
description: "Script management and execution functions for stored and inline scripts."
resource: "https://getknot.dev/reference/libraries/script/"
tags:
  - "scripting"
  - "api"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/script/"
---
# knot.script

The `knot.script` library provides script management and script execution functions.

---

## Functions

| Function | Description |
|----------|-------------|
| `list(owner=None, all_zones=False, include_inactive=False, script_type=None)` | List scripts visible to the current user; `owner="current"` limits to your own scripts, inactive scripts are skipped unless `include_inactive=True`, and `script_type` filters by type (e.g. `"tool"`) |
| `list_global(all_zones=False)` | List global scripts available for template editing |
| `get(script_id)` | Get script details by UUID |
| `get_by_name(name)` | Get script details by name (your own script shadows a global script of the same name) |
| `get_content(name, script_type='script')` | Get script content (a string) by name and type |
| `create(name, content, description='', owner=None, groups=None, zones=None, active=True, script_type='script', mcp_input_schema_toml='', mcp_keywords=None, discoverable=False)` | Create a script and return its ID; `owner="current"` creates an own script, omitted creates a global script |
| `update(script_id, name=None, content=None, description=None, groups=None, zones=None, active=None, script_type=None, mcp_input_schema_toml=None, mcp_keywords=None, discoverable=None)` | Update a script, keeping fields you do not pass; returns `True` |
| `delete(script_id)` | Delete a script; returns `True` |
| `execute(space_name, script_name=None, script_id=None, content=None, args=None)` | Execute a script in a running space |
| `execute_content(space_name, content, args=None)` | Execute inline script content in a running space |

---

## Usage

```python
import knot.script as script

# List visible scripts
for s in script.list():
    print(f"{s['name']}: {s['script_type']}")

# Create an own script
script_id = script.create(
    "hello",
    "print('hello from script')",
    description="Example script",
    owner="current",
)

# Execute a named script in a running space
result = script.execute("my-space", script_name="hello", args=["--verbose"])
print(result["output"])
```

---

## Script Properties

Scripts contain:
- `id` - Script ID
- `user_id` - Owner user ID, or empty for global scripts
- `name` - Script name
- `description` - Description
- `content` - Script content, returned by detail functions
- `groups` - Group IDs that can access a global script
- `zones` - Zone restrictions
- `active` - Whether the script is active
- `script_type` - Script type (`script`, `lib`, or `tool`)
- `mcp_input_schema_toml` - MCP tool input schema TOML for tool scripts
- `mcp_keywords` - Keywords used for MCP tool discovery
- `discoverable` - Whether an MCP tool script is available through discovery rather than native tool listing
- `is_managed` - Whether the script is managed by the system

---

## Creating Scripts

```python
import knot.script as script

# Global script
global_id = script.create(
    "backup",
    "print('backup')",
    description="Run backup",
)

# Own script
own_id = script.create(
    "my-helper",
    "print('helper')",
    owner="current",
)

# MCP tool script
tool_id = script.create(
    "check_status",
    "import scriptling.mcp.tool as tool\ntool.return_string('ok')",
    script_type="tool",
    mcp_keywords=["status", "check"],
    discoverable=True,
)
```

---

## Execution

`execute()` needs one of `script_id`, `script_name`, or `content`; if more than one is given, they are used in that order of precedence (`script_id` first).

```python
import knot.script as script

# By script name
script.execute("my-space", script_name="deploy", args=["prod"])

# By script ID
script.execute("my-space", script_id="script-uuid")

# Inline content
script.execute_content("my-space", "print('hello')")
```

Execution returns:
- `output` - Script output
- `error` - Error text if the script failed
- `exit_code` - Script exit code
