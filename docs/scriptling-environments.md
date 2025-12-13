# Scriptling Execution Environments

Knot provides three distinct scriptling execution environments, each tailored for specific use cases with different library availability and security constraints.

## Local Environment

**Used by:** `knot run-script` command on desktop/agent

**Purpose:** Execute scripts locally with full system access and on-demand library loading from disk.

**Available Libraries:**

### Standard Libraries
- All Python-like standard libraries via `stdlib.RegisterAll()`

### Extended Libraries
- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **subprocess** - Process execution
- **htmlparser** - HTML parsing and manipulation
- **threads** - Threading support
- **os** - Operating system interface
- **pathlib** - Filesystem path operations
- **sys** - System-specific parameters (argv)

### Database Libraries
- All library scripts from the database (filtered by user groups)

### On-Demand Loading
- Enabled: Local `.py` files are automatically loaded when imported
- Files are loaded from the current working directory

**Example:**
```bash
# Run local script with server libraries
knot run-script myscript.py arg1 arg2

# Import local .py files automatically
import mylib  # Loads mylib.py from current directory
```

---

## MCP Environment

**Used by:** MCP tool execution via AI assistants

**Purpose:** Execute tool scripts in a controlled environment for AI integration.

**Available Libraries:**

### Standard Libraries
- All Python-like standard libraries via `stdlib.RegisterAll()`

### Extended Libraries
- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **htmlparser** - HTML parsing and manipulation

### Special Libraries
- **mcp** - MCP-specific functions with access to tool parameters

### Database Libraries
- All library scripts from the database (filtered by user groups)

### On-Demand Loading
- Disabled: No filesystem access for security

**Security Note:** This environment is intentionally restricted to prevent AI tools from executing arbitrary system commands or accessing the filesystem.

---

## Remote Environment

**Used by:** Script execution within spaces (containers)

**Purpose:** Execute scripts remotely in user spaces with full capabilities but no on-demand loading.

**Available Libraries:**

### Standard Libraries
- All Python-like standard libraries via `stdlib.RegisterAll()`

### Extended Libraries
- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **subprocess** - Process execution
- **htmlparser** - HTML parsing and manipulation
- **threads** - Threading support
- **os** - Operating system interface
- **pathlib** - Filesystem path operations
- **sys** - System-specific parameters (argv)

### Database Libraries
- All library scripts from the database (filtered by user groups)

### On-Demand Loading
- Disabled: All libraries must be pre-registered

**Example:**
```bash
# Execute script in a space
knot spaces run myspace myscript arg1 arg2
```

---

## Library Comparison Matrix

| Library | Local | MCP | Remote |
|---------|-------|-----|--------|
| stdlib | ✓ | ✓ | ✓ |
| requests | ✓ | ✓ | ✓ |
| secrets | ✓ | ✓ | ✓ |
| htmlparser | ✓ | ✓ | ✓ |
| subprocess | ✓ | ✗ | ✓ |
| threads | ✓ | ✗ | ✓ |
| os | ✓ | ✗ | ✓ |
| pathlib | ✓ | ✗ | ✓ |
| sys | ✓ | ✗ | ✓ |
| mcp | ✗ | ✓ | ✗ |
| Database libs | ✓ | ✓ | ✓ |
| On-demand .py | ✓ | ✗ | ✗ |

---

## Security Considerations

### Local Environment
- Full system access - use with trusted scripts only
- Can read/write files via pathlib
- Can execute system commands via subprocess
- Can load arbitrary .py files from disk

### MCP Environment
- Restricted for AI safety
- No filesystem access
- No command execution
- Cannot load external files

### Remote Environment
- Runs in isolated container
- Full capabilities within container
- No access to host filesystem
- Libraries must be pre-defined in database
