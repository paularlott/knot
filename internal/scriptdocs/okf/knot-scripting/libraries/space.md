---
type: "API Reference"
title: "knot.space"
description: "Space management functions for creating, running, and editing spaces."
resource: "https://getknot.dev/reference/libraries/space/"
tags:
  - "spaces"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/space/"
---
# knot.space

The `knot.space` library provides space management functions for scripts.

Inside AI scripts (`run_script`) the port forward functions (`port_forward`, `port_apply`, `port_list`, `port_stop`, `port_throttle`) and the tunnel functions (`tunnel_start`, `tunnel_list`, `tunnel_stop`) are refused: they call `/space-io/` endpoints, and an AI script may only call `/api/` endpoints.

---

## Functions

| Function | Description |
|----------|-------------|
| `create(name, template_name, description='', shell='bash', depends_on=None, stack='', selected_node_id='', alt_names=None, icon_url='', custom_fields=None, startup_script_id='', ip_address='', start_on_create=False)` | Create a new space |
| `delete(name)` | Delete a space by name |
| `get(name)` | Get detailed space information |
| `update(name, new_name=None, ...)` | Update space properties |
| `start(name)` | Start a space |
| `stop(name)` | Stop a space |
| `restart(name)` | Restart a space |
| `stop_for_user(user)` | Stop every running space a user owns in this zone |
| `list(all_zones=False, user_id=None)` | List all spaces for the current user, or another user's |
| `is_running(name)` | Check if a space is running (the container is up — its agent may still be connecting; see `is_ready`) |
| `is_ready(name)` | Check if a space is running with its agent connected — the state required by `run()`, file operations, and other agent-backed calls |
| `wait_for_start(name, timeout=30, interval=2)` | Wait for a space to be running with its agent connected; returns `False` on timeout, never stops an already-running space |
| `usage_current(name)` | Get current resource usage for a space |
| `usage_history(name, range='1h')` | Get historical resource usage for a space |
| `get_description(name)` | Get the description of a space |
| `set_description(name, description)` | Set the description of a space |
| `get_ip_address(name)` | Get a bridged KVM space's static IP address (empty for other platforms) |
| `set_ip_address(name, ip_address)` | Set a bridged KVM space's static IP address (space must be stopped) |
| `get_dependencies(name)` | Get the dependency space IDs for a space |
| `set_dependencies(name, depends_on)` | Set the dependency spaces for a space |
| `get_stack(name)` | Get the stack name for a space |
| `set_stack(name, stack)` | Set the stack name for a space |
| `get_field(name, field)` | Get a custom field value |
| `set_field(name, field, value)` | Set a custom field value |
| `restrict(name=None, buckets=None, files='', no_secrets=False, lock=False, secrets_allow=None, secrets_deny=None)` | Restrict what a space can reach |
| `restrictions(name=None)` | Get a space's restrictions |
| `clear_restrictions(name)` | Lift a space's restrictions (not from inside a space) |
| `transfer(name, user_id)` | Transfer space ownership |
| `share(name, user_ids)` | Share space with one or more users |
| `unshare(name, user_id=None)` | Remove space share, optionally for a specific user |
| `run_script(name, script_name, args=None)` | Execute a script in a space |
| `eval(name, code, args=None)` | Execute inline Scriptling code in a space |
| `run(name, command, args=None, timeout=30, workdir='', check=True)` | Execute a command in a space; raises if it fails unless `check=False` |
| `read_file(name, file_path, offset=0, limit=0)` | Read file contents (or a 1-based line range) from a space |
| `write_file(name, file_path, content, mode='overwrite', ...)` | Write content to a file (overwrite/append/prepend; optional mtime + perm) |
| `grep(name, pattern, path, ...)` | Search file contents (regex or literal) |
| `find(name, path='.', ...)` | Find files/directories by name, type, mtime, size |
| `find_entries(name, path='.', ...)` | Like `find()` but each entry includes size, mtime, is_dir; opt-in hash, link_target, file_perm |
| `sed_replace(name, old, new, path, ...)` | Replace literal string in files (in-place) |
| `sed_replace_pattern(name, pattern, new, path, ...)` | Replace regex matches in files (in-place) |
| `sed_extract(name, pattern, path, ...)` | Extract regex capture groups from files |
| `edit_file(name, file_path, search, replace, ...)` | Targeted search-and-replace edit (unique match required) |
| `delete_file(name, file_path, recursive=False, ...)` | Delete a file or directory (idempotent on missing paths) |
| `port_forward(source_space, local_port, remote_space, remote_port, persistent=False, force=False)` | Forward a port between spaces |
| `port_apply(source_space, forwards)` | Replace all port forwards with the given list |
| `port_list(name)` | List active port forwards |
| `port_stop(name, local_port)` | Stop a port forward |
| `port_throttle(name, local_port, latency_ms=0, jitter_ms=0, bandwidth_kb=0, timeout_ms=0, down=False, reset=False)` | Apply latency, jitter, and/or bandwidth limits to a port forward |
| `tunnel_start(space, protocol, port, name, server="", token="")` | Start an agent-owned web tunnel in a space, optionally on another knot server |
| `tunnel_list(space)` | List agent-owned web tunnels in a space |
| `tunnel_stop(space, name)` | Stop an agent-owned web tunnel in a space |
| `emit_event(name, event_type, payload=None)` | Raise a custom event for a space |
| `list_methods()` | List the space methods you may call |
| `call_method(method, params=None, space=None, timeout=420)` | Call a space method over JSON-RPC and return its result |

---

## Usage

```python
import knot.space as space

# List all spaces
spaces = space.list()
for s in spaces:
    print(f"{s['name']}: {'running' if s['is_running'] else 'stopped'}")

# Create a new space
space_id = space.create("my-space", "ubuntu", description="My dev environment")

# Start a space
space.start("my-space")

# Execute a command
output = space.run("my-space", "ls", args=["-la", "/tmp"])
print(output)

# Read a file
content = space.read_file("my-space", "/etc/hostname")
print(content)
```

---

## Function Details

### create(name, template_name, description='', shell='bash', depends_on=None, stack='', selected_node_id='', alt_names=None, icon_url='', custom_fields=None, startup_script_id='', ip_address='', start_on_create=False) {#create}

Create a new space.

**Parameters:**
- `name` (string): Name for the new space
- `template_name` (string): Name or ID of the template to use
- `description` (string, optional): Description for the space
- `shell` (string, optional): Shell to use (default: "bash")
- `depends_on` (list, optional): List of dependency space names or IDs
- `stack` (string, optional): Stack name to group this space under
- `selected_node_id` (string, optional): Node ID to assign for local-container spaces. Leave empty to auto-select.
- `alt_names` (list, optional): Additional HTTP route names, each with `name` and `port`
- `icon_url` (string, optional): Icon URL
- `custom_fields` (list, optional): Custom field values as `{"name": "...", "value": "..."}`. Required fields must be set and select/autocomplete values must be one of the field's options — [template.get](../template/) with `resolve_options=True` lists each field's definition and valid values, and the API error names what to fix when a value is rejected
- `startup_script_id` (string, optional): Startup script ID
- `ip_address` (string, optional): Static IP for bridged KVM templates, chosen from the template's network range (validated against the range and the addresses already in use). Unused by other platforms; NAT KVM templates never take an IP
- `start_on_create` (bool, optional): Start the space immediately after it is created

**Returns:** `string` - The space ID of the newly created space

---

### update(name, new_name=None, ...)

Update a space while preserving fields you do not pass.

**Parameters:**
- `name` (string): Name or ID of the space to update
- `new_name` (string, optional): New space name
- `description` (string, optional): New description
- `shell` (string, optional): New default shell
- `template_name` (string, optional): New template name or ID
- `depends_on` (list, optional): New dependency space names or IDs
- `stack` (string, optional): New stack name, or empty string to unstack
- `selected_node_id` (string, optional): New node ID where allowed
- `alt_names` (list, optional): New additional route names
- `icon_url` (string, optional): New icon URL
- `custom_fields` (list, optional): New custom field values
- `startup_script_id` (string, optional): New startup script ID
- `ip_address` (string, optional): New static IP for bridged KVM spaces — requires the space stopped; the address is validated against the template's range and applied to the VM at its next boot

**Returns:** `bool` - True on success

---

### is_running(name)

Check if a space is running.

Note that a freshly started space reports running as soon as its container is up, which can be seconds before the in-container agent connects back to the server. Calls that need the agent — `run()`, `read_file()`, `grep()`, and friends — fail with an "Agent session not found for space" error in that window. Use `is_ready()` to check for the agent-backed state.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True if the space is running

---

### is_ready(name)

Check if a space is running **with its agent connected** to the server.

True only when the space is running and its agent session is live — the state from which `run()`, `read_file()`, and other agent-backed calls succeed. A starting space passes `is_running()` before it passes `is_ready()`.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True if the space is running with a live agent session

---

### wait_for_start(name, timeout=30, interval=2)

Wait for a space to be running with its agent connected.

Returns True immediately if the space is already ready — the space is never stopped or restarted. Otherwise polls every `interval` seconds until the space reports running and its agent has registered, or until `timeout` expires.

Waits for the agent as well as the container: the space reports running before the in-container agent connects, and command execution fails until it does, so waiting on `is_running()` alone races.

**Parameters:**
- `name` (string): Name or ID of the space
- `timeout` (int, optional): Maximum seconds to wait (default 30)
- `interval` (int, optional): Seconds between polls (default 2)

**Returns:** `bool` - True if the space is running with its agent connected, False if the timeout expired

---

### start(name)

Start a stopped space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True on success; raises on API error

---

### stop(name)

Stop a running space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True on success; raises on API error

---

### restart(name)

Restart a running space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True on success; raises on API error

---

### delete(name)

Delete a space and its data.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `bool` - True on success; raises on API error

---

### run(name, command, args=None, timeout=30, workdir='', check=True)

Execute a command in a running space.

**Parameters:**
- `name` (string): Name or ID of the space
- `command` (string): Command to execute
- `args` (list, optional): Arguments for the command (default: `None`, no arguments)
- `timeout` (int, optional): Timeout in seconds (default: 30)
- `workdir` (string, optional): Working directory
- `check` (bool, optional): When `True` (the default), a failed command raises. When `False`, nothing is raised for a failed command and a dict is returned instead

**Returns:** with `check=True`, `string` - the command output. With `check=False`, a `dict` with `output` (given for a failed command too), `exit_code` (`124` when the command timed out), `success` (`True` when the command exited with code 0) and `error` (empty string on success).

**Raises:** on API error, and with `check=True` when the command fails — a non-zero exit, a timeout or no usable shell. The exception message holds the error, the exit code and the command's output.

```python
import knot.space as space

# Raises if the build fails
print(space.run("my-space", "make", args=["build"]))

# Inspect the exit code yourself
result = space.run("my-space", "test", args=["-f", "/tmp/ready"], check=False)
if result["exit_code"] != 0:
    print("not ready yet")
```

---

### usage_current(name)

Get the current resource usage point for a space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `dict` containing the current usage point: `bucket_start` and `bucket_kind` (`"minute"`), plus `is_live` (only present when `True`, i.e. a fresh reading from the running agent) and `resource_usage` (only present when the space is running in this zone with its agent connected). Read the optional keys with `.get()`. `resource_usage` has `cpu_percent`, `memory_used_bytes`, `memory_limit_bytes`, `disk_used_bytes`, `disk_limit_bytes` and, for live readings, `cpu_cores`.

---

### usage_history(name, range='1h')

Get historical resource usage points for a space.

**Parameters:**
- `name` (string): Name or ID of the space
- `range` (string, optional): `"1h"` for minute samples or `"7d"` for daily samples

**Returns:** `dict` containing `space_id`, `range`, `bucket_kind`, and `points`.

---

### run_script(name, script_name, args=None)

Execute a named script in a running space.

**Parameters:**
- `name` (string): Name or ID of the space
- `script_name` (string): Name of the script to execute
- `args` (list, optional): Script arguments

**Returns:** `dict` with `output` and `exit_code`

---

### eval(name, code, args=None)

Execute inline Scriptling code in a running space. Unlike `run_script`, which
looks up a stored script by name, `eval` sends the `code` directly so no script
needs to exist in the database. The code runs in the target space's agent with
the same permissions, libraries, and argument conventions as a named script
(`argv[0]` is `"inline"`).

**Parameters:**
- `name` (string): Name or ID of the space
- `code` (string): Scriptling source to evaluate
- `args` (list, optional): Script arguments

**Returns:** `dict` with `output`, `error` (empty string on success), and `exit_code`

```python
import knot.space as space

result = space.eval("my-space", "print('hello from space')")
print(result["output"])
```

---

### read_file(name, file_path, offset=0, limit=0)

Read file contents from a running space, optionally a 1-based line range. When
both `offset` and `limit` are 0/omitted the whole file is returned (the default).
The full file is read by the agent, but the response is sliced server-side so
the client only receives the requested lines.

**Parameters:**
- `name` (string): Name or ID of the space
- `file_path` (string): Path to the file
- `offset` (int, optional): 1-based line number to start at. `0` = from the beginning
- `limit` (int, optional): Maximum number of lines to return. `0` = no limit (whole file)

**Returns:** `string` - File contents (or the requested line range)

**Raises:** on API error, and when the agent cannot read the file — for example when it does not exist.

```python
import knot.space as space
# Read lines 100-119 of a large file
chunk = space.read_file("web", "/var/log/app.log", offset=100, limit=20)
```

---

### write_file(name, file_path, content, mode='overwrite', mtime_ns=None, file_perm=None)

Write content to a file in a running space. By default the file is overwritten;
pass `mode='append'` to add to the end, or `mode='prepend'` to add to the
beginning of an existing file.

**Parameters:**
- `name` (string): Name or ID of the space
- `file_path` (string): Path to the file
- `content` (string): Content to write
- `mode` (string, optional): `"overwrite"` (default), `"append"`, or `"prepend"`
- `mtime_ns` (int, optional): Modification time as Unix nanoseconds. When set, the agent applies it via `os.Chtimes` after the write so the destination file carries the same mtime as the source — useful for sync tools that compare mtimes to detect changes.
- `file_perm` (int, optional): Permission bits as an int (e.g. `0o644`, `0o755`). When set, the agent applies them via `os.Chmod` after the write. Pass the raw int, not a string.

**Returns:** `bool` - True on success

**Raises:** on API error, and when the agent cannot write the file (for example a missing directory or no permission).

---

### grep(name, pattern, path, literal=False, recursive=False, ignore_case=False, glob='', follow_links=False, max_size=0, workdir='')

Search file contents in a running space using a regex or literal pattern. Runs
in the space's agent via a parallel worker pool — no file contents leave the
space, only matching lines are returned.

**Parameters:**
- `name` (string): Name or ID of the space
- `pattern` (string): Regular expression (or literal string when `literal=True`)
- `path` (string): File or directory to search (relative to `workdir` if given)
- `literal` (bool, optional): Treat `pattern` as a literal string. Default `False`
- `recursive` (bool, optional): Recurse into subdirectories. Default `False`
- `ignore_case` (bool, optional): Case-insensitive matching. Default `False`
- `glob` (string, optional): Only search files matching this glob, e.g. `"*.py"`
- `follow_links` (bool, optional): Follow symlinks. Default `False`
- `max_size` (int, optional): Skip files larger than this many bytes. `0` = default 1 MiB, negative = unlimited
- `workdir` (string, optional): Resolve relative `path` against this directory

**Returns:** `list` of match dicts `{"file": str, "line": int, "text": str}`

```python
import knot.space as space
for m in space.grep("web", "TODO", "src", recursive=True, glob="*.py"):
    print(f"{m['file']}:{m['line']}: {m['text']}")
```

---

### find(name, path='.', recursive=True, type='any', name_glob='', mtime_min=None, mtime_max=None, size_min=None, size_max=None, include_hidden=False, follow_links=False, max_depth=0, workdir='')

Find files and directories in a running space by name, type, modification time,
or size. Runs in the space's agent via a concurrent walker; recursive by default.

**Parameters:**
- `name` (string): Name or ID of the space
- `path` (string, optional): Directory (or file) to search under. Default `"."`
- `recursive` (bool, optional): Descend into subdirectories. Default `True`
- `type` (string, optional): `"file"`, `"dir"`, or `"any"`. Default `"any"`
- `name_glob` (string, optional): Shell-style glob matched against the base name
- `mtime_min` / `mtime_max` (float, optional): Epoch-time bounds (seconds)
- `size_min` / `size_max` (int, optional): Size bounds in bytes
- `include_hidden` (bool, optional): Match dot-entries. Default `False`
- `follow_links` (bool, optional): Follow symlinks. Default `False`
- `max_depth` (int, optional): Maximum recursion depth. `0` = unlimited
- `workdir` (string, optional): Resolve relative `path` against this directory

**Returns:** `list` of matching path strings (arbitrary order)

---

### find_entries(name, path='.', recursive=True, type='any', name_glob='', mtime_min=None, mtime_max=None, size_min=None, size_max=None, include_hidden=False, follow_links=False, max_depth=0, include_hash=False, include_symlinks=False, workdir='')

Same search semantics as `find()`, but each entry is returned as a dict with
`path`, `size`, `mtime`, and `is_dir` — enough to decide whether an entry has
changed without re-reading the bytes (e.g. for differential sync). If you only
need paths, prefer `find()`.

**Parameters:** Same as `find()`, plus:
- `include_hash` (bool): When `True`, each file is crc64-hashed and the checksum is returned in `hash`. Use for definitive change detection — two entries with matching `hash` have identical bytes.
- `include_symlinks` (bool): When `True`, symlink entries are yielded with their `link_target` instead of being followed.

**Returns:** `list` of entry dicts (arbitrary order), each containing:
- `path` (str): Matching path
- `size` (int): Size in bytes (`0` for directories)
- `mtime` (float): Modification time, epoch seconds
- `is_dir` (bool): `True` if the entry is a directory
- `file_perm` (int, optional): File permission bits (e.g. `420` for `0o644`); omitted when zero
- `hash` (int, optional): crc64 (ISO) checksum as an unsigned integer (only with `include_hash=True`; omitted for directories and empty files)
- `link_target` (str, optional): Symlink target path (only with `include_symlinks=True`)

---

### sed_replace(name, old, new, path, recursive=False, ignore_case=False, glob='', follow_links=False, max_size=0, workdir='')

Replace every literal occurrence of `old` with `new` in a file (or files under a
directory). `old` is matched literally, not as a regular expression. Files are
modified in place using an atomic temp-file + rename.

**Parameters:**
- `name` (string): Name or ID of the space
- `old` (string): Literal string to search for
- `new` (string): Replacement string
- `path` (string): File or directory to modify
- `recursive` (bool, optional): Default `False`
- `ignore_case` (bool, optional): Default `False`
- `glob` (string, optional): Only modify files matching this glob
- `follow_links` (bool, optional): Default `False`
- `max_size` (int, optional): Skip files larger than this many bytes
- `workdir` (string, optional): Resolve relative `path` against this directory

**Returns:** `int` - the number of files modified

---

### sed_replace_pattern(name, pattern, new, path, recursive=False, ignore_case=False, glob='', follow_links=False, max_size=0, workdir='')

Replace every regex match of `pattern` with `new`. Capture groups may be
referenced in `new` as `${1}`, `${2}`, or `${name}`. Files are modified in place
using an atomic temp-file + rename.

**Parameters:** as `sed_replace`, with `pattern` interpreted as a regular
expression (Go regexp syntax) instead of a literal string.

**Returns:** `int` - the number of files modified

```python
import knot.space as space
# Rename get_* functions to fetch_*
n = space.sed_replace_pattern("web", r"def get_(\w+)\(", "def fetch_${1}(", "src/app.py")
print(f"{n} file(s) modified")
```

---

### sed_extract(name, pattern, path, recursive=False, ignore_case=False, glob='', follow_links=False, max_size=0, workdir='')

Extract regex capture groups from a file (or files under a directory). Read-only
— does not modify files.

**Parameters:**
- `name` (string): Name or ID of the space
- `pattern` (string): Regular expression with capture groups
- `path` (string): File or directory to search
- `recursive` (bool, optional): Default `False`
- `ignore_case` (bool, optional): Default `False`
- `glob` (string, optional): Only search files matching this glob
- `follow_links` (bool, optional): Default `False`
- `max_size` (int, optional): Skip files larger than this many bytes
- `workdir` (string, optional): Resolve relative `path` against this directory

**Returns:** `list` of match dicts `{"file": str, "line": int, "text": str, "groups": [str, ...]}`. `groups` is absent when the pattern has no capture groups, so read it with `.get("groups", [])`.

---

### edit_file(name, file_path, search, replace, workdir='')

Perform a targeted search-and-replace edit on a single file. The search text
must appear **exactly once** in the file; the operation fails if it matches zero
or multiple times. The modification is written atomically (temp file + rename).

Unlike `sed_replace` (which replaces **all** occurrences), `edit_file` targets
**one** specific occurrence with uniqueness verification — the gold standard for
coding-agent edits where "replace all" is dangerous. Provide enough surrounding
context in `search` to make the match unique.

**Parameters:**
- `name` (string): Name or ID of the space
- `file_path` (string): Path to the file to edit
- `search` (string): Exact text to find (may span multiple lines)
- `replace` (string): Replacement text
- `workdir` (string, optional): Resolve relative `file_path` against this directory

**Returns:** `int` - bytes written

**Raises:** Exception if the search text is not found, matches multiple times, or the agent fails.

```python
import knot.space as space
# Replace a specific function definition (unique match)
n = space.edit_file("web", "src/app.py",
    search="def get_user(id):\n    pass",
    replace="def fetch_user(user_id):\n    return User(user_id)")
```

---

### delete_file(name, file_path, recursive=False, workdir='')

Delete a file or directory in a running space. Missing paths return success
(idempotent), which lets sync tools feed it a delete list computed against a
slightly stale remote snapshot without race-condition errors.

**Parameters:**
- `name` (string): Name or ID of the space
- `file_path` (string): File or directory to delete (relative to `workdir` if given)
- `recursive` (bool, optional): When `True` and `file_path` is a directory, remove it and its entire contents. When `False` (the default), a non-empty directory delete fails.
- `workdir` (string, optional): Resolve relative `file_path` against this directory

**Returns:** `int` — number of entries removed (`0` if path did not exist, `1` for a single file, `N` for a recursive directory delete).

---

### get_description(name) / set_description(name, description)

Get or set a space's description. `set_description` reads the space and writes it back with only the description changed.

**Parameters:**
- `name` (string): Name or ID of the space
- `description` (string): New description (`set_description` only)

**Returns:** `get_description`: `string` - the description (empty if none). `set_description`: `bool` - True on success

---

### get_field(name, field) / set_field(name, field, value)

Get or set one of the space's custom field values (fields are declared by the space's template).

**Parameters:**
- `name` (string): Name or ID of the space
- `field` (string): Custom field name
- `value` (string): New value (`set_field` only); select/autocomplete fields only accept one of the field's options

**Returns:** `get_field`: `string` - the field's value (empty if unset). `set_field`: `bool` - True on success

---

### get_ip_address(name)

Get the static IP address of a bridged KVM space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `string` - The VM's static IP address (empty string for other platforms or when none is set)

---

### set_ip_address(name, ip_address)

Set or change the static IP address of a bridged KVM space. The space must be stopped; the address is validated against the template's network range and the addresses already in use, and applied to the VM by cloud-init at its next boot.

**Parameters:**
- `name` (string): Name or ID of the space
- `ip_address` (string): New static IP address

**Returns:** `bool` - True on success

---

### get_dependencies(name)

Get the dependency space IDs for a space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `list` - List of dependency space IDs

---

### set_dependencies(name, depends_on)

Set the dependency spaces for a space. Dependencies are required to be started before the space starts.

**Parameters:**
- `name` (string): Name or ID of the space
- `depends_on` (list): List of dependency space names or IDs

**Returns:** `bool` - True on success

---

### list(all_zones=False, user_id=None)

List all spaces for the current user.

**Parameters:**
- `all_zones` (bool, optional): If True, include spaces from all zones. Default is False (only spaces in the current server's zone).
- `user_id` (string, optional): List another user's spaces instead. Needs the **Manage Spaces** permission; without it the list is empty.

**Returns:** `list` of dicts with the same keys as [`get()`](#getname), except that `shell`, `created_at`, `node_id` and `startup_script_id` are always `""` in list results — call `get()` for a space when you need them.

---

### get(name)

Get detailed information about a space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `dict` containing:
- `id` (string): Space ID
- `name` (string): Space name
- `description` (string): Space description
- `note` (string): Space note (set by the agent, e.g. `knot agent set-note`)
- `template_id` (string): Template ID
- `template_name` (string): Template name
- `user_id` (string): Owner user ID
- `username` (string): Owner username
- `shares` (list): List of shared user IDs
- `depends_on` (list): List of dependency space IDs
- `shell` (string): Default shell
- `platform` (string): The template's platform — `docker`, `podman`, `apple`, `container` (local container, backend chosen automatically), `nomad`, `kvm`, or `manual`
- `zone` (string): Zone name
- `is_running` (bool): Whether the space is running
- `has_state` (bool): Whether the space's agent is connected to the server (see `is_ready`)
- `is_pending` (bool): Whether the space is pending
- `is_deleting` (bool): Whether the space is being deleted
- `node_id` (string): ID of the node assigned to host the space
- `node_hostname` (string): Hostname of the node assigned to host the space
- `ip_address` (string): Static IP address (bridged KVM spaces only, otherwise empty)
- `has_code_server` / `has_ssh` / `has_terminal` / `has_http_vnc` / `has_vscode_tunnel` (bool): Services the agent reports as available
- `vscode_tunnel_name` (string): VS Code tunnel name, if any
- `tcp_ports` / `http_ports` (dict): Ports the space exposes
- `created_at` (string): Creation timestamp
- `started_at` (string): When the space was last started
- `alt_names` (list of objects): Additional space names, each with `name` (string) and `port` (string, the HTTP port number to route to)
- `icon_url` (string): Icon URL
- `custom_fields` (list): Custom field values
- `startup_script_id` (string): Startup script ID
- `stack` (string): Stack name (empty if unstacked)
- `healthy` (bool): Health check status (`True` when no health check is failing)
- `resource_usage` (dict or `None`): Latest resource usage sample, when available

---

### get_stack(name)

Get the stack name for a space.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `string` - The stack name (empty string if unstacked)

---

### set_stack(name, stack)

Set the stack name for a space. Spaces with the same stack name are grouped together.

**Parameters:**
- `name` (string): Name or ID of the space
- `stack` (string): Stack name (empty string to unstack)

**Returns:** `bool` - True on success

---

### share(name, user_ids)

Share a space with one or more users.

**Parameters:**
- `name` (string): Name or ID of the space
- `user_ids` (string or list): User ID, username, or email to share with, or a list of those values

**Returns:** `bool` - True on success

---

### unshare(name, user_id=None)

Remove a space share.

**Parameters:**
- `name` (string): Name or ID of the space
- `user_id` (string, optional): User ID, username, or email to remove sharing for. If omitted, owners stop all sharing and recipients leave their own share.

**Returns:** `bool` - True on success

---

### restrict(name=None, buckets=None, files='', no_secrets=False, lock=False, secrets_allow=None, secrets_deny=None)

Restrict what a space's own credential can reach, until lifted from outside the space, and confine it (see [Space Restrictions](https://getknot.dev/docs/spaces/restrictions/)). From inside a space restrictions only tighten, so a startup script can lock the space down.

**Parameters:**
- `name` (string): Name or ID of the space; default the space the script runs in
- `buckets` (list): Bucket names to keep, each `"name"` or `"name:ro"` for read-only; every other bucket is dropped. `None` keeps them all
- `files` (string): `"ro"` to make every bucket read-only, `"none"` to switch file storage off
- `no_secrets` (bool): Switch personal secrets off {{< pro-badge >}} (ignored on servers without personal secrets)
- `lock` (bool): Lock the space's processes out of everything but file storage and personal secrets, which their own restrictions govern, apart from restarting or shutting it down; scripts started from outside the space, such as startup scripts, keep working
- `secrets_allow` (list): Prefixes, such as `"op://Dev/"`, under which alone personal secrets can be read {{< pro-badge >}}
- `secrets_deny` (list): Prefixes under which personal secrets are refused {{< pro-badge >}}

**Returns:** `dict` - the space's restrictions, as `restrictions()` returns them

```python
knot.space.restrict(buckets=["notes:ro", "scratch"], no_secrets=True)
```

---

### restrictions(name=None)

Get a space's restrictions, including what its template switches off.

**Parameters:**
- `name` (string): Name or ID of the space; default the space the script runs in

**Returns:** `dict` - `space_id`, `space_name`, `restricted`, `no_secrets`, `locked`, `files` (`""`, `"ro"` or `"none"`), `limit_buckets`, `buckets` (list of `{name, read_only}`), `limit_secrets`, `secrets_allow` and `secrets_deny` (lists of prefixes), `template_no_files` and `template_no_secrets`

---

### clear_restrictions(name)

Lift all of a space's restrictions. Refused from inside a space: they're lifted only from outside it.

**Parameters:**
- `name` (string): Name or ID of the space

**Returns:** `dict` - the space's restrictions, as `restrictions()` returns them

---

### transfer(name, user_id)

Transfer space ownership to another user.

**Parameters:**
- `name` (string): Name or ID of the space
- `user_id` (string): User ID, username, or email of the new owner

**Returns:** `bool` - True on success

---

### port_forward(source_space, local_port, remote_space, remote_port, persistent=False, force=False)

Forward a local port in one space to a port in another space (or pool).

**Parameters:**
- `source_space` (string): Source space name or ID
- `local_port` (int): Local port number in the source space
- `remote_space` (string): Target — a space or pool name you own, another user's space or pool as `user--space` (only ports its template declares shared), or a space ID
- `remote_port` (int): Port number on the target
- `persistent` (bool, optional): Persist the forward across agent restarts (default: False)
- `force` (bool, optional): Skip the up-front check that the target space is running, so a forward can be created before its target is started (default: False). It does not skip authorization: the target must still resolve and you must be allowed to reach the port, and access is enforced again when connections are made.

**Returns:** `bool` - True on success

---

### port_apply(source_space, forwards)

Replace all port forwards for a space with the given list. Any existing forwards not in the list are stopped, and any new forwards in the list are started. Forwards that already exist with the same local port, space, and remote port are left unchanged.

**Parameters:**
- `source_space` (string): Source space name or ID
- `forwards` (list): List of dicts, each containing:
  - `local_port` (int): Local port number
  - `space` (string): Remote space name or ID
  - `remote_port` (int): Remote port number
  - `persistent` (bool, optional): Persist the forward across restarts (default: False)
  - `force` (bool, optional): Skip the target-running check, as for `port_forward` (default: False)

**Returns:** `dict` containing:
- `applied` (list or `None`): Forwards that were started, each a dict shaped like a `port_list()` entry; `None` when none were
- `stopped` (list or `None`): Forwards that were stopped; `None` when none were
- `errors` (list, optional): Error messages; the key is absent when there were none, so read it with `.get("errors")`

On a running space a stopped entry carries the forward's last `mode` and throttle settings, and an applied entry has `mode` `"—"` (no connection yet) and no throttle. When the space is not running only its saved forwards change, and every entry has `mode` `""` and zero throttle settings.

---

### port_list(name)

List the port forwards of a space. For a running space the agent reports its live forwards; for a stopped space the persistent forwards saved for it are listed, with `mode` `""` and zero throttle settings.

**Parameters:**
- `name` (string): Space name or ID

**Returns:** `list` of dicts, each containing:
- `local_port` (int): Local port in the space
- `space` (string): Target space or pool, as given when the forward was created
- `remote_port` (int): Port on the target
- `persistent` (bool): Whether the forward survives agent restarts
- `mode` (string): `"direct"`, `"relay"`, or `"—"` before the first connection
- `latency_ms`, `jitter_ms`, `bandwidth_kb`, `timeout_ms` (int), `down` (bool): The forward's throttle settings (see `port_throttle`)

---

### port_stop(name, local_port)

Stop a port forward.

**Parameters:**
- `name` (string): Space name or ID
- `local_port` (int): Local port of the forward to stop

**Returns:** `bool` - True on success

---

### port_throttle(name, local_port, latency_ms=0, jitter_ms=0, bandwidth_kb=0, timeout_ms=0, down=False, reset=False)

Apply network simulation to an existing port forward in a space. All values are optional; pass `reset=True` to clear all limits.

**Parameters:**
- `name` (string): Space name or ID
- `local_port` (int): Local port of the forward to throttle
- `latency_ms` (int): Latency in milliseconds (default: 0)
- `jitter_ms` (int): Jitter in milliseconds (default: 0)
- `bandwidth_kb` (int): Bandwidth limit in KB/s (default: 0, unlimited)
- `timeout_ms` (int): Connection timeout in milliseconds — kills the connection after this duration (default: 0, none)
- `down` (bool): Block all traffic on this forward; the port definition stays (default: False)
- `reset` (bool): Clear all throttle settings (default: False)

**Returns:** `bool` - True on success

```python
import knot.space as space

# Simulate a slow, lossy link
space.port_throttle("web", 8080, latency_ms=200, jitter_ms=50, bandwidth_kb=1024)

# Clear all limits
space.port_throttle("web", 8080, reset=True)
```

---

### tunnel_start(space, protocol, port, name, server="", token="")

Start an agent-owned web tunnel in a space, exposing a port inside the space on the internet as `<user>--<name>.<domain>`. The tunnel is owned by the space's agent and runs until the agent exits or the tunnel is stopped; it is not persisted. The space must be running.

**Parameters:**
- `space` (string): Space name or ID
- `protocol` (string): `"http"` or `"https"`
- `port` (int): The port within the space to tunnel
- `name` (string): The tunnel name (forms `<user>--<name>.<domain>`)
- `server` (string, optional): Create the tunnel on this knot server instead of the space's own; the space must be able to reach it
- `token` (string, optional): API token valid on `server` — required when `server` is given, and both must be given together

**Returns:** `string` - The public tunnel URL. The URL is on the target server's tunnel domain, built from your username on that server.

---

### tunnel_list(space)

List the agent-owned web tunnels active in a space.

**Parameters:**
- `space` (string): Space name or ID

**Returns:** `list` of dicts, or `None` when the space is not running (tunnels are not persisted, so a stopped space has none). Each dict contains:
- `port` (int): Port within the space
- `protocol` (string): `"http"` or `"https"`
- `name` (string): Tunnel name
- `url` (string): Public tunnel URL — its domain shows which server the tunnel runs on

---

### tunnel_stop(space, name)

Stop an agent-owned web tunnel in a space by name.

**Parameters:**
- `space` (string): Space name or ID
- `name` (string): The tunnel name

**Returns:** `bool` - True on success

---

### stop_for_user(user)

Stop every running space a user owns in this server's zone. Spaces shared with the user are left alone.

**Parameters:**
- `user` (string): User ID or username. Stopping another user's spaces needs the **Manage Users** permission.

**Returns:** `bool` - True on success. On an error, spaces already stopped stay stopped.

---

### emit_event(name, event_type, payload=None)

Raise a [custom event](https://getknot.dev/reference/events/#custom-events) for a space, as if the space raised it. The event belongs to the space's owner and carries the space's ID and name, so sinks and methods filtering on the space see it; the caller may be someone the space is shared with. `custom.` is prepended to the type automatically. For an event with no space, use [`knot.event.emit`](../event/).

**Parameters:**
- `name` (string): Space name or ID
- `event_type` (string): Event type without the `custom.` prefix, e.g. `"myapp.deployed"`
- `payload` (dict, optional): Data sent with the event

**Returns:** `bool` - True when the event was accepted

---

### list_methods()

List the JSON-RPC methods running spaces offer that the current user may call — those registered with [`knot.methods`](../methods/).

**Returns:** `list` of dicts, each containing:
- `name` (string): Method name to call
- `local_name` (string): Name inside the providing space
- `description` (string): Description
- `keywords` (list): Search keywords
- `scope` (string): `"private"` (owner only) or `"shared"` (members of `groups`)
- `groups` (list): Groups a shared method is offered to
- `mcp_tool` (bool): Whether the method is also an MCP tool
- `events` (list): Event patterns the method subscribes to
- `event_sinks` (list): JSON-RPC sinks formatting those events
- `params_schema` (dict or None): JSON Schema of the params
- `result_schema` (dict or None): JSON Schema of the result
- `owner_id` (string), `owner` (string): The method's owner
- `provider_count` (int): Number of spaces currently serving the method

---

### call_method(method, params=None, space=None, timeout=420)

Call a space method over JSON-RPC and return its result. The server routes the call to a running space providing the method, spreading load when several do, or to one space when `space` is given — for example the pool member held through [`knot.pool.acquire`](../pool/).

**Parameters:**
- `method` (string): Method name, as `list_methods()` gives it
- `params` (dict or list, optional): Named or positional params
- `space` (string, optional): Space name or ID to pin the call to
- `timeout` (number, optional): Seconds to wait for the answer (default 420)

**Returns:** The method's result (any JSON value)

**Raises:** An exception carrying the JSON-RPC error message when the method fails, no space provides it, or you may not call it.

```python
import knot.space as space

for m in space.list_methods():
    print(m["name"], m["provider_count"])

greeting = space.call_method("greet", {"name": "Ada"})
```
