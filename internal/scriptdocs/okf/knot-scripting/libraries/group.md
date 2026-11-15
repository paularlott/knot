---
type: "API Reference"
title: "knot.group"
description: "Group management functions for groups and their quotas."
resource: "https://getknot.dev/reference/libraries/group/"
tags:
  - "security"
  - "authentication"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/group/"
---
# knot.group

The `knot.group` library manages groups and their quotas. It has no membership functions: a user's groups are changed in the web UI (or the REST API), and read from the `groups` key of [`knot.user.get()`](../user/) (a list of group IDs). `list()` works for any user; the other functions need the **Manage Groups** permission.

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List all groups |
| `get(group_id)` | Get group by ID (UUID; use `list()` to find an ID by name) |
| `create(name, max_spaces=0, compute_units=0, storage_units=0, max_tunnels=0, file_storage_mb=0, max_buckets=0)` | Create a new group; returns its ID |
| `update(group_id, name=None, max_spaces=None, compute_units=None, storage_units=None, max_tunnels=None, file_storage_mb=None, max_buckets=None)` | Update group properties; arguments left as `None` keep their current values (including `file_storage_mb` and `max_buckets`). Returns `True` |
| `delete(group_id)` | Delete a group; returns `True` |

---

## Usage

```python
import knot.group as group

# List groups
groups = group.list()
for g in groups:
    print(f"{g['name']}")
```

---

## Group Properties

Groups contain:
- `id` - Group ID
- `name` - Group name
- `max_spaces` - Maximum spaces allowed
- `compute_units` - Compute units quota
- `storage_units` - Storage units quota
- `max_tunnels` - Maximum tunnels allowed
- `file_storage_mb` - File storage quota in MB
- `max_buckets` - Maximum file storage buckets
