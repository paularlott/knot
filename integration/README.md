# knot integration tests

Black-box integration tests that boot a **real knot server** (badgerdb on
disk, spaces as docker containers) and exercise the product **entirely
through the API** — no mocks of the server, no direct database access. The
same suite runs against the OSS repo and the pro repo; the pro build adds
the `pro` build tag (`task test:integration` in each repo does the right
thing) and lights up the extra `pro:*` feature areas.

## Running

```sh
task test:integration        # build + full suite
task test:integration-fast   # skip the slowest boot-time waits
```

Configuration comes from `.env` (see `.env.example`): `KNOT_TEST_RUNTIME`
(docker / apple-containers), `KNOT_TEST_IMAGE_NAMESPACE` + `KNOT_TEST_IMAGE`
(base image for spaces), `KNOT_TEST_DOCKER_CACHE` (optional registry-cache
prefix; empty = pull straight from docker hub), `KNOT_TEST_CONTAINER_HOST`
(the host address containers use to reach the server), `KNOT_TEST_ZONE`,
plus `KNOT_TEST_KEEP` (keep containers/servers after the run),
`KNOT_TEST_VERBOSE_SERVER`, `KNOT_TEST_NO_BUILD` and
`KNOT_TEST_SPACE_READY_TIMEOUT_SECONDS`.

While a run is in progress the feature-matrix report is rewritten live at
`build/test/integration-report.md` — `tail -f` it to see what the suite is
doing. The same file is the final pass/fail report, one row per feature
area.

## How a run works

`TestMain` builds the server + agents with `task build`, makes sure the
space image is available, boots one shared "default" server, provisions an
admin plus one or two users (pro's built-in tier is capped at two users, so
on pro the admin doubles as the second user and cross-user assertions run
from the unprivileged side), and creates the shared template. A single
**workhorse space** is booted lazily and reused by every read-only suite
(files, commands, logs, ports, usage) — this is what keeps the run fast.
Spaces are torn down by background reapers while later tests already run.
Feature areas that need their own boot-time configuration (rate limiting is
on by default; chat, TOTP, tunnels) boot dedicated servers inside their
tests, as do the cluster/leaf/migration tests.

## The tests

### Server & auth

- **`TestHealthEndpoint`** (`00_server`) — `/health` answers 200 on an
  unauthenticated request, proving the HTTP server is up before anything
  else runs.
- **`TestPing`** (`00_server`) — the authenticated ping endpoint answers,
  the lightweight liveness probe clients use.
- **`TestServerInfo`** (`00_server`) — `/api/server` reports the running
  version/build info.
- **`TestClusterInfo`** (`00_server`) — cluster-info lists the server
  itself as a node on a standalone (non-clustered) server.
- **`TestLoginWrongPassword`** (`01_auth`) — a login with a bad password is
  refused with 401.
- **`TestLoginLogout`** (`01_auth`) — full session lifecycle: login returns
  a session token, `WhoAmI` resolves it, logout invalidates it (verified by
  polling until `WhoAmI` fails).
- **`TestTokenAuthAndScopes`** (`01_auth`) — API tokens: a full token can
  call anything, a token scoped to `methods` is refused on `/api/users` but
  allowed on methods, and deleting a token revokes it immediately.
- **`TestTOTPSecondFactor`** (`01b_totp`) — TOTP end to end on a dedicated
  `--enable-totp` server: the shared (TOTP-off) server reports
  `using-totp=false`; a fresh user's first login mints and reveals a secret
  (also visible on the user record to the admin); after that password alone
  and a wrong code are refused; the correct code (computed locally with the
  same RFC 6238 algorithm) logs in and yields a working session; the
  configured ±1-step window accepts the previous 30s slice but rejects two
  slices ahead; and an admin clearing the secret (the API path behind
  `knot admin reset-totp`) makes the next login mint a fresh secret with no
  code needed.
- **`TestAuthRateLimitAndFlush`** (`02_ratelimit`) — eight bad passwords
  exhaust the allowance so even the correct password is blocked, then the
  admin flush API (`DELETE /api/auth/blocked`) clears the blocks and login
  works again.
- **`TestServerRestartPreservesState`** (`02_ratelimit`) — restart the
  server: users and API tokens survive (badger), and the server comes back
  serving requests.

### Users, roles, groups, templates

- **`TestUsersCRUD`** (`03_users`) — create/get/update/list/delete a user,
  including a duplicate-username rejection. On pro this runs on its own
  server because the shared one is capped at two users.
- **`TestUserPermissionsEnforced`** (`03_users`) — a user without
  user-management permission cannot create users (403) or write templates,
  but can still read the template list (the space form needs it).
- **`TestUserQuotaMaxSpaces`** (`03_users`) — with `MaxSpaces=1`, a second
  space creation for that user is refused; the quota is restored afterwards.
- **`TestRolesCRUD`** (`03b_roles`) — role create/get/list/update/delete
  through the API.
- **`TestRoleGrantsPermissions`** (`03b_roles`) — a role carrying specific
  grants actually unlocks those permissions for a user holding it (and not
  others).
- **`TestGroupsCRUD`** (`03b_roles`) — group create/list/update/delete.
- **`TestTemplatesCRUD`** (`04_templates`) — template lifecycle plus export
  to YAML and import of the same definition under a new name (the id must
  change), proving the export format round-trips.

### Spaces

- **`TestSpaceLifecycle`** (`05_spaces`) — create/start a space, update its
  description and custom fields, stop (no longer deployed), start again,
  restart, and prove liveness by running `echo` in it after each step.
- **`TestSpaceUserIsolation`** (`05_spaces`) — user1 cannot read, list, or
  run commands in another user's space, and that space never appears in
  user1's own list.
- **`TestSpaceStacks`** (`05_spaces`) — two spaces in one stack: the stack
  exists, starts, stops together, and `DeleteStack` removes both.
- **`TestSpaceFiles`** (`06_files`) — the full file API against a live
  space: write/read, ranged read with line counts, grep with match
  positions, recursive find, sed replace, unique search/replace edit,
  symlink creation (verified via `readlink`), and delete.
- **`TestSpaceRunCommandBehaviour`** (`06_files`) — command execution
  semantics: shell pipes work (args are joined and run through a shell),
  stdout and stderr are combined, commands run as the space user in their
  home, and a failing command reports an error.
- **`TestSpaceSharing`** (`07_sharing`) — another user shares their space
  to user1, who can then see and work inside it.
- **`TestSpaceTransfer`** (`07_sharing`) — ownership transfer of a stopped
  space; afterwards it shows under the new owner only.

### Resources

- **`TestVolumesCRUD`** (`08_resources`) — volume create/get/list/update/
  delete.
- **`TestPoolLifecycle`** (`08_resources`) — a pool provisions a live
  member space, and stopping the pool scales it back to zero members.
- **`TestScriptsSkillsCommandsCRUD`** (`08_resources`) — create/list/delete
  for python scripts, markdown skills and slash commands.
- **`TestStackDefinitionsAndTemplateVars`** (`08_resources`) — stack
  definitions (create, fetch by name, validate endpoint, delete) and
  template variables (create, list, delete).
- **`TestMCPServersCRUD`** (`08_resources`) — MCP server registry entries
  can be created, listed and deleted.
- **`TestSearch`** (`08_resources`) — global search returns results for a
  known template name.
- **`TestSpaceUsageHistory`** (`08_resources`) — after generating disk
  activity, the usage-history endpoint returns sampled points for the
  space.

### Events, audit, ports, logs

- **`TestEventSinkWebhook`** (`09_events`) — an event sink subscribed to a
  custom event type receives the emitted event as a webhook delivery.
- **`TestEventsSSE`** (`09_events`) — the server-sent-events stream
  carries domain events (a group change produces `groups:changed`).
- **`TestAuditLogRecordsActivity`** (`09b_audit`) — an audited action (a
  space creation) appears in the audit log with the right actor/event.
- **`TestPortForwarding`** (`10_ports`) — forward a local port in one space
  to sshd in the workhorse space, verify data actually flows through it
  (the SSH banner is read via `/dev/tcp`), apply latency/jitter/bandwidth
  throttling and see it reflected in the port list, then stop the forward.
- **`TestSyslogToLogStream`** (`11_logs`) — a syslog line written inside a
  space reaches the space's log-stream endpoint.

### Dedicated-server features

- **`TestChatOpenAIEndpoint`** (`12_dedicated`) — a chat-enabled server
  pointed at a fake OpenAI-compatible SSE endpoint streams a chat reply
  through the API.
- **`TestSpaceTunnels`** (`12b_tunnels`) — an http tunnel declared by a
  template port publishes a space web port and proxies requests to it.

### Clustering

- **`TestClusterGossip`** (`14_cluster`) — two servers gossip over TCP with
  a shared key: each accepts the join, and a user created on one node is
  usable (login) on the other, as is a replicated template — config
  replication both directions.
- **`TestLeafNode`** (`14b_leaf`) — a leaf server joined to an origin:
  origin credentials log in on the leaf, templates replicate to it, and the
  leaf boots and runs a space of its own.

### Pro-only (`-tags "integration pro"`)

- **`TestProLogOutputAndForwarding`** (`15b`) — with
  `--forward-space-logs` and `--log-output-url`, both space syslog lines
  and audit records land in a real VictoriaLogs container, queried back
  through its HTTP API.
- **`TestProLogSpoolReplay`** (`15b`) — while the log output is down, log
  events spool to disk batches under `log-spool/`; once it recovers, a
  fresh event triggers replay and the spooled entries appear in
  VictoriaLogs.
- **`TestProLogSinkIsolation`** (`15c`) — a per-user log sink (a capture
  server inside a space receiving the `KNOT_LOG_SINK_PORT` feed) gets the
  user's own space logs and **never** another user's logs.
- **`TestProAnomalyDetection`** (`15d`) — with a low failed-login
  threshold, enough bad logins produce an "Anomaly Detected" audit record.
- **`TestProVaultSecrets`** (`15e`) — a dev-mode Vault container seeded
  with a KV secret, wired as a knot secret provider, expands
  `${{ secret ... }}` in a template into a space environment variable.
- **`TestProUserActivity`** (`15f`) — the activity API summarises recent
  user actions into counters.
- **`TestProUserAccessOverview`** (`15f`) — the access overview lists the
  spaces a user has access to.
- **`TestProPeerMeshDirectForwards`** (`15f`) — a peermesh port forward
  between users is established and recorded (via relay in the test
  environment; the direct NAT-hole-punch path can't be proven on one
  docker host).
- **`TestProFailedNodeMigration`** (`15g`) — two clustered nodes, a space
  running on node A; killing node A's process causes the space to be
  re-launched on node B, where commands run again.

## What is deliberately not covered

These areas are **not** exercised by this suite (mostly by agreement — see
the report's `no tests` rows for anything that regresses):

- **The web UI** — every screen; tested manually.
- **OAuth/SSO** — external identity providers, and knot's own OAuth2
  provider web flow (authorize/grant/token).
- **Dev URL routing** via the built-in DNS server + wildcard domain —
  removed from the suite, tested manually.
- **1Password** secret provider (Vault is the covered provider).
- **TLS** — the server is tested over plain HTTP only.
- **Runtimes other than docker** — nomad and apple-containers are not
  exercised by the suite.
- **Storage backends other than badgerdb** — mysql and redis.
- **The CLI beyond the server** — `knot connect`, and the direct-to-database
  `knot admin` tools (backup, restore, renamezone, setpassword,
  refresh-base-images, reset-totp — reset-totp's *behaviour* is covered via
  its API path in the TOTP test).
- **VS Code tunnels** and the **VNC/terminal web frontends** (they need
  external services or a browser; the underlying TCP proxy path is covered
  by the port-forwarding test).
- **Max-uptime auto-stop** — would need time compression; the test
  templates disable it.
- **MCP bridge protocol** — only the registry CRUD is covered, not live MCP
  proxying.
- **Real AI providers** — chat is proven against a local fake OpenAI
  endpoint only.
- **Licensing** beyond the built-in two-user pro tier (no license key is
  available to the suite).
- **Single-node health-check auto-restart** of an unhealthy space (node
  failure *with* migration is covered on pro; the plain auto-restart path
  is not separately driven).
