# Scriptling Template Library

The `knot.template` library provides template management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

Templates define the configuration for creating spaces. They specify the container image, resources, and default settings for new spaces.

## Available Functions

| Function | Description |
|----------|-------------|
| `list()` | List all templates |
| `get(template_id)` | Get template by ID or name |
| `create(name, ...)` | Create a new template |
| `update(template_id, ...)` | Update template properties |
| `delete(template_id)` | Delete a template |
| `get_icons()` | Get list of available icons |

## Usage

```python
import knot.template

# List all templates
templates = knot.template.list()
for t in templates:
    print(f"{t['name']}: {t['platform']}")

# Create a new template
template_id = knot.template.create(
    name="python-dev",
    job="python:3.11-slim",
    platform="linux/amd64",
    description="Python development environment"
)

# Update a template
knot.template.update(template_id, description="Updated description")
```

## Functions

### list()

List all templates.

**Parameters:** None

**Returns:**

- `list`: List of template objects, each containing:
  - `id` (string): Template ID
  - `name` (string): Template name
  - `description` (string): Template description
  - `platform` (string): Platform (e.g., "linux/amd64")
  - `active` (bool): Whether the template is active
  - `usage` (int): Current usage count
  - `deployed` (int): Number of deployed spaces

**Example:**

```python
import knot.template

# List all templates
templates = knot.template.list()

print(f"Total templates: {len(templates)}")
for tmpl in templates:
    status = "active" if tmpl['active'] else "inactive"
    print(f"- {tmpl['name']}: {tmpl['platform']} ({status})")
    print(f"  {tmpl['description']}")
    print(f"  Usage: {tmpl['usage']}, Deployed: {tmpl['deployed']}")
```

---

### get(template_id)

Get a template by ID or name.

**Parameters:**

- `template_id` (string): Template ID or name

**Returns:**

- `dict`: Template object containing:
  - `id` (string): Template ID
  - `name` (string): Template name
  - `description` (string): Template description
  - `platform` (string): Platform
  - `job` (string): Container image/job specification
  - `volumes` (string): Volume configuration
  - `active` (bool): Whether active
  - `is_managed` (bool): Whether managed
  - `compute_units` (int): Compute units required
  - `storage_units` (int): Storage units required
  - `usage` (int): Current usage
  - `deployed` (int): Deployed count
  - `hash` (string): Template hash
  - `with_terminal` (bool): Web terminal enabled
  - `with_vscode_tunnel` (bool): VSCode tunnel enabled
  - `with_code_server` (bool): Code-server enabled
  - `with_ssh` (bool): SSH enabled
  - `with_run_command` (bool): Run command enabled
  - `schedule_enabled` (bool): Schedule enabled
  - `auto_start` (bool): Auto-start enabled
  - `max_uptime` (int): Maximum uptime
  - `max_uptime_unit` (string): Uptime unit
  - `icon_url` (string): Icon URL
  - `groups` (list): List of group IDs
  - `zones` (list): Allowed zones
  - `schedule` (list): Schedule configuration (7 days, each with `enabled`, `from`, `to`)
  - `custom_fields` (list): Custom field definitions (each with `name`, `description`)

**Example:**

```python
import knot.template

# Get template by ID
tmpl = knot.template.get("abc123...")
print(tmpl['name'])

# Get template by name
tmpl = knot.template.get("ubuntu")
print(f"Template ID: {tmpl['id']}")
print(f"Platform: {tmpl['platform']}")
print(f"Compute units: {tmpl['compute_units']}")
print(f"Storage units: {tmpl['storage_units']}")
```

---

### create(name, ...)

Create a new template.

**Parameters:**

- `name` (string): Template name (required)

**Optional Keyword Arguments:**

- `job` (string): Container image/job definition
- `description` (string): Template description
- `platform` (string): Platform (default: "")
- `volumes` (string): Volume configuration
- `active` (bool): Whether template is active (default: true)
- `compute_units` (int): Compute units required
- `storage_units` (int): Storage units required
- `with_terminal` (bool): Enable web terminal access
- `with_vscode_tunnel` (bool): Enable VSCode tunnel
- `with_code_server` (bool): Enable code-server
- `with_ssh` (bool): Enable SSH access
- `with_run_command` (bool): Enable run command
- `schedule_enabled` (bool): Enable schedule restrictions
- `icon_url` (string): Icon URL (use `get_icons()` to find available URLs)
- `groups` (list): List of group IDs
- `zones` (list): List of zone names
- `schedule` (list): Schedule configuration (7 day objects with `enabled`, `from`, `to`)
- `custom_fields` (list): Custom field definitions (objects with `name`, `description`)

**Returns:**

- `string`: The ID of the newly created template

**Example:**

```python
import knot.template

# Create a basic template
template_id = knot.template.create(
    name="ubuntu-basic",
    job="ubuntu:22.04"
)
print(f"Created template: {template_id}")

# Create a Python development template with full configuration
template_id = knot.template.create(
    name="python-dev",
    job="python:3.11-slim",
    platform="docker",
    description="Python 3.11 development environment",
    compute_units=10,
    storage_units=20,
    with_terminal=True,
    with_ssh=True,
    with_run_command=True,
    active=True,
    icon_url="/icons/python.svg"
)
print(f"Created template: {template_id}")

# Create template with schedule restrictions
template_id = knot.template.create(
    name="scheduled-template",
    job="ubuntu:22.04",
    schedule_enabled=True,
    schedule=[
        {"enabled": False, "from": "12:00am", "to": "11:59pm"},  # Sunday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Monday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Tuesday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Wednesday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Thursday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Friday
        {"enabled": False, "from": "12:00am", "to": "11:59pm"},  # Saturday
    ]
)

# Create template with custom fields
template_id = knot.template.create(
    name="custom-template",
    job="ubuntu:22.04",
    custom_fields=[
        {"name": "project_name", "description": "Project name"},
        {"name": "environment", "description": "Environment (dev/staging/prod)"}
    ]
)
```

---

### update(template_id, ...)

Update a template's properties.

**Parameters:**

- `template_id` (string): Template ID or name (required)

**Optional Keyword Arguments:**

- `name` (string): New template name
- `job` (string): New container image/job
- `description` (string): New description
- `volumes` (string): Volume configuration
- `platform` (string): New platform
- `active` (bool): Set active/inactive
- `compute_units` (int): Compute units
- `storage_units` (int): Storage units
- `with_terminal` (bool): Enable web terminal access
- `with_vscode_tunnel` (bool): Enable VSCode tunnel
- `with_code_server` (bool): Enable code-server
- `with_ssh` (bool): Enable SSH access
- `with_run_command` (bool): Enable run command
- `schedule_enabled` (bool): Enable schedule restrictions
- `icon_url` (string): Icon URL
- `groups` (list): List of group IDs
- `zones` (list): List of zone names
- `schedule` (list): Schedule configuration (7 day objects with `enabled`, `from`, `to`)
- `custom_fields` (list): Custom field definitions (objects with `name`, `description`)

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.template

# Update template description
knot.template.update("python-dev", description="Updated description")

# Update multiple properties
knot.template.update(
    "python-dev",
    job="python:3.12-slim",
    platform="docker",
    compute_units=15,
    storage_units=25,
    active=True
)

# Enable features
knot.template.update(
    "python-dev",
    with_terminal=True,
    with_ssh=True,
    with_run_command=True
)

# Update schedule
knot.template.update(
    "python-dev",
    schedule_enabled=True,
    schedule=[
        {"enabled": False, "from": "12:00am", "to": "11:59pm"},  # Sunday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},     # Monday-Friday
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},
        {"enabled": True, "from": "9:00am", "to": "5:00pm"},
        {"enabled": False, "from": "12:00am", "to": "11:59pm"},  # Saturday
    ]
)

# Deactivate a template
knot.template.update("old-template", active=False)
```

---

### delete(template_id)

Delete a template.

**Parameters:**

- `template_id` (string): Template ID or name

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.template

# Delete a template
if knot.template.delete("old-template"):
    print("Template deleted successfully")
```

---

### get_icons()

Get list of available icons for templates and spaces.

**Parameters:** None

**Returns:**

- `list`: List of icon objects, each containing:
  - `description` (string): Icon description
  - `source` (string): Icon source (e.g., "built-in")
  - `url` (string): Icon URL

**Example:**

```python
import knot.template

# Get all available icons
icons = knot.template.get_icons()

print(f"Total icons: {len(icons)}")
for icon in icons:
    print(f"- {icon['description']}: {icon['url']}")
    print(f"  Source: {icon['source']}")
```

---

## Usage Examples

### Example 1: Setting Up Development Templates

```python
import knot.template

def setup_dev_templates():
    """Create standard development templates"""

    templates = [
        {
            "name": "ubuntu",
            "job": "ubuntu:22.04",
            "description": "Ubuntu 22.04 base image",
            "platform": "linux/amd64",
            "compute_units": 5,
            "storage_units": 10,
        },
        {
            "name": "python-dev",
            "job": "python:3.11-slim",
            "description": "Python 3.11 development environment",
            "platform": "linux/amd64",
            "compute_units": 10,
            "storage_units": 20,
        },
        {
            "name": "nodejs-dev",
            "job": "node:20-alpine",
            "description": "Node.js 20 development environment",
            "platform": "linux/amd64",
            "compute_units": 10,
            "storage_units": 20,
        },
        {
            "name": "golang-dev",
            "job": "golang:1.21-alpine",
            "description": "Go 1.21 development environment",
            "platform": "linux/amd64",
            "compute_units": 10,
            "storage_units": 20,
        },
    ]

    created = []
    for tmpl in templates:
        # Check if template exists
        existing = knot.template.list()
        if any(t['name'] == tmpl['name'] for t in existing):
            print(f"Template '{tmpl['name']}' already exists")
        else:
            template_id = knot.template.create(**tmpl)
            print(f"Created template: {tmpl['name']} ({template_id})")
            created.append(template_id)

    return created

setup_dev_templates()
```

### Example 2: Template Management

```python
import knot.template

def manage_templates():
    """List and manage templates"""

    templates = knot.template.list()

    print(f"{'Name':<20} {'Platform':<15} {'Compute':<10} {'Storage':<10} {'Active':<10}")
    print("-" * 65)

    for tmpl in templates:
        print(f"{tmpl['name']:<20} {tmpl['platform']:<15} "
              f"{tmpl['compute_units']:<10} {tmpl['storage_units']:<10} "
              f"{'Yes' if tmpl['active'] else 'No':<10}")

    # Find inactive templates
    inactive = [t for t in templates if not t['active']]
    if inactive:
        print(f"\nInactive templates: {len(inactive)}")
        for tmpl in inactive:
            print(f"  - {tmpl['name']}")

    # Find unused templates
    unused = [t for t in templates if t['usage'] == 0]
    if unused:
        print(f"\nUnused templates: {len(unused)}")
        for tmpl in unused:
            print(f"  - {tmpl['name']}")

manage_templates()
```

### Example 3: Template Update Workflow

```python
import knot.template

def update_template_workflow():
    """Complete template update workflow"""

    # Create a new template
    template_id = knot.template.create(
        name="temp-template",
        job="python:3.10-slim",
        description="Temporary template"
    )
    print(f"Created template: {template_id}")

    # Get template details
    tmpl = knot.template.get(template_id)
    print(f"Template: {tmpl['name']}")
    print(f"Job: {tmpl['job']}")
    print(f"Description: {tmpl['description']}")

    # Update the template
    knot.template.update(
        template_id,
        job="python:3.11-slim",
        description="Updated Python 3.11 template",
        active=True
    )
    print("Updated template")

    # Verify update
    tmpl = knot.template.get(template_id)
    print(f"New job: {tmpl['job']}")
    print(f"New description: {tmpl['description']}")

    return template_id

# update_template_workflow()
```

### Example 4: Template Cloning

```python
import knot.template

def clone_template(source_name, new_name, new_description=None):
    """Clone an existing template"""

    # Get source template
    source = knot.template.get(source_name)

    # Create new template with same settings
    template_id = knot.template.create(
        name=new_name,
        job=source.get('job', ''),
        platform=source.get('platform', ''),
        description=new_description or f"Clone of {source_name}",
        active=source.get('active', True),
        compute_units=int(source.get('compute_units', 0)),
        storage_units=int(source.get('storage_units', 0)),
    )

    print(f"Cloned '{source_name}' to '{new_name}': {template_id}")
    return template_id

# Usage
clone_template("python-dev", "python-dev-v2", "Updated Python dev environment")
```

### Example 5: Resource Planning

```python
import knot.template

def calculate_template_resources():
    """Calculate total resources across all templates"""

    templates = knot.template.list()

    total_compute = 0
    total_storage = 0
    total_deployed = 0

    print(f"{'Template':<20} {'Compute':<10} {'Storage':<10} {'Deployed':<10}")
    print("-" * 50)

    for tmpl in templates:
        compute = tmpl.get('compute_units', 0)
        storage = tmpl.get('storage_units', 0)
        deployed = tmpl.get('deployed', 0)

        total_compute += compute * deployed
        total_storage += storage * deployed
        total_deployed += deployed

        print(f"{tmpl['name']:<20} {compute:<10} {storage:<10} {deployed:<10}")

    print("-" * 50)
    print(f"{'Total':<20} {total_compute:<10} {total_storage:<10} {total_deployed:<10}")
    print()
    print(f"If all deployed spaces were active:")
    print(f"  Total compute units: {total_compute}")
    print(f"  Total storage units: {total_storage}")

calculate_template_resources()
```

---

## Notes

### Template Job Format

The `job` parameter specifies the container image to use:
- Docker Hub images: `ubuntu:22.04`, `python:3.11-slim`
- Custom images: `registry.example.com/myimage:tag`
- Complex jobs: May include build commands or scripts

### Active vs Inactive Templates

- **Active templates**: Available for creating new spaces
- **Inactive templates**: Cannot be used for new spaces, but existing spaces continue to work

### Template Deletion

When you delete a template:
- The template is removed from the list
- Existing spaces created from the template continue to work
- New spaces cannot be created from the deleted template

### Resource Units

- **Compute units**: CPU resources required
- **Storage units**: Disk space required
- These are multiplied by the number of deployed spaces

---

## Related Libraries

- **knot.space** - For creating spaces from templates
- **knot.volume** - For volume management
- **knot.vars** - For template variable management
