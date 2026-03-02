# Scriptling Execution Environments

Knot provides three distinct scriptling execution environments, each tailored for specific use cases with different library availability and security constraints.

## Overview

| Environment | Used By          | System Access    | API Client    | Best For                          |
| ----------- | ---------------- | ---------------- | ------------- | --------------------------------- |
| **Local**   | CLI `run-script` | Full (host)      | Yes           | Local development and testing     |
| **MCP**     | AI tool scripts  | None.            | No (internal) | Safe AI tool execution            |
| **Remote**  | Space execution  | Full (container) | Yes           | Scripts in user spaces/containers |

---

## Quick Reference Matrix

| Feature                      | Local         | MCP             | Remote      |
| ---------------------------- | ------------- | --------------- | ----------- |
| **System Access**            | ✓ Full (host) | ✗ None          | ✓ Container |
| **API Client**               | ✓ Yes         | ✗ No (internal) | ✓ Yes       |
| **knot.space lib**           | ✓ API         | ✓ API           | ✓ API       |
| **knot.ai lib**              | ✓ API         | ✓ MCP           | ✓ API       |
| **knot.mcp lib**             | ✓ Tools       | ✓ Special       | ✓ Tools     |
| **subprocess**               | ✓             | ✗               | ✓           |
| **scriptling.runtime**       | ✓             | ✗               | ✓           |
| **scriptling.runtime.kv**    | ✓             | ✗               | ✓           |
| **scriptling.runtime.sync**  | ✓             | ✗               | ✓           |
| **os/pathlib**               | ✓             | ✗               | ✓           |
| **sys**                      | ✓             | ✗               | ✓           |
| **scriptling.glob**          | ✓             | ✗               | ✓           |
| **scriptling.console**       | ✓             | ✗               | ✓           |
| **scriptling.ai.agent**      | ✓             | ✓               | ✓           |
| **scriptling.fuzzy**         | ✓             | ✓               | ✓           |
| **scriptling.mcp**           | ✓             | ✓               | ✓           |
| **scriptling.mcp.tool**      | ✓             | ✓               | ✓           |
| **scriptling.toon**          | ✓             | ✓               | ✓           |
| **scriptling.ai.tools**      | ✓             | ✓               | ✓           |
| **toml**                     | ✓             | ✓               | ✓           |
| **logging**                  | ✓             | ✓               | ✓           |
| **yaml**                     | ✓             | ✓               | ✓           |
| **waitFor**                  | ✓             | ✓               | ✓           |
| **Load from disk**           | ✓ First       | ✗               | ✗           |
| **Load from server**         | ✓ Fallback    | ✓ Only          | ✓ Only      |

---

## Decision Tree: Which Environment?

```
Is it an MCP tool for AI?
├─ YES → MCP Environment (NewMCPScriptlingEnv)
└─ NO → Is it running in a space/container?
    ├─ YES → Remote Environment (NewRemoteScriptlingEnv)
    └─ NO → Local Environment (NewLocalScriptlingEnv)
```

---

## 1. Local Environment

**Function:** `internal/service/scriptling_env.go::NewLocalScriptlingEnv()`

**Used By:**

- Desktop CLI: `knot run-script`
- Agent CLI: `knot-agent run-script`

**Purpose:** Execute scripts locally with full system access and on-demand library loading from disk or server.

### Available Libraries

#### Standard Libraries

- All Python-like standard libraries via `stdlib.RegisterAll()`

#### Extended Libraries

- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **subprocess** - Process execution
- **htmlparser** - HTML parsing and manipulation
- **yaml** - YAML parsing and manipulation
- **waitFor** - Wait for conditions
- **scriptling.runtime** - Runtime utilities (background functions)
- **scriptling.runtime.kv** - Key-value store for runtime state
- **scriptling.runtime.sync** - Concurrency primitives (mutex, wait groups)
- **scriptling.console** - Console output utilities
- **scriptling.glob** - File globbing patterns
- **os** - Operating system interface
- **pathlib** - Filesystem path operations
- **sys** - System-specific parameters (argv)
- **scriptling.ai** - AI agent framework
- **scriptling.ai.agent** - Agentic AI loop with automatic tool execution
- **scriptling.fuzzy** - Fuzzy string matching library
- **scriptling.mcp** - MCP tool helpers
- **scriptling.mcp.tool** - MCP tool parameter access and output
- **scriptling.toon** - TOON encoding/decoding
- **scriptling.ai.tools** - AI tools registry
- **toml** - TOML parsing and manipulation
- **logging** - Logging library
- **knot.space** - Space management operations (via API)
- **knot.ai** - AI completion functions (via API)
- **knot.mcp** - MCP tools library (via API)

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically as imports are encountered
- **Priority**: Local `.py` files are tried first, then fetches from server
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint

### Security Characteristics

- Full system access - use with trusted scripts only
- Can read/write files via pathlib on local machine
- Can execute system commands via subprocess
- Can load arbitrary .py files from disk (tried first)
- Falls back to server libraries if not found locally

### Code Locations

- Environment creation: `internal/service/scriptling_env.go`
- Command implementation: `command/cmdutil/runscript.go`

### Example

```bash
# Run local script with dynamic library loading
knot run-script myscript.py arg1 arg2

# Import local .py files first, then server libraries
import mylib  # Tries mylib.py locally, then fetches from server
```

---

## 2. MCP Environment

**Function:** `internal/service/scriptling_env.go::NewMCPScriptlingEnv()`

**Used By:**

- AI assistants executing MCP tools
- Scripts with `script_type = "tool"`

**Purpose:** Execute tool scripts in a controlled environment for AI integration.

### Available Libraries

#### Standard Libraries

- All Python-like standard libraries via `stdlib.RegisterAll()`

#### Extended Libraries

- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **htmlparser** - HTML parsing and manipulation
- **yaml** - YAML parsing and manipulation
- **waitFor** - Wait for conditions
- **logging** - Logging library
- **scriptling.ai** - AI agent framework
- **scriptling.ai.agent** - Agentic AI loop with automatic tool execution
- **scriptling.fuzzy** - Fuzzy string matching library
- **scriptling.mcp** - MCP tool helpers
- **scriptling.mcp.tool** - MCP tool parameter access and output
- **scriptling.toon** - TOON encoding/decoding
- **ai.tools** - AI tools registry
- **toml** - TOML parsing and manipulation
- **knot.space** - Space management operations (via internal API)
- **knot.ai** - AI completion functions (via MCP server)
- **knot.mcp** - MCP-specific functions with access to tool parameters

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically from server only
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint
- **No filesystem access** for security

### Security Characteristics

- Restricted for AI safety
- No filesystem access
- No command execution
- Cannot load external files
- Only fetches libraries from server
- Limited to safe operations for AI tool usage

### Code Locations

- Environment creation: `internal/service/scriptling_env.go`
- Tool execution: `internal/service/scripts.go::ExecuteScriptWithMCP()`

### Example

```python
import knot.mcp

# Access tool parameters
name = knot.mcp.get_string("name", "default")

# Return result
return knot.mcp.return_string(f"Hello, {name}!")
```

---

## 3. Remote Environment

**Function:** `internal/service/scriptling_env.go::NewRemoteScriptlingEnv()`

**Used By:**

- Space script execution: `knot space run-script`
- Agent-based script execution in containers

**Purpose:** Execute scripts remotely in user spaces with full capabilities and dynamic library loading.

### Available Libraries

#### Standard Libraries

- All Python-like standard libraries via `stdlib.RegisterAll()`

#### Extended Libraries

- **requests** - HTTP client library
- **secrets** - Secure random number generation
- **subprocess** - Process execution
- **htmlparser** - HTML parsing and manipulation
- **yaml** - YAML parsing and manipulation
- **waitFor** - Wait for conditions
- **scriptling.runtime** - Runtime utilities (background functions)
- **scriptling.runtime.kv** - Key-value store for runtime state
- **scriptling.runtime.sync** - Concurrency primitives (mutex, wait groups)
- **scriptling.console** - Console output utilities
- **scriptling.glob** - File globbing patterns
- **os** - Operating system interface
- **pathlib** - Filesystem path operations
- **sys** - System-specific parameters (argv)
- **scriptling.ai** - AI agent framework
- **scriptling.ai.agent** - Agentic AI loop with automatic tool execution
- **scriptling.fuzzy** - Fuzzy string matching library
- **scriptling.mcp** - MCP tool helpers
- **scriptling.mcp.tool** - MCP tool parameter access and output
- **scriptling.toon** - TOON encoding/decoding
- **scriptling.ai.tools** - AI tools registry
- **toml** - TOML parsing and manipulation
- **logging** - Logging library
- **knot.space** - Space management operations (via API)
- **knot.ai** - AI completion functions (via API)
- **knot.mcp** - MCP tools library (via API)

### On-Demand Loading

- **Enabled**: Libraries are loaded dynamically from server only
- **API**: Uses `GET /api/scripts/library/{library_name}` endpoint

### Security Characteristics

- Runs in isolated container (space)
- Full capabilities within container
- No access to host filesystem
- Libraries fetched dynamically from server
- Has API access to spaces, ai, and mcp from within space
- **Requires active agent connection** - script execution fails if space agent is not connected

### Code Locations

- Environment creation: `internal/service/scriptling_env.go`
- Agent handler: `internal/agentapi/agent_client/execute_script.go`

### Example

```bash
# Execute script in a space
knot space run-script myspace myscript arg1 arg2

# Or with pipe support
echo "data" | knot space run-script myspace myscript | jq
```

---

## Library Comparison Matrix

| Library                       | Local            | MCP        | Remote     |
| ----------------------------- | ---------------- | ---------- | ---------- |
| stdlib                        | ✓                | ✓          | ✓          |
| requests                      | ✓                | ✓          | ✓          |
| secrets                       | ✓                | ✓          | ✓          |
| htmlparser                    | ✓                | ✓          | ✓          |
| yaml                          | ✓                | ✓          | ✓          |
| waitFor                       | ✓                | ✓          | ✓          |
| logging                       | ✓                | ✓          | ✓          |
| subprocess                    | ✓                | ✗          | ✓          |
| scriptling.runtime            | ✓                | ✗          | ✓          |
| scriptling.runtime.kv         | ✓                | ✗          | ✓          |
| scriptling.runtime.sync       | ✓                | ✗          | ✓          |
| os                            | ✓                | ✗          | ✓          |
| pathlib                       | ✓                | ✗          | ✓          |
| sys                           | ✓                | ✗          | ✓          |
| scriptling.glob               | ✓                | ✗          | ✓          |
| scriptling.console            | ✓                | ✗          | ✓          |
| scriptling.ai                 | ✓                | ✓          | ✓          |
| scriptling.ai.agent           | ✓                | ✓          | ✓          |
| scriptling.fuzzy              | ✓                | ✓          | ✓          |
| scriptling.mcp                | ✓                | ✓          | ✓          |
| scriptling.mcp.tool           | ✓                | ✓          | ✓          |
| scriptling.toon               | ✓                | ✓          | ✓          |
| scriptling.ai.tools           | ✓                | ✓          | ✓          |
| toml                          | ✓                | ✓          | ✓          |
| knot.space                    | ✓ API            | ✓ API      | ✓ API      |
| knot.ai                       | ✓ API            | ✓ MCP      | ✓ API      |
| knot.mcp                      | ✓ Tools          | ✓ Special  | ✓ Tools    |
| On-demand libs                | ✓ (local+server) | ✓ (server) | ✓ (server) |

---

## Implementation Details

### Dynamic Library Loading

All environments use scriptling's on-demand callback mechanism:

- Libraries loaded when import statements are encountered
- Not bulk-loaded at environment creation
- Different sources based on environment

### Authentication Flow

- **Local/Remote**: Use API client with agent token
- **MCP**: Uses internal API (no network calls)

### Security Isolation

- **MCP**: Most restricted (no system access)
- **Remote**: Container isolated
- **Local**: Full local system access

### Space Script Execution

Scripts executed in spaces require an active agent connection. If the space agent is not connected, script execution will fail with a "Space agent is not connected" error rather than falling back to server execution for security reasons.

---

## Additional Documentation

- [Space Library Reference](scriptling-space-library.md) - Complete documentation for the `knot.space` library
- [AI Library Reference](scriptling-ai-library.md) - Complete documentation for the `knot.ai` library
- [MCP Library Reference](scriptling-mcp-library.md) - Complete documentation for the `knot.mcp` library
