# Scriptling Execution Environments

Knot provides three distinct scriptling execution environments, each tailored for specific use cases with different library availability and security constraints.

## Library Loading

All environments now use **dynamic library loading** via scriptling's on-demand callback mechanism. Libraries are fetched from the server as needed when import statements are encountered, rather than being bulk-loaded at environment creation.

## Local Environment

**Used by:** `knot run-script` command on desktop/agent

**Purpose:** Execute scripts locally with full system access and on-demand library loading from disk or server.

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
- **spaces** - Space management operations (via API)
- **ai** - AI completion functions (via API)
- **mcp** - MCP tools library (via API)

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically as imports are encountered
- **Priority**: Local `.py` files are tried first, then fetches from server
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint

**Example:**

```bash
# Run local script with dynamic library loading
knot run-script myscript.py arg1 arg2

# Execute script in a space
knot space run-script myspace myscript arg1 arg2

# Import local .py files first, then server libraries
import mylib  # Tries mylib.py locally, then fetches from server
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
- **spaces** - Space management operations (via internal API)
- **ai** - AI completion functions (via MCP server)

### Special Libraries

- **mcp** - MCP-specific functions with access to tool parameters

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically from server only
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint
- **No filesystem access** for security

**Security Note:** This environment is intentionally restricted to prevent AI tools from executing arbitrary system commands or accessing the filesystem.

---

## Remote Environment

**Used by:** Script execution within spaces (containers)

**Purpose:** Execute scripts remotely in user spaces with full capabilities and dynamic library loading.

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
- **spaces** - Space management operations (via API)
- **ai** - AI completion functions (via API)
- **mcp** - MCP tools library (via API)

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically from server only
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint

**Example:**

```bash
# Execute script in a space
knot space run-script myspace myscript arg1 arg2
```

---

## Library Comparison Matrix

| Library        | Local            | MCP        | Remote     |
| -------------- | ---------------- | ---------- | ---------- |
| stdlib         | ✓                | ✓          | ✓          |
| requests       | ✓                | ✓          | ✓          |
| secrets        | ✓                | ✓          | ✓          |
| htmlparser     | ✓                | ✓          | ✓          |
| subprocess     | ✓                | ✗          | ✓          |
| threads        | ✓                | ✗          | ✓          |
| os             | ✓                | ✗          | ✓          |
| pathlib        | ✓                | ✗          | ✓          |
| sys            | ✓                | ✗          | ✓          |
| spaces         | ✓                | ✓          | ✓          |
| ai             | ✓                | ✓          | ✓          |
| mcp            | ✓                | ✓          | ✓          |
| On-demand libs | ✓ (local+server) | ✓ (server) | ✓ (server) |

---

## Security Considerations

### Local Environment

- Full system access - use with trusted scripts only
- Can read/write files via pathlib on local machine
- Can execute system commands via subprocess
- Can load arbitrary .py files from disk (tried first)
- Falls back to server libraries if not found locally
- Has API access to spaces, ai, and mcp

### MCP Environment

- Restricted for AI safety
- No filesystem access
- No command execution
- Cannot load external files
- Only fetches libraries from server
- Limited to safe operations for AI tool usage

### Remote Environment

- Runs in isolated container (space)
- Full capabilities within container
- No access to host filesystem
- Libraries fetched dynamically from server
- Has API access to spaces, ai, and mcp from within space
- **Requires active agent connection** - script execution fails if space agent is not connected

---

## Additional Documentation

- [Spaces Library Reference](scriptling-spaces-library.md) - Complete documentation for the `spaces` library
