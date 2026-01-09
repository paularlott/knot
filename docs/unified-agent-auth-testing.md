# Unified Agent Authentication - Testing Guide

## Quick Summary

The unified agent authentication system is now fully implemented! This allows commands to work identically whether run from a desktop client or from within an agent/space.

## What Changed

### For Agents (In-Space)

- **Agent tokens** are now automatically generated when an agent connects
- Token is deterministic: `agt_<signature>_<base64_metadata>`
- Token is valid on all servers in the same zone
- Commands in the agent automatically use the agent link for authentication

### For Desktop Clients

- No changes - everything works as before
- `knot connect <server>` still required
- API tokens continue to work normally

### For Commands

- All commands now use `cmdutil.GetClient()` which:
  1. Checks if agent is running (agent link exists)
  2. If yes, gets server URL + agent token + space ID
  3. If no, uses desktop config (server + API token)
- Single code path handles both contexts automatically

## Testing Instructions

### 1. Test Desktop Client (Existing Functionality)

```bash
# Connect to server
knot connect https://your-server.com

# Test commands
knot scripts list
knot spaces list
knot spaces start my-space
knot spaces run my-space "echo hello"
```

**Expected**: Everything works as before

### 2. Test Agent Authentication

Start a space and connect to it, then from within the space:

```bash
# Agent should be running
# Check agent token generation in server logs
# You should see: "registered with server" with version info

# From within the space, test commands
knot scripts list
knot spaces list
knot spaces run my-space "echo hello"
```

**Expected**:

- Commands work without needing `knot connect`
- Agent link socket provides credentials automatically
- API calls authenticated with agent token

### 3. Verify Agent Token

Check server logs for agent token generation:

```
INFO: registered with server server=<addr> version=<version>
```

Check middleware logs for agent token validation:

```
DEBUG: agent token authenticated space_id=<id> user_id=<id>
```

### 4. Test Multi-Server (If Applicable)

If you have multiple servers in the same zone:

```bash
# Start agent
# It should connect to all servers
# Agent token should work on all servers
```

**Expected**: Token valid on all zone servers

### 5. Security Tests

Test that invalid tokens are rejected:

```bash
# Try to use agent token with wrong zone (should fail)
# Try to use malformed token (should fail)
# Verify regular API tokens still work
```

## Debugging

### Check Agent Is Running

```bash
# Agent link socket should exist
ls -la ~/.knot/agent.sock
```

### Check Agent Token

Look in server logs when agent connects:

- Registration should succeed
- Token should be generated
- Token should be sent to agent

### Check Middleware

Look for authentication logs:

- Agent tokens should be detected (agt\_ prefix)
- Validation should succeed
- Space ID should be in context

### Common Issues

1. **"Agent not running" error**

   - Make sure agent is started: `knot agent start`
   - Check agent socket exists: `ls ~/.knot/agent.sock`

2. **"Authentication failed" error**

   - Check encryption key is set on server
   - Verify zone name matches
   - Check space exists and belongs to user

3. **Commands don't work in agent**
   - Verify agent connected successfully
   - Check agent token was generated
   - Look at middleware logs for auth errors

## Architecture Overview

```
Desktop Context:
  knot command
    └─> cmdutil.GetClient()
          ├─> Check agent link (not found)
          └─> Use desktop config
                └─> Use API token

Agent Context:
  knot command (in space)
    └─> cmdutil.GetClient()
          ├─> Check agent link (found!)
          ├─> Get server URL + agent token + space ID
          └─> Create client with agent credentials
                └─> API calls use agent token (agt_...)
                     └─> Middleware validates agent token
                           ├─> Verify signature (HMAC)
                           ├─> Check zone matches
                           ├─> Verify space exists
                           └─> Load user and continue
```

## Key Files

### Token Generation

- `internal/util/crypt/agent_token.go` - Token generation and validation

### Agent Registration

- `internal/agentapi/agent_server/handle_connections.go` - Generates token
- `internal/agentapi/agent_client/agent_server.go` - Stores token

### Authentication

- `internal/middleware/authmiddleware.go` - Validates agent tokens

### Command Infrastructure

- `command/cmdutil/client.go` - Unified client factory
- `internal/agentlink/client.go` - Agent link helpers

## Success Criteria

✅ Desktop commands work as before
✅ Agent commands work without explicit auth
✅ Same command code for both contexts
✅ Agent tokens validated correctly
✅ Multi-server support maintained
✅ Security enforced (zone, space, user validation)
✅ Old token request code removed

## Next Steps

1. Run through test scenarios above
2. Check logs for any errors
3. Verify authentication works in both contexts
4. Test edge cases (invalid tokens, zone mismatches)
5. Deploy and monitor in production

If issues arise, check the detailed implementation plan in `unified-agent-auth-plan.md`.
