# Scriptling User Library

The `knot.user` library provides user management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

The user library allows you to manage users in Knot, including creating, updating, deleting users, checking permissions, and managing user quotas.

## Available Functions

| Function | Description |
|----------|-------------|
| `get_me()` | Get current user details as a dict |
| `get(user_id)` | Get user by ID or username |
| `list(state='', zone='')` | List all users with optional state/zone filter |
| `create(username, email, password, ...)` | Create a new user |
| `update(user_id, ...)` | Update user properties |
| `delete(user_id)` | Delete a user |
| `get_quota(user_id)` | Get user quota and usage as a dict |
| `list_permissions(user_id)` | List all permissions for a user |
| `has_permission(user_id, permission_id)` | Check if user has specific permission |

## Usage

```python
import knot.user
import knot.permission as perm

# Get current user
me = knot.user.get_me()
print(f"Hello, {me['username']}!")

# List all users
users = knot.user.list()
for user in users:
    print(f"{user['username']}: {user['email']}")

# Check permissions
my_perms = knot.user.list_permissions(me['id'])
if perm.MANAGE_SPACES in my_perms:
    print("You can manage spaces")
```

## Functions

### get_me()

Get the current authenticated user's details.

**Parameters:** None

**Returns:**

- `dict`: User object containing:
  - `id` (string): User ID
  - `username` (string): Username
  - `email` (string): Email address
  - `active` (bool): Whether the user is active
  - `max_spaces` (int): Maximum spaces allowed
  - `compute_units` (int): Allocated compute units
  - `storage_units` (int): Allocated storage units
  - `max_tunnels` (int): Maximum tunnels allowed
  - `preferred_shell` (string): Preferred shell (e.g., "bash")
  - `timezone` (string): User's timezone
  - `github_username` (string): GitHub username (if set)
  - `number_spaces` (int): Current number of spaces
  - `number_spaces_deployed` (int): Number of deployed spaces
  - `used_compute_units` (int): Compute units in use
  - `used_storage_units` (int): Storage units in use
  - `used_tunnels` (int): Tunnels in use
  - `roles` (list): List of role names
  - `groups` (list): List of group names
  - `current` (bool): True if this is the current user

**Example:**

```python
import knot.user

# Get current user info
me = knot.user.get_me()
print(f"User: {me['username']}")
print(f"Email: {me['email']}")
print(f"Spaces: {me['number_spaces']}/{me['max_spaces']}")
print(f"Roles: {', '.join(me['roles'])}")
print(f"Groups: {', '.join(me['groups'])}")
```

---

### get(user_id)

Get a user by ID or username.

**Parameters:**

- `user_id` (string): User ID or username

**Returns:**

- `dict`: User object (same structure as `get_me()`)

**Example:**

```python
import knot.user

# Get user by ID
user = knot.user.get("abc123...")
print(user['username'])

# Get user by username
user = knot.user.get("john.doe")
print(f"User ID: {user['id']}")
```

---

### list(state='', zone='')

List all users with optional filtering.

**Parameters:**

- `state` (string, optional): Filter by user state (e.g., "active", "inactive")
- `zone` (string, optional): Filter by zone

**Returns:**

- `list`: List of user objects, each containing:
  - `id` (string): User ID
  - `username` (string): Username
  - `email` (string): Email address
  - `active` (bool): Whether the user is active
  - `number_spaces` (int): Current number of spaces

**Example:**

```python
import knot.user

# List all users
all_users = knot.user.list()
print(f"Total users: {len(all_users)}")

# List only active users
active_users = knot.user.list(state="active")
print(f"Active users: {len(active_users)}")

# List users in a specific zone
zone_users = knot.user.list(zone="us-east")
print(f"Users in us-east: {len(zone_users)}")
```

---

### create(username, email, password, ...)

Create a new user.

**Parameters:**

- `username` (string): Username for the new user
- `email` (string): Email address
- `password` (string): Initial password

**Optional Keyword Arguments:**

- `active` (bool): Whether the user is active (default: true)
- `max_spaces` (int): Maximum spaces allowed
- `compute_units` (int): Allocated compute units
- `storage_units` (int): Allocated storage units
- `max_tunnels` (int): Maximum tunnels allowed

**Returns:**

- `string`: The ID of the newly created user

**Example:**

```python
import knot.user

# Create a basic user
user_id = knot.user.create(
    username="johndoe",
    email="john@example.com",
    password="securepassword"
)
print(f"Created user: {user_id}")

# Create a user with quotas
user_id = knot.user.create(
    username="developer",
    email="dev@example.com",
    password="devpass123",
    max_spaces=10,
    compute_units=50,
    storage_units=100,
    max_tunnels=5
)
print(f"Created developer user: {user_id}")
```

---

### update(user_id, ...)

Update a user's properties.

**Parameters:**

- `user_id` (string): User ID or username

**Optional Keyword Arguments:**

- `username` (string): New username
- `email` (string): New email address
- `password` (string): New password
- `active` (bool): Set user active/inactive
- `max_spaces` (int): Update max spaces

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.user

# Update user email
knot.user.update("john.doe", email="john.new@example.com")

# Update multiple properties
knot.user.update(
    "john.doe",
    username="johndoe2",
    max_spaces=20
)

# Disable a user
knot.user.update("john.doe", active=False)

# Change password
knot.user.update("john.doe", password="newpassword123")
```

---

### delete(user_id)

Delete a user.

**Parameters:**

- `user_id` (string): User ID or username

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.user

# Delete a user
if knot.user.delete("olduser"):
    print("User deleted successfully")
```

---

### get_quota(user_id)

Get a user's quota limits and current usage.

**Parameters:**

- `user_id` (string): User ID or username

**Returns:**

- `dict`: Quota information containing:
  - `max_spaces` (int): Maximum spaces allowed
  - `compute_units` (int): Allocated compute units
  - `storage_units` (int): Allocated storage units
  - `max_tunnels` (int): Maximum tunnels allowed
  - `number_spaces` (int): Current number of spaces
  - `number_spaces_deployed` (int): Number of deployed spaces
  - `used_compute_units` (int): Compute units in use
  - `used_storage_units` (int): Storage units in use
  - `used_tunnels` (int): Tunnels in use

**Example:**

```python
import knot.user

# Get current user quota
me = knot.user.get_me()
quota = knot.user.get_quota(me['id'])

print(f"Spaces: {quota['number_spaces']}/{quota['max_spaces']}")
print(f"Compute: {quota['used_compute_units']}/{quota['compute_units']}")
print(f"Storage: {quota['used_storage_units']}/{quota['storage_units']}")
print(f"Tunnels: {quota['used_tunnels']}/{quota['max_tunnels']}")

# Check if user can create more spaces
if quota['number_spaces'] < quota['max_spaces']:
    print("User can create more spaces")
else:
    print("User has reached space limit")
```

---

### list_permissions(user_id)

List all permissions assigned to a user (directly or via roles/groups).

**Parameters:**

- `user_id` (string): User ID or username

**Returns:**

- `list`: List of permission IDs (integers)

**Example:**

```python
import knot.user
import knot.permission as perm

# Get current user permissions
me = knot.user.get_me()
permissions = knot.user.list_permissions(me['id'])

print(f"User has {len(permissions)} permissions")

# Check for specific permissions
if perm.MANAGE_SPACES in permissions:
    print("Can manage spaces")
if perm.MANAGE_USERS in permissions:
    print("Can manage users")
if perm.USE_SPACES in permissions:
    print("Can use spaces")
```

---

### has_permission(user_id, permission_id)

Check if a user has a specific permission.

**Parameters:**

- `user_id` (string): User ID or username
- `permission_id` (int or string): Permission ID to check

**Returns:**

- `bool`: True if user has the permission, False otherwise

**Example:**

```python
import knot.user
import knot.permission as perm

# Get current user
me = knot.user.get_me()

# Check specific permissions
if knot.user.has_permission(me['id'], perm.MANAGE_SPACES):
    print("User can manage spaces")

if knot.user.has_permission(me['id'], perm.USE_SSH):
    print("User can use SSH")

# Check using integer directly
if knot.user.has_permission(me['id'], 7):  # USE_SPACES permission
    print("User can use spaces")
```

---

## Usage Examples

### Example 1: User Onboarding

```python
import knot.user
import knot.permission as perm
import knot.role as role

def onboard_developer(username, email):
    """Onboard a new developer with appropriate permissions"""

    # Create the user
    user_id = knot.user.create(
        username=username,
        email=email,
        password="temp123",
        max_spaces=10,
        compute_units=50,
        storage_units=100
    )
    print(f"Created user: {user_id}")

    # Get or create Developer role
    roles = role.list()
    dev_role = next((r for r in roles if r['name'] == 'Developer'), None)

    if not dev_role:
        # Create Developer role with permissions
        dev_perms = [
            perm.USE_SPACES,
            perm.USE_SSH,
            perm.USE_WEB_TERMINAL,
            perm.RUN_COMMANDS,
            perm.COPY_FILES,
        ]
        dev_role_id = role.create(name="Developer", permissions=dev_perms)
        print(f"Created Developer role: {dev_role_id}")
    else:
        dev_role_id = dev_role['id']

    # Assign role to user (via user update)
    user = knot.user.get(user_id)
    user['roles'].append(dev_role_id)
    knot.user.update(user_id, roles=user['roles'])

    print(f"Onboarded {username} successfully")
    return user_id

# Onboard a developer
onboard_developer("alice", "alice@example.com")
```

### Example 2: Permission-Based Access Control

```python
import knot.user
import knot.permission as perm
import knot.space as space

def create_space_if_allowed(space_name, template):
    """Create a space only if user has permission"""

    # Get current user
    me = knot.user.get_me()

    # Check if user can manage spaces
    if not knot.user.has_permission(me['id'], perm.MANAGE_SPACES):
        print("Permission denied: You cannot manage spaces")
        return False

    # Check quota
    quota = knot.user.get_quota(me['id'])
    if quota['number_spaces'] >= quota['max_spaces']:
        print("Quota exceeded: Maximum spaces reached")
        return False

    # Create the space
    space_id = space.create(
        name=space_name,
        template_name=template
    )
    print(f"Created space: {space_id}")
    return True

# Usage
create_space_if_allowed("my-space", "ubuntu")
```

### Example 3: User Report

```python
import knot.user

def generate_user_report():
    """Generate a report of all users and their usage"""

    users = knot.user.list()

    print(f"{'Username':<20} {'Email':<30} {'Spaces':<10} {'Status':<10}")
    print("-" * 70)

    for user in users:
        status = "active" if user['active'] else "inactive"
        print(f"{user['username']:<20} {user['email']:<30} {user['number_spaces']:<10} {status:<10}")

    print("-" * 70)
    print(f"Total users: {len(users)}")
    print(f"Active users: {sum(1 for u in users if u['active'])}")

generate_user_report()
```

### Example 4: Bulk User Operations

```python
import knot.user

def deactivate_inactive_users(days_threshold=90):
    """Deactivate users who haven't been active"""
    # This is a simplified example - in reality you'd check last login time

    users = knot.user.list()
    deactivated = 0

    for user in users:
        if user['active'] and user['number_spaces'] == 0:
            # Simplified logic - in reality check last login
            print(f"Deactivating {user['username']}...")
            knot.user.update(user['id'], active=False)
            deactivated += 1

    print(f"Deactivated {deactivated} users")

def set_user_quotas(max_spaces=5):
    """Set quotas for all users"""

    users = knot.user.list()
    updated = 0

    for user in users:
        if user['max_spaces'] != max_spaces:
            print(f"Updating quota for {user['username']}...")
            knot.user.update(user['id'], max_spaces=max_spaces)
            updated += 1

    print(f"Updated {updated} users")

# Usage
deactivate_inactive_users()
set_user_quotas(max_spaces=10)
```

### Example 5: User Management with Permissions

```python
import knot.user
import knot.permission as perm

def check_user_capabilities(user_id):
    """Check and display user capabilities"""

    # Get user details
    user = knot.user.get(user_id)
    permissions = knot.user.list_permissions(user_id)
    quota = knot.user.get_quota(user_id)

    print(f"User: {user['username']}")
    print(f"Email: {user['email']}")
    print(f"Status: {'Active' if user['active'] else 'Inactive'}")
    print()

    # Show quota
    print("Quota:")
    print(f"  Spaces: {quota['number_spaces']}/{quota['max_spaces']}")
    print(f"  Compute: {quota['used_compute_units']}/{quota['compute_units']}")
    print(f"  Storage: {quota['used_storage_units']}/{quota['storage_units']}")
    print()

    # Show permissions
    print("Permissions:")
    permission_checks = [
        ("Manage Users", perm.MANAGE_USERS),
        ("Manage Spaces", perm.MANAGE_SPACES),
        ("Use Spaces", perm.USE_SPACES),
        ("Use SSH", perm.USE_SSH),
        ("Run Commands", perm.RUN_COMMANDS),
    ]

    for name, perm_id in permission_checks:
        has = perm_id in permissions
        status = "✓" if has else "✗"
        print(f"  {status} {name}")

# Check current user
me = knot.user.get_me()
check_user_capabilities(me['id'])
```

---

## Related Libraries

- **knot.permission** - For permission constants and checking
- **knot.role** - For role management with permissions
- **knot.group** - For group management
- **knot.space** - For space operations (requires permissions)
