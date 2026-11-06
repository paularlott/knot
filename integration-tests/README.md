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
| `KNOT_TEST_POSTGRES` | DSN of a PostgreSQL server; when set, every server runs on its own fresh database there instead of BadgerDB |
| `KNOT_TEST_MYSQL` | the same for MySQL or MariaDB, e.g. `mysql://root:secret@127.0.0.1:3306` |
| `KNOT_TEST_KEEP` | `1` = keep servers/data dirs for debugging |
| `KNOT_TEST_VERBOSE_SERVER` | `1` = stream server logs into the test output |
| `KNOT_TEST_NO_BUILD` | `1` = skip `task build`, use existing `bin/knot` |
| `KNOT_TEST_SPACE_READY_TIMEOUT` | seconds to wait for a space agent (default 600) |
| `KNOT_TEST_NFS_VERS` | the NFS version tests mount with: `4` (4.0, the default) or `3` |
| `KNOT_TEST_FILES_ROOT` | where file storage tests keep the server's storage and their local folders, to test on another disk (default: a temporary folder) |
| `KNOT_TEST_BIG_TREE` | runs `TestFileSyncBigTree`: `1` for 180,000 files, or a number; skipped when unset, as it takes minutes |
| `KNOT_TEST_LOAD` | multiplies the writers and the time spent writing in `TestFileMultiWriter` (default `1`, about 35 seconds; `4` takes about 70), and the time spent writing in `TestFileSharedFolder` (15 seconds at `1`) |
| `KNOT_TEST_PREV_BIN` | the previous release's `knot` binary, for `TestRestoreFromPreviousRelease`; unset, that test is skipped. Build one from the tag in a worktree, with this checkout's generated `web/` assets and `legal/` files copied in; for the pro suite, a pro build (`go build -tags pro`), as the suite starts servers with `--license` |

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
| `TestOAuthMCPDiscovery` | an unauthenticated `/mcp` request gets a `WWW-Authenticate` challenge whose protected resource metadata leads to the authorization server metadata and its `/register` endpoint; GET and DELETE `/mcp` are authenticated; API tokens still reach `/mcp` |
| `TestOAuthUnregisteredClient` | the OAuth2 flow for a client that never registered: browser login, authorize, grant, PKCE code exchange, the MCP-scoped token works on `/mcp` but not the API, codes are single use, refresh works, and a wrong verifier is refused |
| `TestOAuthRegisteredClient` | dynamic client registration refuses non-https redirects, the registered client completes the same flow with its name on the grant page, and its codes can't be sent to another redirect URI through authorize or a forged approval, nor with a tampered client_id |

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
| `TestCustomEventsReachSharedMethods` | in a real space, a private method with `groups` fails to register and the space keeps the methods it had; another user's custom event reaches the methods shared with its group or with everyone but not a private one or one shared with another group, the owner's own event reaches all of them, and a sender without the shared-method permission reaches none, directly or through a space shared with them; events raised inside a space with `knot event` and `knot.event.emit()` count as its owner's and follow the same rules |
| `TestEventsSSE` | the SSE stream carries domain events (a group change produces `groups:changed`) |
| `TestAuditLogRecordsActivity` | an audited action appears in the audit log with the right actor and event |
| `TestPortForwarding` | a forward between spaces carries real data (the SSH banner), throttling applies and is reflected in the port list, stop tears it down |
| `TestSyslogToLogStream` | a syslog line written inside a space reaches the space's log-stream endpoint |

### Dedicated-server features

| Test | What it proves |
|---|---|
| `TestChatOpenAIEndpoint` | a chat-enabled server streams a reply from a fake OpenAI-compatible endpoint |
| `TestSpaceTunnels` | an http tunnel publishes a space web port and proxies real requests to it, and the service behind it never receives knot's own cookies (plain or `__Host-`), only its own |
| `TestWebVNCSignIn` | the session cookie is host-only; a web VNC host signs in through a single-use handoff bound to that host and gets its own host-only cookie; a code can't be replayed or used on another host; signing out ends VNC access |
| `TestHostCookiesBehindTLSProxy` | with an https `server.url` but plain-http requests (a load balancer ending TLS), cookies take the `__Host-` prefix and Secure; a session cookie planted under the plain name is ignored |
| `TestAgentRegistrationRequiresProof` | a peer with no registration key — or a forged proof — cannot register as a space, gets no secrets, and cannot disturb the live agent session |
| `TestSpaceRestrictions` | a space restricts itself to chosen buckets (read-only refuses writes), can only tighten its restrictions from inside, is confined, and its owner lifts them from outside; a restart clears them |
| `TestFileModes` | the `knot file` command keeps file modes: `cp` both ways, `sync` up, down, two ways and between buckets, a `chmod` alone synced either way; a write that gives no mode keeps it; `admin file restore` brings it back; a mode in the decimal form knot wrote before 0.42 reads as none until `admin file fsck` reports it and `--repair` rewrites it in octal; over a real NFS mount (macOS, or Linux with `nfs-common` and `sudo` without a password), files written by the command show their modes, a file made over NFS keeps its mode through a write and a rename, and a chmod there reaches the bucket; symbolic links sync and copy as links, are links over NFS, and one made over NFS syncs down as a link |
| `TestFileStorageInterchange` | two clustered servers, each with its own database and storage (under `KNOT_TEST_FILES_ROOT` when set): files written on one with `knot file cp` and `sync` (modes and a link), the web interface (an upload gets 0644, an edit keeps the mode), over NFS and, on Knot Pro, S3 (rclone with and without `--metadata`), and a mode `fsck --repair` rewrote, all read the same on the other through `knot file sync`, NFS from macOS, S3, and a Linux space with the bucket mounted over an NFS template volume; files, a chmod and a link written in the space read the same back on the first. macOS only |
| `TestFileSyncBigTree` | with `KNOT_TEST_BIG_TREE`: a tree of 180,000 files syncs with the knot command for this machine and, inside a space, for Linux; a second sync reads no file for its checksum; a watch sees a change and a folder moved in quickly, never walks the whole tree, and can watch every folder (FSEvents on macOS, inotify on Linux); on this machine a two-way sync with nothing changed writes no state, and one with a file changed adds one record to it |
| `TestFileMultiWriter` | two clustered servers, each with its own database and storage, take writes at once from many writers on both: files of their own reach both servers with their content; the same files written by all end the same on both, as one of the versions written; files sharing content (empty ones too) keep it while the other server deletes some; many writers making the same new folders at once over NFS each finish, the folders made once; a file held open and locked over NFS while the nfs-server restarts is written, unlocked and locked again; writes over NFS mounts of both servers do the same as over the API (NFS parts on macOS, or Linux as for `TestFileModes`); afterwards `fsck` finds nothing and neither server logged a file, NFS or cluster error. `KNOT_TEST_LOAD` scales it |
| `TestNFSWithTools` | a bucket mounted over NFS (macOS, or Linux as for `TestFileModes`) used with `tar`, `rsync` and `git`: a tree of text files, scripts, a private file, a 5 MB file, links (one dangling), empty folders and old times goes in and out with `tar -p` and `rsync -a` the same, content, modes, links and times; `rsync` then finds nothing to do, sends one changed file alone and `--delete` removes; a git repository on the mount commits, repacks and passes `fsck`, and a bare one there is cloned, pushed to and cloned again |
| `TestFileSharedFolder` | one folder used as shared data by everything at once: four NFS clients (two on each of two clustered servers, each with its own nfs-server), the API and `knot file cp` on both servers. They write files of their own, all write the same few files (over NFS in place and by renaming a temporary file over), change their own parts of 24 MB files in place, and read throughout. Nothing hangs or fails; API reads are always a whole version; NFS reads are a whole version, or, when the file is replaced as it is read, runs of versions' bytes split where a newer version arrived, or not found or a stale handle (counted), never bytes no version had there; NFS writes made stale by a rename over them are counted; both servers end with every file as last written, the shared files as a version written; a large file changed by one server's clients keeps every client's last change, one changed through both servers holds a written change in every part (a close reporting its save deferred is counted); every NFS client then sees what the servers hold; fsck is clean. Needs NFS as `TestFileModes` does |
| `TestFileServersKilled` | two clustered servers, written through the API of each and an NFS mount of each (files of their own, a few files shared by all, a 16 MB file changed a part at a time in place, every NFS write fsync'd), while each nfs-server and each server in turn is killed with SIGKILL and started again, and at the end a server and its nfs-server together. Nothing hangs; writes fail only while what they go through is down, and every writer's succeed again after the last kill; both servers end with the same files; every file and part holds its last acknowledged write or one tried after it (after failed writes, blocks of those over it, counted), never an older one or none; the shared files hold a version written; each mount then reads what the servers hold; fsck is clean and no process panicked. Needs NFS as `TestFileModes` does |
| `TestFileSnapshot` | two clustered servers, each with a folder of 40 files rewritten through its API as fast as one client can, one file at a time in an order unlike the files' own (write k to file 7k mod 40), while `knot file snapshot` copies the folder on that server ten times. Every snapshot holds, for each file, its last write numbered no more than the newest write the snapshot holds, so it is of one moment, which a copy made a file at a time is not once a write lands while it is made; each has its manifest; both servers end with every snapshot, every file of which reads back from both; refusals (an existing destination, into itself, nothing there) write nothing; fsck is clean |
| `TestFilesNoSync` | a server with `--files-no-sync` warns at start that writes are not forced to disk, takes writes of small and large files, and still has them after it is killed and started again |
| `TestRestoreFromPreviousRelease` | a backup made by the previous release's `knot admin backup` from its server restores with this build: users, groups, roles, template variables, a template with its jobs, and files; a file mode in the decimal form before 0.42 is rewritten by `knot admin file fsck --repair`. Needs `KNOT_TEST_PREV_BIN` |

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
