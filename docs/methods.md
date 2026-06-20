# Space Methods

Space methods let a running space publish JSON-RPC methods through Knot. The
Knot server handles discovery, access control, MCP tool projection, and routing
calls back to the space's long-running stdio method server.

## Registering Methods

Inside a space, create a TOML file:

```toml
[server]
type = "stdio"
command = "./bin/notes-rpc"
timeout = 30
mode = "concurrent"

[[methods]]
name = "{{space}}.search"
local_name = "search"
description = "Search indexed notes in this space"
keywords = ["notes", "search", "documents"]
scope = "private"
groups = []
mcp_tool = true

[methods.params_schema]
type = "object"
required = ["query", "tag"]

[methods.params_schema.properties.query]
type = "string"

[methods.params_schema.properties.tag]
type = "string"

[methods.params_schema.properties.limit]
type = "integer"
default = 10

[methods.result_schema]
type = "object"
```

Then register it from the running space:

```sh
knot methods register methods.toml
```

`knot methods register` also accepts a Scriptling file (`methods.py`) so a
Python space can register from a standalone file rather than maintaining a TOML
equivalent:

```sh
knot methods register methods.py
```

The method server is a long-running stdio JSON-RPC process. Knot writes
JSON-RPC requests to the server's stdin and reads responses from its stdout,
correlating each response by JSON-RPC id. Logs should be written to stderr.

## Startup Script Registration

Startup scripts (and `knot methods register file.py`) use the agent-only
`knot.methods` library. Construct a `Server`, attach one or more method
definitions, then call `register()`:

```python
from knot.methods import Server
from knot.methods import schema as s

server = Server("./bin/notes-rpc", timeout=30)

server.method(
    name="{{space}}.search",
    local_name="search",
    description="Search indexed notes in this space",
    keywords=["notes", "search", "documents"],
    scope="private",
    groups=[],
    mcp_tool=True,
    params=s.object(
        query=s.string(),
        tag=s.string(),
        limit=s.optional(s.integer(), default=10),
    ),
    result=s.object(),
)
server.register()
```

The TOML and Python examples above describe the same method.

A space can only have one active method registration at a time. Calling
`register()` replaces everything the space previously registered — every
method from any prior `Server` is removed and only the methods from the most
recent `register()` call remain. If you need to publish methods from more than
one process, combine them into a single `Server` and a single `register()`
call. (This matches the TOML path: a `knot methods register` overwrites the
space's previous registration too.)

### Server constructor

```python
Server(command, *, type="stdio", timeout=30, args=None, mode="concurrent")
```

`type` defaults to `"stdio"` (the only currently supported transport) and is
optional. `mode` is `"concurrent"` (default) or `"serial"`. See
[Concurrency Mode](#concurrency-mode) below.

`args` defaults to `None` and is normalized to an empty list internally; pass
an explicit list to add arguments to the server command.

### Schema builder

`knot.methods.schema` (conventionally imported as `s`) builds JSON Schema
fragments for a method's `params` and `result`. The common kwargs
(`description`, `default`, `enum`, `extra`) are accepted by every builder;
unknown kwargs raise an error. `extra={...}` is an escape hatch for less-common
JSON Schema keywords, and explicit kwargs win over keys in `extra`.

Constraint kwargs are snake_case in the builder and emitted as JSON Schema's
camelCase equivalents (`min_length` → `minLength`, `additional_properties` →
`additionalProperties`, and so on). If a JSON Schema keyword isn't in the
curated list below, use `extra={"keyword": value}`.

- `s.string(*, description, default, enum, format, min_length, max_length, pattern, extra)`
- `s.integer(*, description, default, enum, minimum, maximum, extra)`
- `s.number(*, description, default, enum, minimum, maximum, extra)`
- `s.boolean(*, description, default, enum, extra)`
- `s.null(*, description, default, enum, extra)`
- `s.array(items, *, description, default, enum, min_items, max_items, extra)`
- `s.object(**properties, *, description, default, enum, additional_properties, extra)`
- `s.optional(schema, *, default=None)`

Every property passed to `s.object(...)` is added to the schema's `required`
list unless it is wrapped in `s.optional(...)`. `additional_properties` defaults
to `false`.

If a builder does not cover a schema shape you need, pass a raw JSON Schema
dict to `params=`/`result=` instead — both forms are accepted.

## Concurrency Mode

The `[server]` table accepts a `mode` option that controls how many requests
are in flight on the method server at once:

- `concurrent` (default) — Knot may send many JSON-RPC requests to the method
  server before any response arrives. Responses are matched to callers by id,
  so they can arrive in any order. Use this for method servers that handle
  requests concurrently or that are happy to read requests ahead of producing
  responses.
- `serial` — Knot lets only one request be in flight on the method server at a
  time. The next request is sent only after the previous response is received.
  Use this for method servers that are not re-entrant.

```toml
[server]
type = "stdio"
command = "./bin/notes-rpc"
timeout = 30
mode = "concurrent"
```

In both modes the server's `timeout` applies to each call.

## Scriptling Method Servers

A Scriptling script can be the method server process itself, using the
`scriptling.runtime.jsonrpc` library. The registration metadata
(`name`, `local_name`, `description`, `params_schema`, scope, etc.) is declared
in Knot exactly the same way as for any other method server — the only
difference is that `[server].command` points at `scriptling --json-rpc`.

TOML form (`methods.toml`):

```toml
[server]
type = "stdio"
command = "scriptling"
args = ["--json-rpc", "./setup.py"]
mode = "concurrent"

[[methods]]
name = "{{space}}.search"
local_name = "search"
description = "Search indexed notes in this space"
scope = "private"
mcp_tool = true

[methods.params_schema]
type = "object"
required = ["query", "tag"]

[methods.params_schema.properties.query]
type = "string"

[methods.params_schema.properties.tag]
type = "string"

[methods.params_schema.properties.limit]
type = "integer"
default = 10
```

Scriptling form (`methods.py`):

```python
from knot.methods import Server
from knot.methods import schema as s

server = Server("scriptling", args=["--json-rpc", "./setup.py"], mode="concurrent")

server.method(
    name="{{space}}.search",
    local_name="search",
    description="Search indexed notes in this space",
    scope="private",
    mcp_tool=True,
    params=s.object(
        query=s.string(),
        tag=s.string(),
        limit=s.optional(s.integer(), default=10),
    ),
    result=s.object(),
)
server.register()
```

Register with `knot methods register methods.toml` or `knot methods register
methods.py` from inside the space.

`setup.py` routes each method's `local_name` (the name Knot forwards over
stdio) to a handler function referenced as `"library.function"`:

```python
import scriptling.runtime as runtime

# "search" matches the local_name of the registered method.
runtime.jsonrpc.method("search", "handlers.search")
```

And `handlers.py` defines the handler. Each handler takes a `params` dict
(matching the method's `params_schema`) and returns a JSON-serializable result
(matching `result_schema`). Handlers are referenced as `"library.function"`
strings rather than closures so the server can spin up a fresh, isolated
evaluator per request:

```python
def search(params):
    query = params["query"]
    tag = params.get("tag", "")
    limit = params.get("limit", 10)
    # ...your search logic...
    return {
        "results": [
            {"title": "Receipt for " + query, "tag": tag},
        ],
    }
```

Each request runs on a fresh, isolated evaluator, so the server is concurrent
by default — leave `mode = "concurrent"` (the default). See the
[Scriptling JSON-RPC example](https://scriptling.dev/examples/jsonrpc-server)
for a complete working server with notifications and structured errors.

## Visibility

`scope` defaults to `private`.

- `private` methods are visible only to the owning user.
- `shared` methods are visible to users with the shared-method permission.
- `groups` restricts a shared method to users in at least one listed group.
  Entries may be group **names** (e.g. `"Group 3b"`) or group **IDs** (UUIDs);
  both are accepted. Unknown names fail registration so a typo doesn't silently
  exclude every caller.
- An empty `groups` list means all authenticated users in the zone with the
  shared-method permission.

The owner always sees and calls their own methods under the bare canonical
name — never under `user.<self>.<name>`. Other users always see and call a
shared method under `user.<owner>.<canonical>`, regardless of whether the
canonical name contains a dot. So a method registered as `notes.search` by
`paul` is shown as `notes.search` to paul and as `user.paul.notes.search` to
everyone else.

## Calling Methods

List visible methods:

```sh
knot method list
```

Call a method with JSON params:

```sh
knot method call notes.search '{"query":"receipt"}'
```

Or pipe params:

```sh
echo '{"query":"receipt"}' | knot method call notes.search | jq
```

The HTTP API is:

```text
GET /api/methods
POST /api/methods/call
```

`POST /api/methods/call` accepts a JSON-RPC 2.0 request:

```json
{
  "jsonrpc": "2.0",
  "method": "notes.search",
  "params": { "query": "receipt" },
  "id": 1
}
```

Returns a JSON-RPC response:

```json
{
  "jsonrpc": "2.0",
  "result": { "results": [ ... ] },
  "id": 1
}
```

### Batch requests

Send a JSON array to call multiple methods in one request. Each item is routed
independently — items can target different spaces and Knot naturally splits the
batch by destination agent:

```json
[
  {"jsonrpc":"2.0","method":"containers.search","params":{"query":"a"},"id":1},
  {"jsonrpc":"2.0","method":"user.paul.notes.search","params":{"query":"b"},"id":2}
]
```

Returns a JSON array of responses (order is not guaranteed by JSON-RPC 2.0 but
Knot preserves request order):

```json
[
  {"jsonrpc":"2.0","result":{...},"id":1},
  {"jsonrpc":"2.0","result":{...},"id":2}
]
```

### Notifications

A notification is a JSON-RPC request without an `id` field. Knot forwards it
to the method server but does not return a JSON-RPC response. For a single
notification, the HTTP response is `204 No Content`.

```json
{
  "jsonrpc": "2.0",
  "method": "containers.healthcheck",
  "params": {}
}
```

Notifications can also appear inside a batch — they're forwarded but produce no
entry in the response array. If all items in a batch are notifications, the
HTTP response is `204 No Content`.

## MCP Tools

If `mcp_tool = true`, Knot registers the method as a discoverable MCP tool for
users who can see it. Dots in method names are rewritten to underscores for the
MCP tool name:

```text
notes.search -> notes_search
```

`mcp_tool` defaults to `false`.

## Scoped API Tokens

By default, an API token inherits the full authenticated surface of the user it
belongs to — every endpoint the user can reach, the token can reach. For
machine-to-machine access (e.g. handing a token to an external consumer that
only needs to call methods or use MCP), tokens can be **scoped** to limit which
endpoint groups they can reach.

Scopes are set at token-creation time in the web UI (API Tokens → New Token)
or via the API (`POST /api/tokens` with a `scopes` array). An empty or absent
`scopes` field means unrestricted (full access). The available scopes are:

- `methods` — discover and call space methods (`GET /api/methods`,
  `POST /api/methods/call`)
- `mcp` — use the MCP server (`POST /mcp`)

A scoped token can combine multiple scopes. For example, a token with
`scopes: ["methods", "mcp"]` can reach both the methods REST endpoints and the
MCP endpoint. A token with `scopes: ["methods"]` can reach methods REST only —
it will get `403 Forbidden` on every other endpoint, including `/mcp` and
`/api/spaces`.

Scope enforcement is global: the check runs inside the server's authentication
middleware and applies to every API route. Tokens without scopes (including all
pre-existing tokens) are unaffected. Session cookies and agent tokens bypass
the check entirely.

### Typical setup

1. Create a user (e.g. `rpc-bot`) with `PermissionUseMethods` and/or
   `PermissionUseMCPServer`, plus the group memberships needed for the target
   methods.
2. Create an API token on that user. In the web UI, leave "Full Access"
   unchecked and tick the relevant scope(s).
3. Hand the token to the consumer.

Even if the user account is later granted broader permissions, the scoped token
still can't exercise them — only the endpoint groups named in its scopes are
reachable.

## Schemas

`params_schema` (TOML) / `params` (Scriptling) and `result_schema` / `result`
use JSON Schema. Knot validates that schemas are well-formed at registration
time and passes them through to discovery and MCP. Runtime parameter and
result validation is left to the method server.

In TOML, write the schema inline as shown in the registration example above.
In startup scripts and `methods.py` files, prefer the `knot.methods.schema`
builder. If the builder does not cover a case you need, fall back to a raw
dict.

The supported schema types are:

- `object`
- `array`
- `string`
- `integer`
- `number`
- `boolean`
- `null`
