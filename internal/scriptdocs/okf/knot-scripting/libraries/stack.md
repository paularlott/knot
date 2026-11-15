---
type: "API Reference"
title: "knot.stack"
description: "Stack template management and stack lifecycle functions."
resource: "https://getknot.dev/reference/libraries/stack/"
tags:
  - "stacks"
  - "templates"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/stack/"
---
# knot.stack

The `knot.stack` library provides stack template (definition) management and stack lifecycle functions. Stack templates are blueprints describing which spaces make up a stack and how they are wired together. When you create a stack from a template, spaces are created with the stack field set, dependencies resolved, and port forwards applied.

> The function names use the historical `_def` suffix (short for "definition") — these operate on what the web UI now calls **stack templates**. The two terms are interchangeable.

---

## Functions

### Template Management

| Function | Description |
|----------|-------------|
| `list_defs(include_inactive=False)` | List all stack templates visible to the current user (active ones unless `include_inactive=True`) |
| `get_def(name)` | Get a template by name or ID |
| `create_def(name, description="", icon_url="", scope="user", active=True, groups=None, zones=None, spaces=None)` | Create a new stack template; returns its ID |
| `update_def(name, **fields)` | Update an existing template (by name or ID); returns `True`. Only the keys you pass change: `name`, `description`, `icon_url`, `scope`, `active`, `groups`, `zones`, `spaces` |
| `delete_def(name)` | Delete a template; returns `True` |
| `validate_def(spaces, name="", description="", icon_url="", scope="user", active=True, groups=None, zones=None)` | Validate a template without creating it |
| `add_component(stack_definition, template, name, description="", shell="", startup_script="", depends_on=None)` | Add a component (template binding) to an existing template; returns `True` |
| `remove_component(stack_definition, name)` | Remove a component from a template by name (also dropping it from other components' `depends_on` and port forwards); returns `True` |

`create_def` arguments:
- `scope` - `"user"` for a personal template, or `"global"` for a system template (needs **Manage Stack Definitions**)
- `active` - Whether the template is offered for creating stacks
- `groups` - Group **IDs** allowed to create stacks from a global template (empty = everyone)
- `zones` - Zone restrictions; a `!` prefix excludes a zone
- `spaces` - Component dicts, as described in [Creating Templates](#creating-templates)

`add_component` arguments: `stack_definition` is the template's name or ID; `template` is the space template's name or ID (resolved for you); `name` must be unique within the template; `startup_script` must be a startup script **ID** (not a name — look it up with `knot.script.get_by_name()`); `depends_on` lists other component names.

### Stack Operations

| Function | Description |
|----------|-------------|
| `create(definition_name, prefix, stack_name=None)` | Create spaces from a stack template (by name or ID); returns `{"spaces": {component_name: space_id, ...}}` |
| `delete(stack_name, user_id=None)` | Delete all spaces in a stack |
| `start(stack_name, user_id=None)` | Start all spaces in a stack in dependency order |
| `stop(stack_name, user_id=None)` | Stop all spaces in reverse dependency order |
| `restart(stack_name, user_id=None)` | Restart all spaces in a stack |
| `exists(stack_name, user_id=None)` | `True` if at least one space belongs to the stack |
| `list()` | List stacks by grouping spaces by stack name |

The stack operations act on the current user's stack; `user_id` names another user's stack instead, which needs the **Manage Spaces** permission.

---

## Usage

```python
import knot.stack as stack

# List available templates
defs = stack.list_defs()
for d in defs:
    print(f"{d['name']}: {len(d['spaces'])} spaces")

# Get template details
defn = stack.get_def("lamp")
print(f"Template: {defn['name']}")
for comp in defn['spaces']:
    print(f"  {comp['name']}: {comp.get('template_id', 'unknown')}")

# Create a stack from a template
result = stack.create("lamp", "myproject")
for name, space_id in result['spaces'].items():
    print(f"  {name}: {space_id}")

# Start the stack
stack.start("myproject")

# List all stacks — each space dict has id, name and is_running
for s in stack.list():
    running = sum(1 for sp in s['spaces'] if sp['is_running'])
    print(f"{s['name']}: {running}/{len(s['spaces'])} running")

# Stop and delete
stack.stop("myproject")
stack.delete("myproject")
```

---

## Creating Templates

Each entry in `spaces` references a space template by **ID** (`template_id`) and, optionally, a startup script by ID (`startup_script_id`). Look the IDs up by name with `knot.template.get()` and `knot.script.get_by_name()`:

```python
import knot.stack as stack
import knot.template as template
import knot.script as script

mysql_id = template.get("mysql-8")["id"]      # get() accepts a name or ID
apache_id = template.get("apache-2.4")["id"]
init_id = script.get_by_name("init-db")["id"]

# Create a personal stack template
stack.create_def(
    "my-stack",
    description="My development stack",
    icon_url="/icons/my-stack.svg",
    scope="user",
    spaces=[
        {
            "name": "db",
            "template_id": mysql_id,
            "description": "MySQL database",
            "startup_script_id": init_id,
            "custom_fields": [
                {"name": "MYSQL_ROOT_PASSWORD", "value": "secret"},
                {"name": "MYSQL_DATABASE", "value": "appdb"},
            ],
            "port_forwards": [
                {"to_space": "web", "local_port": 3306, "remote_port": 3306},
            ],
        },
        {
            "name": "web",
            "template_id": apache_id,
            "depends_on": ["db"],
        },
    ],
)
```

Space entry keys:

| Key | Description |
|-----|-------------|
| `name` | Component name — spaces are created as `{prefix}-{name}`; also used in `depends_on` and `to_space` references |
| `template_id` | Space template ID (required) |
| `description` | Space description |
| `shell` | Shell override (`bash`, `zsh`, `fish`, `sh`) |
| `startup_script_id` | Startup script ID |
| `depends_on` | List of component names this space depends on |
| `custom_fields` | List of `{"name": ..., "value": ...}` dicts |
| `port_forwards` | List of `{"to_space": ..., "local_port": ..., "remote_port": ...}` dicts |

Alternatively, `add_component(stack_definition, template, name, ...)` resolves a template name to its ID for you.

---

## Validating Templates

Use `validate_def` to check a template for errors before creating it. This catches structural problems like circular dependencies, missing required fields, and invalid references.

```python
import knot.stack as stack

# Validate before creating
result = stack.validate_def(
    name="my-stack",
    spaces=[
        {"name": "db", "template_id": "template-uuid"},
        {"name": "web", "template_id": "template-uuid", "depends_on": ["db"]},
    ],
)

if result["valid"]:
    print("Template is valid")
else:
    for err in result.get("errors", []):
        if err.get("space"):
            print(f"  [{err['space']}] {err['field']}: {err['message']}")
        else:
            print(f"  {err['field']}: {err['message']}")
```

Validation checks for:
- Required fields (`name`, `template_id`)
- Duplicate space names
- Invalid `depends_on` references
- Circular dependencies
- Invalid `port_forwards.to_space` references
- Port number ranges (1-65535)

---

## Creating from a Template

```python
import knot.stack as stack

# Create spaces from the "lamp" template
# Spaces will be named: webapp-db, webapp-web
# and grouped under the stack name "webapp"
result = stack.create("lamp", "webapp")

# Or use a different stack name from the prefix
result = stack.create("lamp", "webapp", stack_name="my-lamp-stack")
```

`create` returns `{"spaces": {component_name: space_id, ...}}`. It applies each component's port forwards through the space's `/space-io/` endpoint, so inside AI scripts (`run_script`), which may only call `/api/` endpoints, creating a stack whose definition gives a component port forwards fails at that step, after the spaces are created (they are not removed).

---

## Template Properties

Templates returned by `get_def()` contain:
- `id` - Stack definition ID
- `name` - Template name
- `description` - Description
- `icon_url` - Icon URL
- `user_id` - Owner user ID (empty for global templates)
- `active` - Whether the template is available for creating stacks
- `groups` - List of group IDs allowed to create instances
- `zones` - List of zone restrictions
- `spaces` - List of space dicts, using the keys described in [Creating Templates](#creating-templates)

---

## Backward Compatibility

The `knot.space` functions `get_stack` and `set_stack` read and write the `Space.Stack` string field on individual spaces. They are complementary to this library — `knot.stack.create` sets the stack field automatically when creating spaces from a definition.
