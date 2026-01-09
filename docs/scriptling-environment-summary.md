# Scriptling Environment Summary

## Overview

Knot has **three** distinct scriptling execution environments, each optimized for specific use cases:

## 1. Local Environment (`NewLocalScriptlingEnv`)

**Function:** `internal/service/scriptling_env.go::NewLocalScriptlingEnv()`

**Used By:**

- Desktop CLI: `knot run-script`
- Agent CLI: `knot-agent run-script`

**Characteristics:**

- Full system access (subprocess, os, pathlib)
- API client access (spaces, ai, mcp libraries)
- On-demand loading: Tries local `.py` files first, then fetches from server
- Best for: Local development and testing with full capabilities

**Code Location:**

- Environment creation: `internal/service/scriptling_env.go`
- Command implementation: `agent/cmd/agentcmd/runscript.go`

---

## 2. MCP Environment (`NewMCPScriptlingEnv`)

**Function:** `internal/service/scriptling_env.go::NewMCPScriptlingEnv()`

**Used By:**

- AI assistants executing MCP tools
- Scripts with `script_type = "tool"`

**Characteristics:**

- **Restricted** system access (no subprocess, os, pathlib)
- Limited API access (spaces via internal API, ai via MCP server)
- Special `mcp` library with tool parameters
- On-demand loading: Server only (no filesystem access)
- Best for: Safe AI tool execution

**Code Location:**

- Environment creation: `internal/service/scriptling_env.go`
- Tool execution: `internal/service/scripts.go::ExecuteScriptWithMCP()`

---

## 3. Remote Environment (`NewRemoteScriptlingEnv`)

**Function:** `internal/service/scriptling_env.go::NewRemoteScriptlingEnv()`

**Used By:**

- Space script execution: `knot space run-script`
- Agent-based script execution in containers

**Characteristics:**

- Full system access within container (subprocess, os, pathlib)
- API client access (spaces, ai, mcp libraries)
- On-demand loading: Server only
- Best for: Executing scripts inside user spaces/containers

**Code Location:**

- Environment creation: `internal/service/scriptling_env.go`
- Agent handler: `internal/agentapi/agent_client/execute_script.go`

---

## Quick Reference Matrix

| Feature              | Local          | MCP          | Remote          |
| -------------------- | -------------- | ------------ | --------------- |
| **Used By**          | CLI run-script | AI MCP tools | Space execution |
| **System Access**    | ✓ Full         | ✗ None       | ✓ Container     |
| **API Client**       | ✓ Yes          | ✗ No         | ✓ Yes           |
| **spaces lib**       | ✓ API          | ✓ Internal   | ✓ API           |
| **ai lib**           | ✓ API          | ✓ MCP        | ✓ API           |
| **mcp lib**          | ✓ Tools        | ✓ Special    | ✓ Tools         |
| **subprocess**       | ✓              | ✗            | ✓               |
| **os/pathlib**       | ✓              | ✗            | ✓               |
| **Load from disk**   | ✓ First        | ✗            | ✗               |
| **Load from server** | ✓ Fallback     | ✓            | ✓               |

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

## Implementation Notes

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
