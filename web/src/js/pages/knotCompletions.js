// AUTO-GENERATED from ../knot-vscode/stubs/knot — do not edit by hand.
// Regenerate with: task knot-completions
// Descriptions are derived from docstrings where present, function names otherwise.

export const knotLibraries = [
  {
    "module": "knot.event",
    "description": "Knot event functions.",
    "functions": [
      {
        "name": "emit",
        "signature": "emit(type, payload, Any, default, default, default, default, default, Any, Any, Any)",
        "description": "Emit ",
        "returns": "dict<str, str>"
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
    "module": "knot.space",
    "description": "Manage development spaces.",
    "functions": [
      {
        "name": "list",
        "signature": "list(all_zones, Any, Any, template_name, description, shell, depends_on, icon_url, custom_fields, str, start_on_create, ) -> str: \"\"\"Create a new space\"\"\" ... def update( name: str, new_name, description, shell, template_name, icon_url, custom_fields, str, start, stop, restart, ) -> bool: \"\"\"Update space properties while preserving fields not passed\"\"\" ... def delete(name: str) -> bool: \"\"\"Delete a space by name\"\"\" ... def start(name: str) -> bool: \"\"\"Start a space by name\"\"\" ... def stop(name: str) -> bool: \"\"\"Stop a space by name\"\"\" ... def restart(name: str) -> bool: \"\"\"Restart a space by name\"\"\" ... def is_running(name: str) -> bool: \"\"\"Check if a space is running. A freshly started space reports running before its agent connects — see is_ready() for the state required by run() and file operations.\"\"\" ... def is_ready(name: str) -> bool: \"\"\"Check if a space is running with its agent connected. True only when run(), read_file, and, timeout, interval, Any, range, Any, description, depends_on, stack, field, field, value, user_id, email, or, user_ids, emails, or, user_id, optionally, command, args, timeout, workdir, Any, script_name, args, Any, code, args, Any, file_path, offset, limit, file_path, content, mode, mtime_ns, file_perm, pattern, path, literal, recursive, ignore_case, glob, follow_links, max_size, workdir, ) -> builtins.list[dict[str, Any, path, recursive, type, name_glob, mtime_min, mtime_max, size_min, size_max, include_hidden, follow_links, max_depth, workdir, ) -> builtins.list[str]: \"\"\"Find files and directories in a running space by name, type, mtime, or, path, recursive, type, name_glob, mtime_min, mtime_max, size_min, size_max, include_hidden, follow_links, max_depth, include_hash, include_symlinks, workdir, ) -> builtins.list[dict[str, Any, size, mtime, is_dir, file_perm, include_symlinks, old, new, path, recursive, ignore_case, glob, follow_links, max_size, workdir, ) -> int: \"\"\"Replace literal occurrences of old with new in files (atomic in-place edit)\"\"\" ... def sed_replace_pattern( name: str, pattern, new, path, recursive, ignore_case, glob, follow_links, max_size, workdir, ) -> int: \"\"\"Replace regex matches in files; capture groups as ${1}, ${name} (atomic in-place edit)\"\"\" ... def sed_extract( name: str, pattern, path, recursive, ignore_case, glob, follow_links, max_size, workdir, ) -> builtins.list[dict[str, Any, file_path, search, replace, workdir, ) -> int: \"\"\"Targeted search-and-replace edit on a single file; search must be unique (fails if 0 or >1 matches)\"\"\" ... def delete_file( name: str, file_path, recursive, workdir)",
        "description": "List items",
        "returns": "int"
      }
    ]
  }
];
