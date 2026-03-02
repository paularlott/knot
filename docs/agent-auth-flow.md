# Agent Authentication Flow

## Overview

The agent authentication system uses deterministic HMAC-SHA256 tokens that are generated once during agent registration and reused for all API calls within the same zone.

## Key Principles

1. **Deterministic Token Generation**: Tokens are generated using HMAC-SHA256 with inputs:

   - Space ID
   - User ID
   - Zone name
   - Server encryption key

2. **Zone-Specific**: Each zone generates tokens using its own encryption key, so tokens are only valid within the zone they were created in.

3. **Multi-Server Compatibility**: Since tokens are deterministic, all servers in the same zone generate identical tokens for the same space/user combination.

## Authentication Flow

### 1. Agent Registration

```
Agent                    Server
  |                         |
  |--- Register Request --->|
  |    (SpaceId, Version)   |
  |                         |
  |                      [Generate Token]
  |                      HMAC-SHA256(
  |                        spaceId + userId +
  |                        zone + encryptionKey
  |                      )
  |                         |
  |<-- Register Response ---|
  |    (AgentToken, URL,    |
  |     SSH keys, etc.)     |
  |                         |
[Store Token & URL]         |
```

**Code locations**:

- Token generation: `internal/util/crypt/agent_token.go::GenerateAgentToken()`
- Server-side: `internal/agentapi/agent_server/handle_connections.go::handleRegister()`
- Client-side storage: `internal/agentapi/agent_client/agent_server.go` (stores in AgentClient on first registration)
- Token is stored once at `AgentClient` level, not per-server (since all servers generate identical tokens)

### 2. Agent Link Communication

When commands run inside the agent space, they communicate via Unix socket:

```
Command                 Agent Link              Agent Client
  |                         |                         |
  |--- IsAgentRunning() --->|                         |
  |    (Check socket)       |                         |
  |<-- true ----------------|                         |
  |                         |                         |
  |--- GetConnectionInfo -->|                         |
  |    (via socket)         |                         |
  |                         |--- GetServerURL() ----->|
  |                         |<-- stored URL ----------|
  |                         |                         |
  |                         |--- GetAgentToken() ---->|
  |                         |<-- stored token --------|
  |                         |                         |
  |                         |--- GetSpaceId() ------->|
  |                         |<-- space ID ------------|
  |                         |                         |
  |<-- ConnectResponse -----|                         |
  |    (URL, Token, SpaceId)|                         |
```

**Code locations**:

- `internal/agentlink/client.go::IsAgentRunning()` - Just checks socket connectivity
- `internal/agentlink/client.go::GetConnectionInfo()` - Uses `SendWithResponseMsg()` pattern
- `internal/agentlink/handle_connect.go::handleConnect()` - Returns stored credentials
- `internal/agentapi/agent_client/client.go::Get*()` methods - Return stored values

### 3. API Authentication

Commands use the retrieved token to authenticate API calls:

```
Command              Middleware                Database              Server
  |                       |                       |                     |
  |--- API Request ------>|                       |                     |
  |    Bearer: agt_sp_..  |                       |                     |
  |                       |                       |                     |
  |                    [Extract SpaceId]          |                     |
  |                    from token                 |                     |
  |                       |                       |                     |
  |                       |--- GetSpace(spaceId)->|                     |
  |                       |<-- space (userId) ----|                     |
  |                       |                       |                     |
  |                    [Generate HMAC]            |                     |
  |                    with spaceId+userId+zone   |                     |
  |                       |                       |                     |
  |                    [Compare signatures]       |                     |
  |                    ✓ Valid!                   |                     |
  |                       |                       |                     |
  |                       |--- Authorized -------------------->|        |
  |                       |                       |            |        |
  |<------------------------------------------------ Response -|        |
```

**Code locations**:

- Token detection: `internal/util/crypt/agent_token.go::IsAgentToken()`
- Space ID extraction: `internal/util/crypt/agent_token.go::ExtractSpaceIdFromToken()`
- Token validation: `internal/util/crypt/agent_token.go::ValidateAgentToken()`
- Middleware: `internal/middleware/authmiddleware.go::ApiAuth()`
  - Extracts spaceId from token
  - Looks up space to get userId
  - Validates HMAC signature
  - Adds space_id to context

## Multi-Server Architecture

Agents can connect to multiple servers within the same zone for redundancy:

```
Agent                Server A              Server B
  |                      |                     |
  |--- Register -------->|                     |
  |<-- Token: abc123 ----|                     |
[Store Token Once]       |                     |
  |                      |                     |
  |--- State Report ---->|                     |
  |<-- Endpoints: [A,B] -|                     |
  |                      |                     |
  |--- Register ---------------------->|       |
  |<-- Token: abc123 -------------------|      |
[Already stored, skip]   |                     |
```

**Why same token?**

- Both servers use the same inputs: `spaceId`, `userId`, `zone`, `encryptionKey`
- HMAC is deterministic, so identical inputs → identical output
- No coordination needed between servers
- Token is stored once on first registration, subsequent servers provide identical token

**Code locations**:

- Server discovery: `internal/agentapi/agent_client/report_state.go`
- Token storage: `internal/agentapi/agent_client/agent_server.go` (stores on first registration only)
- Token retrieval: `internal/agentapi/agent_client/client.go::GetAgentToken()` (simple getter)

## Security Properties

1. **Zone Isolation**: Tokens only work in the zone they were created in

   - Each zone has its own `encryptionKey`
   - Zone name is embedded in token metadata
   - Middleware validates zone match

2. **Space Binding**: Tokens are bound to a specific space/user

   - Space ID and User ID are in the HMAC
   - Cannot be used for other spaces
   - Middleware verifies space ownership

3. **No Database Lookups**: Validation is cryptographic only

   - HMAC signature proves authenticity
   - Metadata is embedded in token
   - Fast validation without database queries

4. **Tamper Resistance**: Cannot modify token without detection
   - Any change to metadata breaks HMAC signature
   - Cannot forge without knowing `encryptionKey`

## Token Format

```
agt_<spaceId>_<signature>
│   │         │
│   │         └─ Base64URL(HMAC-SHA256(spaceId|userId|zone, encryptionKey))
│   └─ Space ID (plaintext, easy extraction)
└─ Prefix for identification
```

Example:

```
agt_space_123_4n8fk2JdlK9s3mNq8r7tYw2vB5xC6zD8eF1gH3jK5lM7nP9qR
```

**Benefits of this format:**

- Space ID is plaintext → easy extraction, no decoding needed
- Single DB lookup gets userId and zone from space record
- Signature validates authenticity (HMAC cannot be forged)
- Clean, readable format
- Simpler validation logic

## Comparison: Desktop vs Agent Context

| Aspect        | Desktop Context             | Agent Context                    |
| ------------- | --------------------------- | -------------------------------- |
| Auth Token    | User API token from config  | Agent token from registration    |
| Token Type    | `knot_*` or `temp_*`        | `agt_*`                          |
| Token Source  | User creates in UI          | Server generates at registration |
| Storage       | `~/.knot/config.yaml`       | In-memory in agent process       |
| Retrieval     | Read from config file       | Query via Unix socket            |
| Validation    | Database lookup             | HMAC signature check             |
| Space Binding | User has access to multiple | Bound to single space            |

## Command Execution Paths

### Desktop Command (e.g., `knot spaces list`)

```
1. cmdutil.GetClient()
2. IsAgentRunning() → false
3. Load config from ~/.knot/config.yaml
4. Create API client with user token
5. Make API request
6. Middleware validates user token (DB lookup)
```

### Agent Command (same command, run inside space)

```
1. cmdutil.GetClient()
2. IsAgentRunning() → true (socket exists)
3. GetConnectionInfo() via Unix socket
4. Agent returns stored token/URL/spaceID
5. Create API client with agent token
6. Make API request with X-Space-ID header
7. Middleware validates agent token (HMAC check)
```

**Code location**: `command/cmdutil/client.go::GetClient()`

## Benefits of This Design

1. **No Round-Trips**: Token created once, reused forever
2. **No Token Management**: No creation/deletion/expiry logic
3. **Stateless Validation**: No database lookups during auth
4. **Multi-Server Safe**: Deterministic generation ensures consistency
5. **Secure**: Zone-bound, space-bound, cryptographically verified
6. **Simple Commands**: Same command code works in both contexts

## Implementation Notes

- `IsAgentRunning()` only checks socket existence - no commands sent
- `GetConnectionInfo()` uses `SendWithResponseMsg()` for clean request/response
- `handleConnect()` simply returns credentials stored during registration
- All servers in zone generate identical tokens (deterministic HMAC)
- No coordination needed between servers for token generation
- Middleware adds `space_id` to request context for downstream handlers
