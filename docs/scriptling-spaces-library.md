# Scriptling Spaces Library

The `spaces` library provides functions to manage development spaces programmatically from within scriptling scripts. This library is available in all three scriptling execution environments (Local, MCP, and Remote), with the implementation automatically adapting to the environment.

## Available Functions

- `start(name)` - Start a space
- `stop(name)` - Stop a space
- `restart(name)` - Restart a space
- `is_running(name)` - Check if a space is running
- `list()` - List all spaces
- `create(name, template_name, description='', shell='bash')` - Create a new space
- `delete(name)` - Delete a space
- `get_field(name, field)` - Get a custom field value
- `set_field(name, field, value)` - Set a custom field value
- `get_description(name)` - Get space description
- `set_description(name, description)` - Set space description
- `run_script(space_name, script_name, *args)` - Execute a script in a space
- `run(space_name, command, args=[], timeout=30, workdir='')` - Execute a command in a space
- `port_forward(source_space, local_port, remote_space, remote_port)` - Forward a local port to a remote space port
- `port_list(space)` - List active port forwards for a space
- `port_stop(space, local_port)` - Stop a port forward

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

### port_forward(source_space, local_port, remote_space, remote_port)

Forward a local port from one space to a port in another space. This allows you to access services running in one space from another space.

**Parameters:**
- `source_space` (string): The name of the source space (where the listener is created)
- `local_port` (int): The local port to listen on in the source space (1-65535)
- `remote_space` (string): The name of the target space to connect to
- `remote_port` (int): The port in the target space to connect to (1-65535)

**Returns:**
- `True` on success
- Raises error if spaces not found, not running, or forward cannot be established

**Example:**
```python
import spaces

# Forward port 8080 in web-dev space to port 3000 in api-dev space
spaces.port_forward("web-dev", 8080, "api-dev", 3000)
print("Port forward established: web-dev:8080 -> api-dev:3000")

# Now web-dev can access api-dev's service at localhost:8080
```

---

### port_list(space)

List all active port forwards for a space.

**Parameters:**
- `space` (string): The name of the space

**Returns:**
- List of dictionaries, each containing:
  - `local_port` (int): The local port number
  - `space` (string): The name of the target space
  - `remote_port` (int): The remote port number
- Raises error if space not found

**Example:**
```python
import spaces

# List all active port forwards
forwards = spaces.port_list("web-dev")
for forward in forwards:
    print(f"Port {forward['local_port']} -> {forward['space']}:{forward['remote_port']}")
```

---

### port_stop(space, local_port)

Stop an active port forward.

**Parameters:**
- `space` (string): The name of the space
- `local_port` (int): The local port of the forward to stop

**Returns:**
- `True` on success
- Raises error if space not found or forward not active

**Example:**
```python
import spaces

# Stop a port forward
spaces.port_stop("web-dev", 8080)
print("Port forward stopped")
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


---

### run_script(space_name, script_name, *args)

Execute a script in a space.

**Parameters:**
- `space_name` (string): The name of the space
- `script_name` (string): The name of the script to execute
- `*args` (strings, optional): Additional arguments to pass to the script

**Returns:**
- String containing the script output
- Raises error if space or script not found, or execution fails

**Example:**
```python
import spaces

# Execute script without arguments
output = spaces.run_script("my-dev-space", "deploy-script")
print(output)

# Execute script with arguments
output = spaces.run_script("my-dev-space", "build-script", "production", "v1.2.3")
print(output)
```

---

### run(space_name, command, args=[], timeout=30, workdir='')

Execute a command in a space.

**Parameters:**
- `space_name` (string): The name of the space
- `command` (string): The command to execute
- `args` (list, optional): List of command arguments (default: [])
- `timeout` (int, optional): Timeout in seconds (default: 30)
- `workdir` (string, optional): Working directory for command execution (default: '')

**Returns:**
- String containing the command output
- Raises error if space not found, command fails, or execution times out

**Example:**
```python
import spaces

# Simple command
output = spaces.run("my-dev-space", "ls", args=["-la", "/home"])
print(output)

# Command with timeout and workdir
output = spaces.run(
    "my-dev-space",
    "npm",
    args=["install"],
    timeout=120,
    workdir="/app"
)
print(output)
```

---
