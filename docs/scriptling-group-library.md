# Scriptling Group Library

The `knot.group` library provides group management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

Groups allow you to organize users and apply permissions collectively. Users can be members of multiple groups, and groups can be assigned roles.

## Available Functions

| Function | Description |
|----------|-------------|
| `list()` | List all groups |
| `get(group_id)` | Get group by ID (UUID only) |
| `create(name, description='')` | Create a new group |
| `update(group_id, ...)` | Update group properties |
| `delete(group_id)` | Delete a group |

## Usage

```python
import knot.group

# List all groups
groups = knot.group.list()
for g in groups:
    print(f"{g['name']}: {g.get('description', '')}")

# Create a new group
group_id = knot.group.create("developers", "Development team")

# Update group description
knot.group.update(group_id, description="All developers")
```

## Functions

### list()

List all groups.

**Parameters:** None

**Returns:**

- `list`: List of group objects, each containing:
  - `id` (string): Group UUID
  - `name` (string): Group name
  - `max_spaces` (int): Maximum spaces
  - `compute_units` (int): Compute units
  - `storage_units` (int): Storage units

**Example:**

```python
import knot.group

# List all groups
groups = knot.group.list()

print(f"Total groups: {len(groups)}")
for group in groups:
    print(f"- {group['name']}")
```

---

### get(group_id)

Get a group by ID (UUID only - groups cannot be looked up by name).

**Parameters:**

- `group_id` (string): Group UUID

**Returns:**

- `dict`: Group object containing:
  - `id` (string): Group UUID
  - `name` (string): Group name
  - `max_spaces` (int): Maximum spaces
  - `compute_units` (int): Compute units
  - `storage_units` (int): Storage units
  - `max_tunnels` (int): Maximum tunnels

**Example:**

```python
import knot.group

# Get group by UUID
group = knot.group.get("550e8400-e29b-41d4-a716-446655440000")
print(f"Group: {group['name']}")
```

---

### create(name, description='')

Create a new group.

**Parameters:**

- `name` (string): Group name
- `description` (string, optional): Group description (default: "")

**Returns:**

- `string`: The UUID of the newly created group

**Example:**

```python
import knot.group

# Create a basic group
group_id = knot.group.create("admins")
print(f"Created group: {group_id}")

# Create a group with description
group_id = knot.group.create(
    name="developers",
    description="Development team members"
)
print(f"Created group: {group_id}")
```

---

### update(group_id, ...)

Update a group's properties.

**Parameters:**

- `group_id` (string): Group UUID

**Optional Keyword Arguments:**

- `name` (string): New group name
- `description` (string): New group description

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.group

# Update group description (use UUID)
knot.group.update("550e8400-e29b-41d4-a716-446655440000", description="All software developers")

# Update group name and description
knot.group.update(
    "550e8400-e29b-41d4-a716-446655440000",
    name="developers",
    description="Development team"
)
```

---

### delete(group_id)

Delete a group.

**Parameters:**

- `group_id` (string): Group UUID

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.group

# Delete a group (use UUID)
if knot.group.delete("550e8400-e29b-41d4-a716-446655440000"):
    print("Group deleted successfully")
```

---

## Usage Examples

### Example 1: Setting Up Team Groups

```python
import knot.group
import knot.user
import knot.role as role
import knot.permission as perm

def setup_team_structure():
    """Set up groups for different teams"""

    # Create team groups
    teams = [
        ("developers", "Software development team"),
        ("designers", "UI/UX designers"),
        ("ops", "Operations and DevOps"),
        ("managers", "Team managers and leads"),
    ]

    created_groups = {}
    for name, desc in teams:
        # Check if group exists
        groups = knot.group.list()
        existing = next((g for g in groups if g['name'] == name), None)

        if existing:
            print(f"Group '{name}' already exists")
            created_groups[name] = existing['id']
        else:
            group_id = knot.group.create(name, desc)
            print(f"Created group: {name}")
            created_groups[name] = group_id

    return created_groups

groups = setup_team_structure()
```

### Example 2: Group User Management

```python
import knot.group
import knot.user

def add_user_to_group(username, group_id):
    """Add a user to a group"""
    # Note: This is done by updating the user's groups list

    # Get the user
    user = knot.user.get(username)

    # Add group to user if not already a member
    if group_id not in user['groups']:
        user['groups'].append(group_id)
        knot.user.update(username, groups=user['groups'])
        print(f"Added {username} to group")
    else:
        print(f"{username} is already a member of this group")

def remove_user_from_group(username, group_id):
    """Remove a user from a group"""

    user = knot.user.get(username)

    if group_id in user['groups']:
        user['groups'].remove(group_id)
        knot.user.update(username, groups=user['groups'])
        print(f"Removed {username} from group")
    else:
        print(f"{username} is not a member of this group")

def list_group_members(group_id):
    """List all users in a group"""

    users = knot.user.list()

    members = []
    for user in users:
        if group_id in user['groups']:
            members.append(user['username'])

    print(f"Group members: {', '.join(members)}")

    return members

# Usage - first create a group and get its UUID
group_id = knot.group.create("developers", "Development team")
add_user_to_group("alice", group_id)
add_user_to_group("bob", group_id)
list_group_members(group_id)
remove_user_from_group("alice", group_id)
```

### Example 3: Group-Based Permissions

```python
import knot.group
import knot.role as role
import knot.permission as perm

def setup_developer_permissions():
    """Create a role with developer permissions for a group"""

    # Define developer permissions
    dev_permissions = [
        perm.USE_SPACES,
        perm.USE_SSH,
        perm.USE_WEB_TERMINAL,
        perm.RUN_COMMANDS,
        perm.COPY_FILES,
        perm.USE_CODE_SERVER,
        perm.USE_VSCODE_TUNNEL,
    ]

    # Create the role
    role_id = role.create(
        name="Developer",
        permissions=dev_permissions
    )
    print(f"Created Developer role: {role_id}")

    # Create or get the developers group
    groups = knot.group.list()
    dev_group = next((g for g in groups if g['name'] == 'developers'), None)

    if not dev_group:
        dev_group_id = knot.group.create("developers", "Development team")
        print(f"Created developers group: {dev_group_id}")
    else:
        dev_group_id = dev_group['id']
        print(f"Using existing developers group: {dev_group_id}")

    return role_id, dev_group_id

setup_developer_permissions()
```

### Example 4: Group Report

```python
import knot.group
import knot.user

def generate_group_report():
    """Generate a report of all groups and their members"""

    groups = knot.group.list()
    users = knot.user.list()

    print(f"{'Group':<20} {'Description':<40} {'Members':<10}")
    print("-" * 70)

    for group in groups:
        # Count members
        member_count = sum(1 for u in users if group['id'] in u['groups'])
        print(f"{group['name']:<20} {group.get('description', ''):<40} {member_count:<10}")

    print("-" * 70)
    print(f"Total groups: {len(groups)}")
    print(f"Total users: {len(users)}")

generate_group_report()
```

### Example 5: Group Management Workflow

```python
import knot.group
import knot.user

def manage_group_lifecycle():
    """Complete group management workflow"""

    # Create a new group
    group_id = knot.group.create(
        name="qa-team",
        description="Quality Assurance team"
    )
    print(f"Created group: {group_id}")

    # Get group details
    group = knot.group.get(group_id)
    print(f"Group: {group['name']}")

    # Add users to the group
    qa_users = ["tester1", "tester2", "qa-lead"]
    for username in qa_users:
        try:
            user = knot.user.get(username)
            if group_id not in user['groups']:
                user['groups'].append(group_id)
                knot.user.update(username, groups=user['groups'])
                print(f"Added {username} to group")
        except Exception as e:
            print(f"Failed to add {username}: {e}")

    # Update group description
    knot.group.update(
        group_id,
        description="QA team for manual and automated testing"
    )
    print("Updated group description")

    # List members
    users = knot.user.list()
    members = [u['username'] for u in users if group_id in u['groups']]
    print(f"Group members: {', '.join(members)}")

    return group_id

# manage_group_lifecycle()
```

---

## Notes

### Groups are UUID-only

Groups can only be accessed by their UUID, not by name. To get a group by name, you must:

1. List all groups using `knot.group.list()`
2. Find the group with the desired name
3. Use the group's UUID for subsequent operations

```python
import knot.group

# Find group by name
groups = knot.group.list()
dev_group = next((g for g in groups if g['name'] == 'developers'), None)

if dev_group:
    # Now use the UUID
    knot.group.update(dev_group['id'], description="Updated description")
```

### Adding Users to Groups

Users are added to groups by updating the user's `groups` list with the group UUID:

```python
import knot.user
import knot.group

# Get user and group
user = knot.user.get("username")
group = knot.group.get("550e8400-e29b-41d4-a716-446655440000")

# Add group to user's groups
if group['id'] not in user['groups']:
    user['groups'].append(group['id'])
    knot.user.update("username", groups=user['groups'])
```

### Group-Based Permissions

Groups themselves don't have permissions directly. Instead:

1. Create a **role** with the desired permissions (see `knot.role`)
2. Assign the role to users who are members of the group
3. Or use roles that are designed for group membership

### Deleting Groups

When you delete a group:
- The group is removed from the system
- Users who were members will have the group UUID removed from their `groups` list automatically

---

## Related Libraries

- **knot.user** - For user management (including group membership)
- **knot.role** - For role management with permissions
- **knot.permission** - For permission constants
