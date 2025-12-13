# Scriptling Spaces Library

The `spaces` library provides functions to manage development spaces programmatically from within scriptling scripts. This library is available in all three scriptling execution environments (Local, MCP, and Remote), with the implementation automatically adapting to the environment.

## Availability

| Environment | Available | Implementation |
|-------------|-----------|----------------|
| Local       | ✓         | API Client     |
| MCP         | ✓         | Internal API   |
| Remote      | ✓         | API Client     |

## Usage

```python
import spaces

# List all spaces
for space in spaces.list():
    print(f"{space['name']}: {'running' if space['is_running'] else 'stopped'}")

# Start a space
spaces.start("my-dev-space")

# Check if running
if spaces.is_running("my-dev-space"):
    print("Space is running")

# Get custom field value
api_key = spaces.get_field("my-dev-space", "api_key")

# Stop the space
spaces.stop("my-dev-space")
```

## Functions

### start(name)

Start a space by name.

**Parameters:**
- `name` (string): The name of the space to start

**Returns:**
- `True` on success
- Raises error if space not found or cannot be started

**Example:**
```python
import spaces

spaces.start("my-dev-space")
print("Space started successfully")
```

---

### stop(name)

Stop a running space by name.

**Parameters:**
- `name` (string): The name of the space to stop

**Returns:**
- `True` on success
- Raises error if space not found or cannot be stopped

**Example:**
```python
import spaces

spaces.stop("my-dev-space")
print("Space stopped successfully")
```

---

### restart(name)

Restart a running space by name.

**Parameters:**
- `name` (string): The name of the space to restart

**Returns:**
- `True` on success
- Raises error if space not found or cannot be restarted

**Example:**
```python
import spaces

spaces.restart("my-dev-space")
print("Space restarted successfully")
```

---

### is_running(name)

Check if a space is currently running.

**Parameters:**
- `name` (string): The name of the space to check

**Returns:**
- `True` if the space is deployed and running
- `False` if the space is stopped
- Raises error if space not found

**Example:**
```python
import spaces

if spaces.is_running("my-dev-space"):
    print("Space is running")
else:
    print("Space is stopped")
```

---

### list()

List all spaces for the current user.

**Parameters:**
- None

**Returns:**
- List of dictionaries, each containing:
  - `name` (string): Space name
  - `id` (string): Space ID
  - `is_running` (bool): Whether the space is running
  - `description` (string): Space description

**Example:**
```python
import spaces

all_spaces = spaces.list()
for space in all_spaces:
    status = "running" if space["is_running"] else "stopped"
    print(f"{space['name']}: {status} - {space['description']}")
```

---

### get_field(name, field)

Get the value of a custom field from a space.

**Parameters:**
- `name` (string): The name of the space
- `field` (string): The name of the custom field

**Returns:**
- String value of the custom field
- Empty string if field is not set
- Raises error if space or field not found

**Example:**
```python
import spaces

api_key = spaces.get_field("my-dev-space", "api_key")
print(f"API Key: {api_key}")
```

---

### set_field(name, field, value)

Set the value of a custom field on a space.

**Parameters:**
- `name` (string): The name of the space
- `field` (string): The name of the custom field
- `value` (string): The value to set

**Returns:**
- `True` on success
- Raises error if space or field not found

**Example:**
```python
import spaces

spaces.set_field("my-dev-space", "api_key", "sk-1234567890")
print("Field updated successfully")
```

---

### create(name, template_name, description='', shell='bash')

Create a new space.

**Parameters:**
- `name` (string): The name for the new space
- `template_name` (string): The name of the template to use
- `description` (string, optional): Description of the space (default: '')
- `shell` (string, optional): Default shell (bash, zsh, fish, sh) (default: 'bash')

**Returns:**
- String containing the space ID
- Raises error if creation fails

**Example:**
```python
import spaces

# Positional arguments
space_id = spaces.create("new-space", "my-template", "My development space", "zsh")

# Keyword arguments
space_id = spaces.create("new-space", "my-template", description="My dev space", shell="zsh")

print(f"Created space with ID: {space_id}")
```

---

### delete(name)

Delete a space by name. The space must be stopped before it can be deleted.

**Parameters:**
- `name` (string): The name of the space to delete

**Returns:**
- `True` on success
- Raises error if space not found or cannot be deleted

**Example:**
```python
import spaces

# Stop the space first
spaces.stop("old-space")

# Then delete it
spaces.delete("old-space")
print("Space deleted successfully")
```

---

### set_description(name, description)

Set the description of a space.

**Parameters:**
- `name` (string): The name of the space
- `description` (string): The new description

**Returns:**
- `True` on success
- Raises error if space not found

**Example:**
```python
import spaces

spaces.set_description("my-dev-space", "Updated description for my space")
print("Description updated successfully")
```

---

### get_description(name)

Get the description of a space.

**Parameters:**
- `name` (string): The name of the space

**Returns:**
- String containing the space description
- Raises error if space not found

**Example:**
```python
import spaces

description = spaces.get_description("my-dev-space")
print(f"Description: {description}")
```

---

## Complete Example

```python
import spaces

# Create a new development space
space_id = spaces.create(
    "test-space",
    "my-template",
    description="Test environment",
    shell="bash"
)
print(f"Created space: {space_id}")

# Set custom fields
spaces.set_field("test-space", "environment", "testing")
spaces.set_field("test-space", "api_key", "sk-test-key")

# Start the space
spaces.start("test-space")
print("Space started")

# Wait for it to be running
import time
while not spaces.is_running("test-space"):
    print("Waiting for space to start...")
    time.sleep(2)

print("Space is now running")

# Get custom field values
env = spaces.get_field("test-space", "environment")
api_key = spaces.get_field("test-space", "api_key")
print(f"Environment: {env}")
print(f"API Key: {api_key}")

# Update description
spaces.set_description("test-space", "Updated test environment")

# Stop the space when done
spaces.stop("test-space")
print("Space stopped")

# Clean up
spaces.delete("test-space")
print("Space deleted")
```

## Error Handling

All functions raise errors when operations fail. Use try/except blocks to handle errors gracefully:

```python
import spaces

try:
    spaces.start("my-space")
    print("Space started successfully")
except Exception as e:
    print(f"Failed to start space: {e}")
```

## Permissions

Space operations are subject to user permissions:

- **UseSpaces**: Required to manage your own spaces
- **ManageSpaces**: Required to manage spaces owned by other users

Operations will fail with permission errors if the user lacks the required permissions.

## Notes

- Space names must be unique per user
- Spaces must be stopped before they can be deleted
- Custom fields must be defined in the space's template before they can be used
- The `is_running()` function checks the `IsDeployed` status of the space
- In Local and Remote environments, operations use the API client and require valid authentication
- In MCP environment, operations use internal services directly for better performance
