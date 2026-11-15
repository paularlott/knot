// AUTO-GENERATED from ../knot-vscode/stubs/knot — do not edit by hand.
// Regenerate with: task scriptling-completions
// Descriptions come from the stub docstrings; names are the fallback.

export const knotLibraries = [
  {
    "module": "knot.ai",
    "description": "AI client access (delegates to scriptling.ai).",
    "functions": [
      {
        "name": "get_default_model",
        "signature": "get_default_model()",
        "description": "Get the default model name. Returns \"\" in embedded contexts (the server then uses its configured default model when \"\" is passed to completion()); standalone it returns the ai_model given to knot.apiclient.configure() or KNOT_AI_MODEL, or \"\" when unset.",
        "returns": "string"
      },
      {
        "name": "Client",
        "signature": "Client()",
        "description": "Get a pre-configured AI client instance connected to the server's AI provider with MCP tools available",
        "returns": "Any"
      }
    ]
  },
  {
    "module": "knot.apiclient",
    "description": "API client — configured automatically by the knot runtime.",
    "functions": [
      {
        "name": "configure",
        "signature": "configure(url, token, insecure, ai_url, ai_token, ai_model, ai_provider)",
        "description": "Configure the API client for use outside the knot runtime (spaces and server scripts get it pre-configured). ai_url defaults to url + \"/v1\", ai_token to token; ai_provider is openai, claude, gemini, ollama, mistral, zai or grok",
        "returns": "bool"
      },
      {
        "name": "is_configured",
        "signature": "is_configured()",
        "description": "Check if the client has been configured (explicitly or via env vars)",
        "returns": "bool"
      },
      {
        "name": "get",
        "signature": "get(path, params, timeout)",
        "description": "GET request to the Knot API. params is an optional dict of query parameters; waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit). Returns the decoded JSON (dict or list), or None for an empty body",
        "returns": "dict<str,> | list<Any> | None"
      },
      {
        "name": "post",
        "signature": "post(path, body, timeout, headers)",
        "description": "POST request to the Knot API. body is a dict; headers are extra request headers (a Content-Type given replaces application/json); waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit). Returns the decoded JSON (dict or list), or None for an empty body",
        "returns": "dict<str,> | list<Any> | None"
      },
      {
        "name": "put",
        "signature": "put(path, body, timeout)",
        "description": "PUT request to the Knot API. body is a dict; waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit). Returns the decoded JSON (dict or list), or None for an empty body",
        "returns": "dict<str,> | list<Any> | None"
      },
      {
        "name": "delete",
        "signature": "delete(path, headers, timeout)",
        "description": "DELETE request to the Knot API, with any extra request headers (such as If-Match); waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit). Returns the decoded JSON (dict or list), or None for an empty body",
        "returns": "dict<str,> | list<Any> | None"
      },
      {
        "name": "get_bytes",
        "signature": "get_bytes(path, timeout)",
        "description": "GET request to the Knot API returning the response body as bytes (file content); waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit)",
        "returns": "bytes"
      },
      {
        "name": "put_bytes",
        "signature": "put_bytes(path, data, content_type, timeout)",
        "description": "PUT request to the Knot API with a string or bytes body (file content); waits up to timeout seconds if given, otherwise as long as the runtime allows (standalone: DEFAULT_TIMEOUT, or CONTENT_TIMEOUT for get_bytes/put_bytes; None means no limit). Returns the decoded JSON (dict or list), or None for an empty body",
        "returns": "dict<str,> | list<Any> | None"
      },
      {
        "name": "DEFAULT_TIMEOUT",
        "signature": "DEFAULT_TIMEOUT",
        "description": "Constant (Final = 30)",
        "returns": "Final = 30"
      },
      {
        "name": "CONTENT_TIMEOUT",
        "signature": "CONTENT_TIMEOUT",
        "description": "Constant (Final = 600)",
        "returns": "Final = 600"
      }
    ]
  },
  {
    "module": "knot.audit",
    "description": "Query the audit log.",
    "functions": [
      {
        "name": "list",
        "signature": "list(start, max_items, q, actor, actor_type, event, from_time, to_time)",
        "description": "List audit log entries with optional filtering",
        "returns": "dict"
      },
      {
        "name": "search",
        "signature": "search(q, start, max_items, actor, actor_type, event, from_time, to_time)",
        "description": "Search audit logs with a text query across actor, event, and details",
        "returns": "dict"
      },
      {
        "name": "export",
        "signature": "export(format, q, actor, actor_type, event, from_time, to_time)",
        "description": "Export every matching audit log entry, unpaged (needs download audit logs). format \"json\" (default) returns a list of entry dicts; \"csv\" returns CSV text with columns time, zone, actor, actor_type, event, details",
        "returns": "list<dict<str,>> | str"
      }
    ]
  },
  {
    "module": "knot.event",
    "description": "Knot event functions.",
    "functions": [
      {
        "name": "emit",
        "signature": "emit(event_type, payload)",
        "description": "Emit a custom event from this space. The 'custom.' prefix is added automatically.",
        "returns": "bool"
      },
      {
        "name": "get_string",
        "signature": "get_string(name, default)",
        "description": "Get a payload parameter as string (sink scripts only)",
        "returns": "string"
      },
      {
        "name": "get_int",
        "signature": "get_int(name, default)",
        "description": "Get a payload parameter as integer (sink scripts only)",
        "returns": "int"
      },
      {
        "name": "get_bool",
        "signature": "get_bool(name, default)",
        "description": "Get a payload parameter as boolean (sink scripts only)",
        "returns": "bool"
      },
      {
        "name": "get_list",
        "signature": "get_list(name, default)",
        "description": "Get a payload parameter as list (sink scripts only)",
        "returns": "list<Any>"
      },
      {
        "name": "get_dict",
        "signature": "get_dict(name, default)",
        "description": "Get a payload parameter as dict (sink scripts only)",
        "returns": "dict"
      },
      {
        "name": "type",
        "signature": "type()",
        "description": "Get the event type string (sink scripts only)",
        "returns": "string"
      },
      {
        "name": "id",
        "signature": "id()",
        "description": "Get the event UUIDv7 id (sink scripts only)",
        "returns": "string"
      },
      {
        "name": "ts",
        "signature": "ts()",
        "description": "Get the event HLC timestamp string (sink scripts only)",
        "returns": "string"
      },
      {
        "name": "space",
        "signature": "space()",
        "description": "Get the source space dict (sink scripts only)",
        "returns": "dict"
      },
      {
        "name": "space_urls",
        "signature": "space_urls()",
        "description": "Space urls",
        "returns": "dict<str, str>"
      },
      {
        "name": "actor",
        "signature": "actor()",
        "description": "Get the actor dict with id, username, kind (sink scripts only)",
        "returns": "dict"
      },
      {
        "name": "custom",
        "signature": "custom()",
        "description": "Get custom fields dict (sink scripts only)",
        "returns": "dict<str, str>"
      }
    ]
  },
  {
    "module": "knot.event_sink",
    "description": "Manage event sinks: webhook, script and json-rpc listeners for space lifecycle and custom events.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List the event sinks you may manage (your own with manage events, global ones with manage global events): id, name, description, user_id, is_global, events, sink_type (\"webhook\", \"script\" or \"json-rpc\"), webhook ({url, secret (masked here), headers, body_template, skip_tls_verify} or None), script_id, active",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(name_or_id)",
        "description": "Get an event sink by name or ID, as list() gives it but with the webhook secret in plain text. A name shared by a global sink and your own is ambiguous; give the ID",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, events, sink_type, description, url, secret, headers, body_template, skip_tls_verify, script, active, is_global)",
        "description": "Create an event sink and return its ID. events are type patterns such as [\"space.started\", \"custom.myapp.*\", \"*\"]. sink_type \"webhook\" (default; POSTs to url, signed with secret, generated when empty), \"script\" (runs script, a name or ID) or \"json-rpc\" (a named formatter for subscribed methods, using body_template). is_global=True makes a global sink (needs manage global events); otherwise it is your own (needs manage events)",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name_or_id, name, description, events, sink_type, url, secret, headers, body_template, skip_tls_verify, script, active)",
        "description": "Update an event sink; only the arguments given change, and the webhook secret is kept unless a new one is given",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(name_or_id)",
        "description": "Delete an event sink by name or ID",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.files",
    "description": "Store, read, copy and move files in buckets replicated across the cluster, snapshot folders, and manage the buckets. In a space, knot.files.sync runs sync projects.",
    "functions": [
      {
        "name": "list_buckets",
        "signature": "list_buckets(all)",
        "description": "List the buckets you own or that are shared with you; all=True lists every bucket (file storage managers). Each has name (<owner>--<name>), display_name, owner, owner_id, access (owner, write or read), size, count, shared (type, name and access of each share) and created_at",
        "returns": "list of dicts"
      },
      {
        "name": "get_bucket",
        "signature": "get_bucket(name)",
        "description": "Get one bucket by its short name (your own) or full name; same fields as list_buckets",
        "returns": "dict"
      },
      {
        "name": "create_bucket",
        "signature": "create_bucket(name)",
        "description": "Create a bucket named <your username>--<name>; name is 3 to 30 lowercase letters, digits and hyphens with no --",
        "returns": "dict"
      },
      {
        "name": "delete_bucket",
        "signature": "delete_bucket(name, force)",
        "description": "Delete a bucket; a bucket that holds files is refused unless force=True, which deletes them with it",
        "returns": "bool"
      },
      {
        "name": "share_bucket",
        "signature": "share_bucket(name, user, group, everyone, access)",
        "description": "Share a bucket with exactly one of a user (username or email), a group or everyone; access is \"read\" (default) or \"write\". Sharing again changes the access",
        "returns": "dict"
      },
      {
        "name": "unshare_bucket",
        "signature": "unshare_bucket(name, user, group, everyone)",
        "description": "Stop sharing a bucket with exactly one of a user, a group or everyone",
        "returns": "dict"
      },
      {
        "name": "transfer_bucket",
        "signature": "transfer_bucket(name, user, force)",
        "description": "Give a bucket to another user, who must be allowed to own buckets; it is renamed into their namespace and their quota is charged. force allows going over their quota (file storage managers only)",
        "returns": "dict"
      },
      {
        "name": "usage",
        "signature": "usage()",
        "description": "Your file storage usage and limits: used_bytes, files, buckets, quota_bytes and max_buckets (0 for no limit)",
        "returns": "dict"
      },
      {
        "name": "list_files",
        "signature": "list_files(bucket, prefix, recursive)",
        "description": "List the files in a bucket. Returns files (as stat gives for a file) and folders (prefixes ending in /). prefix restricts to keys starting with it, so end it with / for a folder; recursive=True lists every file below the prefix and returns no folders",
        "returns": "dict"
      },
      {
        "name": "list_changes",
        "signature": "list_changes(bucket, prefix, cursor)",
        "description": "List what changed in a bucket since a cursor, to follow it without listing it again. Without a cursor every file is returned; cursor=\"now\" returns none, only a cursor to follow from now; with a cursor, each file changed since, once, as it is now (deleted=True for one that was deleted). Returns changes (as stat gives for a file, with deleted), cursor (pass it next time) and reset (True when the cursor could not be followed: start again without one)",
        "returns": "dict"
      },
      {
        "name": "read_file",
        "signature": "read_file(bucket, key)",
        "description": "Read a file's content as bytes (at most 64 MB in a script)",
        "returns": "bytes"
      },
      {
        "name": "read_text",
        "signature": "read_text(bucket, key)",
        "description": "Read a file's content as UTF-8 text",
        "returns": "string"
      },
      {
        "name": "write_file",
        "signature": "write_file(bucket, key, data, content_type)",
        "description": "Write a file, replacing any file with that key; folders need no creating. data is a string (written as UTF-8) or bytes, at most 64 MB at a time. content_type defaults to text/plain for a string, else application/octet-stream. Returns the file, as stat gives it",
        "returns": "dict"
      },
      {
        "name": "delete_file",
        "signature": "delete_file(bucket, key, if_match)",
        "description": "Delete a file. With if_match (an etag from list_files, list_changes, write_file or copy_file), only if it is still that version: a file changed since is left alone and an exception raised (HTTP 412)",
        "returns": "bool"
      },
      {
        "name": "copy_file",
        "signature": "copy_file(source_bucket, source_key, dest_bucket, dest_key)",
        "description": "Copy a file within a bucket or to another, on the server without transferring its content; keeps its content type, metadata and modification time and replaces any file at the destination",
        "returns": "dict"
      },
      {
        "name": "file_exists",
        "signature": "file_exists(bucket, key)",
        "description": "Check whether there is a file with exactly this key",
        "returns": "bool"
      },
      {
        "name": "stat",
        "signature": "stat(bucket, key)",
        "description": "Say what a path is without reading any content: None for nothing there, else a dict with type \"file\" or \"folder\". A file's has key, size, etag, sha256, content_type, modified_at, changed_at (when this version was saved), mode (permission bits such as 0o755, or None), link (True for a symbolic link) and meta; a folder's has key (ending in /) and marker (True when an empty-folder marker keeps it). key \"\" is the top of the bucket",
        "returns": "dict<str,> | None"
      },
      {
        "name": "move",
        "signature": "move(bucket, source, dest, overwrite)",
        "description": "Rename a file, or a folder and everything in it, within a bucket, on the server without transferring content. A destination holding a file is refused unless overwrite=True. Returns the number of files moved",
        "returns": "int"
      },
      {
        "name": "snapshot",
        "signature": "snapshot(source_bucket, source, dest_bucket, dest)",
        "description": "Copy a folder (\"\" for the whole bucket), as it was at one moment, to a new folder that must not exist, on the server: no content is transferred. Returns files, folders, bytes, taken_at, source_bucket, source and manifest (the .knot-snapshot.json written last, so a folder holding it is a complete snapshot)",
        "returns": "dict"
      },
      {
        "name": "set_attrs",
        "signature": "set_attrs(bucket, key, mode, mtime)",
        "description": "Change a file's permission bits (0 to 0o777; a link stays a link) or modification time (Unix seconds or an RFC 3339 string), keeping its content and other metadata. Returns the file, as stat gives it; HTTP 412 when the file changed meanwhile",
        "returns": "dict"
      },
      {
        "name": "share_targets",
        "signature": "share_targets()",
        "description": "Users ({id, username, can_own, self}) and groups ({id, name}) your buckets can be shared with or transferred to. Needs share or transfer buckets",
        "returns": "dict"
      },
      {
        "name": "fsck",
        "signature": "fsck(repair, deep, timeout)",
        "description": "Check file storage (file storage administrators only); repair fixes what it can, deep verifies every content checksum. Returns buckets, files, found, repaired, issues, notes",
        "returns": "dict"
      },
      {
        "name": "start_upload",
        "signature": "start_upload(bucket, key, content_type)",
        "description": "Start an upload in parts of a file larger than write_file takes (64 MB); returns the upload ID",
        "returns": "string"
      },
      {
        "name": "upload_part",
        "signature": "upload_part(bucket, key, upload_id, number, data)",
        "description": "Send part number (from 1, in file order; at most 64 MB) of an upload; sending a number again replaces it. Returns {number, etag, size}",
        "returns": "dict"
      },
      {
        "name": "list_upload_parts",
        "signature": "list_upload_parts(bucket, key, upload_id)",
        "description": "The parts of an upload the server holds, {number, etag, size}, to resume a stopped upload",
        "returns": "list of dicts"
      },
      {
        "name": "complete_upload",
        "signature": "complete_upload(bucket, key, upload_id, parts, timeout)",
        "description": "Make the parts (default: every part held) the file, replacing any file with that key; returns the file as stat gives it",
        "returns": "dict"
      },
      {
        "name": "abort_upload",
        "signature": "abort_upload(bucket, key, upload_id)",
        "description": "Discard an upload and the parts held",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.files.sync",
    "description": "Run sync projects in this space's agent: keep local folders in step with bucket folders. Works in scripts that run in a space: startup and shutdown scripts, knot space run-script and knot run-script.",
    "functions": [
      {
        "name": "start",
        "signature": "start(project, name, wait, timeout)",
        "description": "Start a sync project, or replace one of the same name; one already running unchanged is left running, so a startup script can start it on every boot. project is a dict like a project file, {\"name\": \"code\", \"exclude\": [...], \"no_default_excludes\": False, \"sync\": [{\"name\", \"local\", \"remote\": \"bucket:folder/\", \"snapshot\": \"bucket:folder/\", \"mode\": \"two-way\" | \"download\" | \"upload\", \"watch\", \"delete\", \"exclude\", \"exclude_from\", \"no_default_excludes\", \"allow_mass_delete\"}]}, or a project file's TOML; name names a project that gives none. wait=True waits up to timeout seconds (default 600) for every job's first pass, raising if one fails. Returns the project's status, as status gives it",
        "returns": "dict"
      },
      {
        "name": "stop",
        "signature": "stop(name)",
        "description": "Stop a sync project after sending its last local changes, and forget it. The template's project, named template, cannot be stopped",
        "returns": "bool"
      },
      {
        "name": "list",
        "signature": "list()",
        "description": "The sync projects running in this space, the template's included, each as status gives it",
        "returns": "list of dicts"
      },
      {
        "name": "status",
        "signature": "status(name)",
        "description": "How a sync project is doing, or None when none of that name runs: name, template (True for the template's) and jobs, each with name, local, remote, mode, state (starting, snapshotting, syncing, watching, done, failed or stopped), passes, last_pass, error, uploaded, downloaded, deleted and conflicts",
        "returns": "dict<str,> | None"
      }
    ]
  },
  {
    "module": "knot.globals",
    "description": "The `user` global user-created MCP tools see, and the User class.",
    "functions": [
      {
        "name": "has_permission",
        "signature": "has_permission(key)",
        "description": "Check a permission. The argument picks the check: an integer is a built-in permission id (the knot.permission constants, e.g. knot.permission.MANAGE_SPACES), a \"plugin.\"-prefixed string is a qualified grant (\"plugin.metrics.read\"), any other string is a built-in's stable key (\"manage_spaces\"). Admins pass every check.",
        "returns": "bool"
      },
      {
        "name": "in_group",
        "signature": "in_group(group_id)",
        "description": "Check membership of one group by its ID (not its name); compares against groups.",
        "returns": "bool"
      }
    ],
    "classes": [
      {
        "name": "User",
        "description": "The requesting user's identity and effective permissions.\n\n    Bound as the `user` global in user-created MCP tools; returned by\n    knot.identity.user() everywhere identity matters. Admins pass every\n    permission check by construction.",
        "methods": [
          {
            "name": "id",
            "signature": "id",
            "description": "Id ",
            "returns": "string"
          },
          {
            "name": "name",
            "signature": "name",
            "description": "Name ",
            "returns": "string"
          },
          {
            "name": "is_admin",
            "signature": "is_admin",
            "description": "Is admin",
            "returns": "bool"
          },
          {
            "name": "groups",
            "signature": "groups",
            "description": "Groups ",
            "returns": "list of strings"
          },
          {
            "name": "permissions",
            "signature": "permissions",
            "description": "Permissions ",
            "returns": "list of strings"
          },
          {
            "name": "plugin_permissions",
            "signature": "plugin_permissions",
            "description": "Plugin permissions",
            "returns": "list of strings"
          },
          {
            "name": "user",
            "signature": "user",
            "description": "User ",
            "returns": "User"
          },
          {
            "name": "has_permission",
            "signature": "has_permission(self, key: str | int)",
            "description": "Check a permission. The argument picks the check: an integer is a built-in permission id (the knot.permission constants, e.g. knot.permission.MANAGE_SPACES), a \"plugin.\"-prefixed string is a qualified grant (\"plugin.metrics.read\"), any other string is a built-in's stable key (\"manage_spaces\"). Admins pass every check.",
            "returns": "bool"
          },
          {
            "name": "in_group",
            "signature": "in_group(self, group_id: str)",
            "description": "Check membership of one group by its ID (not its name); compares against groups.",
            "returns": "bool"
          }
        ]
      }
    ]
  },
  {
    "module": "knot.group",
    "description": "Manage groups.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List all groups",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(group_id)",
        "description": "Get group by ID (UUID only)",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, max_spaces, compute_units, storage_units, max_tunnels, file_storage_mb, max_buckets)",
        "description": "Create a new group and return its ID",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(group_id, name, max_spaces, compute_units, storage_units, max_tunnels, file_storage_mb, max_buckets)",
        "description": "Update group properties; arguments left as None keep their current values",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(group_id)",
        "description": "Delete a group by UUID",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.healthcheck",
    "description": "Space health check functions.",
    "functions": [
      {
        "name": "http_head",
        "signature": "http_head(url, skip_ssl_verify, timeout)",
        "description": "HTTP HEAD check, returns True if status 200, False otherwise",
        "returns": "bool"
      },
      {
        "name": "tcp_port",
        "signature": "tcp_port(port, timeout)",
        "description": "TCP port check, returns True if port is open",
        "returns": "bool"
      },
      {
        "name": "program",
        "signature": "program(command, timeout)",
        "description": "Run command, returns True if exit code 0",
        "returns": "bool"
      },
      {
        "name": "check_result",
        "signature": "check_result(healthy)",
        "description": "Report health check result and exit. Use with combined checks",
        "returns": "Any"
      }
    ]
  },
  {
    "module": "knot.identity",
    "description": "The authoritative identity surface: who the code runs as.",
    "functions": [
      {
        "name": "user",
        "signature": "user()",
        "description": "The requesting user as a User instance — id, name, is_admin, groups, permissions (stable keys), plugin_permissions (qualified grants), with has_permission and in_group. Re-bound on every dispatch, so it always answers with the current user; the admin role passes every check.",
        "returns": "User"
      }
    ]
  },
  {
    "module": "knot.jobs",
    "description": "Manage the scheduled jobs of a space.",
    "functions": [
      {
        "name": "list",
        "signature": "list(space)",
        "description": "List a space's job definitions and runner state. Works while the space is stopped.",
        "returns": "dict"
      },
      {
        "name": "run",
        "signature": "run(space, name)",
        "description": "Trigger a job immediately by name. Works for disabled and manual-only jobs; the space must be running.",
        "returns": "bool"
      },
      {
        "name": "add",
        "signature": "add(space, name, command, schedule, enabled)",
        "description": "Add a job to a space. Schedule is a 5-field cron expression, e.g. \"0 2 * * *\"; empty for a manual-only job.",
        "returns": "bool"
      },
      {
        "name": "update",
        "signature": "update(space, name, command, schedule, enabled)",
        "description": "Update a job's command, schedule or enabled state; only given arguments change. Pass schedule='' for manual-only.",
        "returns": "bool"
      },
      {
        "name": "remove",
        "signature": "remove(space, name)",
        "description": "Remove a job from a space.",
        "returns": "bool"
      },
      {
        "name": "enable",
        "signature": "enable(space, name)",
        "description": "Enable a job so it fires automatically.",
        "returns": "bool"
      },
      {
        "name": "disable",
        "signature": "disable(space, name)",
        "description": "Disable a job so it does not fire automatically. Manual runs still work.",
        "returns": "bool"
      },
      {
        "name": "enable_runner",
        "signature": "enable_runner(space)",
        "description": "Start the space's job runner: scheduled jobs fire.",
        "returns": "bool"
      },
      {
        "name": "disable_runner",
        "signature": "disable_runner(space)",
        "description": "Stop the space's job runner. Manual runs still work.",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.mcp",
    "description": "MCP tool discovery and execution.",
    "functions": [
      {
        "name": "list_tools",
        "signature": "list_tools()",
        "description": "Get list of available MCP tools and their parameters",
        "returns": "list of dicts"
      },
      {
        "name": "call_tool",
        "signature": "call_tool(name, arguments)",
        "description": "Call an MCP tool directly. Arguments should be a dict",
        "returns": "Any"
      },
      {
        "name": "tool_search",
        "signature": "tool_search(query, max_results)",
        "description": "Search for tools by keyword. Returns list of matching tools",
        "returns": "list of dicts"
      },
      {
        "name": "execute_tool",
        "signature": "execute_tool(name, arguments)",
        "description": "Execute a discovered tool. Use full name for namespaced tools",
        "returns": "Any"
      }
    ]
  },
  {
    "module": "knot.mcp_server",
    "description": "Manage remote MCP servers whose tools knot exposes under a namespace (call the tools with knot.mcp).",
    "functions": [
      {
        "name": "list",
        "signature": "list(user_id)",
        "description": "List remote MCP servers, sorted by namespace: id, user_id, namespace, url, command, args, env (\"KEY=value\" entries), enabled, tool_visibility (\"native\" or \"discoverable\"), disabled_tools, remote_search. user_id lists another user's servers (needs manage MCP servers)",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(namespace_or_id)",
        "description": "Get a remote MCP server by namespace (your own) or ID: list()'s fields plus auth_type (\"\", \"bearer\" or \"oauth2\"), oauth_client_id, oauth_token_url, and has_token / has_oauth_tokens flags — token values are never returned",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(namespace, url, command, args, env, auth_type, token, oauth_client_id, oauth_token_url, oauth_access_token, oauth_refresh_token, enabled, tool_visibility, remote_search)",
        "description": "Add a remote MCP server for the current user and return its ID: url for HTTP(S), or command (with args and env) for stdio. namespace must be unique among your servers and not contain \"__\". Needs manage MCP servers",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(namespace_or_id, namespace, url, command, args, env, auth_type, token, oauth_client_id, oauth_token_url, oauth_access_token, oauth_refresh_token, enabled, tool_visibility, remote_search)",
        "description": "Update a remote MCP server; only the arguments given change (namespace renames it), stored tokens are kept unless new ones are given",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(namespace_or_id)",
        "description": "Delete a remote MCP server",
        "returns": "bool"
      },
      {
        "name": "tools",
        "signature": "tools(namespace_or_id)",
        "description": "Connect to the server and list its tools: name (no namespace prefix), description, enabled, is_app. Raises with the reason when the server cannot be reached",
        "returns": "list of dicts"
      },
      {
        "name": "set_tool_enabled",
        "signature": "set_tool_enabled(namespace_or_id, tool_name, enabled)",
        "description": "Switch one of the server's tools on or off",
        "returns": "bool"
      },
      {
        "name": "protocol_version",
        "signature": "protocol_version(namespace_or_id)",
        "description": "The MCP protocol version negotiated with the server, e.g. \"2025-06-18\"; \"\" when it cannot be reached",
        "returns": "string"
      }
    ]
  },
  {
    "module": "knot.methods",
    "description": "Knot methods library.",
    "functions": [
      {
        "name": "method",
        "signature": "method(name, *, local_name, description, scope, keywords, groups, mcp_tool, params, result, events, event_sinks)",
        "description": "Method ",
        "returns": "bool"
      },
      {
        "name": "register",
        "signature": "register()",
        "description": "Register ",
        "returns": "bool"
      },
      {
        "name": "unregister",
        "signature": "unregister(name)",
        "description": "Unregister ",
        "returns": "bool"
      }
    ],
    "classes": [
      {
        "name": "Server",
        "description": "",
        "methods": [
          {
            "name": "command",
            "signature": "command",
            "description": "Command ",
            "returns": "str,"
          },
          {
            "name": "name",
            "signature": "name",
            "description": "Name ",
            "returns": "str,"
          },
          {
            "name": "__init__",
            "signature": "__init__(self, command: str, *, type: str = \"stdio\", timeout: int = 30, args: list[str] | None = None, mode: str = \"concurrent\",)",
            "description": "  init  ",
            "returns": "None"
          },
          {
            "name": "method",
            "signature": "method(self, name: str, *, local_name: str = \"\", description: str = \"\", scope: str = \"\", keywords: list[str] | None = None, groups: list[str] | None = None, mcp_tool: bool = False, params: dict[str, Any] | None = None, result: dict[str, Any] | None = None, events: list[str] | None = None, event_sinks: list[str] | None = None,)",
            "description": "Method ",
            "returns": "bool"
          },
          {
            "name": "register",
            "signature": "register(self)",
            "description": "Register ",
            "returns": "bool"
          },
          {
            "name": "unregister",
            "signature": "unregister(self, name: str | None = None)",
            "description": "Unregister ",
            "returns": "bool"
          }
        ]
      }
    ]
  },
  {
    "module": "knot.methods.schema",
    "description": "JSON Schema builders for knot.methods Server params and result schemas.",
    "functions": [
      {
        "name": "string",
        "signature": "string(*, description, default, enum, format, min_length, max_length, pattern, extra)",
        "description": "String ",
        "returns": "dict"
      },
      {
        "name": "integer",
        "signature": "integer(*, description, default, enum, minimum, maximum, extra)",
        "description": "Integer ",
        "returns": "dict"
      },
      {
        "name": "number",
        "signature": "number(*, description, default, enum, minimum, maximum, extra)",
        "description": "Number ",
        "returns": "dict"
      },
      {
        "name": "boolean",
        "signature": "boolean(*, description, default, enum, extra)",
        "description": "Boolean ",
        "returns": "dict"
      },
      {
        "name": "null",
        "signature": "null(*, description, default, enum, extra)",
        "description": "Null ",
        "returns": "dict"
      },
      {
        "name": "array",
        "signature": "array(items, *, description, default, enum, min_items, max_items, extra)",
        "description": "Array ",
        "returns": "dict"
      },
      {
        "name": "object",
        "signature": "object(*, description, default, enum, additional_properties, extra, **properties: dict[str, Any])",
        "description": "Object ",
        "returns": "dict"
      },
      {
        "name": "optional",
        "signature": "optional(schema, *, default)",
        "description": "Optional ",
        "returns": "dict"
      }
    ]
  },
  {
    "module": "knot.permission",
    "description": "Permission constants for role management.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List all built-in permissions with their IDs, names, and groups",
        "returns": "list of dicts"
      },
      {
        "name": "list_plugin",
        "signature": "list_plugin()",
        "description": "List the permissions declared by loaded plugins: dicts with id (the qualified grant, e.g. \"plugin.metrics.read\"), plugin (the declaring plugin's name) and label (the declared id).",
        "returns": "list of dicts"
      },
      {
        "name": "MANAGE_USERS",
        "signature": "MANAGE_USERS",
        "description": "Constant (Final = 0)",
        "returns": "Final = 0"
      },
      {
        "name": "MANAGE_GROUPS",
        "signature": "MANAGE_GROUPS",
        "description": "Constant (Final = 4)",
        "returns": "Final = 4"
      },
      {
        "name": "MANAGE_ROLES",
        "signature": "MANAGE_ROLES",
        "description": "Constant (Final = 5)",
        "returns": "Final = 5"
      },
      {
        "name": "MANAGE_SPACES",
        "signature": "MANAGE_SPACES",
        "description": "Constant (Final = 2)",
        "returns": "Final = 2"
      },
      {
        "name": "MANAGE_TEMPLATES",
        "signature": "MANAGE_TEMPLATES",
        "description": "Constant (Final = 1)",
        "returns": "Final = 1"
      },
      {
        "name": "MANAGE_VOLUMES",
        "signature": "MANAGE_VOLUMES",
        "description": "Constant (Final = 3)",
        "returns": "Final = 3"
      },
      {
        "name": "MANAGE_VARIABLES",
        "signature": "MANAGE_VARIABLES",
        "description": "Constant (Final = 6)",
        "returns": "Final = 6"
      },
      {
        "name": "USE_SPACES",
        "signature": "USE_SPACES",
        "description": "Constant (Final = 7)",
        "returns": "Final = 7"
      },
      {
        "name": "TRANSFER_SPACES",
        "signature": "TRANSFER_SPACES",
        "description": "Constant (Final = 10)",
        "returns": "Final = 10"
      },
      {
        "name": "SHARE_SPACES",
        "signature": "SHARE_SPACES",
        "description": "Constant (Final = 11)",
        "returns": "Final = 11"
      },
      {
        "name": "USE_TUNNELS",
        "signature": "USE_TUNNELS",
        "description": "Constant (Final = 8)",
        "returns": "Final = 8"
      },
      {
        "name": "EDIT_SPACE_JOBS",
        "signature": "EDIT_SPACE_JOBS",
        "description": "Constant (Final = 43)",
        "returns": "Final = 43"
      },
      {
        "name": "VIEW_AUDIT_LOGS",
        "signature": "VIEW_AUDIT_LOGS",
        "description": "Constant (Final = 9)",
        "returns": "Final = 9"
      },
      {
        "name": "CLUSTER_INFO",
        "signature": "CLUSTER_INFO",
        "description": "Constant (Final = 12)",
        "returns": "Final = 12"
      },
      {
        "name": "USE_VNC",
        "signature": "USE_VNC",
        "description": "Constant (Final = 13)",
        "returns": "Final = 13"
      },
      {
        "name": "USE_WEB_TERMINAL",
        "signature": "USE_WEB_TERMINAL",
        "description": "Constant (Final = 14)",
        "returns": "Final = 14"
      },
      {
        "name": "USE_SSH",
        "signature": "USE_SSH",
        "description": "Constant (Final = 15)",
        "returns": "Final = 15"
      },
      {
        "name": "USE_CODE_SERVER",
        "signature": "USE_CODE_SERVER",
        "description": "Constant (Final = 16)",
        "returns": "Final = 16"
      },
      {
        "name": "USE_VSCODE_TUNNEL",
        "signature": "USE_VSCODE_TUNNEL",
        "description": "Constant (Final = 17)",
        "returns": "Final = 17"
      },
      {
        "name": "USE_LOGS",
        "signature": "USE_LOGS",
        "description": "Constant (Final = 18)",
        "returns": "Final = 18"
      },
      {
        "name": "RUN_COMMANDS",
        "signature": "RUN_COMMANDS",
        "description": "Constant (Final = 19)",
        "returns": "Final = 19"
      },
      {
        "name": "COPY_FILES",
        "signature": "COPY_FILES",
        "description": "Constant (Final = 20)",
        "returns": "Final = 20"
      },
      {
        "name": "USE_MCP_SERVER",
        "signature": "USE_MCP_SERVER",
        "description": "Constant (Final = 21)",
        "returns": "Final = 21"
      },
      {
        "name": "USE_WEB_ASSISTANT",
        "signature": "USE_WEB_ASSISTANT",
        "description": "Constant (Final = 22)",
        "returns": "Final = 22"
      },
      {
        "name": "MANAGE_SCRIPTS",
        "signature": "MANAGE_SCRIPTS",
        "description": "Constant (Final = 23)",
        "returns": "Final = 23"
      },
      {
        "name": "EXECUTE_SCRIPTS",
        "signature": "EXECUTE_SCRIPTS",
        "description": "Constant (Final = 24)",
        "returns": "Final = 24"
      },
      {
        "name": "MANAGE_OWN_SCRIPTS",
        "signature": "MANAGE_OWN_SCRIPTS",
        "description": "Constant (Final = 25)",
        "returns": "Final = 25"
      },
      {
        "name": "EXECUTE_OWN_SCRIPTS",
        "signature": "EXECUTE_OWN_SCRIPTS",
        "description": "Constant (Final = 26)",
        "returns": "Final = 26"
      },
      {
        "name": "MANAGE_GLOBAL_SKILLS",
        "signature": "MANAGE_GLOBAL_SKILLS",
        "description": "Constant (Final = 27)",
        "returns": "Final = 27"
      },
      {
        "name": "MANAGE_OWN_SKILLS",
        "signature": "MANAGE_OWN_SKILLS",
        "description": "Constant (Final = 28)",
        "returns": "Final = 28"
      },
      {
        "name": "SET_SPACE_DEPENDENCIES",
        "signature": "SET_SPACE_DEPENDENCIES",
        "description": "Constant (Final = 29)",
        "returns": "Final = 29"
      },
      {
        "name": "USE_SPACE_STARTUP_SCRIPT",
        "signature": "USE_SPACE_STARTUP_SCRIPT",
        "description": "Constant (Final = 30)",
        "returns": "Final = 30"
      },
      {
        "name": "DOWNLOAD_AUDIT_LOGS",
        "signature": "DOWNLOAD_AUDIT_LOGS",
        "description": "Constant (Final = 31)",
        "returns": "Final = 31"
      },
      {
        "name": "MANAGE_STACK_DEFINITIONS",
        "signature": "MANAGE_STACK_DEFINITIONS",
        "description": "Constant (Final = 32)",
        "returns": "Final = 32"
      },
      {
        "name": "MANAGE_OWN_STACK_DEFINITIONS",
        "signature": "MANAGE_OWN_STACK_DEFINITIONS",
        "description": "Constant (Final = 33)",
        "returns": "Final = 33"
      },
      {
        "name": "USE_STACK_DEFINITIONS",
        "signature": "USE_STACK_DEFINITIONS",
        "description": "Constant (Final = 34)",
        "returns": "Final = 34"
      },
      {
        "name": "USE_METHODS",
        "signature": "USE_METHODS",
        "description": "Constant (Final = 35)",
        "returns": "Final = 35"
      },
      {
        "name": "USE_POOLS",
        "signature": "USE_POOLS",
        "description": "Constant (Final = 36)",
        "returns": "Final = 36"
      },
      {
        "name": "MANAGE_EVENTS",
        "signature": "MANAGE_EVENTS",
        "description": "Constant (Final = 37)",
        "returns": "Final = 37"
      },
      {
        "name": "MANAGE_GLOBAL_EVENTS",
        "signature": "MANAGE_GLOBAL_EVENTS",
        "description": "Constant (Final = 38)",
        "returns": "Final = 38"
      },
      {
        "name": "MANAGE_GLOBAL_SLASH_COMMANDS",
        "signature": "MANAGE_GLOBAL_SLASH_COMMANDS",
        "description": "Constant (Final = 39)",
        "returns": "Final = 39"
      },
      {
        "name": "MANAGE_OWN_SLASH_COMMANDS",
        "signature": "MANAGE_OWN_SLASH_COMMANDS",
        "description": "Constant (Final = 40)",
        "returns": "Final = 40"
      },
      {
        "name": "MANAGE_MCP_SERVERS",
        "signature": "MANAGE_MCP_SERVERS",
        "description": "Constant (Final = 41)",
        "returns": "Final = 41"
      },
      {
        "name": "USE_LOG_SINKS",
        "signature": "USE_LOG_SINKS",
        "description": "Constant (Final = 42)",
        "returns": "Final = 42"
      },
      {
        "name": "VIEW_PLUGINS",
        "signature": "VIEW_PLUGINS",
        "description": "Constant (Final = 44)",
        "returns": "Final = 44"
      },
      {
        "name": "LINK_USERS",
        "signature": "LINK_USERS",
        "description": "Constant (Final = 45)",
        "returns": "Final = 45"
      },
      {
        "name": "USE_FILES",
        "signature": "USE_FILES",
        "description": "Constant (Final = 46)",
        "returns": "Final = 46"
      },
      {
        "name": "MANAGE_FILES",
        "signature": "MANAGE_FILES",
        "description": "Constant (Final = 47)",
        "returns": "Final = 47"
      },
      {
        "name": "SHARE_BUCKETS",
        "signature": "SHARE_BUCKETS",
        "description": "Constant (Final = 48)",
        "returns": "Final = 48"
      },
      {
        "name": "TRANSFER_BUCKETS",
        "signature": "TRANSFER_BUCKETS",
        "description": "Constant (Final = 49)",
        "returns": "Final = 49"
      },
      {
        "name": "BACKUP_SERVER",
        "signature": "BACKUP_SERVER",
        "description": "Constant (Final = 50)",
        "returns": "Final = 50"
      },
      {
        "name": "USE_USER_SECRETS",
        "signature": "USE_USER_SECRETS",
        "description": "Constant (Final = 51)",
        "returns": "Final = 51"
      },
      {
        "name": "RUN_AI_SCRIPTS",
        "signature": "RUN_AI_SCRIPTS",
        "description": "Constant (Final = 52)",
        "returns": "Final = 52"
      },
      {
        "name": "SPACE_MANAGE",
        "signature": "SPACE_MANAGE",
        "description": "Constant (Final = 2)",
        "returns": "Final = 2"
      },
      {
        "name": "SPACE_USE",
        "signature": "SPACE_USE",
        "description": "Constant (Final = 7)",
        "returns": "Final = 7"
      },
      {
        "name": "SCRIPT_MANAGE",
        "signature": "SCRIPT_MANAGE",
        "description": "Constant (Final = 23)",
        "returns": "Final = 23"
      },
      {
        "name": "SCRIPT_EXECUTE",
        "signature": "SCRIPT_EXECUTE",
        "description": "Constant (Final = 24)",
        "returns": "Final = 24"
      }
    ]
  },
  {
    "module": "knot.plugin",
    "description": "Calls to declared plugin handlers, as the requesting user.",
    "functions": [
      {
        "name": "call",
        "signature": "call(plugin, handler, params, method)",
        "description": "Call a plugin's [[tool.knot.handlers]]-declared handler as the requesting user. params become the query string (GET, the default) or a JSON body (POST). Returns the handler's JSON answer as a dict; raises on a refused gate (permission denied), an undeclared handler, or an unknown plugin.",
        "returns": "dict"
      }
    ]
  },
  {
    "module": "knot.pool",
    "description": "Manage space pools — fixed-size, self-healing groups of identical spaces,",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List visible pools with utilization",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(name)",
        "description": "Get pool details, utilization, and member stats by name or ID",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, template_name, startup_script_id, desired_count, active, lease_max_time, lease_max_extensions, custom_fields)",
        "description": "Create a pool with the given number of spaces and return its ID.\n\n    lease_max_time enables exclusive member leases: 0 = off (default), -1 = no timeout,\n    >0 = maximum lease length in seconds. lease_max_extensions: 0 = extending forbidden\n    (default), -1 = unlimited, >0 = maximum extensions. custom_fields: values given to\n    every member, as {\"name\": ..., \"value\": ...}; fields left out get the template default.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name, desired_count, active, lease_max_time, lease_max_extensions, custom_fields)",
        "description": "Update pool desired count, active state, lease settings or custom fields; omitted values are kept.\n\n    custom_fields replaces the values given to members ([] clears them); members pick them up\n    when next created or started. Name, template, and startup script are immutable.",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(name)",
        "description": "Delete a stopped pool and all its spaces",
        "returns": "bool"
      },
      {
        "name": "set_size",
        "signature": "set_size(name, desired_count)",
        "description": "Set pool target size. The sweep loop creates, drains, or deletes spaces asynchronously.",
        "returns": "bool"
      },
      {
        "name": "start",
        "signature": "start(name)",
        "description": "Start a stopped pool: starts all member spaces",
        "returns": "bool"
      },
      {
        "name": "stop",
        "signature": "stop(name)",
        "description": "Stop a running pool: stops all member spaces without deleting them",
        "returns": "bool"
      },
      {
        "name": "acquire",
        "signature": "acquire(name, time, wait)",
        "description": "Acquire a pool member exclusively until the lease ends (requires a\n    lease-enabled pool). time: None = the pool's maximum, \"none\" = never\n    expire (unlimited pools only), seconds or \"5m\"-style string. wait:\n    optionally wait this long for a free member before raising. Returns the\n    lease dict — the held member is space_name / space_id.",
        "returns": "dict"
      },
      {
        "name": "extend",
        "signature": "extend(space, time)",
        "description": "Extend the lease held on a pool member (space name or id): the new\n    deadline is now + time (or never, on unlimited pools). Bounded by the\n    pool's max extension count.",
        "returns": "dict"
      },
      {
        "name": "release",
        "signature": "release(space, destroy)",
        "description": "Release the lease held on a pool member (space name or id — the same\n    identifier acquire returned). The member returns to the pool after\n    in-flight work drains (normally within ~15s). With destroy=True the\n    member is deleted and a fresh replacement is created, so the next\n    acquire gets a clean space.",
        "returns": "dict"
      },
      {
        "name": "leases",
        "signature": "leases(name)",
        "description": "List the pool's held leases — active plus draining",
        "returns": "list of dicts"
      }
    ],
    "classes": [
      {
        "name": "leased",
        "description": "Context manager for exclusive pool member use; releases on exit\n    (destroys the member instead when destroy=True, leaving a fresh\n    replacement behind).",
        "methods": [
          {
            "name": "lease",
            "signature": "lease",
            "description": "Lease ",
            "returns": "dict"
          },
          {
            "name": "__init__",
            "signature": "__init__(self, name: str, time: str | int | None = ..., wait: str | int | None = ..., destroy: bool = ...)",
            "description": "  init  ",
            "returns": "None"
          }
        ]
      }
    ]
  },
  {
    "module": "knot.role",
    "description": "Manage roles.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List all roles",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(role_id)",
        "description": "Get role by ID (UUID only)",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, permissions, plugin_permissions)",
        "description": "Create a new role",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(role_id, name, permissions, plugin_permissions)",
        "description": "Update role properties",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(role_id)",
        "description": "Delete a role by UUID",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.script",
    "description": "Manage and execute scripts.",
    "functions": [
      {
        "name": "list",
        "signature": "list(owner, all_zones, include_inactive, script_type)",
        "description": "List scripts visible to the current user. Defaults to active scripts in the current zone; use script_type='script' to filter to runnable scripts (exclude MCP tool definitions).",
        "returns": "list of dicts"
      },
      {
        "name": "list_global",
        "signature": "list_global(all_zones)",
        "description": "List global scripts available for template editing",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(script_id)",
        "description": "Get script details by UUID",
        "returns": "dict"
      },
      {
        "name": "get_by_name",
        "signature": "get_by_name(name)",
        "description": "Get script details by name, respecting user script shadowing",
        "returns": "dict"
      },
      {
        "name": "get_content",
        "signature": "get_content(name, script_type)",
        "description": "Get script content by name and type",
        "returns": "string"
      },
      {
        "name": "create",
        "signature": "create(name, content, description, owner, groups, zones, active, script_type, mcp_input_schema_toml, mcp_keywords, discoverable)",
        "description": "Create a script and return its ID. Use owner='current' for an own script; omit owner for a global script. script_type is \"script\" (default), \"lib\" or \"tool\"; mcp_input_schema_toml, mcp_keywords and discoverable describe an MCP tool.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(script_id, name, content, description, groups, zones, active, script_type, mcp_input_schema_toml, mcp_keywords, discoverable)",
        "description": "Update a script while preserving fields not passed",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(script_id)",
        "description": "Delete a script by UUID",
        "returns": "bool"
      },
      {
        "name": "execute",
        "signature": "execute(space_name, script_name, script_id, content, args)",
        "description": "Execute a named, ID-based, or inline script in a running space",
        "returns": "dict"
      },
      {
        "name": "execute_content",
        "signature": "execute_content(space_name, content, args)",
        "description": "Execute inline script content in a running space",
        "returns": "dict"
      }
    ]
  },
  {
    "module": "knot.secret",
    "description": "Read your own secrets from 1Password with the token on your profile (Knot Pro). Works only inside a knot space.",
    "functions": [
      {
        "name": "get",
        "signature": "get(path)",
        "description": "Read a secret by its 1Password reference, op://vault/item/field (op://vault/item/section/field for a field in a section); only inside a space",
        "returns": "string"
      }
    ]
  },
  {
    "module": "knot.server",
    "description": "Server information, and the hosts space web ports are reached at.",
    "functions": [
      {
        "name": "info",
        "signature": "info()",
        "description": "Get server-wide information: version, wildcard_domain (space web-port URLs, e.g. \"*.knot.example.com\") and tunnel_domain (dot-prefixed suffix, e.g. \".tunnel.example.com\", appended straight after a tunnel name) — the domains are empty when not configured.",
        "returns": "dict"
      },
      {
        "name": "host_label",
        "signature": "host_label(user, name, port)",
        "description": "The first label of the host a space's web port is reached at, before the wildcard domain: \"<user>--<name>--<port>\", lowercased; name is a space, alt name or pool, port a number or \"vnc\". A label longer than the 63 characters DNS allows has the name cut short and ended with a hyphen and an 8-character hash, which knot routes by",
        "returns": "string"
      },
      {
        "name": "ping",
        "signature": "ping()",
        "description": "Check the server is reachable and the credentials work: status, version, zone",
        "returns": "dict"
      },
      {
        "name": "cluster_info",
        "signature": "cluster_info()",
        "description": "List the cluster's server nodes (needs view cluster info): id, address, state, zone, hostname, allocated_spaces, running_spaces, runtimes, metadata",
        "returns": "list of dicts"
      },
      {
        "name": "search",
        "signature": "search(query)",
        "description": "Search everything you may see by name (case-insensitive substring, at most 5 hits per type): query plus lists of {id, name, description} for pages, spaces, templates, variables, volumes, stacks, scripts, skills, commands, mcp_servers, event_sinks, users, groups, roles, tokens",
        "returns": "dict"
      },
      {
        "name": "plugins",
        "signature": "plugins()",
        "description": "List loaded server plugins (needs manage roles): count, plugins (name, version, description, permissions, menus, pages, mcp_tools, handlers, field_handlers, libs, ...), failed, warnings. Raises when no plugins are loaded",
        "returns": "dict"
      }
    ]
  },
  {
    "module": "knot.skill",
    "description": "Manage skills.",
    "functions": [
      {
        "name": "list",
        "signature": "list(owner)",
        "description": "List skills (filtered by permissions/groups/zones)",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(name_or_id)",
        "description": "Get skill by name or UUID",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(content, is_global, groups, zones, owner, active)",
        "description": "Create a new skill and return its ID: a user skill owned by owner (default: the current user) unless is_global=True. A global skill, or another user's, needs Manage Global Skills; groups (group IDs) apply to global skills only",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name_or_id, content, groups, zones, active)",
        "description": "Update a skill; arguments left as None keep their current values",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(name_or_id)",
        "description": "Delete skill",
        "returns": "bool"
      },
      {
        "name": "search",
        "signature": "search(query)",
        "description": "Fuzzy search skills by name/description: up to 3 matches, best first, each with id, name, description, user_id, content and score (0 to 1)",
        "returns": "list of dicts"
      }
    ]
  },
  {
    "module": "knot.slash_command",
    "description": "Manage slash commands.",
    "functions": [
      {
        "name": "list",
        "signature": "list(owner, all_zones)",
        "description": "List slash commands the user has access to. Pass owner to filter to a user's own commands, all_zones=True to include other zones",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(name_or_id)",
        "description": "Get a slash command by name or UUID",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(content, is_global, groups, zones, active)",
        "description": "Create a slash command from markdown content with YAML frontmatter (name, description, argument-hint, allowed-tools). Use is_global=True for admin (requires permission)",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name_or_id, content, groups, zones, active)",
        "description": "Update a slash command while preserving fields not passed",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(name_or_id)",
        "description": "Delete a slash command by name or UUID",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.space",
    "description": "Manage development spaces.",
    "functions": [
      {
        "name": "list",
        "signature": "list(all_zones, user_id)",
        "description": "List spaces visible to the current user. Defaults to the current server's zone; pass all_zones=True to include other zones. user_id lists another user's spaces (needs manage spaces; empty without it).",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(name)",
        "description": "Get space details as a dict",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, template_name, description, shell, depends_on, stack, selected_node_id, alt_names, icon_url, custom_fields, startup_script_id, ip_address, start_on_create)",
        "description": "Create a new space and return its ID. alt_names is a list of {\"name\": str, \"port\": int} dicts. ip_address sets the static IP for bridged KVM templates (validated against the template's range); unused by other platforms.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name, new_name, description, shell, template_name, depends_on, stack, selected_node_id, alt_names, icon_url, custom_fields, startup_script_id, ip_address)",
        "description": "Update space properties while preserving fields not passed. Lifecycle changes use start()/stop()/restart(). ip_address (bridged KVM only) requires the space stopped; applied at next boot.",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(name)",
        "description": "Delete a space by name",
        "returns": "bool"
      },
      {
        "name": "start",
        "signature": "start(name)",
        "description": "Start a space by name",
        "returns": "bool"
      },
      {
        "name": "stop_for_user",
        "signature": "stop_for_user(user)",
        "description": "Stop every running space a user (ID or username) owns in this server's zone; shared spaces are left alone. Another user's needs the manage users permission",
        "returns": "bool"
      },
      {
        "name": "stop",
        "signature": "stop(name)",
        "description": "Stop a space by name",
        "returns": "bool"
      },
      {
        "name": "restart",
        "signature": "restart(name)",
        "description": "Restart a space by name",
        "returns": "bool"
      },
      {
        "name": "is_running",
        "signature": "is_running(name)",
        "description": "Check if a space is running. A freshly started space reports running before its agent connects — see is_ready() for the state required by run() and file operations.",
        "returns": "bool"
      },
      {
        "name": "is_ready",
        "signature": "is_ready(name)",
        "description": "Check if a space is running with its agent connected. True only when run(), read_file(), and other agent-backed calls will succeed; false while a starting space's agent is still connecting.",
        "returns": "bool"
      },
      {
        "name": "wait_for_start",
        "signature": "wait_for_start(name, timeout, interval)",
        "description": "Wait for a space to be running with its agent connected (ready). Returns True immediately if already ready (never stops the space); polls every interval seconds until the agent registers or timeout expires. Returns False on timeout.",
        "returns": "bool"
      },
      {
        "name": "usage_current",
        "signature": "usage_current(name)",
        "description": "Get the current resource usage point for a space",
        "returns": "dict"
      },
      {
        "name": "usage_history",
        "signature": "usage_history(name, range)",
        "description": "Get historical resource usage for a space: a dict with space_id, range, bucket_kind and points. range is \"1h\" (minute samples, the default) or \"7d\" (daily samples).",
        "returns": "dict"
      },
      {
        "name": "set_description",
        "signature": "set_description(name, description)",
        "description": "Set space description",
        "returns": "bool"
      },
      {
        "name": "get_description",
        "signature": "get_description(name)",
        "description": "Get space description",
        "returns": "string"
      },
      {
        "name": "get_ip_address",
        "signature": "get_ip_address(name)",
        "description": "Get a bridged KVM space's static IP address (empty for other platforms)",
        "returns": "string"
      },
      {
        "name": "set_ip_address",
        "signature": "set_ip_address(name, ip_address)",
        "description": "Set or change a bridged KVM space's static IP; requires the space stopped, applied at next boot",
        "returns": "bool"
      },
      {
        "name": "get_dependencies",
        "signature": "get_dependencies(name)",
        "description": "Get dependency space IDs for a space",
        "returns": "list of strings"
      },
      {
        "name": "set_dependencies",
        "signature": "set_dependencies(name, depends_on)",
        "description": "Set dependency spaces by name or ID",
        "returns": "bool"
      },
      {
        "name": "get_stack",
        "signature": "get_stack(name)",
        "description": "Get the stack name for a space",
        "returns": "string"
      },
      {
        "name": "set_stack",
        "signature": "set_stack(name, stack)",
        "description": "Set the stack name for a space (empty string to unstack)",
        "returns": "bool"
      },
      {
        "name": "get_field",
        "signature": "get_field(name, field)",
        "description": "Get custom field value from space",
        "returns": "string"
      },
      {
        "name": "set_field",
        "signature": "set_field(name, field, value)",
        "description": "Set custom field value on space",
        "returns": "bool"
      },
      {
        "name": "restrict",
        "signature": "restrict(name, buckets, files, no_secrets, lock, secrets_allow, secrets_deny)",
        "description": "Restrict what a space's own credential can do; from inside a space restrictions only tighten. name defaults to the space this runs in. buckets keeps only the named buckets, each \"name\" or \"name:ro\"; files is \"ro\", \"none\" or \"\"; no_secrets switches personal secrets off and secrets_allow / secrets_deny limit them by prefix such as \"op://vault/\" (Knot Pro); lock locks the space's processes out of everything but file storage and personal secrets. Returns the restrictions, as restrictions() does.",
        "returns": "dict"
      },
      {
        "name": "restrictions",
        "signature": "restrictions(name)",
        "description": "Get a space's restrictions (name defaults to the space this runs in): restricted, no_secrets, locked, files, limit_buckets, buckets ({name, read_only}), limit_secrets, secrets_allow, secrets_deny, template_no_files and template_no_secrets.",
        "returns": "dict"
      },
      {
        "name": "clear_restrictions",
        "signature": "clear_restrictions(name)",
        "description": "Lift all of a space's restrictions. Refused from inside a space.",
        "returns": "dict"
      },
      {
        "name": "transfer",
        "signature": "transfer(name, user_id)",
        "description": "Transfer space to another user (user_id can be username, email, or UUID)",
        "returns": "bool"
      },
      {
        "name": "share",
        "signature": "share(name, user_ids)",
        "description": "Share space with one or more users; user_ids is one user or a list (usernames, emails, or UUIDs)",
        "returns": "bool"
      },
      {
        "name": "unshare",
        "signature": "unshare(name, user_id)",
        "description": "Remove a space share, optionally for a specific user",
        "returns": "bool"
      },
      {
        "name": "run",
        "signature": "run(name, command, args, timeout, workdir, check)",
        "description": "Execute a command in a space. With check=True (the default) returns the output as str and raises if the command fails (non-zero exit, timeout as exit code 124, no shell), with the error, exit code and output in the message. With check=False never raises for a failed command and returns a dict with output, exit_code, success and error (empty on success).",
        "returns": "str | dict<str,>"
      },
      {
        "name": "run_script",
        "signature": "run_script(name, script_name, args)",
        "description": "Execute a script in a space",
        "returns": "dict"
      },
      {
        "name": "eval",
        "signature": "eval(name, code, args)",
        "description": "Execute inline Scriptling code in a running space (no stored script required)",
        "returns": "dict"
      },
      {
        "name": "read_file",
        "signature": "read_file(name, file_path, offset, limit)",
        "description": "Read file contents from a running space; offset/limit select a 1-based line range. Raises if the agent cannot read the file (for example it does not exist).",
        "returns": "string"
      },
      {
        "name": "write_file",
        "signature": "write_file(name, file_path, content, mode, mtime_ns, file_perm)",
        "description": "Write content to a file in a running space (overwrite/append/prepend). Optional mtime_ns (Unix nanoseconds) and file_perm (int bits like 0o644) are applied after the write so the destination matches a source file's metadata — useful for sync tools. Raises if the agent cannot write the file.",
        "returns": "bool"
      },
      {
        "name": "grep",
        "signature": "grep(name, pattern, path, literal, recursive, ignore_case, glob, follow_links, max_size, workdir)",
        "description": "Search file contents in a running space via a parallel worker pool in the agent",
        "returns": "list of dicts"
      },
      {
        "name": "find",
        "signature": "find(name, path, recursive, type, name_glob, mtime_min, mtime_max, size_min, size_max, include_hidden, follow_links, max_depth, workdir)",
        "description": "Find files and directories in a running space by name, type, mtime, or size. Returns path strings only.",
        "returns": "list of strings"
      },
      {
        "name": "find_entries",
        "signature": "find_entries(name, path, recursive, type, name_glob, mtime_min, mtime_max, size_min, size_max, include_hidden, follow_links, max_depth, include_hash, include_symlinks, workdir)",
        "description": "Same as find() but each entry is a dict with path, size, mtime, is_dir, file_perm. Pass include_hash=True for a crc64 hash field, include_symlinks=True for symlink entries with link_target.",
        "returns": "list of dicts"
      },
      {
        "name": "sed_replace",
        "signature": "sed_replace(name, old, new, path, recursive, ignore_case, glob, follow_links, max_size, workdir)",
        "description": "Replace literal occurrences of old with new in files (atomic in-place edit)",
        "returns": "int"
      },
      {
        "name": "sed_replace_pattern",
        "signature": "sed_replace_pattern(name, pattern, new, path, recursive, ignore_case, glob, follow_links, max_size, workdir)",
        "description": "Replace regex matches in files; capture groups as ${1}, ${name} (atomic in-place edit)",
        "returns": "int"
      },
      {
        "name": "sed_extract",
        "signature": "sed_extract(name, pattern, path, recursive, ignore_case, glob, follow_links, max_size, workdir)",
        "description": "Extract regex capture groups from files in a running space (read-only)",
        "returns": "list of dicts"
      },
      {
        "name": "edit_file",
        "signature": "edit_file(name, file_path, search, replace, workdir)",
        "description": "Targeted search-and-replace edit on a single file; search must be unique (fails if 0 or >1 matches)",
        "returns": "int"
      },
      {
        "name": "delete_file",
        "signature": "delete_file(name, file_path, recursive, workdir)",
        "description": "Delete file",
        "returns": "int"
      },
      {
        "name": "port_forward",
        "signature": "port_forward(source_space, local_port, remote_space, remote_port, persistent, force)",
        "description": "Forward a local port to a remote port. remote_space is a target reference: a space or pool name you own, another user's space or pool as user--space (only ports its template declares shared), or a space ID.",
        "returns": "bool"
      },
      {
        "name": "port_list",
        "signature": "port_list(name)",
        "description": "List port forwards for a space: local_port, space, remote_port, persistent, mode (\"direct\", \"relay\" or \"—\" before first use) and the throttle settings latency_ms, jitter_ms, bandwidth_kb, timeout_ms and down. For a stopped space its saved persistent forwards, with mode \"\" and zero throttle settings.",
        "returns": "list of dicts"
      },
      {
        "name": "port_stop",
        "signature": "port_stop(name, local_port)",
        "description": "Stop a port forward",
        "returns": "bool"
      },
      {
        "name": "port_throttle",
        "signature": "port_throttle(name, local_port, latency_ms, jitter_ms, bandwidth_kb, timeout_ms, down, reset)",
        "description": "Apply latency, jitter, bandwidth limits, connection timeout, and/or traffic blocking (down) to a port forward. Pass reset=True to clear.",
        "returns": "bool"
      },
      {
        "name": "port_apply",
        "signature": "port_apply(source_space, forwards)",
        "description": "Replace all port forwards for a space. Returns a dict with applied and stopped (lists of port_list-shaped entries, or None when empty) and errors (a list, absent when there were none).",
        "returns": "dict"
      },
      {
        "name": "tunnel_start",
        "signature": "tunnel_start(space, protocol, port, name, server, token)",
        "description": "Start an agent-owned web tunnel in a space, exposing <port> as <user>--<name>.<domain>. Owned by the space's agent; not persisted. Pass both server and token to create the tunnel on a different knot server the space can reach.",
        "returns": "string"
      },
      {
        "name": "tunnel_list",
        "signature": "tunnel_list(space)",
        "description": "List agent-owned web tunnels in a space",
        "returns": "list of dicts"
      },
      {
        "name": "tunnel_stop",
        "signature": "tunnel_stop(space, name)",
        "description": "Stop an agent-owned web tunnel in a space by name",
        "returns": "bool"
      },
      {
        "name": "emit_event",
        "signature": "emit_event(name, event_type, payload)",
        "description": "Raise a custom event for a space (name or ID) as if the space raised it: it belongs to the owner and carries the space; \"custom.\" is prepended to event_type. Use knot.event.emit for an event with no space",
        "returns": "bool"
      },
      {
        "name": "list_methods",
        "signature": "list_methods()",
        "description": "List the JSON-RPC methods running spaces offer that you may call: name, local_name, description, keywords, scope (\"private\"/\"shared\"), groups, mcp_tool, events, event_sinks, params_schema, result_schema, owner_id, owner, provider_count",
        "returns": "list of dicts"
      },
      {
        "name": "call_method",
        "signature": "call_method(method, params, space, timeout)",
        "description": "Call a space method over JSON-RPC and return its result. space (name or ID) pins the call to one providing space, e.g. a leased pool member; otherwise the server picks one. Raises with the JSON-RPC error message when the call fails",
        "returns": "Any"
      }
    ]
  },
  {
    "module": "knot.stack",
    "description": "Manage stacks and stack definitions.",
    "functions": [
      {
        "name": "list_defs",
        "signature": "list_defs(include_inactive)",
        "description": "List stack definitions visible to the current user; active ones only unless include_inactive=True",
        "returns": "list of dicts"
      },
      {
        "name": "get_def",
        "signature": "get_def(name)",
        "description": "Get a stack definition by name or ID",
        "returns": "dict"
      },
      {
        "name": "create_def",
        "signature": "create_def(name, description, icon_url, scope, active, groups, zones, spaces)",
        "description": "Create a new stack definition",
        "returns": "string"
      },
      {
        "name": "update_def",
        "signature": "update_def(name, **fields: Any)",
        "description": "Update a stack definition while preserving other fields",
        "returns": "bool"
      },
      {
        "name": "delete_def",
        "signature": "delete_def(name)",
        "description": "Delete a stack definition",
        "returns": "bool"
      },
      {
        "name": "validate_def",
        "signature": "validate_def(spaces, name, description, icon_url, scope, active, groups, zones)",
        "description": "Validate a stack definition without saving",
        "returns": "dict"
      },
      {
        "name": "add_component",
        "signature": "add_component(stack_definition, template, name, description, shell, startup_script, depends_on)",
        "description": "Add a component (template binding) to an existing stack definition. Resolves template name to ID and rejects duplicate component names. startup_script is a startup script ID; depends_on lists other component names this one depends on.",
        "returns": "bool"
      },
      {
        "name": "remove_component",
        "signature": "remove_component(stack_definition, name)",
        "description": "Remove a component from a stack definition (by name or ID) by component name. Also cleans up depends_on and port_forward references in sibling components.",
        "returns": "bool"
      },
      {
        "name": "create",
        "signature": "create(definition_name, prefix, stack_name)",
        "description": "Create spaces from a stack definition (by name or ID), named {prefix}-{component}. Returns {\"spaces\": {component_name: space_id, ...}}",
        "returns": "dict<str, dict<str, str>>"
      },
      {
        "name": "exists",
        "signature": "exists(stack_name, user_id)",
        "description": "True if at least one space belongs to the stack. user_id names another user's stack (needs manage spaces)",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(stack_name, user_id)",
        "description": "Delete all spaces in a stack. user_id names another user's stack (needs manage spaces)",
        "returns": "bool"
      },
      {
        "name": "start",
        "signature": "start(stack_name, user_id)",
        "description": "Start all spaces in a stack in dependency order. user_id names another user's stack (needs manage spaces)",
        "returns": "bool"
      },
      {
        "name": "stop",
        "signature": "stop(stack_name, user_id)",
        "description": "Stop all spaces in a stack in reverse dependency order. user_id names another user's stack (needs manage spaces)",
        "returns": "bool"
      },
      {
        "name": "restart",
        "signature": "restart(stack_name, user_id)",
        "description": "Restart all spaces in a stack. user_id names another user's stack (needs manage spaces)",
        "returns": "bool"
      },
      {
        "name": "list",
        "signature": "list()",
        "description": "List stacks by grouping spaces by stack name",
        "returns": "list of dicts"
      }
    ]
  },
  {
    "module": "knot.template",
    "description": "Manage space templates.",
    "functions": [
      {
        "name": "field_options",
        "signature": "field_options(handler_id, query)",
        "description": "Resolve a plugin field handler's option keys as the requesting user — the exact values the space form offers and the API's validation accepts. query is passed to the handler as the text typed so far (autocomplete). None when the handler cannot be reached.",
        "returns": "list of strings or None"
      },
      {
        "name": "list",
        "signature": "list(include_inactive, resolve_options)",
        "description": "List templates visible to the current user. Defaults to active templates only; pass include_inactive=True to include retired ones. Each entry has runtime_available: whether a node in the zone currently offers the runtime the template needs (always True for manual and Nomad templates). With resolve_options=True, handler-backed custom fields carry their options resolved live (one plugin call per field) instead of naming their handler.",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(template_id, resolve_options)",
        "description": "Get template by ID or name. runtime_available reports whether a node in the zone currently offers the runtime the template needs (always True for manual and Nomad templates). With resolve_options=True, handler-backed custom fields carry their options resolved live instead of naming their handler.",
        "returns": "dict"
      },
      {
        "name": "validate",
        "signature": "validate(platform, job, volumes)",
        "description": "Validate template job and volume specifications without saving",
        "returns": "dict"
      },
      {
        "name": "build_spec",
        "signature": "build_spec(platform, spec, original_job, original_volumes)",
        "description": "Build native job and volume text from a unified spec (image, environment, ports, storage, memory, cpus). Same conversion as the UI spec wizard. Patch into originals to preserve hand-written content.",
        "returns": "dict<str, str>"
      },
      {
        "name": "nodes",
        "signature": "nodes(template_id)",
        "description": "List available placement nodes for a local-container or KVM template (node_id, hostname, running_spaces, total_spaces); empty for other platforms",
        "returns": "list of dicts"
      },
      {
        "name": "create",
        "signature": "create(name, job, description, platform, volumes, active, compute_units, storage_units, with_terminal, with_vscode_tunnel, with_code_server, with_ssh, with_run_command, allow_node_migration, schedule_enabled, icon_url, with_vnc, groups, zones, paths, disable_user_activity, disable_file_storage, disable_user_secrets, health_check_type, health_check_config, health_check_skip_ssl_verify, health_check_timeout, health_check_interval, health_check_max_failures, health_check_auto_restart, ports, jobs, max_uptime, max_uptime_unit, idle_timeout, idle_timeout_unit, port_forwards, custom_fields)",
        "description": "Create a new template. health_check_type can be none, agent, tcp, http, program, or custom. ports is a list of {name, port, protocol} objects, protocol one of \"http\", \"https\", \"tcp\" or \"shared\" (a shared port is reachable by every user in the same zone through a port forward as user--space, not just the space's owner; http/https get dev URLs, tcp is published on the host); port_forwards is a list of {local_port, space, remote_port} dicts seeded into spaces created from the template and connected when they start (space: a space or pool name, or user--name for another user's shared port); jobs is a list of {name, command, schedule, enabled} objects copied into new spaces; custom_fields declares the template's custom fields. max_uptime / max_uptime_unit and idle_timeout / idle_timeout_unit control the max-runtime and idle auto-stops (unit one of \"minute\", \"hour\", \"day\"; \"disabled\" or a 0 value turns the stop off). disable_file_storage refuses file storage (knot file, the files API) from the template's spaces; disable_user_secrets refuses personal secrets (knot secret get, knot.secret.get()) from them (Knot Pro); disable_user_activity stops filesystem user activity collection (Knot Pro).\n\n    custom_fields declares the template's custom fields: a list of dicts with\n    name, description, type (\"text\", \"masked\", \"number\", \"bool\", \"select\", \"autocomplete\"\n    or \"textarea\"), handler (a plugin field handler id) or options (a manual\n    option list) — select and autocomplete take exactly one of the two,\n    language (the editor language, textarea only), default — a bool default\n    becomes the string \"true\"/\"false\"; values are stored as strings — and\n    required (bool): a required field cannot be blank when creating or editing\n    a space, and the default can satisfy the requirement — and show_on_create\n    (bool): an optional field shown on the create-space form without opening\n    its optional fields toggle. The list order is the order the form shows\n    the fields in.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(template_id, name, job, description, platform, volumes, active, compute_units, storage_units, with_terminal, with_vscode_tunnel, with_code_server, with_ssh, with_run_command, allow_node_migration, schedule_enabled, with_vnc, icon_url, groups, zones, paths, disable_user_activity, disable_file_storage, disable_user_secrets, health_check_type, health_check_config, health_check_skip_ssl_verify, health_check_timeout, health_check_interval, health_check_max_failures, health_check_auto_restart, ports, jobs, max_uptime, max_uptime_unit, idle_timeout, idle_timeout_unit, port_forwards, custom_fields)",
        "description": "Update template properties, including health_check_type, health_check_auto_restart, max_uptime, idle_timeout, ports, jobs and port_forwards. Omitted properties are left unchanged. disable_file_storage and disable_user_secrets switch file storage and personal secrets (Knot Pro) off for the template's spaces; disable_user_activity stops user activity collection (Knot Pro). max_uptime / idle_timeout units are one of \"minute\", \"hour\", \"day\"; \"disabled\" (or a 0 value for idle) turns the stop off. custom_fields, when given, replaces the template's custom fields (same shape as create); omitted leaves them unchanged.",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(template_id)",
        "description": "Delete a template by ID or name",
        "returns": "bool"
      },
      {
        "name": "get_icons",
        "signature": "get_icons()",
        "description": "Get list of available icons, each a dict with description, source and url",
        "returns": "list of dicts"
      },
      {
        "name": "field_handlers",
        "signature": "field_handlers()",
        "description": "List the field handlers loaded plugins declare (id, label, plugin), for a custom field's handler. Needs manage templates; raises when no plugins are loaded",
        "returns": "list of dicts"
      },
      {
        "name": "export",
        "signature": "export(template_id)",
        "description": "Export a template (name or ID) as portable YAML, script IDs replaced by names. Needs manage templates",
        "returns": "string"
      },
      {
        "name": "import_yaml",
        "signature": "import_yaml(yaml_text)",
        "description": "Create or update (by name) a template from YAML as export() writes it and return its ID (None if the server returns no body); scripts are looked up by name. Needs manage templates",
        "returns": "str | None"
      },
      {
        "name": "parse_spec",
        "signature": "parse_spec(platform, job, volumes)",
        "description": "Convert native job/volume text into the unified spec build_spec() takes: wizardable, reason, spec, fully_representable, advanced_reason. Needs manage templates",
        "returns": "dict"
      },
      {
        "name": "base_images",
        "signature": "base_images()",
        "description": "The spec wizard's base image catalog: version, manifest_version, description, registry_auth and images (name, display_name, image, category, default_memory, ...)",
        "returns": "dict"
      },
      {
        "name": "refresh_base_images",
        "signature": "refresh_base_images()",
        "description": "Fetch the base image catalog from its update URL now: updated, active_version, update_url. Needs manage templates; HTTP 409 when updates are not configured",
        "returns": "dict"
      },
      {
        "name": "capabilities",
        "signature": "capabilities()",
        "description": "Linux capabilities the spec wizard offers for cap_add/cap_drop: name, description, common",
        "returns": "list of dicts"
      }
    ]
  },
  {
    "module": "knot.token",
    "description": "Manage API tokens.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List the current user's API tokens; id is the bearer key itself, and scopes is empty for full access",
        "returns": "list of dicts"
      },
      {
        "name": "create",
        "signature": "create(name, scopes)",
        "description": "Create an API token and return its value (the bearer key). scopes narrows it to endpoint groups: \"methods\" (/api/methods*), \"mcp\" (/mcp), \"tunnels\" (/tunnel/* and /api/tunnels*: tunnels only), \"files\" (/api/files*, and S3 on Knot Pro) and \"backup\" (/api/backup* and /api/restore*); None means full access",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(token_id, name, scopes)",
        "description": "Rename a token or change its scopes; only the arguments given change. scopes=[] clears them (full access)",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(token_id)",
        "description": "Delete an API token by its value, revoking it immediately",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.tunnel",
    "description": "List and close your tunnels (start tunnels from a space with knot.space.tunnel_start). Needs the use tunnels permission.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List your open tunnels from every client and space, sorted by name: name (\"<username>--<tunnel>\") and address (public URL)",
        "returns": "list of dicts"
      },
      {
        "name": "server_info",
        "signature": "server_info()",
        "description": "Tunnel server settings: domain (suffix such as \".tunnel.example.com\") and tunnel_servers (list of URLs)",
        "returns": "dict"
      },
      {
        "name": "delete",
        "signature": "delete(name)",
        "description": "Close one of your tunnels, disconnecting its client; name is the full \"<username>--<tunnel>\" or the bare tunnel name",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.user",
    "description": "Manage users.",
    "functions": [
      {
        "name": "get_me",
        "signature": "get_me()",
        "description": "Get current user",
        "returns": "dict"
      },
      {
        "name": "get",
        "signature": "get(user_id)",
        "description": "Get user by ID or username",
        "returns": "dict"
      },
      {
        "name": "list",
        "signature": "list(state, zone)",
        "description": "List all users with optional state/zone filter",
        "returns": "list of dicts"
      },
      {
        "name": "create",
        "signature": "create(username, email, password, active, max_spaces, compute_units, storage_units, max_tunnels, ssh_public_key, github_username, file_storage_mb, max_buckets)",
        "description": "Create a new user",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(user_id, username, email, password, active, max_spaces, ssh_public_key, github_username, compute_units, storage_units, max_tunnels, file_storage_mb, max_buckets)",
        "description": "Update user properties; arguments left as None (and roles, groups, shell, timezone) keep their current values",
        "returns": "bool"
      },
      {
        "name": "set_ssh_public_key",
        "signature": "set_ssh_public_key(ssh_public_key, github_username)",
        "description": "Set SSH public keys for the current user. Use one public key per line",
        "returns": "bool"
      },
      {
        "name": "set_ssh_private_key",
        "signature": "set_ssh_private_key(ssh_private_key)",
        "description": "Set the SSH private key for the current user",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(user_id)",
        "description": "Delete a user by ID or username",
        "returns": "bool"
      },
      {
        "name": "link_user",
        "signature": "link_user(user_id, linked_user_id)",
        "description": "Grant user_id the ability to become linked_user_id (one way; requires the link_users permission)",
        "returns": "bool"
      },
      {
        "name": "unlink_user",
        "signature": "unlink_user(user_id, linked_user_id)",
        "description": "Remove linked_user_id from user_id's become-list (requires the link_users permission)",
        "returns": "bool"
      },
      {
        "name": "get_quota",
        "signature": "get_quota(user_id)",
        "description": "Get user quota and usage",
        "returns": "dict"
      },
      {
        "name": "list_permissions",
        "signature": "list_permissions(user_id)",
        "description": "List a user's built-in permissions as integer permission IDs (compare with the knot.permission constants). Plugin grants are separate: see list_plugin_permissions()",
        "returns": "list<int>"
      },
      {
        "name": "list_plugin_permissions",
        "signature": "list_plugin_permissions(user_id)",
        "description": "List the plugin permissions a user holds: qualified grant strings (e.g. \"plugin.metrics.read\") resolved from their roles; admins hold every grant.",
        "returns": "list of strings"
      },
      {
        "name": "has_permission",
        "signature": "has_permission(user_id, permission_id)",
        "description": "Check if user has a specific permission",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.vars",
    "description": "Manage template variables.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List all template variables",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(var_id)",
        "description": "Get variable value",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, value, zones, local, protected, restricted)",
        "description": "Create a new variable",
        "returns": "string"
      },
      {
        "name": "set_value",
        "signature": "set_value(var_id, value)",
        "description": "Set variable value (updates existing)",
        "returns": "bool"
      },
      {
        "name": "update",
        "signature": "update(var_id, value, zones, local, protected, restricted)",
        "description": "Update variable properties",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(var_id)",
        "description": "Delete a variable",
        "returns": "bool"
      }
    ]
  },
  {
    "module": "knot.volume",
    "description": "Manage volumes.",
    "functions": [
      {
        "name": "list",
        "signature": "list()",
        "description": "List all volumes",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(volume_id)",
        "description": "Get volume by ID or name",
        "returns": "dict"
      },
      {
        "name": "nodes",
        "signature": "nodes(platform)",
        "description": "List available nodes for a platform",
        "returns": "list of dicts"
      },
      {
        "name": "validate",
        "signature": "validate(platform, definition)",
        "description": "Validate a volume definition without saving",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(name, definition, platform, node_id)",
        "description": "Create a new volume",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(volume_id, name, definition, platform, node_id)",
        "description": "Update volume properties",
        "returns": "bool"
      },
      {
        "name": "delete",
        "signature": "delete(volume_id)",
        "description": "Delete a volume by ID or name",
        "returns": "bool"
      },
      {
        "name": "start",
        "signature": "start(volume_id)",
        "description": "Start a stopped volume",
        "returns": "bool"
      },
      {
        "name": "stop",
        "signature": "stop(volume_id)",
        "description": "Stop a running volume",
        "returns": "bool"
      },
      {
        "name": "is_running",
        "signature": "is_running(volume_id)",
        "description": "Check if a volume is currently running",
        "returns": "bool"
      }
    ]
  }
];
