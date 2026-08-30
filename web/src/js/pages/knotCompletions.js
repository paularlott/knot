// AUTO-GENERATED from ../knot-vscode/stubs/knot — do not edit by hand.
// Regenerate with: task knot-completions
// Descriptions are derived from docstrings where present, function names otherwise.

export const knotLibraries = [
  {
    "module": "knot.ai",
    "description": "AI client access (delegates to scriptling.ai).",
    "functions": [
      {
        "name": "get_default_model",
        "signature": "get_default_model()",
        "description": "Get the server-configured default model name",
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
        "name": "get",
        "signature": "get(path)",
        "description": "GET request to the Knot API. params is an optional dict of query parameters",
        "returns": "dict"
      },
      {
        "name": "post",
        "signature": "post(path, body, Any, expect)",
        "description": "POST request to the Knot API. body is a dict",
        "returns": "dict"
      },
      {
        "name": "put",
        "signature": "put(path, body, Any, expect)",
        "description": "PUT request to the Knot API. body is a dict",
        "returns": "dict"
      },
      {
        "name": "delete",
        "signature": "delete(path, expect)",
        "description": "DELETE request to the Knot API",
        "returns": "dict"
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
      }
    ]
  },
  {
    "module": "knot.event",
    "description": "Knot event functions.",
    "functions": [
      {
        "name": "emit",
        "signature": "emit(type, payload, Any)",
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
        "returns": "list[Any>"
      },
      {
        "name": "get_dict",
        "signature": "get_dict(name, default, Any)",
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
        "signature": "create(name, max_spaces, compute_units, storage_units, max_tunnels)",
        "description": "Create a new group (optional kwargs: max_spaces, compute_units, storage_units, max_tunnels)",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(group_id, name, max_spaces, compute_units, storage_units)",
        "description": "Update group properties",
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
        "signature": "call_tool(name, arguments, Any)",
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
        "signature": "execute_tool(name, arguments, Any)",
        "description": "Execute a discovered tool. Use full name for namespaced tools",
        "returns": "Any"
      }
    ]
  },
  {
    "module": "knot.methods",
    "description": "Knot methods library.",
    "functions": [
      {
        "name": "method",
        "signature": "method(name, *, local_name, description, scope, keywords, groups, mcp_tool, params, Any, result, Any, events, event_sinks)",
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
    ]
  },
  {
    "module": "knot.methods.schema",
    "description": "JSON Schema builders for knot.methods Server params and result schemas.",
    "functions": [
      {
        "name": "string",
        "signature": "string(*, description, default, enum, format, min_length, max_length, pattern, extra, Any)",
        "description": "String ",
        "returns": "dict"
      },
      {
        "name": "integer",
        "signature": "integer(*, description, default, enum, minimum, maximum, extra, Any)",
        "description": "Integer ",
        "returns": "dict"
      },
      {
        "name": "number",
        "signature": "number(*, description, default, enum, minimum, maximum, extra, Any)",
        "description": "Number ",
        "returns": "dict"
      },
      {
        "name": "boolean",
        "signature": "boolean(*, description, default, enum, extra, Any)",
        "description": "Boolean ",
        "returns": "dict"
      },
      {
        "name": "null",
        "signature": "null(*, description, default, enum, extra, Any)",
        "description": "Null ",
        "returns": "dict"
      },
      {
        "name": "array",
        "signature": "array(items, Any, *, description, default, enum, min_items, max_items, extra, Any)",
        "description": "Array ",
        "returns": "dict"
      },
      {
        "name": "object",
        "signature": "object(*, description, default, enum, additional_properties, extra, Any, **properties: dict[str, Any)",
        "description": "Object ",
        "returns": "dict"
      },
      {
        "name": "optional",
        "signature": "optional(schema, Any, *, default)",
        "description": "Optional ",
        "returns": "dict"
      }
    ]
  },
  {
    "module": "knot.pool",
    "description": "Manage space pools — fixed-size, self-healing groups of identical spaces.",
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
        "signature": "create(name, template_name, startup_script_id, desired_count, active)",
        "description": "Create a pool with the given number of spaces",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name, desired_count, active)",
        "description": "Update pool desired count or active state. Name, template, and startup script are immutable.",
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
        "signature": "create(name, permissions)",
        "description": "Create a new role",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(role_id, name, permissions)",
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
        "signature": "create(name, content, description, owner, groups, zones, **kwargs: Any)",
        "description": "Create a script. Use owner='current' for an own script.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(script_id, name, content, description, groups, **kwargs: Any)",
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
    "module": "knot.server",
    "description": "Server information.",
    "functions": [
      {
        "name": "info",
        "signature": "info()",
        "description": "Get server-wide information",
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
        "signature": "create(content, is_global, groups, zones)",
        "description": "Create a new skill",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name_or_id, content, groups, zones)",
        "description": "Update skill",
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
        "description": "Fuzzy search skills by name/description",
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
        "signature": "list(all_zones)",
        "description": "List spaces visible to the current user. Defaults to the current server's zone; pass all_zones=True to include other zones.",
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
        "signature": "create(name, template_name, description, shell, depends_on, icon_url, custom_fields, str, start_on_create)",
        "description": "Create a new space",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(name, new_name, description, shell, template_name, icon_url, custom_fields, str, start, stop, restart)",
        "description": "Update space properties while preserving fields not passed",
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
        "description": "Get historical resource usage for a space",
        "returns": "list of dicts"
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
        "name": "transfer",
        "signature": "transfer(name, user_id)",
        "description": "Transfer space to another user (user_id can be username, email, or UUID)",
        "returns": "bool"
      },
      {
        "name": "share",
        "signature": "share(name, user_ids)",
        "description": "Share space with one or more users (user_ids can be usernames, emails, or UUIDs)",
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
        "signature": "run(name, command, args, timeout, workdir)",
        "description": "Execute a command in a space",
        "returns": "dict"
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
        "description": "Read file contents from a running space; offset/limit select a 1-based line range",
        "returns": "string"
      },
      {
        "name": "write_file",
        "signature": "write_file(name, file_path, content, mode, mtime_ns, file_perm)",
        "description": "Write content to a file in a running space (overwrite/append/prepend). Optional mtime_ns (Unix nanoseconds) and file_perm (int bits like 0o644) are applied after the write so the destination matches a source file's metadata — useful for sync tools.",
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
        "description": "Forward a local port to a remote space port",
        "returns": "bool"
      },
      {
        "name": "port_list",
        "signature": "port_list(name)",
        "description": "List active port forwards for a space",
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
        "signature": "port_apply(source_space, forwards, Any)",
        "description": "Replace all port forwards for a space",
        "returns": "bool"
      },
      {
        "name": "tunnel_start",
        "signature": "tunnel_start(space, protocol, port, name)",
        "description": "Start an agent-owned web tunnel in a space, exposing <port> as <user>--<name>.<domain>. Owned by the space's agent; not persisted.",
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
      }
    ]
  },
  {
    "module": "knot.stack",
    "description": "Manage stacks and stack definitions.",
    "functions": [
      {
        "name": "list_defs",
        "signature": "list_defs()",
        "description": "List stack definitions visible to the current user",
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
        "signature": "create_def(name, description, icon_url, scope, active, groups, zones, spaces, Any)",
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
        "signature": "validate_def(spaces, Any, name, description, icon_url, scope, active, groups, zones)",
        "description": "Validate a stack definition without saving",
        "returns": "dict"
      },
      {
        "name": "add_component",
        "signature": "add_component(stack_definition, Any, template, name, description, shell, **kwargs: Any)",
        "description": "Add a component (template binding) to an existing stack definition. Resolves template name to ID and rejects duplicate component names.",
        "returns": "dict"
      },
      {
        "name": "remove_component",
        "signature": "remove_component(stack_definition, Any, name)",
        "description": "Remove a component from a stack definition by name. Also cleans up depends_on and port_forward references in sibling components.",
        "returns": "dict"
      },
      {
        "name": "create",
        "signature": "create(definition_name, prefix, stack_name)",
        "description": "Create spaces from a stack definition",
        "returns": "list of strings"
      },
      {
        "name": "delete",
        "signature": "delete(stack_name)",
        "description": "Delete all spaces in a stack",
        "returns": "bool"
      },
      {
        "name": "start",
        "signature": "start(stack_name)",
        "description": "Start all spaces in a stack in dependency order",
        "returns": "bool"
      },
      {
        "name": "stop",
        "signature": "stop(stack_name)",
        "description": "Stop all spaces in a stack in reverse dependency order",
        "returns": "bool"
      },
      {
        "name": "restart",
        "signature": "restart(stack_name)",
        "description": "Restart all spaces in a stack",
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
        "name": "list",
        "signature": "list(include_inactive)",
        "description": "List templates visible to the current user. Defaults to active templates only; pass include_inactive=True to include retired ones.",
        "returns": "list of dicts"
      },
      {
        "name": "get",
        "signature": "get(template_id)",
        "description": "Get template by ID or name",
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
        "signature": "build_spec(platform, spec, Any, original_job, original_volumes)",
        "description": "Build native job and volume text from a unified spec (image, environment, ports, storage, memory, cpus). Same conversion as the UI spec wizard. Patch into originals to preserve hand-written content.",
        "returns": "dict<str, str>"
      },
      {
        "name": "nodes",
        "signature": "nodes(template_id)",
        "description": "List available placement nodes for a local-container template",
        "returns": "list of dicts"
      },
      {
        "name": "create",
        "signature": "create(name, job, description, platform, volumes, active, **kwargs: Any)",
        "description": "Create a new template. health_check_type can be none, agent, tcp, http, program, or custom. ports is a list of {name, port, protocol} objects; jobs is a list of {name, command, schedule, enabled} objects copied into new spaces.",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(template_id, name, job, description, platform, **kwargs: Any)",
        "description": "Update template properties, including health_check_type, health_check_auto_restart, ports and jobs",
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
        "description": "Get list of available icons",
        "returns": "list of dicts"
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
        "signature": "create(username, email, password, active, max_spaces, compute_units, storage_units, max_tunnels, ssh_public_key, github_username)",
        "description": "Create a new user",
        "returns": "string"
      },
      {
        "name": "update",
        "signature": "update(user_id, username, email, password, active, max_spaces, ssh_public_key, github_username)",
        "description": "Update user properties",
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
        "name": "get_quota",
        "signature": "get_quota(user_id)",
        "description": "Get user quota and usage",
        "returns": "dict"
      },
      {
        "name": "list_permissions",
        "signature": "list_permissions(user_id)",
        "description": "List all permissions for a user",
        "returns": "list of dicts"
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
