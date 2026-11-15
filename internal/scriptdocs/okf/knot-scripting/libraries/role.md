---
type: "API Reference"
title: "knot.role"
description: "Role management functions for creating roles and assigning permissions."
resource: "https://getknot.dev/reference/libraries/role/"
tags:
  - "security"
  - "authentication"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/role/"
---
# knot.role

The `knot.role` library provides role management functions.

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List all roles |
| `get(role_id)` | Get role by ID (UUID; use `list()` to find an ID by name) |
| `create(name, permissions=None, plugin_permissions=None)` | Create a new role — built-in permission IDs and/or qualified plugin grant strings |
| `update(role_id, name=None, permissions=None, plugin_permissions=None)` | Update role properties; omitted lists keep their current values |
| `delete(role_id)` | Delete a role |

---

## Usage

```python
import knot.role as role

# List roles
roles = role.list()
for r in roles:
    print(f"{r['id']}: {r['name']}")

# Get role details with permissions
r = role.get(roles[0]['id'])
print(f"{r['name']}: {r['permissions']}")
```

---

## Role Properties

`list()` returns:
- `id` - Role ID
- `name` - Role name

`get()` also includes:
- `permissions` - List of permission IDs (integers)
- `plugin_permissions` - List of qualified plugin grant strings (e.g. `plugin.metrics.read`)
