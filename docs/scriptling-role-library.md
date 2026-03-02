# Scriptling Role Library

The `knot.role` library provides role management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

Roles are collections of permissions that can be assigned to users. Roles simplify permission management by allowing you to define permission sets once and assign them to multiple users.

## Available Functions

| Function | Description |
|----------|-------------|
| `list()` | List all roles |
| `get(role_id)` | Get role by ID (UUID only) |
| `create(name, permissions=[])` | Create a new role |
| `update(role_id, ...)` | Update role properties |
| `delete(role_id)` | Delete a role |

## Usage

```python
import knot.role
import knot.permission as perm

# List all roles
roles = knot.role.list()
for r in roles:
    print(f"{r['name']}: {len(r['permissions'])} permissions")

# Create a new role
role_id = knot.role.create("Developer", [
    perm.USE_SPACES,
    perm.USE_SSH,
    perm.RUN_COMMANDS,
])

# Update role permissions
knot.role.update(role_id, permissions=[
    perm.USE_SPACES,
    perm.USE_SSH,
    perm.RUN_COMMANDS,
    perm.COPY_FILES,
])
```

## Functions

### list()

List all roles.

**Parameters:** None

**Returns:**

- `list`: List of role objects, each containing:
  - `id` (string): Role ID
  - `name` (string): Role name
  - `permissions` (list): List of permission IDs

**Example:**

```python
import knot.role

# List all roles
roles = knot.role.list()

print(f"Total roles: {len(roles)}")
for role in roles:
    print(f"- {role['name']}: {len(role['permissions'])} permissions")
```

---

### get(role_id)

Get a role by ID (UUID only - roles cannot be looked up by name).

**Parameters:**

- `role_id` (string): Role UUID

**Returns:**

- `dict`: Role object containing:
  - `id` (string): Role UUID
  - `name` (string): Role name
  - `permissions` (list): List of permission IDs

**Example:**

```python
import knot.role

# Get role by UUID
role = knot.role.get("550e8400-e29b-41d4-a716-446655440000")
print(f"Role: {role['name']}")
print(f"Permissions: {role['permissions']}")
```

---

### create(name, permissions=[])

Create a new role.

**Parameters:**

- `name` (string): Role name
- `permissions` (list, optional): List of permission IDs (default: [])

**Returns:**

- `string`: The ID of the newly created role

**Example:**

```python
import knot.role
import knot.permission as perm

# Create a role with no permissions
role_id = knot.role.create("Custom Role")
print(f"Created role: {role_id}")

# Create a Developer role with permissions
dev_perms = [
    perm.USE_SPACES,
    perm.USE_SSH,
    perm.USE_WEB_TERMINAL,
    perm.RUN_COMMANDS,
    perm.COPY_FILES,
]
role_id = knot.role.create("Developer", dev_perms)
print(f"Created Developer role: {role_id}")

# Create an Admin role
admin_perms = [
    perm.MANAGE_USERS,
    perm.MANAGE_SPACES,
    perm.MANAGE_TEMPLATES,
    perm.VIEW_AUDIT_LOGS,
]
role_id = knot.role.create("Admin", admin_perms)
print(f"Created Admin role: {role_id}")
```

---

### update(role_id, ...)

Update a role's properties.

**Parameters:**

- `role_id` (string): Role UUID

**Optional Keyword Arguments:**

- `name` (string): New role name
- `permissions` (list): New list of permission IDs

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.role
import knot.permission as perm

# Update role name (use UUID)
knot.role.update("550e8400-e29b-41d4-a716-446655440000", name="Developer")

# Add a permission to a role (use UUID)
role = knot.role.get("550e8400-e29b-41d4-a716-446655440000")
new_perms = role['permissions'] + [perm.COPY_FILES]
knot.role.update("550e8400-e29b-41d4-a716-446655440000", permissions=new_perms)

# Completely replace permissions (use UUID)
new_perms = [
    perm.USE_SPACES,
    perm.USE_SSH,
    perm.RUN_COMMANDS,
    perm.COPY_FILES,
    perm.USE_CODE_SERVER,
]
knot.role.update("550e8400-e29b-41d4-a716-446655440000", permissions=new_perms)
```

---

### delete(role_id)

Delete a role.

**Parameters:**

- `role_id` (string): Role UUID

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.role

# Delete a role (use UUID)
if knot.role.delete("550e8400-e29b-41d4-a716-446655440000"):
    print("Role deleted successfully")
```

---

## Usage Examples

### Example 1: Setting Up Standard Roles

```python
import knot.role
import knot.permission as perm

def setup_standard_roles():
    """Create standard roles for the organization"""

    # Admin role - full system access
    admin_perms = [
        perm.MANAGE_USERS,
        perm.MANAGE_GROUPS,
        perm.MANAGE_ROLES,
        perm.MANAGE_SPACES,
        perm.MANAGE_TEMPLATES,
        perm.MANAGE_VOLUMES,
        perm.MANAGE_VARIABLES,
        perm.VIEW_AUDIT_LOGS,
        perm.CLUSTER_INFO,
    ]
    admin_role = knot.role.create("Admin", admin_perms)
    print(f"Created Admin role: {admin_role}")

    # Developer role - development tools
    dev_perms = [
        perm.USE_SPACES,
        perm.TRANSFER_SPACES,
        perm.SHARE_SPACES,
        perm.USE_TUNNELS,
        perm.USE_VNC,
        perm.USE_WEB_TERMINAL,
        perm.USE_SSH,
        perm.USE_CODE_SERVER,
        perm.USE_VSCODE_TUNNEL,
        perm.USE_LOGS,
        perm.RUN_COMMANDS,
        perm.COPY_FILES,
        perm.EXECUTE_SCRIPTS,
        perm.EXECUTE_OWN_SCRIPTS,
    ]
    dev_role = knot.role.create("Developer", dev_perms)
    print(f"Created Developer role: {dev_role}")

    # Viewer role - read-only access
    viewer_perms = [
        perm.USE_SPACES,
        perm.USE_LOGS,
    ]
    viewer_role = knot.role.create("Viewer", viewer_perms)
    print(f"Created Viewer role: {viewer_role}")

    return {
        "admin": admin_role,
        "developer": dev_role,
        "viewer": viewer_role,
    }

roles = setup_standard_roles()
```

### Example 2: Permission Builder

```python
import knot.role
import knot.permission as perm

def create_custom_role(name, capabilities):
    """Create a role based on desired capabilities"""

    # Map capabilities to permissions
    capability_map = {
        "spaces_manage": perm.MANAGE_SPACES,
        "spaces_use": perm.USE_SPACES,
        "spaces_transfer": perm.TRANSFER_SPACES,
        "spaces_share": perm.SHARE_SPACES,
        "ssh": perm.USE_SSH,
        "terminal": perm.USE_WEB_TERMINAL,
        "vnc": perm.USE_VNC,
        "commands": perm.RUN_COMMANDS,
        "files": perm.COPY_FILES,
        "templates_manage": perm.MANAGE_TEMPLATES,
        "users_manage": perm.MANAGE_USERS,
        "audit": perm.VIEW_AUDIT_LOGS,
        "scripts_execute": perm.EXECUTE_SCRIPTS,
    }

    # Build permissions list
    permissions = []
    for cap in capabilities:
        if cap in capability_map:
            permissions.append(capability_map[cap])

    # Create the role
    role_id = knot.role.create(name, permissions)
    print(f"Created role '{name}' with {len(permissions)} permissions")

    return role_id

# Create custom roles
create_custom_role("DevOps", [
    "spaces_use",
    "spaces_transfer",
    "ssh",
    "terminal",
    "commands",
    "files",
    "templates_manage",
])

create_custom_role("Manager", [
    "spaces_use",
    "spaces_transfer",
    "spaces_share",
    "audit",
])
```

### Example 3: Role Audit and Comparison

```python
import knot.role
import knot.permission as perm

def compare_roles(role1_name, role2_name):
    """Compare permissions between two roles"""

    role1 = knot.role.get(role1_name)
    role2 = knot.role.get(role2_name)

    perms1 = set(role1['permissions'])
    perms2 = set(role2['permissions'])

    only_in_role1 = perms1 - perms2
    only_in_role2 = perms2 - perms1
    common = perms1 & perms2

    print(f"Comparing '{role1_name}' vs '{role2_name}':")
    print(f"\nOnly in {role1_name}: {len(only_in_role1)}")
    print(f"Only in {role2_name}: {len(only_in_role2)}")
    print(f"Common: {len(common)}")

    return {
        "only_in_role1": only_in_role1,
        "only_in_role2": only_in_role2,
        "common": common,
    }

def audit_role_permissions():
    """Audit all roles and their permissions"""

    roles = knot.role.list()

    print(f"{'Role':<20} {'Permissions':<10}")
    print("-" * 30)

    for role in roles:
        print(f"{role['name']:<20} {len(role['permissions']):<10}")

        # Show permission names
        for perm_id in role['permissions']:
            # You could look up permission names here
            pass

# Usage
compare_roles("Admin", "Developer")
audit_role_permissions()
```

### Example 4: Dynamic Role Management

```python
import knot.role
import knot.permission as perm
import knot.user

def grant_role_to_user(username, role_name):
    """Grant a role to a user"""

    # Get user and role
    user = knot.user.get(username)
    role = knot.role.get(role_name)

    # Add role to user if not already assigned
    if role['id'] not in user['roles']:
        user['roles'].append(role['id'])
        knot.user.update(username, roles=user['roles'])
        print(f"Granted role '{role_name}' to {username}")
    else:
        print(f"{username} already has role '{role_name}'")

def revoke_role_from_user(username, role_name):
    """Revoke a role from a user"""

    user = knot.user.get(username)
    role = knot.role.get(role_name)

    if role['id'] in user['roles']:
        user['roles'].remove(role['id'])
        knot.user.update(username, roles=user['roles'])
        print(f"Revoked role '{role_name}' from {username}")
    else:
        print(f"{username} doesn't have role '{role_name}'")

def list_users_with_role(role_name):
    """List all users with a specific role"""

    role = knot.role.get(role_name)
    users = knot.user.list()

    members = [u['username'] for u in users if role['id'] in u['roles']]

    print(f"Users with role '{role_name}':")
    for member in members:
        print(f"  - {member}")

    return members

# Usage
grant_role_to_user("alice", "Developer")
grant_role_to_user("bob", "Developer")
list_users_with_role("Developer")
revoke_role_from_user("alice", "Developer")
```

### Example 5: Role Template System

```python
import knot.role
import knot.permission as perm

# Define role templates
ROLE_TEMPLATES = {
    "Administrator": {
        "description": "Full system access",
        "permissions": [
            perm.MANAGE_USERS,
            perm.MANAGE_GROUPS,
            perm.MANAGE_ROLES,
            perm.MANAGE_SPACES,
            perm.MANAGE_TEMPLATES,
            perm.MANAGE_VOLUMES,
            perm.VIEW_AUDIT_LOGS,
        ]
    },
    "Developer": {
        "description": "Development team access",
        "permissions": [
            perm.USE_SPACES,
            perm.USE_SSH,
            perm.USE_WEB_TERMINAL,
            perm.RUN_COMMANDS,
            perm.COPY_FILES,
            perm.EXECUTE_SCRIPTS,
        ]
    },
    "Analyst": {
        "description": "Data analysis access",
        "permissions": [
            perm.USE_SPACES,
            perm.USE_CODE_SERVER,
            perm.EXECUTE_OWN_SCRIPTS,
        ]
    },
    "ReadOnly": {
        "description": "Read-only access",
        "permissions": [
            perm.USE_SPACES,
            perm.USE_LOGS,
        ]
    },
}

def create_role_from_template(template_name, custom_name=None):
    """Create a role from a predefined template"""

    if template_name not in ROLE_TEMPLATES:
        print(f"Template '{template_name}' not found")
        return None

    template = ROLE_TEMPLATES[template_name]
    role_name = custom_name or template_name

    role_id = knot.role.create(role_name, template['permissions'])
    print(f"Created role '{role_name}' from template '{template_name}'")
    print(f"Description: {template['description']}")

    return role_id

def list_templates():
    """List all available role templates"""

    print("Available role templates:")
    for name, template in ROLE_TEMPLATES.items():
        print(f"  - {name}: {template['description']}")
        print(f"    Permissions: {len(template['permissions'])}")

# Usage
list_templates()
create_role_from_template("Developer")
create_role_from_template("Developer", "Frontend Developer")
create_role_from_template("ReadOnly", "Auditor")
```

---

## Notes

### Roles are UUID-only

Roles can only be accessed by their UUID, not by name. To get a role by name, you must:

1. List all roles using `knot.role.list()`
2. Find the role with the desired name
3. Use the role's UUID for subsequent operations

```python
import knot.role

# Find role by name
roles = knot.role.list()
dev_role = next((r for r in roles if r['name'] == 'Developer'), None)

if dev_role:
    # Now use the UUID
    knot.role.update(dev_role['id'], permissions=[...])
```

### Assigning Roles to Users

Roles are assigned to users by updating the user's `roles` list:

```python
import knot.user
import knot.role

# Get user and role
user = knot.user.get("username")
role = knot.role.get("Developer")

# Add role to user's roles
if role['id'] not in user['roles']:
    user['roles'].append(role['id'])
    knot.user.update("username", roles=user['roles'])
```

### Permission Inheritance

Users get permissions from:
1. Direct permissions assigned to the user
2. Permissions from all roles assigned to the user
3. Permissions from all groups the user is a member of

Use `knot.user.list_permissions()` to see all permissions a user has.

### Deleting Roles

When you delete a role:
- The role is removed from the system
- Users who had the role will have the role ID removed from their `roles` list automatically

---

## Related Libraries

- **knot.user** - For user management and role assignment
- **knot.permission** - For permission constants
- **knot.group** - For group management
