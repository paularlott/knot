---
type: "API Reference"
title: "knot.user"
description: "User management functions including permissions, quotas, and SSH keys."
resource: "https://getknot.dev/reference/libraries/user/"
tags:
  - "security"
  - "authentication"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/user/"
---
# knot.user

The `knot.user` library provides user management functions.

---

## Functions

| Function | Description |
|----------|-------------|
| `list(state='', zone='')` | List users; `state` is `all` (default), `active` or `inactive`, and `zone` defaults to the server's zone. Each entry has `id`, `username`, `email`, `active` and `number_spaces` |
| `get(user_id)` | Get user by ID or username |
| `get_me()` | Get the current user |
| `create(username, email, password, active=True, max_spaces=0, compute_units=0, storage_units=0, max_tunnels=0, ssh_public_key='', github_username='', file_storage_mb=0, max_buckets=0)` | Create a new user and return its ID |
| `update(user_id, username=None, email=None, password=None, active=None, max_spaces=None, ssh_public_key=None, github_username=None, compute_units=None, storage_units=None, max_tunnels=None, file_storage_mb=None, max_buckets=None)` | Update user properties; omitted arguments (and roles, groups, shell and timezone) keep their current values, including `file_storage_mb` and `max_buckets`. Returns `True` |
| `set_ssh_public_key(ssh_public_key, github_username=None)` | Set the current user's SSH public keys |
| `set_ssh_private_key(ssh_private_key)` | Set the current user's SSH private key |
| `delete(user_id)` | Delete a user; returns `True` |
| `link_user(user_id, linked_user_id)` | Grant `user_id` the ability to become `linked_user_id` — one way (requires the `link_users` permission) |
| `unlink_user(user_id, linked_user_id)` | Remove `linked_user_id` from `user_id`'s become-list (requires the `link_users` permission) |
| `get_quota(user_id)` | Get user quota and usage |
| `list_permissions(user_id)` | List the user's built-in permissions (integer IDs) |
| `list_plugin_permissions(user_id)` | List the user's plugin permissions — qualified grant strings (`plugin.<name>.<id>`) resolved from their roles |
| `has_permission(user_id, permission_id)` | Check if user has a specific permission — an integer checks a built-in, a `"plugin."`-prefixed string checks a plugin grant |

---

## Usage

```python
import knot.user as user

# Get current user
me = user.get_me()
print(me['username'])

# List users
users = user.list()
for u in users:
    print(f"{u['username']}: {u['email']}")

# Get a specific user
u = user.get("some-user-id")
print(u['username'])

# Check permissions
if user.has_permission(me['id'], 2):  # MANAGE_SPACES = 2
    print("User can manage spaces")

# Get quota information
quota = user.get_quota(me['id'])
print(f"Spaces: {quota['number_spaces']}/{quota['max_spaces']}")

# Let user A become user B (one way; needs the link_users permission)
user.link_user("user-a-id", "user-b-id")

# Take that ability away again
user.unlink_user("user-a-id", "user-b-id")

# Update SSH keys for the current user
user.set_ssh_public_key("ssh-ed25519 AAAA...")
user.set_ssh_private_key("-----BEGIN OPENSSH PRIVATE KEY-----\n...")
```

---

## User Properties

`get()` and `get_me()` return:
- `id` - User ID
- `username` - Username
- `email` - Email address
- `active` - Whether user is active
- `max_spaces` - Maximum spaces allowed
- `compute_units` - Compute units quota
- `storage_units` - Storage units quota
- `max_tunnels` - Maximum tunnels allowed
- `file_storage_mb` - File storage quota in MB
- `max_buckets` - Maximum file storage buckets
- `preferred_shell` - Preferred shell
- `timezone` - User timezone
- `github_username` - GitHub username
- `ssh_public_key` - SSH public keys, one key per line
- `ssh_private_key` - SSH private key, only populated for the current user
- `number_spaces` - Current number of spaces (`get()` only)
- `number_spaces_deployed` - Number of running spaces (`get()` only)
- `used_compute_units` - Used compute units (`get()` only)
- `used_storage_units` - Used storage units (`get()` only)
- `used_tunnels` - Used tunnels (`get()` only)
- `current` - Whether this is the current user
- `roles` - List of role IDs
- `groups` - List of group IDs
- `linked_users` - The accounts this user may become, each with `user_id` and `username` (`get()` only)

`get_me()` reads the lighter `whoami` endpoint, which does not fill the usage fields: `number_spaces`, `number_spaces_deployed`, `used_compute_units`, `used_storage_units` and `used_tunnels` are always `0` and `linked_users` is `None`. Call `get(get_me()["id"])` when you need them.

`get_me()` also withholds credentials in two cases: with a scoped API token, `ssh_public_key` and `ssh_private_key` are empty, and in a space with restrictions active, `ssh_private_key` is empty.

---

## Quota Properties

`get_quota()` returns:
- `max_spaces` - Maximum spaces allowed
- `compute_units` - Compute units quota
- `storage_units` - Storage units quota
- `max_tunnels` - Maximum tunnels allowed
- `file_storage_mb` - File storage quota in MB
- `max_buckets` - Maximum file storage buckets
- `number_spaces` - Current number of spaces
- `number_spaces_deployed` - Number of running spaces
- `used_compute_units` - Used compute units
- `used_storage_units` - Used storage units
- `used_tunnels` - Used tunnels
- `used_file_storage_mb` - File storage used, in MB
- `used_buckets` - Buckets owned
