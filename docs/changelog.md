# Changelog

## 0.27.0

- Added space methods: spaces can register JSON-RPC methods backed by one
  long-running stdio method server.
- Added `knot methods register <file>.toml` (or `.py`) for in-space method
  registration.
- Added `knot method list` and `knot method call` for desktop discovery and
  invocation.
- Added `GET /api/methods` and `POST /api/methods/call`.
- Added private/shared method visibility with group filtering and a new shared
  method permission.
- Added discoverable MCP projection for methods with `mcp_tool = true`.
- Added the agent-only `knot.methods` library (`Server` class plus
  `knot.methods.schema` JSON Schema builder) for startup script and `.py`
  registration.
- Added `[server].mode` (`concurrent` default, or `serial`) controlling how many
  requests are in flight on a stdio method server at once. Concurrent mode
  pipelines requests and correlates responses by JSON-RPC id; serial mode sends
  one request at a time for non-re-entrant servers.
- Documented `scriptling.runtime.jsonrpc` (via `scriptling --json-rpc`) as the
  recommended stdio method server for Scriptling-implemented servers. It runs
  each request on a fresh evaluator, so Knot's `[server].mode` should stay
  `concurrent`.
