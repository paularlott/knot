# knot integration tests

Black-box tests that boot a **real knot server** (badgerdb, spaces as docker
containers) and drive it **entirely through the API** — no server mocks, no
direct database access. The same suite runs against the OSS and pro repos;
pro adds the `pro` build tag and extra `pro:*` feature areas. Pro-only tests
are documented in `PRO-TESTS.md` in the pro repository.

## Quick start

```sh
task test:integration        # incremental build, then the full suite
```

While it runs, the feature-matrix report is rewritten live at
`build/test/integration-report.md` — `tail -f` it to watch progress. The
same file is the final pass/fail report: one row per feature area, any area
with no tests shows up as a visible gap.

## Configuration

Copy `.env.example` to `.env`. The knobs:

| Variable | Meaning |
|---|---|
| `KNOT_TEST_RUNTIME` | `docker` (default) or `apple` containers |
| `KNOT_TEST_IMAGE_NAMESPACE` / `KNOT_TEST_IMAGE` | base image for spaces |
| `KNOT_TEST_DOCKER_CACHE` | registry-cache prefix (default `hub.anaconda.ovh/docker`; falls back to docker hub when the cache is unreachable; empty = docker hub direct) |
| `KNOT_TEST_CONTAINER_HOST` | host address as seen from inside containers |
| `KNOT_TEST_ZONE` | zone name for test servers |
| `KNOT_TEST_KEEP` | `1` = keep servers/data dirs for debugging |
| `KNOT_TEST_VERBOSE_SERVER` | `1` = stream server logs into the test output |
| `KNOT_TEST_NO_BUILD` | `1` = skip `task build`, use existing `bin/knot` |
| `KNOT_TEST_SPACE_READY_TIMEOUT` | seconds to wait for a space agent (default 600) |

## How a run works

The harness builds the server and agents with `task build`, pulls the space
image, then boots one shared **default server** with an admin and two users
(pro's built-in tier is capped at two users, so there the admin doubles as
the second user and cross-user checks run from the unprivileged side).
A single **workhorse space** boots lazily and is reused by all read-only
tests — that is what keeps the run fast. Spaces are torn down by background
reapers while later tests run. Feature areas that need their own boot-time
configuration (chat, TOTP) or their own servers (cluster, leaf) boot them
inside their tests.

## What each test proves

Files are numbered in journey order: server → auth → users → spaces →
resources → events → ports → logs → clustering.

### Server & auth

| Test | What it proves |
|---|---|
| `TestHealthEndpoint` | `/health` answers 200 unauthenticated |
| `TestPing` | the authenticated ping endpoint answers |
| `TestServerInfo` | `/api/server` reports version/build info |
| `TestClusterInfo` | a standalone server still lists itself as a cluster node |
| `TestLoginWrongPassword` | a bad password is refused with 401 |
| `TestLoginLogout` | login mints a session, `WhoAmI` resolves it, logout kills it |
| `TestTokenAuthAndScopes` | API tokens work, scopes restrict them, deletion revokes immediately |
| `TestTOTPSecondFactor` | full TOTP lifecycle on a dedicated `--enable-totp` server: first login reveals the secret, password alone and wrong codes are then refused, the correct code (computed locally with the same RFC 6238 algorithm) logs in, the ±30s window accepts the previous slice but not two ahead, and an admin clearing the secret resets the flow |
| `TestAuthRateLimitAndFlush` | 8 bad passwords block even the correct one, then the admin flush API clears the blocks |
| `TestServerRestartPreservesState` | a restart keeps users and tokens (badger) and the server comes back serving |

### Users, roles, groups, templates

| Test | What it proves |
|---|---|
| `TestUsersCRUD` | create/get/update/list/delete a user; duplicate username rejected (runs on its own server on pro, which is capped at two users) |
| `TestUserPermissionsEnforced` | a user without permissions cannot create users or write templates, but can still read the template list |
| `TestUserQuotaMaxSpaces` | with `MaxSpaces=1` a second space is refused; quota restored afterwards |
| `TestRolesCRUD` | role create/get/list/update/delete |
| `TestRoleGrantsPermissions` | a role's grants actually unlock those permissions for a holder |
| `TestGroupsCRUD` | group create/list/update/delete |
| `TestTemplatesCRUD` | template lifecycle plus YAML export → import under a new name (the id must change) |

### Spaces

| Test | What it proves |
|---|---|
| `TestSpaceLifecycle` | create/start, update description and custom fields, stop, start, restart — with `echo` proving liveness after each step |
| `TestSpaceUserIsolation` | user1 cannot read, list, or run commands in another user's space |
| `TestSpaceStacks` | two spaces in one stack start, stop together, and `DeleteStack` removes both |
| `TestSpaceFiles` | write/read, ranged read, grep, find, sed, search/replace edit, symlink (checked via `readlink`), delete |
| `TestSpaceRunCommandBehaviour` | commands run through a shell (pipes work), stdout+stderr are combined, they run as the space user in its home, failures report errors |
| `TestSpaceSharing` | a space shared to user1 becomes visible and usable by user1 |
| `TestSpaceTransfer` | a stopped space transfers to another owner and shows under the new owner only |

### Resources

| Test | What it proves |
|---|---|
| `TestVolumesCRUD` | volume create/get/list/update/delete |
| `TestPoolLifecycle` | a pool provisions a live member space and stopping it scales back to zero |
| `TestScriptsSkillsCommandsCRUD` | create/list/delete for python scripts, skills and slash commands |
| `TestStackDefinitionsAndTemplateVars` | stack definitions (create, fetch by name, validate, delete) and template variables |
| `TestMCPServersCRUD` | MCP server registry entries can be managed |
| `TestSearch` | global search returns results for a known name |
| `TestSpaceUsageHistory` | after disk activity the usage-history endpoint returns sampled points |

### Events, audit, ports, logs

| Test | What it proves |
|---|---|
| `TestEventSinkWebhook` | an event sink subscribed to a custom event receives it as a webhook delivery |
| `TestEventsSSE` | the SSE stream carries domain events (a group change produces `groups:changed`) |
| `TestAuditLogRecordsActivity` | an audited action appears in the audit log with the right actor and event |
| `TestPortForwarding` | a forward between spaces carries real data (the SSH banner), throttling applies and is reflected in the port list, stop tears it down |
| `TestSyslogToLogStream` | a syslog line written inside a space reaches the space's log-stream endpoint |

### Dedicated-server features

| Test | What it proves |
|---|---|
| `TestChatOpenAIEndpoint` | a chat-enabled server streams a reply from a fake OpenAI-compatible endpoint |
| `TestSpaceTunnels` | an http tunnel declared by a template port publishes and proxies a space web port |
| `TestAgentRegistrationRequiresProof` | a peer with no registration key — or a forged proof — cannot register as a space, gets no secrets, and cannot disturb the live agent session |

### Clustering

| Test | What it proves |
|---|---|
| `TestClusterGossip` | two servers gossip over TCP: a user and a template created on one node are usable on the other — replication both directions |
| `TestLeafNode` | a leaf server accepts origin credentials, replicates templates, and boots a space of its own |

## What is deliberately not covered

- **The web UI** — every screen; tested manually.
- **OAuth/SSO** — external identity providers and knot's own OAuth2 provider web flow.
- **Dev URL routing** via the built-in DNS server + wildcard domain — manual.
- **1Password** secret provider (Vault is the covered one).
- **TLS** — everything runs over plain HTTP.
- **Runtimes other than docker** — nomad and apple-containers are not exercised by the suite.
- **Storage other than badgerdb** — mysql and redis.
- **The CLI beyond the server** — `knot connect` and the direct-to-database `knot admin` tools (backup, restore, renamezone, setpassword, refresh-base-images; reset-totp's *behaviour* is covered via its API path in the TOTP test).
- **VS Code tunnels, VNC/terminal web frontends** — need external services or a browser; the underlying TCP proxy is covered by the port-forwarding test.
- **Max-uptime auto-stop** — would need time compression; templates disable it.
- **MCP bridge protocol** — registry CRUD only, not live MCP proxying.
- **Real AI providers** — chat is proven against a local fake endpoint only.
