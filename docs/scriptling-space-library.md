# Scriptling Space Library

The `knot.space` library provides direct space management functions for scriptling scripts. This library is available in all three environments (Local, MCP, and Remote).

## Available Functions

- `create(name, template_name, description='', shell='bash')` - Create a new space
- `delete(name)` - Delete a space by name
- `start(name)` - Start a space by name
- `stop(name)` - Stop a space by name
- `restart(name)` - Restart a space by name
- `list()` - List all spaces for the current user
- `is_running(name)` - Check if a space is running
- `get_description(name)` - Get the description of a space
- `set_description(name, description)` - Set the description of a space
- `get_field(name, field)` - Get a custom field value from a space
- `set_field(name, field, value)` - Set a custom field value on a space
- `run_script(space_name, script_name, *args)` - Execute a script in a space, returns {output: str, exit_code: int}
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
import knot.space

# List all spaces
spaces = knot.space.list()
for space in spaces:
    print(f"{space['name']}: {'running' if space['is_running'] else 'stopped'}")

# Start a space
knot.space.start("my-space")

# Execute a command in a space
output = knot.space.run("my-space", "ls", args=["-la", "/tmp"])
print(output)
```

## Functions

### create(name, template_name, description='', shell='bash')

Create a new space.

**Parameters:**
- `name` (string): Name for the new space
- `template_name` (string): Name of the template to use
- `description` (string, optional): Description for the space (default: "")
- `shell` (string, optional): Shell to use (default: "bash")

**Returns:**
- `string`: The space ID of the newly created space

**Example:**
```python
import knot.space

# Create a new space from a template
space_id = knot.space.create(
    name="my-dev-space",
    template_name="python-dev",
    description="My Python development environment",
    shell="bash"
)
print(f"Created space: {space_id}")

# Create with minimal options
space_id = knot.space.create("test-space", "ubuntu")
```

---

### delete(name)

Delete a space and all its data.

**Parameters:**
- `name` (string): Name of the space to delete

**Returns:**
- `bool`: True if successfully deleted, raises error on failure

**Example:**
```python
import knot.space

# Delete a space
if knot.space.delete("old-space"):
    print("Space deleted successfully")
```

---

### start(name)

Start a stopped space.

**Parameters:**
- `name` (string): Name of the space to start

**Returns:**
- `bool`: True if successfully started, raises error on failure

**Example:**
```python
import knot.space

# Start a space
if knot.space.start("dev-space"):
    print("Space started")
```

---

### stop(name)

Stop a running space.

**Parameters:**
- `name` (string): Name of the space to stop

**Returns:**
- `bool`: True if successfully stopped, raises error on failure

**Example:**
```python
import knot.space

# Stop a space
if knot.space.stop("dev-space"):
    print("Space stopped")
```

---

### restart(name)

Restart a running space.

**Parameters:**
- `name` (string): Name of the space to restart

**Returns:**
- `bool`: True if successfully restarted, raises error on failure

**Example:**
```python
import knot.space

# Restart a space
if knot.space.restart("dev-space"):
    print("Space restarted")
```

---

### list()

List all spaces for the current user.

**Parameters:** None

**Returns:**
- `list`: List of space objects, each containing:
  - `name` (string): Space name
  - `id` (string): Space ID
  - `is_running` (bool): Whether the space is running
  - `description` (string): Space description

**Example:**
```python
import knot.space

# List all spaces
spaces = knot.space.list()

print(f"Total spaces: {len(spaces)}")
for space in spaces:
    status = "running" if space['is_running'] else "stopped"
    print(f"- {space['name']}: {status}")
    if space['description']:
        print(f"  {space['description']}")
```

---

### is_running(name)

Check if a space is currently running.

**Parameters:**
- `name` (string): Name of the space to check

**Returns:**
- `bool`: True if the space is running, False otherwise

**Example:**
```python
import knot.space

# Check space status
if knot.space.is_running("dev-space"):
    print("Space is running")
else:
    print("Space is stopped")

# Start if not running
if not knot.space.is_running("dev-space"):
    knot.space.start("dev-space")
```

---

### get_description(name)

Get the description of a space.

**Parameters:**
- `name` (string): Name of the space

**Returns:**
- `string`: The space's description

**Example:**
```python
import knot.space

desc = knot.space.get_description("dev-space")
print(f"Description: {desc}")
```

---

### set_description(name, description)

Set the description of a space.

**Parameters:**
- `name` (string): Name of the space
- `description` (string): New description

**Returns:**
- `bool`: True if successfully updated, raises error on failure

**Example:**
```python
import knot.space

knot.space.set_description("dev-space", "My main development environment")
```

---

### get_field(name, field)

Get a custom field value from a space.

**Parameters:**
- `name` (string): Name of the space
- `field` (string): Field name

**Returns:**
- `string`: The field value

**Example:**
```python
import knot.space

# Get a custom field
api_key = knot.space.get_field("dev-space", "api_key")
print(f"API Key: {api_key}")
```

---

### set_field(name, field, value)

Set a custom field value on a space.

**Parameters:**
- `name` (string): Name of the space
- `field` (string): Field name
- `value` (string): Field value

**Returns:**
- `bool`: True if successfully set, raises error on failure

**Example:**
```python
import knot.space

# Store custom data
knot.space.set_field("dev-space", "last_deploy", "2024-01-15")
knot.space.set_field("dev-space", "environment", "production")
```

---

### run_script(space_name, script_name, *args)

Execute a named script in a space and capture its output and exit code.

**Parameters:**
- `space_name` (string): Name of the space
- `script_name` (string): Name of the script to execute
- `*args` (strings, optional): Additional arguments to pass to the script

**Returns:**
- `dict`: Dictionary containing:
  - `output` (string): The script output
  - `exit_code` (int): The script exit code

**Example:**
```python
import knot.space

# Run a script without arguments
result = knot.space.run_script("dev-space", "status")
print(result['output'])
if result['exit_code'] == 0:
    print("Script succeeded")
else:
    print(f"Script failed with exit code {result['exit_code']}")

# Run a script with arguments
result = knot.space.run_script("dev-space", "deploy", "production", "force")
print(result['output'])

# Run a test script and check exit code
result = knot.space.run_script("test-space", "run_tests", "--verbose", "--coverage")
if result['exit_code'] != 0:
    print(f"Tests failed!\n{result['output']}")
    exit(1)
```

---

### run(space_name, command, args=[], timeout=30, workdir='')

Execute a command in a space.

**Parameters:**
- `space_name` (string): Name of the space
- `command` (string): Command to execute
- `args` (list, optional): Command arguments (default: [])
- `timeout` (int, optional): Timeout in seconds (default: 30)
- `workdir` (string, optional): Working directory (default: "")

**Returns:**
- `string`: The command output

**Example:**
```python
import knot.space

# Simple command
output = knot.space.run("dev-space", "ls")
print(output)

# Command with arguments
output = knot.space.run("dev-space", "ls", args=["-la", "/tmp"])
print(output)

# Command with timeout and working directory
output = knot.space.run(
    "dev-space",
    "npm",
    args=["test"],
    timeout=120,
    workdir="/app"
)
print(output)

# Build command
output = knot.space.run(
    "build-space",
    "make",
    args=["build"],
    timeout=300,
    workdir="/src"
)
print(output)
```

---

### port_forward(source_space, local_port, remote_space, remote_port)

Forward a local port from one space to a port in another space.

**Parameters:**
- `source_space` (string): Name of the space that will receive the forwarded connection
- `local_port` (int): Local port in the source space
- `remote_space` (string): Name of the space with the target service
- `remote_port` (int): Port in the remote space to forward to

**Returns:**
- `bool`: True if successfully created, raises error on failure

**Example:**
```python
import knot.space

# Forward local port 8080 in web-space to port 3000 in api-space
knot.space.port_forward("web-space", 8080, "api-space", 3000)
print("Port forward created: web-space:8080 -> api-space:3000")

# Access a database from an app space
knot.space.port_forward("app-space", 5432, "db-space", 5432)
print("Can now access database at localhost:5432 from app-space")
```

---

### port_list(space)

List all active port forwards for a space.

**Parameters:**
- `space` (string): Name of the space

**Returns:**
- `list`: List of port forward objects, each containing:
  - `local_port` (int): Local port number
  - `space` (string): Remote space name
  - `remote_port` (int): Remote port number

**Example:**
```python
import knot.space

# List all port forwards
forwards = knot.space.port_list("web-space")

print(f"Active forwards: {len(forwards)}")
for forward in forwards:
    print(f"- Port {forward['local_port']} -> {forward['space']}:{forward['remote_port']}")
```

---

### port_stop(space, local_port)

Stop an active port forward.

**Parameters:**
- `space` (string): Name of the space
- `local_port` (int): Local port to stop forwarding

**Returns:**
- `bool`: True if successfully stopped, raises error on failure

**Example:**
```python
import knot.space

# Stop a specific port forward
knot.space.port_stop("web-space", 8080)
print("Port forward stopped")

# Stop all forwards for a space
forwards = knot.space.port_list("web-space")
for forward in forwards:
    knot.space.port_stop("web-space", forward['local_port'])
print("All port forwards stopped")
```

---

## Implementation Details

### Local and Remote Environments
- All functions use the Knot API client to communicate with the server
- Space names are automatically resolved to space IDs
- Authentication is handled automatically via the API client

### MCP Environment
- Functions use internal services (SpaceService, ContainerService) for direct access
- Space names are automatically resolved to space IDs via database queries
- No network calls - direct server communication
- Same 16 functions available as Local/Remote environments

### Space Name Resolution
All functions that accept a `name` parameter automatically resolve the space name to its ID by querying the user's spaces. This means you can use human-readable names instead of IDs.

---

## Complete Examples

### Example 1: Basic Space Management

```python
import knot.space

def manage_spaces():
    """Basic space management operations"""

    # List all spaces
    print("My spaces:")
    spaces = knot.space.list()
    for space in spaces:
        status = "running" if space['is_running'] else "stopped"
        print(f"  - {space['name']}: {status}")

    # Start stopped spaces
    for space in spaces:
        if not space['is_running']:
            print(f"Starting {space['name']}...")
            knot.space.start(space['name'])

    # Create a new space
    if not any(s['name'] == 'temp-space' for s in spaces):
        print("Creating temporary space...")
        space_id = knot.space.create(
            name="temp-space",
            template_name="ubuntu",
            description="Temporary workspace"
        )
        print(f"Created: {space_id}")

manage_spaces()
```

### Example 2: Deployment Workflow

```python
import knot.space

def deploy_application():
    """Deploy an application using space management"""

    space_name = "app-production"

    # Check if space exists, create if not
    spaces = knot.space.list()
    space_exists = any(s['name'] == space_name for s in spaces)

    if not space_exists:
        print(f"Creating {space_name}...")
        knot.space.create(
            name=space_name,
            template_name="nodejs-app",
            description="Production application server"
        )

    # Start the space
    if not knot.space.is_running(space_name):
        print(f"Starting {space_name}...")
        knot.space.start(space_name)

    # Run deployment script
    print("Running deployment script...")
    result = knot.space.run_script(space_name, "deploy", "production")
    print(result['output'])
    
    if result['exit_code'] != 0:
        print(f"Deployment failed with exit code {result['exit_code']}")
        return

    # Update deployment metadata
    import datetime
    knot.space.set_field(space_name, "last_deploy", str(datetime.datetime.now()))
    knot.space.set_field(space_name, "version", "1.2.3")

    print("Deployment complete!")

deploy_application()
```

### Example 3: Multi-Space Orchestration

```python
import knot.space
import time

def setup_microservices():
    """Set up a multi-space microservices architecture"""

    # Define our services
    services = [
        {"name": "api", "template": "nodejs-api", "port": 3000},
        {"name": "web", "template": "react-app", "port": 8080},
        {"name": "db", "template": "postgresql", "port": 5432},
    ]

    # Create and start all services
    for service in services:
        name = f"{service['name']}-service"

        # Create if doesn't exist
        spaces = knot.space.list()
        if not any(s['name'] == name for s in spaces):
            print(f"Creating {name}...")
            knot.space.create(
                name=name,
                template_name=service['template'],
                description=f"{service['name']} microservice"
            )

        # Start if not running
        if not knot.space.is_running(name):
            print(f"Starting {name}...")
            knot.space.start(name)

        # Store port info
        knot.space.set_field(name, "service_port", str(service['port']))

    # Set up port forwarding for inter-service communication
    print("Setting up port forwards...")

    # web -> api
    knot.space.port_forward("web-service", 3000, "api-service", 3000)
    print("  web:3000 -> api:3000")

    # api -> db
    knot.space.port_forward("api-service", 5432, "db-service", 5432)
    print("  api:5432 -> db:5432")

    # Run health checks
    print("\nRunning health checks...")
    for service in services:
        name = f"{service['name']}-service"
        result = knot.space.run_script(name, "health-check")
        status = "OK" if result['exit_code'] == 0 else "FAILED"
        print(f"{name}: {status}")
        if result['exit_code'] != 0:
            print(f"  Error: {result['output']}")

setup_microservices()
```

### Example 4: Development Workflow

```python
import knot.space

def dev_workflow():
    """A typical development workflow"""

    space_name = "my-dev-space"

    # Create dev space if needed
    spaces = knot.space.list()
    if not any(s['name'] == space_name for s in spaces):
        print("Creating dev space...")
        knot.space.create(
            name=space_name,
            template_name="python-dev",
            description="Python development environment"
        )

    # Ensure it's running
    if not knot.space.is_running(space_name):
        print("Starting dev space...")
        knot.space.start(space_name)

    # Install dependencies
    print("Installing dependencies...")
    output = knot.space.run(
        space_name,
        "pip",
        args=["install", "-r", "requirements.txt"],
        timeout=120
    )
    print(output)

    # Run tests
    print("\nRunning tests...")
    result = knot.space.run_script(space_name, "test")
    print(result['output'])
    if result['exit_code'] != 0:
        print("Tests failed!")
        return

    # Run linting
    print("\nRunning linter...")
    output = knot.space.run(
        space_name,
        "flake8",
        args=["."],
        workdir="/app"
    )
    print(output)

dev_workflow()
```

### Example 5: Combining with AI and MCP

```python
import knot.ai
import knot.space
import knot.mcp

def ai_assisted_deployment():
    """Use AI to help manage spaces"""

    # Get current space status
    spaces = knot.space.list()
    space_summary = []
    for space in spaces:
        status = "running" if space['is_running'] else "stopped"
        space_summary.append(f"{space['name']}: {status}")

    # Ask AI for recommendations
    messages = [
        {
            "role": "system",
            "content": "You are a DevOps assistant. Help manage spaces efficiently."
        },
        {
            "role": "user",
            "content": f"Here are my current spaces:\n" + "\n".join(space_summary) +
                      "\n\nWhich spaces should I start or stop for optimal resource usage?"
        }
    ]

    response = knot.ai.completion(messages)
    print("AI Recommendation:", response)

    # Let AI perform actions using tools
    messages.append({"role": "assistant", "content": response})
    messages.append({
        "role": "user",
        "content": "Please start the spaces you recommended and set up port forwarding between them."
    })

    response = knot.ai.completion(messages)
    print("AI Action:", response)

ai_assisted_deployment()
```

---

## Related Libraries

- **knot.ai** - For AI completions with automatic tool usage
- **knot.mcp** - For direct MCP tool access (alternative to direct space functions)
