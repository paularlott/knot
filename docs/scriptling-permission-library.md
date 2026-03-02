# Scriptling Permission Library

The `knot.permission` library provides permission constants and functions for checking user permissions in Knot. This library is available in all three environments (Local, MCP, and Remote).

## Overview

The permission library exposes permission integer constants that can be used with `knot.user.list_permissions()` and `knot.user.has_permission()` to check what actions a user is authorized to perform.

## Available Constants

| Constant | Description |
|----------|-------------|
| **User Management** | |
| `MANAGE_USERS` | Permission to manage users (0) |
| `MANAGE_GROUPS` | Permission to manage groups (4) |
| `MANAGE_ROLES` | Permission to manage roles (5) |
| **Resource Management** | |
| `MANAGE_SPACES` | Permission to manage spaces (2) |
| `MANAGE_TEMPLATES` | Permission to manage templates (1) |
| `MANAGE_VOLUMES` | Permission to manage volumes (3) |
| `MANAGE_VARIABLES` | Permission to manage template variables (6) |
| **Space Operations** | |
| `USE_SPACES` | Permission to use spaces (7) |
| `TRANSFER_SPACES` | Permission to transfer spaces (10) |
| `SHARE_SPACES` | Permission to share spaces (11) |
| `USE_TUNNELS` | Permission to use tunnels (8) |
| **System & Audit** | |
| `VIEW_AUDIT_LOGS` | Permission to view audit logs (9) |
| `CLUSTER_INFO` | Permission to view cluster info (12) |
| **Space Features** | |
| `USE_VNC` | Permission to use VNC (13) |
| `USE_WEB_TERMINAL` | Permission to use web terminal (14) |
| `USE_SSH` | Permission to use SSH connections (15) |
| `USE_CODE_SERVER` | Permission to use code-server (16) |
| `USE_VSCODE_TUNNEL` | Permission to use VSCode tunnel (17) |
| `USE_LOGS` | Permission to view logs (18) |
| `RUN_COMMANDS` | Permission to run commands (19) |
| `COPY_FILES` | Permission to copy files (20) |
| **AI Tools** | |
| `USE_MCP_SERVER` | Permission to use MCP server (21) |
| `USE_WEB_ASSISTANT` | Permission to use web AI assistant (22) |
| **Scripting** | |
| `MANAGE_SCRIPTS` | Permission to manage system/global scripts (23) |
| `EXECUTE_SCRIPTS` | Permission to execute system/global scripts (24) |
| `MANAGE_OWN_SCRIPTS` | Permission to manage own scripts (25) |
| `EXECUTE_OWN_SCRIPTS` | Permission to execute own scripts (26) |
| **Aliases** | |
| `SPACE_MANAGE` | Alias for `MANAGE_SPACES` |
| `SPACE_USE` | Alias for `USE_SPACES` |
| `SCRIPT_MANAGE` | Alias for `MANAGE_SCRIPTS` |
| `SCRIPT_EXECUTE` | Alias for `EXECUTE_SCRIPTS` |

## Functions

### list()

List all available permissions with their IDs, names, and groups.

**Parameters:** None

**Returns:**

- `list`: List of permission objects, each containing:
  - `id` (int): Permission ID
  - `name` (string): Permission name
  - `group` (string): Permission group

**Example:**

```python
import knot.permission

# List all permissions
permissions = knot.permission.list()
for perm in permissions:
    print(f"{perm['id']}: {perm['name']} ({perm['group']})")
```

## Usage Examples

### Example 1: Check Permission Before Action

```python
import knot.permission as perm
import knot.space as space

# Get current user
user = knot.user.get_me()

# Check if user can manage spaces before creating one
permissions = knot.user.list_permissions(user["id"])
if perm.MANAGE_SPACES in permissions:
    space.create("my-space", "ubuntu-22.04")
else:
    print("You don't have permission to manage spaces")
```

### Example 2: Comprehensive Permission Check

```python
import knot.permission as perm
import knot.user as user

# Get all permissions for current user
me = knot.user.get_me()
my_permissions = user.list_permissions(me["id"])

# Check various permissions
checks = [
    ("Manage Spaces", perm.MANAGE_SPACES),
    ("Use Spaces", perm.USE_SPACES),
    ("Transfer Spaces", perm.TRANSFER_SPACES),
    ("Share Spaces", perm.SHARE_SPACES),
    ("Run Commands", perm.RUN_COMMANDS),
    ("Copy Files", perm.COPY_FILES),
]

print(f"My Permissions:")
for perm_id in my_permissions:
    for name, id_val in checks:
        if id_val == perm_id:
            print(f"  ✓ {name}")

# Check specific permission
if perm.MANAGE_TEMPLATES in my_permissions:
    print("I can manage templates")
else:
    print("I cannot manage templates")
```

### Example 3: Role-Based Access Control

```python
import knot.permission as perm
import knot.user as user
import knot.role as role

# Get all permissions
me = knot.user.get_me()
permissions = knot.user.list_permissions(me["id"])

# Check if user has admin-like permissions
admin_permissions = [
    perm.MANAGE_USERS,
    perm.MANAGE_SPACES,
    perm.MANAGE_TEMPLATES,
    perm.VIEW_AUDIT_LOGS,
]

has_admin = all(p in permissions for p in admin_permissions)
if has_admin:
    print("User has admin-level permissions")
else:
    print("User has limited permissions")
```

### Example 4: Create a Role with Specific Permissions

```python
import knot.permission as perm
import knot.role as role

# Define permissions for a "Developer" role
developer_perms = [
    perm.USE_SPACES,
    perm.USE_SSH,
    perm.USE_WEB_TERMINAL,
    perm.RUN_COMMANDS,
    perm.COPY_FILES,
]

# Create the role
role_id = role.create(
    name="Developer",
    permissions=developer_perms
)
print(f"Created role: {role_id}")
```

### Example 5: Filter Users by Permission

```python
import knot.permission as perm
import knot.user as user

# Find all users who can manage spaces
all_users = knot.user.list()

space_managers = []
for u in all_users:
    permissions = knot.user.list_permissions(u["id"])
    if perm.MANAGE_SPACES in permissions:
        space_managers.append(u["username"])

print(f"Users with space management permission: {space_managers}")
```

---

## Related Libraries

- **knot.user** - For user management and permission checking
- **knot.role** - For role management with permissions
- **knot.space** - For space operations (requires permissions)
