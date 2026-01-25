# Scriptling Variables Library

The `knot.vars` library provides template variable management functions for scriptling scripts. This library is available in Local and Remote environments.

## Overview

Template variables are dynamic values that can be inserted into templates at space creation time. They allow for flexible configuration without modifying template definitions.

## Available Functions

| Function | Description |
|----------|-------------|
| `list()` | List all template variables |
| `get(var_id)` | Get variable value |
| `set(var_id, value)` | Set variable value (updates existing) |
| `create(name, value, ...)` | Create a new variable |
| `delete(var_id)` | Delete a variable |

## Usage

```python
import knot.vars

# List all variables
variables = knot.vars.list()
for v in variables:
    print(f"{v['name']}: {v.get('protected', False)}")

# Create a new variable
var_id = knot.vars.create("API_ENDPOINT", "https://api.example.com")

# Get variable value
var = knot.vars.get(var_id)
print(f"Value: {var['value']}")

# Update variable value
knot.vars.set(var_id, "https://new-api.example.com")
```

## Functions

### list()

List all template variables.

**Parameters:** None

**Returns:**

- `list`: List of variable objects, each containing:
  - `id` (string): Variable ID
  - `name` (string): Variable name
  - `local` (bool): Whether variable is local
  - `protected` (bool): Whether variable is protected
  - `restricted` (bool): Whether variable is restricted

**Example:**

```python
import knot.vars

# List all variables
variables = knot.vars.list()

print(f"Total variables: {len(variables)}")
for var in variables:
    flags = []
    if var.get('local'):
        flags.append("local")
    if var.get('protected'):
        flags.append("protected")
    if var.get('restricted'):
        flags.append("restricted")

    flags_str = ", ".join(flags) if flags else "none"
    print(f"- {var['name']}: {flags_str}")
```

---

### get(var_id)

Get a variable's details and value.

**Parameters:**

- `var_id` (string): Variable ID

**Returns:**

- `dict`: Variable object containing:
  - `id` (string): Variable ID
  - `name` (string): Variable name
  - `value` (string): Variable value
  - `local` (bool): Whether local
  - `protected` (bool): Whether protected
  - `restricted` (bool): Whether restricted

**Example:**

```python
import knot.vars

# Get variable by ID
var = knot.vars.get("var_abc123...")
print(f"Name: {var['name']}")
print(f"Value: {var['value']}")
print(f"Protected: {var['protected']}")
```

---

### set(var_id, value)

Update an existing variable's value.

**Parameters:**

- `var_id` (string): Variable ID
- `value` (string): New value

**Returns:**

- `bool`: True if successfully updated, raises error on failure

**Example:**

```python
import knot.vars

# Update a variable value
knot.vars.set("var_abc123...", "new-value")
print("Variable updated")

# Update API endpoint
knot.vars.set("API_ENDPOINT", "https://new-api.example.com")
```

---

### create(name, value, ...)

Create a new template variable.

**Parameters:**

- `name` (string): Variable name
- `value` (string): Variable value

**Optional Keyword Arguments:**

- `local` (bool): Whether variable is local (default: false)
- `protected` (bool): Whether variable is protected (default: false)

**Returns:**

- `string`: The ID of the newly created variable

**Example:**

```python
import knot.vars

# Create a basic variable
var_id = knot.vars.create("API_KEY", "secret-key")
print(f"Created variable: {var_id}")

# Create a protected variable
var_id = knot.vars.create(
    name="DB_PASSWORD",
    value="secure-password",
    protected=True
)
print(f"Created protected variable: {var_id}")

# Create a local variable
var_id = knot.vars.create(
    name="LOCAL_ENDPOINT",
    value="http://localhost:8080",
    local=True
)
print(f"Created local variable: {var_id}")
```

---

### delete(var_id)

Delete a variable.

**Parameters:**

- `var_id` (string): Variable ID

**Returns:**

- `bool`: True if successfully deleted, raises error on failure

**Example:**

```python
import knot.vars

# Delete a variable
if knot.vars.delete("var_abc123..."):
    print("Variable deleted successfully")
```

---

## Usage Examples

### Example 1: Environment Configuration

```python
import knot.vars

def setup_environment_variables():
    """Set up environment-specific variables"""

    # Create environment variables
    vars_to_create = [
        ("API_ENDPOINT", "https://api.example.com", False, False),
        ("DB_HOST", "db.example.com", False, False),
        ("DB_PORT", "5432", False, False),
        ("REDIS_HOST", "redis.example.com", False, False),
        ("CDN_URL", "https://cdn.example.com", False, False),
    ]

    created = []
    for name, value, local, protected in vars_to_create:
        # Check if variable exists
        existing = knot.vars.list()
        if any(v['name'] == name for v in existing):
            print(f"Variable '{name}' already exists")
        else:
            var_id = knot.vars.create(
                name=name,
                value=value,
                local=local,
                protected=protected
            )
            print(f"Created variable: {name}")
            created.append(var_id)

    return created

setup_environment_variables()
```

### Example 2: Secure Credential Management

```python
import knot.vars

def setup_credentials():
    """Set up protected credential variables"""

    credentials = [
        ("API_KEY", "your-api-key-here"),
        ("DB_PASSWORD", "secure-db-password"),
        ("SECRET_KEY", "your-secret-key"),
        ("OAUTH_TOKEN", "oauth-token-value"),
    ]

    for name, value in credentials:
        # Check if exists
        existing = knot.vars.list()
        if any(v['name'] == name for v in existing):
            print(f"Credential '{name}' already exists")
        else:
            var_id = knot.vars.create(
                name=name,
                value=value,
                protected=True
            )
            print(f"Created protected credential: {name}")

    print("\nImportant: Protected variables should be updated with real values")

setup_credentials()
```

### Example 3: Variable Updates

```python
import knot.vars

def update_variable(name, new_value):
    """Update a variable by name"""

    # Find the variable
    variables = knot.vars.list()
    var = next((v for v in variables if v['name'] == name), None)

    if not var:
        print(f"Variable '{name}' not found")
        return False

    # Update the value
    knot.vars.set(var['id'], new_value)
    print(f"Updated {name}: {new_value}")
    return True

def rotate_credentials():
    """Rotate credential values"""

    # Update with new values
    update_variable("API_KEY", "new-api-key-123")
    update_variable("DB_PASSWORD", "new-secure-password")
    update_variable("SECRET_KEY", "new-secret-key")

    print("Credentials rotated successfully")

# rotate_credentials()
```

### Example 4: Variable Export/Import

```python
import knot.vars
import json

def export_variables():
    """Export all variables to JSON"""

    variables = knot.vars.list()
    output = []

    for var in variables:
        # Get full variable details including value
        details = knot.vars.get(var['id'])
        output.append({
            'name': details['name'],
            'value': details['value'],
            'local': details['local'],
            'protected': details['protected'],
            'restricted': details['restricted'],
        })

    return json.dumps(output, indent=2)

def import_variables(data):
    """Import variables from JSON"""

    vars_data = json.loads(data)

    for var_data in vars_data:
        # Check if variable exists
        existing = knot.vars.list()
        existing_var = next((v for v in existing if v['name'] == var_data['name']), None)

        if existing_var:
            # Update existing variable
            knot.vars.set(existing_var['id'], var_data['value'])
            print(f"Updated: {var_data['name']}")
        else:
            # Create new variable
            var_id = knot.vars.create(
                name=var_data['name'],
                value=var_data['value'],
                local=var_data.get('local', False),
                protected=var_data.get('protected', False)
            )
            print(f"Created: {var_data['name']}")

# Usage
# export_data = export_variables()
# print(export_data)
# import_variables(export_data)
```

### Example 5: Variable Validation

```python
import knot.vars

def validate_variables():
    """Validate that required variables exist"""

    required_vars = [
        "API_ENDPOINT",
        "DB_HOST",
        "DB_PORT",
        "SECRET_KEY",
    ]

    variables = knot.vars.list()
    var_names = {v['name'] for v in variables}

    missing = []
    present = []

    for name in required_vars:
        if name in var_names:
            present.append(name)
        else:
            missing.append(name)

    print(f"Present: {len(present)}/{len(required_vars)}")
    for name in present:
        var = next(v for v in variables if v['name'] == name)
        print(f"  ✓ {name}")

    if missing:
        print(f"\nMissing: {len(missing)}")
        for name in missing:
            print(f"  ✗ {name}")

    return len(missing) == 0

validate_variables()
```

---

## Notes

### Variable Types

1. **Global variables** (local=false): Available to all spaces
2. **Local variables** (local=true): Only available to specific spaces/zones
3. **Protected variables** (protected=true): Cannot be viewed by non-admin users
4. **Restricted variables** (restricted=true): Limited access

### Create vs Set

- `create()`: Creates a new variable with the given name and value
- `set()`: Updates the value of an existing variable
- Use `create()` for new variables, `set()` for updates

### Variable Deletion

When you delete a variable:
- The variable is removed from the system
- Templates referencing the variable may fail to create spaces
- Existing spaces continue to work with their copied values

### Best Practices

1. **Use protected variables** for sensitive data (passwords, API keys)
2. **Use descriptive names** for clarity (e.g., `API_ENDPOINT` not `ENDPOINT`)
3. **Document required variables** in template descriptions
4. **Use variable groups** with prefixes (e.g., `DB_HOST`, `DB_PORT`, `DB_USER`)

---

## Related Libraries

- **knot.template** - For template management
- **knot.space** - For creating spaces with variables
