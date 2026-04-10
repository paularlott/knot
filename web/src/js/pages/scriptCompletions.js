// Auto-completion data for Scriptling in Knot
// Provides intelligent autocomplete with context-aware suggestions

/**
 * Rich library definitions with signatures, return types, and class methods
 * Each library has:
 * - module: Module name
 * - description: Module description
 * - functions: Array of function definitions with signature, description, returns
 * - classes: Array of class definitions with methods
 */
const scriptLibraries = [
  // ============================================================================
  // KNOT LIBRARIES (knot.*) - Server-side API libraries
  // ============================================================================
  {
    module: "knot.space",
    description: "Knot space management functions",
    functions: [
      {
        name: "start",
        signature: "start(name)",
        description: "Start a space by name",
        returns: "bool - True if successful",
      },
      {
        name: "stop",
        signature: "stop(name)",
        description: "Stop a space by name",
        returns: "bool - True if successful",
      },
      {
        name: "restart",
        signature: "restart(name)",
        description: "Restart a space by name",
        returns: "bool - True if successful",
      },
      {
        name: "delete",
        signature: "delete(name)",
        description: "Delete a space by name",
        returns: "bool - True if successful",
      },
      {
        name: "list",
        signature: "list()",
        description: "List all spaces for current user",
        returns:
          "list - List of space dicts with name, id, is_running, description",
      },
      {
        name: "is_running",
        signature: "is_running(name)",
        description: "Check if a space is running",
        returns: "bool - True if space is running",
      },
      {
        name: "get_description",
        signature: "get_description(name)",
        description: "Get space description",
        returns: "str - Space description",
      },
      {
        name: "set_description",
        signature: "set_description(name, description)",
        description: "Set space description",
        returns: "bool - True if successful",
      },
      {
        name: "get_field",
        signature: "get_field(name, field)",
        description: "Get custom field value from space",
        returns: "str - Field value",
      },
      {
        name: "set_field",
        signature: "set_field(name, field, value)",
        description: "Set custom field value on space",
        returns: "bool - True if successful",
      },
      {
        name: "create",
        signature: "create(name, template_name, description='', shell='bash')",
        description: "Create a new space",
        returns: "str - New space ID",
      },
      {
        name: "run",
        signature: "run(space_name, command, args=[], timeout=30, workdir='')",
        description: "Execute a command in a space",
        returns: "str - Command output",
      },
      {
        name: "run_script",
        signature: "run_script(space_name, script_name, *args)",
        description: "Execute a script in a space",
        returns: "dict - Dict with output (str) and exit_code (int)",
      },
      {
        name: "port_forward",
        signature:
          "port_forward(source_space, local_port, remote_space, remote_port)",
        description: "Forward a local port to a remote space port",
        returns: "bool - True if successful",
      },
      {
        name: "port_list",
        signature: "port_list(space)",
        description: "List active port forwards for a space",
        returns: "list - List of port forward dicts",
      },
      {
        name: "port_stop",
        signature: "port_stop(space, local_port)",
        description: "Stop a port forward",
        returns: "bool - True if successful",
      },
      {
        name: "get",
        signature: "get(name)",
        description: "Get space details as a dict",
        returns:
          "dict - Space details with id, name, description, template_id, template_name, user_id, username, shares, shell, platform, zone, is_running, is_pending, is_deleting, node_hostname, created_at",
      },
      {
        name: "update",
        signature: "update(name, description='', shell='')",
        description: "Update space properties",
        returns: "bool - True if successful",
      },
      {
        name: "transfer",
        signature: "transfer(name, user_id)",
        description:
          "Transfer space to another user (user_id can be username, email, or UUID)",
        returns: "bool - True if successful",
      },
      {
        name: "share",
        signature: "share(name, user_id)",
        description:
          "Share space with another user (user_id can be username, email, or UUID)",
        returns: "bool - True if successful",
      },
      {
        name: "unshare",
        signature: "unshare(name)",
        description: "Remove space share",
        returns: "bool - True if successful",
      },
      {
        name: "read_file",
        signature: "read_file(space_name, file_path)",
        description: "Read file contents from a running space",
        returns: "str - File contents",
      },
      {
        name: "write_file",
        signature: "write_file(space_name, file_path, content)",
        description: "Write content to a file in a running space",
        returns: "bool - True if successful",
      },
    ],
  },
  {
    module: "knot.ai",
    description:
      "Knot AI client library - returns pre-configured AI client and default model",
    functions: [
      {
        name: "Client",
        signature: "Client()",
        description:
          "Get a pre-configured AI client instance connected to the server's AI provider with MCP tools available",
        returns:
          "Client - A pre-configured AI client instance with completion(), completion_stream(), and other methods",
      },
      {
        name: "get_default_model",
        signature: "get_default_model()",
        description: "Get the server-configured default model name",
        returns:
          "str - The model name (e.g. 'gpt-4o', 'claude-sonnet-4-20250514'), or empty string if not configured",
      },
    ],
  },
  {
    module: "scriptling.mcp.tool",
    description:
      "MCP tool helper functions - parameter access and result functions for MCP tools",
    functions: [
      {
        name: "get_string",
        signature: 'get_string(name, default="")',
        description:
          "Get MCP parameter value as a trimmed string, handling None and whitespace",
        returns: "str - Parameter value as trimmed string or default",
      },
      {
        name: "get_int",
        signature: "get_int(name, default=0)",
        description:
          "Get MCP parameter value as an integer, handling None, empty strings, and whitespace",
        returns: "int - Parameter value as integer or default",
      },
      {
        name: "get_float",
        signature: "get_float(name, default=0.0)",
        description:
          "Get MCP parameter value as a float, handling None, empty strings, and whitespace",
        returns: "float - Parameter value as float or default",
      },
      {
        name: "get_bool",
        signature: "get_bool(name, default=False)",
        description:
          "Get MCP parameter value as a boolean, handling None, empty strings, and various string representations",
        returns: "bool - Parameter value as boolean or default",
      },
      {
        name: "get_list",
        signature: "get_list(name, default=[])",
        description:
          "Get MCP parameter value as a list, handling comma-separated strings or arrays",
        returns: "list - Parameter value as list or default",
      },
      {
        name: "get_string_list",
        signature: "get_string_list(name, default=[])",
        description:
          "Get MCP array:string parameter as a list of strings",
        returns: "list[str] - String array or default",
      },
      {
        name: "get_int_list",
        signature: "get_int_list(name, default=[])",
        description:
          "Get MCP array:int parameter as a list of integers",
        returns: "list[int] - Integer array or default",
      },
      {
        name: "get_float_list",
        signature: "get_float_list(name, default=[])",
        description:
          "Get MCP array:float parameter as a list of floats",
        returns: "list[float] - Float array or default",
      },
      {
        name: "get_bool_list",
        signature: "get_bool_list(name, default=[])",
        description:
          "Get MCP array:bool parameter as a list of booleans",
        returns: "list[bool] - Boolean array or default",
      },
      {
        name: "return_string",
        signature: "return_string(value)",
        description: "Return a string result and exit",
        returns: "str - The same string",
      },
      {
        name: "return_object",
        signature: "return_object(value)",
        description: "Return a structured object as JSON and exit",
        returns: "str - JSON string representation",
      },
      {
        name: "return_toon",
        signature: "return_toon(value)",
        description:
          "Return a value encoded as toon (compact serialization) and exit",
        returns: "str - Toon-encoded value",
      },
      {
        name: "return_error",
        signature: "return_error(message)",
        description: "Return an error message and exit with error code",
        returns: "str - Error message",
      },
    ],
  },
  {
    module: "knot.mcp",
    description: "Knot MCP tool functions - tool discovery and calling",
    functions: [
      {
        name: "list_tools",
        signature: "list_tools()",
        description: "Get list of available MCP tools and their parameters",
        returns: "list - List of tool dicts with name, description, parameters",
      },
      {
        name: "call_tool",
        signature: "call_tool(name, arguments)",
        description: "Call an MCP tool directly. Arguments should be a dict",
        returns: "any - Decoded tool response",
      },
      {
        name: "tool_search",
        signature: "tool_search(query, max_results=10)",
        description:
          "Search for tools by keyword. Returns list of matching tools",
        returns: "list - List of matching tool dicts",
      },
      {
        name: "execute_tool",
        signature: "execute_tool(name, arguments)",
        description:
          "Execute a discovered tool. Use full name for namespaced tools",
        returns: "any - Tool response",
      },
    ],
  },
  {
    module: "knot.user",
    description: "Knot user management functions",
    functions: [
      {
        name: "list",
        signature: "list(state='', zone='')",
        description: "List all users with optional state/zone filter",
        returns:
          "list - List of user dicts with id, username, email, active, number_spaces",
      },
      {
        name: "get",
        signature: "get(user_id)",
        description: "Get user by ID or username",
        returns: "dict - User object with all user details",
      },
      {
        name: "get_me",
        signature: "get_me()",
        description: "Get current user",
        returns: "dict - Current user object",
      },
      {
        name: "create",
        signature: "create(username, email, password, ...)",
        description: "Create a new user",
        returns: "str - ID of the newly created user",
      },
      {
        name: "update",
        signature: "update(user_id, ...)",
        description: "Update user properties",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(user_id)",
        description: "Delete a user by ID or username",
        returns: "bool - True if successfully deleted",
      },
      {
        name: "list_permissions",
        signature: "list_permissions(user_id)",
        description: "List all permissions for a user",
        returns: "list - List of permission IDs",
      },
      {
        name: "has_permission",
        signature: "has_permission(user_id, permission_id)",
        description: "Check if user has a specific permission",
        returns: "bool - True if user has the permission",
      },
      {
        name: "get_quota",
        signature: "get_quota(user_id)",
        description: "Get user quota and usage",
        returns:
          "dict - Quota details with max_spaces, compute_units, storage_units, max_tunnels, number_spaces, number_spaces_deployed, used_compute_units, used_storage_units, used_tunnels",
      },
    ],
  },
  {
    module: "knot.permission",
    description: "Knot permission constants and functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description:
          "List all available permissions with IDs, names, and groups",
        returns: "list - List of permission dicts with id, name, group",
      },
    ],
    constants: [
      {
        name: "MANAGE_USERS",
        value: 0,
        description: "Permission to manage users",
      },
      {
        name: "MANAGE_TEMPLATES",
        value: 1,
        description: "Permission to manage templates",
      },
      {
        name: "MANAGE_SPACES",
        value: 2,
        description: "Permission to manage spaces",
      },
      {
        name: "MANAGE_VOLUMES",
        value: 3,
        description: "Permission to manage volumes",
      },
      {
        name: "MANAGE_GROUPS",
        value: 4,
        description: "Permission to manage groups",
      },
      {
        name: "MANAGE_ROLES",
        value: 5,
        description: "Permission to manage roles",
      },
      {
        name: "MANAGE_VARIABLES",
        value: 6,
        description: "Permission to manage template variables",
      },
      { name: "USE_SPACES", value: 7, description: "Permission to use spaces" },
      {
        name: "USE_TUNNELS",
        value: 8,
        description: "Permission to use tunnels",
      },
      {
        name: "VIEW_AUDIT_LOGS",
        value: 9,
        description: "Permission to view audit logs",
      },
      {
        name: "TRANSFER_SPACES",
        value: 10,
        description: "Permission to transfer spaces",
      },
      {
        name: "SHARE_SPACES",
        value: 11,
        description: "Permission to share spaces",
      },
      {
        name: "CLUSTER_INFO",
        value: 12,
        description: "Permission to view cluster info",
      },
      { name: "USE_VNC", value: 13, description: "Permission to use VNC" },
      {
        name: "USE_WEB_TERMINAL",
        value: 14,
        description: "Permission to use web terminal",
      },
      {
        name: "USE_SSH",
        value: 15,
        description: "Permission to use SSH connections",
      },
      {
        name: "USE_CODE_SERVER",
        value: 16,
        description: "Permission to use code-server",
      },
      {
        name: "USE_VSCODE_TUNNEL",
        value: 17,
        description: "Permission to use VSCode tunnel",
      },
      { name: "USE_LOGS", value: 18, description: "Permission to view logs" },
      {
        name: "RUN_COMMANDS",
        value: 19,
        description: "Permission to run commands",
      },
      {
        name: "COPY_FILES",
        value: 20,
        description: "Permission to copy files",
      },
      {
        name: "USE_MCP_SERVER",
        value: 21,
        description: "Permission to use MCP server",
      },
      {
        name: "USE_WEB_ASSISTANT",
        value: 22,
        description: "Permission to use web AI assistant",
      },
      {
        name: "MANAGE_SCRIPTS",
        value: 23,
        description: "Permission to manage system/global scripts",
      },
      {
        name: "EXECUTE_SCRIPTS",
        value: 24,
        description: "Permission to execute system/global scripts",
      },
      {
        name: "MANAGE_OWN_SCRIPTS",
        value: 25,
        description: "Permission to manage own scripts",
      },
      {
        name: "EXECUTE_OWN_SCRIPTS",
        value: 26,
        description: "Permission to execute own scripts",
      },
      {
        name: "SPACE_MANAGE",
        value: 2,
        description: "Alias for MANAGE_SPACES",
      },
      { name: "SPACE_USE", value: 7, description: "Alias for USE_SPACES" },
      {
        name: "SCRIPT_MANAGE",
        value: 23,
        description: "Alias for MANAGE_SCRIPTS",
      },
      {
        name: "SCRIPT_EXECUTE",
        value: 24,
        description: "Alias for EXECUTE_SCRIPTS",
      },
    ],
  },
  {
    module: "knot.group",
    description: "Knot group management functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "List all groups",
        returns:
          "list - List of group dicts with id, name, max_spaces, compute_units, storage_units",
      },
      {
        name: "get",
        signature: "get(group_id)",
        description: "Get group by ID (UUID only)",
        returns: "dict - Group object",
      },
      {
        name: "create",
        signature: "create(name, ...)",
        description:
          "Create a new group (optional kwargs: max_spaces, compute_units, storage_units, max_tunnels)",
        returns: "str - UUID of the newly created group",
      },
      {
        name: "update",
        signature: "update(group_id, ...)",
        description: "Update group properties",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(group_id)",
        description: "Delete a group by UUID",
        returns: "bool - True if successfully deleted",
      },
    ],
  },
  {
    module: "knot.role",
    description: "Knot role management functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "List all roles",
        returns: "list - List of role dicts with id, name, permissions",
      },
      {
        name: "get",
        signature: "get(role_id)",
        description: "Get role by ID (UUID only)",
        returns: "dict - Role object",
      },
      {
        name: "create",
        signature: "create(name, permissions=[])",
        description: "Create a new role",
        returns: "str - ID of the newly created role",
      },
      {
        name: "update",
        signature: "update(role_id, ...)",
        description: "Update role properties",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(role_id)",
        description: "Delete a role by UUID",
        returns: "bool - True if successfully deleted",
      },
    ],
  },
  {
    module: "knot.template",
    description: "Knot template management functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "List all templates",
        returns:
          "list - List of template dicts with id, name, description, platform, active, usage, deployed",
      },
      {
        name: "get",
        signature: "get(template_id)",
        description: "Get template by ID or name",
        returns: "dict - Template object",
      },
      {
        name: "create",
        signature: "create(name, job, ...)",
        description: "Create a new template",
        returns: "str - ID of the newly created template",
      },
      {
        name: "update",
        signature: "update(template_id, ...)",
        description: "Update template properties",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(template_id)",
        description: "Delete a template by ID or name",
        returns: "bool - True if successfully deleted",
      },
      {
        name: "get_icons",
        signature: "get_icons()",
        description: "Get list of available icons",
        returns: "list - List of icon dicts with description, source, url",
      },
    ],
  },
  {
    module: "knot.vars",
    description: "Knot template variable management functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "List all template variables",
        returns:
          "list - List of variable dicts with id, name, local, protected, restricted",
      },
      {
        name: "get",
        signature: "get(var_id)",
        description: "Get variable value",
        returns: "dict - Variable object with value",
      },
      {
        name: "set",
        signature: "set(var_id, value)",
        description: "Set variable value (updates existing)",
        returns: "bool - True if successfully updated",
      },
      {
        name: "create",
        signature: "create(name, value, ...)",
        description: "Create a new variable",
        returns: "str - ID of the newly created variable",
      },
      {
        name: "delete",
        signature: "delete(var_id)",
        description: "Delete a variable",
        returns: "bool - True if successfully deleted",
      },
    ],
  },
  {
    module: "knot.volume",
    description: "Knot volume management functions",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "List all volumes",
        returns:
          "list - List of volume dicts with id, name, active, zone, platform",
      },
      {
        name: "get",
        signature: "get(volume_id)",
        description: "Get volume by ID or name",
        returns: "dict - Volume object",
      },
      {
        name: "create",
        signature: "create(name, definition, platform='')",
        description: "Create a new volume",
        returns: "str - ID of the newly created volume",
      },
      {
        name: "update",
        signature: "update(volume_id, ...)",
        description: "Update volume properties",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(volume_id)",
        description: "Delete a volume by ID or name",
        returns: "bool - True if successfully deleted",
      },
      {
        name: "start",
        signature: "start(volume_id)",
        description: "Start a stopped volume",
        returns: "bool - True if successfully started",
      },
      {
        name: "stop",
        signature: "stop(volume_id)",
        description: "Stop a running volume",
        returns: "bool - True if successfully stopped",
      },
      {
        name: "is_running",
        signature: "is_running(volume_id)",
        description: "Check if a volume is currently running",
        returns: "bool - True if the volume is running",
      },
    ],
  },
  {
    module: "knot.skill",
    description: "Knot skill management functions",
    functions: [
      {
        name: "create",
        signature: "create(content, global=False, groups=[], zones=[])",
        description: "Create a new skill",
        returns: "str - ID of the newly created skill",
      },
      {
        name: "get",
        signature: "get(name_or_id)",
        description: "Get skill by name or UUID",
        returns:
          "dict - Skill object with id, user_id, name, description, content, is_managed, groups, zones",
      },
      {
        name: "update",
        signature: "update(name_or_id, content=None, groups=None, zones=None)",
        description: "Update skill",
        returns: "bool - True if successfully updated",
      },
      {
        name: "delete",
        signature: "delete(name_or_id)",
        description: "Delete skill",
        returns: "bool - True if successfully deleted",
      },
      {
        name: "list",
        signature: "list(owner=None)",
        description: "List skills (filtered by permissions/groups/zones)",
        returns:
          "list - List of skill dicts with id, name, description, user_id, is_managed",
      },
      {
        name: "search",
        signature: "search(query)",
        description: "Fuzzy search skills by name/description",
        returns: "list - List of matching skill dicts",
      },
    ],
  },

  {
    module: "knot.audit",
    description: "Knot audit log search and filtering functions",
    functions: [
      {
        name: "list",
        signature: "list(start=0, max_items=10, q='', actor='', actor_type='', event='', from_time='', to_time='')",
        description: "List audit log entries with optional filtering",
        returns: "dict - Dict with count (int) and items (list of audit log entry dicts)",
      },
      {
        name: "search",
        signature: "search(q, start=0, max_items=10, actor='', actor_type='', event='', from_time='', to_time='')",
        description: "Search audit logs with a text query across actor, event, and details",
        returns: "dict - Dict with count (int) and items (list of audit log entry dicts)",
      },
    ],
  },
  {
    module: "knot.healthcheck",
    description: "Health check functions for space monitoring (agent-side only)",
    functions: [
      {
        name: "http_head",
        signature: "http_head(url, skip_ssl_verify=False, timeout=10)",
        description: "HTTP HEAD check, 200 = healthy, anything else = unhealthy. Set skip_ssl_verify=True for self-signed certs",
        returns: "None - Exits immediately with health result",
      },
      {
        name: "tcp_port",
        signature: "tcp_port(port, timeout=10)",
        description: "TCP port check, open = healthy, closed = unhealthy",
        returns: "None - Exits immediately with health result",
      },
      {
        name: "program",
        signature: "program(command, timeout=10)",
        description: "Run command, exit code 0 = healthy, non-zero = unhealthy",
        returns: "None - Exits immediately with health result",
      },
      {
        name: "pass_check",
        signature: "pass_check()",
        description: "Report healthy (for custom checks)",
        returns: "None - Exits immediately with healthy status",
      },
      {
        name: "fail",
        signature: 'fail(reason="")',
        description: "Report unhealthy with optional reason (for custom checks)",
        returns: "None - Exits immediately with unhealthy status",
      },
    ],
  },

  // ============================================================================
  // SCRIPTLING LIBRARIES (scriptling.*) - Standalone scriptling libraries
  // ============================================================================
  {
    module: "scriptling.ai",
    description:
      "AI and LLM functions for interacting with multiple AI provider APIs",
    constants: [
      {
        name: "ToolRegistry",
        description: "Tool registry class for building AI tool schemas",
        type: "class",
      },
      {
        name: "OPENAI",
        description: 'Provider constant for OpenAI ("openai")',
        type: "string",
      },
      {
        name: "CLAUDE",
        description: 'Provider constant for Anthropic Claude ("claude")',
        type: "string",
      },
      {
        name: "GEMINI",
        description: 'Provider constant for Google Gemini ("gemini")',
        type: "string",
      },
      {
        name: "OLLAMA",
        description: 'Provider constant for Ollama ("ollama")',
        type: "string",
      },
      {
        name: "ZAI",
        description: 'Provider constant for ZAi ("zai")',
        type: "string",
      },
      {
        name: "MISTRAL",
        description: 'Provider constant for Mistral AI ("mistral")',
        type: "string",
      },
    ],
    functions: [
      {
        name: "Client",
        signature:
          'Client(base_url, provider="openai", api_key="", max_tokens=0, temperature=0, top_p=0, remote_servers=[])',
        description:
          "Create a new AI client instance for making API calls to supported services",
        returns: "OpenAIClient - A client instance",
        returnType: "OpenAIClient",
      },
      {
        name: "extract_thinking",
        signature: "extract_thinking(text)",
        description:
          "Extract thinking blocks from AI response text and return cleaned content",
        returns: "dict - {'thinking': list of blocks, 'content': cleaned text}",
      },
      {
        name: "text",
        signature: "text(response)",
        description:
          "Get text content from completion response without thinking blocks",
        returns: "str - Response text with thinking removed",
      },
      {
        name: "thinking",
        signature: "thinking(response)",
        description: "Get thinking blocks from completion response",
        returns: "list - List of thinking block strings",
      },
      {
        name: "tool_calls",
        signature: "tool_calls(response_or_message)",
        description:
          "Extract normalized tool calls from a completion response, message dict, or tool call list",
        returns:
          "list[dict] - List of tool call dicts with id, name, arguments",
      },
      {
        name: "execute_tool_calls",
        signature: "execute_tool_calls(registry, tool_calls)",
        description:
          "Execute normalized tool calls using handlers from a ToolRegistry",
        returns: "list - List of tool results",
      },
      {
        name: "collect_stream",
        signature:
          "collect_stream(stream, *, chunk_timeout_ms=None, first_chunk_timeout_ms=None, on_event=None)",
        description:
          "Consume a ChatStream and aggregate content, reasoning, tool calls, and finish status",
        returns:
          "dict - Aggregated result with content, reasoning, tool_calls, finished",
      },
      {
        name: "tool_round",
        signature:
          "tool_round(client, model, messages, registry, *, stream=False, chunk_timeout_ms=None, on_event=None, system_prompt=None, temperature=None, top_p=None, max_tokens=None, timeout_ms=None)",
        description:
          "Run one tool-enabled completion round and return the assistant message, tool calls, and tool results",
        returns:
          "dict - Result with message, tool_calls, tool_results",
      },
      {
        name: "estimate_tokens",
        signature: "estimate_tokens(request, response)",
        description:
          "Estimate token counts for request messages and response using character-based heuristic",
        returns: "tuple - (request_tokens, response_tokens)",
      },
    ],
    classes: [
      {
        name: "OpenAIClient",
        description: "AI client for multiple provider APIs",
        methods: [
          {
            name: "completion",
            signature: "completion(model, messages, **kwargs)",
            description: "Create a chat completion",
            returns: "dict - Response with id, choices, usage",
          },
          {
            name: "completion_stream",
            signature: "completion_stream(model, messages, **kwargs)",
            description: "Create a streaming chat completion",
            returns: "ChatStream - Stream object with next() method",
            returnType: "ChatStream",
          },
          {
            name: "models",
            signature: "models()",
            description: "List available models",
            returns: "list - List of model dicts",
          },
          {
            name: "response_create",
            signature: "response_create(model, input, **kwargs)",
            description: "Create Responses API response",
            returns: "dict - Response object",
          },
          {
            name: "response_get",
            signature: "response_get(id)",
            description: "Get response by ID",
            returns: "dict - Response object",
          },
          {
            name: "response_cancel",
            signature: "response_cancel(id)",
            description: "Cancel in-progress response",
            returns: "dict - Cancelled response",
          },
          {
            name: "response_delete",
            signature: "response_delete(id)",
            description: "Delete a response by ID",
            returns: "None",
          },
          {
            name: "response_compact",
            signature: "response_compact(id)",
            description:
              "Compact a response by removing intermediate reasoning steps",
            returns: "dict - Compacted response object",
          },
          {
            name: "embedding",
            signature: "embedding(model, input)",
            description: "Create embedding vector from input text",
            returns: "dict - Embedding response with vector",
          },
          {
            name: "response_stream",
            signature: "response_stream(model, input, **kwargs)",
            description:
              "Stream a Responses API response, returning a ResponseStream object",
            returns: "ResponseStream - Stream object with next() method",
            returnType: "ResponseStream",
          },
          {
            name: "ask",
            signature: "ask(model, messages, **kwargs)",
            description:
              "Quick completion that returns text directly without thinking blocks",
            returns: "str - Response text",
          },
        ],
      },
      {
        name: "ResponseStream",
        description: "Streaming Responses API event iterator",
        methods: [
          {
            name: "next",
            signature: "next()",
            description:
              "Get next event from stream. Event types include response.output_text.delta, response.completed, etc.",
            returns: "dict or null - Next event dict or null if complete",
          },
        ],
      },
      {
        name: "ToolRegistry",
        description: "Tool registry for building AI tool schemas",
        methods: [
          {
            name: "add",
            signature: "add(name, description, params, handler)",
            description: "Add a tool to the registry",
            returns: "None",
          },
          {
            name: "build",
            signature: "build()",
            description: "Build OpenAI-compatible tool schemas",
            returns: "list - List of tool schema dicts",
          },
          {
            name: "get_handler",
            signature: "get_handler(name)",
            description: "Get tool handler by name",
            returns: "callable - Tool handler function",
          },
        ],
      },
      {
        name: "ChatStream",
        description: "Streaming chat response iterator",
        methods: [
          {
            name: "next",
            signature: "next()",
            description: "Get next chunk from stream",
            returns: "dict or null - Next chunk or null if complete",
          },
          {
            name: "err",
            signature: "err()",
            description: "Get any error that caused the stream to stop",
            returns: "str or None - Error message, or None if no error",
          },
          {
            name: "next_timeout",
            signature: "next_timeout(timeout_ms)",
            description:
              "Get next chunk from stream with timeout. Returns chunk, {timed_out: True}, or None",
            returns:
              "dict or null - Next chunk, timed_out dict, or null if complete",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.mcp",
    description: "MCP client for connecting to remote MCP servers",
    functions: [
      {
        name: "decode_response",
        signature: "decode_response(response)",
        description: "Decode a raw MCP tool response into scriptling objects",
        returns: "object - Decoded response",
      },
      {
        name: "Client",
        signature: 'Client(base_url, *, namespace="", bearer_token="")',
        description:
          "Create a new MCP client for connecting to a remote MCP server",
        returns: "MCPClient - A client instance",
        returnType: "MCPClient",
      },
    ],
    classes: [
      {
        name: "MCPClient",
        description: "MCP client for remote server interaction",
        methods: [
          {
            name: "tools",
            signature: "tools()",
            description: "List all tools available from this MCP server",
            returns: "list - Tool dicts",
          },
          {
            name: "call_tool",
            signature: "call_tool(name, arguments)",
            description: "Execute a tool",
            returns: "dict - Tool response",
          },
          {
            name: "refresh_tools",
            signature: "refresh_tools()",
            description: "Refresh cached tool list",
            returns: "None",
          },
          {
            name: "tool_search",
            signature: "tool_search(query, max_results=10)",
            description: "Search for tools",
            returns: "list - Matching tools",
          },
          {
            name: "execute_discovered",
            signature: "execute_discovered(name, arguments)",
            description: "Execute a discovered tool",
            returns: "dict - Tool response",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.toon",
    description:
      "TOON (Token-Oriented Object Notation) encoding/decoding library",
    functions: [
      {
        name: "encode",
        signature: "encode(data)",
        description: "Encode data to TOON format",
        returns: "str - TOON formatted string",
      },
      {
        name: "decode",
        signature: "decode(text)",
        description: "Decode TOON format to scriptling objects",
        returns: "object - Decoded value",
      },
      {
        name: "encode_options",
        signature: "encode_options(data, indent=2, delimiter=',')",
        description: "Encode with custom options",
        returns: "str - TOON formatted string",
      },
      {
        name: "decode_options",
        signature: "decode_options(text, strict=True, indent_size=0)",
        description: "Decode with custom options",
        returns: "object - Decoded value",
      },
    ],
  },
  {
    module: "scriptling.console",
    description: "TUI console for interactive terminal applications with multi-panel layouts (Local environment only)",
    constants: [
      { name: "PRIMARY", description: "Theme primary color", type: "string" },
      { name: "SECONDARY", description: "Theme secondary color", type: "string" },
      { name: "ERROR", description: "Theme error color", type: "string" },
      { name: "DIM", description: "Theme dim color", type: "string" },
      { name: "USER", description: "Theme user text color", type: "string" },
      { name: "TEXT", description: "Theme default text color", type: "string" },
    ],
    functions: [
      {
        name: "panel",
        signature: 'panel(name="main")',
        description: "Get an existing Panel instance by name",
        returns: "Panel or None - Panel instance",
      },
      {
        name: "main_panel",
        signature: "main_panel()",
        description: "Get the main panel",
        returns: "Panel - Main panel instance",
        returnType: "Panel",
      },
      {
        name: "create_panel",
        signature:
          'create_panel(name="", width=0, height=0, min_width=0, scrollable=False, title="", no_border=False, skip_focus=False)',
        description:
          "Create a new panel. Attach to layout with add_left, add_right, add_row, or add_column",
        returns: "Panel - Panel instance",
        returnType: "Panel",
      },
      {
        name: "add_left",
        signature: "add_left(panel)",
        description: "Add a panel to the left of the main panel",
        returns: "None",
      },
      {
        name: "add_right",
        signature: "add_right(panel)",
        description: "Add a panel to the right of the main panel",
        returns: "None",
      },
      {
        name: "clear_layout",
        signature: "clear_layout()",
        description:
          "Remove the layout tree but keep all panels and their content",
        returns: "None",
      },
      {
        name: "has_panels",
        signature: "has_panels()",
        description: "Check if multi-panel layout is active",
        returns: "bool - True if multi-panel layout is active",
      },
      {
        name: "styled",
        signature: "styled(color, text)",
        description:
          "Apply theme color to text (use color constants or #rrggbb)",
        returns: "str - Styled text string",
      },
      {
        name: "set_status",
        signature: "set_status(left, right)",
        description: "Set both status bar texts",
        returns: "None",
      },
      {
        name: "set_status_left",
        signature: "set_status_left(text)",
        description: "Set left status bar text",
        returns: "None",
      },
      {
        name: "set_status_right",
        signature: "set_status_right(text)",
        description: "Set right status bar text",
        returns: "None",
      },
      {
        name: "set_labels",
        signature: "set_labels(user, assistant, system)",
        description: "Set role labels; empty string leaves label unchanged",
        returns: "None",
      },
      {
        name: "register_command",
        signature: "register_command(name, description, fn)",
        description: "Register a slash command",
        returns: "None",
      },
      {
        name: "remove_command",
        signature: "remove_command(name)",
        description: "Remove a registered slash command",
        returns: "None",
      },
      {
        name: "on_submit",
        signature: "on_submit(fn)",
        description: "Register handler called when user submits input",
        returns: "None",
      },
      {
        name: "on_escape",
        signature: "on_escape(fn)",
        description: "Register a callback for Esc key",
        returns: "None",
      },
      {
        name: "spinner_start",
        signature: "spinner_start(text='Working')",
        description: "Show a spinner with optional text",
        returns: "None",
      },
      {
        name: "spinner_stop",
        signature: "spinner_stop()",
        description: "Hide the spinner",
        returns: "None",
      },
      {
        name: "set_progress",
        signature: "set_progress(label, pct)",
        description: "Set progress bar (0.0-1.0, or <0 to clear)",
        returns: "None",
      },
      {
        name: "run",
        signature: "run()",
        description: "Start the console event loop (blocks until exit)",
        returns: "None",
      },
    ],
    classes: [
      {
        name: "Panel",
        description:
          "A content panel within the TUI layout. Supports text content and message-based streaming",
        methods: [
          {
            name: "write",
            signature: "write(text)",
            description: "Append text to the panel",
            returns: "None",
          },
          {
            name: "set_content",
            signature: "set_content(text)",
            description: "Replace all panel content",
            returns: "None",
          },
          {
            name: "clear",
            signature: "clear()",
            description: "Remove all panel content",
            returns: "None",
          },
          {
            name: "set_title",
            signature: "set_title(title)",
            description: "Set the panel border title (empty string hides title)",
            returns: "None",
          },
          {
            name: "set_color",
            signature: "set_color(color)",
            description:
              "Set the panel border/accent color (color name or hex #RRGGBB)",
            returns: "None",
          },
          {
            name: "set_scrollable",
            signature: "set_scrollable(scrollable)",
            description: "Set whether panel content scrolls",
            returns: "None",
          },
          {
            name: "add_message",
            signature: 'add_message(*args, label="", role="")',
            description:
              "Add a message to the panel. Role can be user, assistant, system, thinking, or tool",
            returns: "None",
          },
          {
            name: "stream_start",
            signature: 'stream_start(label="", role="")',
            description:
              "Begin a streaming message in this panel. Role can be user, assistant, system, thinking, or tool",
            returns: "None",
          },
          {
            name: "stream_chunk",
            signature: "stream_chunk(text)",
            description: "Append a chunk to the current stream",
            returns: "None",
          },
          {
            name: "stream_end",
            signature: "stream_end()",
            description: "Finalize the current stream",
            returns: "None",
          },
          {
            name: "scroll_to_top",
            signature: "scroll_to_top()",
            description: "Scroll to top of panel content",
            returns: "None",
          },
          {
            name: "scroll_to_bottom",
            signature: "scroll_to_bottom()",
            description: "Scroll to bottom of panel content",
            returns: "None",
          },
          {
            name: "size",
            signature: "size()",
            description: "Get the panel dimensions",
            returns: "list - [width, height]",
          },
          {
            name: "styled",
            signature: "styled(color, text)",
            description: "Apply theme color to text",
            returns: "str - Styled text string",
          },
          {
            name: "write_at",
            signature: "write_at(row, col, text)",
            description: "Write text at a specific position (0-indexed)",
            returns: "None",
          },
          {
            name: "clear_line",
            signature: "clear_line(row)",
            description: "Clear a specific line",
            returns: "None",
          },
          {
            name: "add_row",
            signature: "add_row(panel)",
            description: "Add a child panel as a vertical row (top to bottom)",
            returns: "None",
          },
          {
            name: "add_column",
            signature: "add_column(panel)",
            description:
              "Add a child panel as a horizontal column (left to right)",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.similarity",
    description: "Text similarity utilities including fuzzy search and MinHash",
    functions: [
      {
        name: "search",
        signature: "search(query, items, max_results=5, threshold=0.5, key='name')",
        description: "Search for fuzzy matches in a list of items",
        returns: "list - List of match dicts with id, name, score",
      },
      {
        name: "best",
        signature: "best(query, items, entity_type='item', key='name', threshold=0.5)",
        description: "Find best match with error formatting",
        returns: "dict - {found: bool, id: int, name: str, score: float, error: str}",
      },
      {
        name: "score",
        signature: "score(s1, s2)",
        description: "Calculate similarity score between two strings (0.0 to 1.0)",
        returns: "float - Similarity score",
      },
      {
        name: "tokenize",
        signature: "tokenize(text)",
        description: "Split text into lowercase alphanumeric tokens",
        returns: "list - Token list",
      },
      {
        name: "minhash",
        signature: "minhash(text, num_hashes=64)",
        description: "Compute a MinHash signature for text",
        returns: "list - List of 32-bit hash values",
      },
      {
        name: "minhash_similarity",
        signature: "minhash_similarity(a, b)",
        description: "Compare two MinHash signatures",
        returns: "float - Estimated Jaccard similarity",
      },
    ],
  },
  {
    module: "scriptling.websocket",
    description: "WebSocket client for connecting to WebSocket servers (all environments)",
    functions: [
      {
        name: "connect",
        signature: 'connect(url, timeout=10, headers=None)',
        description: "Connect to a WebSocket server (ws:// or wss://)",
        returns: "WebSocketClientConn - Connection object",
        returnType: "WebSocketClientConn",
      },
      {
        name: "is_text",
        signature: "is_text(message)",
        description: "Check if a received message is a text message",
        returns: "bool - True if text message",
      },
      {
        name: "is_binary",
        signature: "is_binary(message)",
        description: "Check if a received message is a binary message",
        returns: "bool - True if binary message",
      },
    ],
    classes: [
      {
        name: "WebSocketClientConn",
        description: "WebSocket client connection for sending/receiving messages",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description:
              "Send a text message (str or dict, dicts are JSON-encoded)",
            returns: "None on success, or error if send fails",
          },
          {
            name: "send_binary",
            signature: "send_binary(data)",
            description: "Send binary data (list of byte values 0-255)",
            returns: "None on success, or error if send fails",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the server",
            returns: "WebSocketMessage or None - Message, or None if timeout/closed",
          },
          {
            name: "connected",
            signature: "connected()",
            description: "Check if the connection is still open",
            returns: "bool - True if connected",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the WebSocket connection",
            returns: "None",
          },
        ],
        properties: [
          {
            name: "remote_addr",
            description: "Remote address of the connected server",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.threads",
    description: "Threading and concurrency primitives for async operations",
    functions: [
      {
        name: "run",
        signature: "run(func, *args, **kwargs)",
        description: "Run function asynchronously in isolated environment",
        returns: "Promise - Promise with .get() and .wait() methods",
        returnType: "Promise",
      },
    ],
    classes: [
      {
        name: "Atomic",
        description: "Atomic integer for thread-safe counting",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Atomically add and return new value",
            returns: "int - New value after addition",
          },
          {
            name: "get",
            signature: "get()",
            description: "Atomically read value",
            returns: "int - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Atomically set value",
            returns: "None",
          },
        ],
      },
      {
        name: "Shared",
        description: "Thread-safe shared value container",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Thread-safe get",
            returns: "any - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Thread-safe set",
            returns: "None",
          },
        ],
      },
      {
        name: "WaitGroup",
        description: "Wait for collection of goroutines to finish",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Add to counter",
            returns: "None",
          },
          {
            name: "done",
            signature: "done()",
            description: "Decrement counter",
            returns: "None",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Block until counter reaches zero",
            returns: "None",
          },
        ],
      },
      {
        name: "Queue",
        description: "Thread-safe queue for passing data between goroutines",
        methods: [
          {
            name: "put",
            signature: "put(item)",
            description: "Add item (blocks if full)",
            returns: "None",
          },
          {
            name: "get",
            signature: "get()",
            description: "Remove item (blocks if empty)",
            returns: "any - Retrieved item",
          },
          {
            name: "size",
            signature: "size()",
            description: "Get number of items in queue",
            returns: "int - Queue size",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close queue",
            returns: "None",
          },
        ],
      },
      {
        name: "Pool",
        description: "Worker pool for concurrent processing",
        methods: [
          {
            name: "submit",
            signature: "submit(data)",
            description: "Submit data for processing",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Stop pool and wait for completion",
            returns: "None",
          },
        ],
      },
      {
        name: "Promise",
        description: "Result from async goroutine execution",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Wait for and return result",
            returns: "any - Function return value",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Wait for completion",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.messaging",
    description:
      "Messaging library for building cross-platform bots (Telegram, Discord, Slack, Console)",
    functions: [
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description:
          "Build a platform-agnostic button keyboard. Rows is a list of button rows, each row is a list of button dicts",
        returns:
          "Keyboard - List of button rows for use with send_message",
      },
    ],
    classes: [
      {
        name: "MessagingClient",
        description:
          "Messaging client shared by all platforms (telegram, discord, slack, console)",
        methods: [
          {
            name: "command",
            signature: "command(name, help_text, handler)",
            description:
              "Register a command handler. Handler receives context dict",
            returns: "None",
          },
          {
            name: "on_callback",
            signature: 'on_callback(handler, prefix="")',
            description:
              "Register a callback/button handler with optional prefix filter",
            returns: "None",
          },
          {
            name: "on_message",
            signature: "on_message(handler)",
            description: "Register default message handler for non-command messages",
            returns: "None",
          },
          {
            name: "on_file",
            signature: "on_file(handler)",
            description: "Register file attachment handler",
            returns: "None",
          },
          {
            name: "auth",
            signature: "auth(handler)",
            description:
              "Register auth handler. Return True to allow, False to deny",
            returns: "None",
          },
          {
            name: "run",
            signature: "run()",
            description: "Start the bot event loop (blocks until stopped)",
            returns: "None",
          },
          {
            name: "capabilities",
            signature: "capabilities()",
            description: "Get list of platform capability strings",
            returns: "list - Platform capabilities",
          },
          {
            name: "send_message",
            signature: 'send_message(dest, message, parse_mode="", keyboard=None)',
            description:
              "Send a message to a destination. Message can be a string or MessageDict",
            returns: "None",
          },
          {
            name: "send_rich_message",
            signature: "send_rich_message(dest, message)",
            description:
              "Send a rich message with title, body, color, image, url",
            returns: "None",
          },
          {
            name: "edit_message",
            signature: "edit_message(dest, message_id, text)",
            description: "Edit a previously sent message",
            returns: "None",
          },
          {
            name: "delete_message",
            signature: "delete_message(dest, message_id)",
            description: "Delete a message",
            returns: "None",
          },
          {
            name: "send_file",
            signature: 'send_file(dest, source, filename="", caption="", base64=False)',
            description:
              "Send a file. Source can be file path, URL, or base64 data",
            returns: "None",
          },
          {
            name: "typing",
            signature: "typing(dest)",
            description: "Send typing indicator to a destination",
            returns: "None",
          },
          {
            name: "answer_callback",
            signature: 'answer_callback(id, text="", token="")',
            description: "Acknowledge a button press",
            returns: "None",
          },
          {
            name: "download",
            signature: "download(ref)",
            description: "Download a file by ID or URL",
            returns: "str - Base64-encoded file data",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.messaging.telegram",
    description: "Telegram bot client for the Telegram messaging platform",
    functions: [
      {
        name: "client",
        signature: "client(token, *, allowed_users=None)",
        description:
          "Create a Telegram bot client using a token from @BotFather",
        returns: "MessagingClient - Client instance",
        returnType: "MessagingClient",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Telegram keyboard",
        returns: "Keyboard - Button keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.discord",
    description: "Discord bot client for the Discord messaging platform",
    functions: [
      {
        name: "client",
        signature: "client(token, *, allowed_users=None)",
        description:
          "Create a Discord bot client using a bot token from Developer Portal",
        returns: "MessagingClient - Client instance",
        returnType: "MessagingClient",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Discord keyboard",
        returns: "Keyboard - Button keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.slack",
    description:
      "Slack bot client using Socket Mode for the Slack messaging platform",
    functions: [
      {
        name: "client",
        signature: "client(bot_token, app_token, *, allowed_users=None)",
        description:
          "Create a Slack bot client. bot_token is xoxb-..., app_token is xapp-...",
        returns: "MessagingClient - Client instance",
        returnType: "MessagingClient",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Slack keyboard",
        returns: "Keyboard - Button keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.console",
    description:
      "Local TUI console bot for testing messaging handlers without network",
    functions: [
      {
        name: "client",
        signature: "client()",
        description:
          "Create a console bot client for testing handlers locally",
        returns: "MessagingClient - Client instance",
        returnType: "MessagingClient",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a console keyboard",
        returns: "Keyboard - Button keyboard",
      },
    ],
  },

  {
    module: "scriptling.ai.agent",
    description:
      "Agentic AI loop with automatic tool execution (all environments)",
    classes: [
      {
        name: "Message",
        description: "Conversation message with content, role, and tool calls",
        properties: [
          {
            name: "content",
            description: "Message text content",
          },
          {
            name: "role",
            description: "Message role (user, assistant, system, tool)",
          },
          {
            name: "tool_calls",
            description: "List of tool call dicts if present",
          },
        ],
      },
      {
        name: "Agent",
        description: "Agentic AI loop that automatically executes tools",
        methods: [
          {
            name: "trigger",
            signature: "trigger(message, *, max_iterations=1)",
            description:
              "Start agentic loop with a user message (str or dict). Returns final Message",
            returns: "Message - Response message with content and tool_calls",
          },
          {
            name: "get_messages",
            signature: "get_messages()",
            description: "Get the current conversation messages",
            returns: "list[dict] - Conversation history",
          },
          {
            name: "set_messages",
            signature: "set_messages(messages)",
            description: "Set the conversation messages",
            returns: "None",
          },
          {
            name: "interact",
            signature: "interact(max_iterations=25)",
            description:
              "Start an interactive terminal session using the TUI console",
            returns: "None",
          },
        ],
        properties: [
          {
            name: "client",
            description: "OpenAIClient instance",
          },
          {
            name: "tools",
            description: "Optional ToolRegistry",
          },
          {
            name: "system_prompt",
            description: "System prompt string",
          },
          {
            name: "model",
            description: "Model name string",
          },
          {
            name: "messages",
            description: "Conversation history list",
          },
          {
            name: "memory",
            description: "Optional MemoryStore for persistent memory",
          },
          {
            name: "max_tokens",
            description: "Maximum token budget (default 32000)",
          },
          {
            name: "compaction_threshold",
            description: "Percentage of max_tokens for auto-compaction (default 80)",
          },
        ],
      },
    ],
    functions: [
      {
        name: "Agent",
        signature:
          "Agent(client, *, tools=None, system_prompt='', model='', memory=None, max_tokens=32000, compaction_threshold=80)",
        description:
          "Create an agentic AI loop with automatic tool execution and optional memory",
        returns: "Agent - Agent instance",
        returnType: "Agent",
      },
    ],
  },
  {
    module: "scriptling.ai.memory",
    description:
      "Long-term memory storage for AI agents backed by a key-value store (all environments)",
    constants: [
      {
        name: "TYPE_FACT",
        description: 'Memory type constant for facts ("fact")',
        type: "string",
      },
      {
        name: "TYPE_PREFERENCE",
        description: 'Memory type constant for preferences ("preference")',
        type: "string",
      },
      {
        name: "TYPE_EVENT",
        description: 'Memory type constant for events ("event")',
        type: "string",
      },
      {
        name: "TYPE_NOTE",
        description: 'Memory type constant for notes ("note")',
        type: "string",
      },
    ],
    functions: [
      {
        name: "new",
        signature: "new(kv_store, ai_client=None, model='')",
        description:
          "Create a memory store backed by a kv store. Optionally provide an AI client for LLM-based compaction",
        returns: "MemoryStore - Memory store instance",
        returnType: "MemoryStore",
      },
    ],
    classes: [
      {
        name: "MemoryStore",
        description:
          "Memory store for storing and recalling memories with semantic similarity search",
        methods: [
          {
            name: "remember",
            signature: 'remember(content, type="note", importance=0.5)',
            description:
              "Store a memory for later recall. Content should be a single concise sentence",
            returns: "dict - Stored memory with id, content, type, importance, created_at, accessed_at",
          },
          {
            name: "recall",
            signature: 'recall(query="", limit=10, type="")',
            description:
              "Search memories by keyword and semantic similarity. Empty query with no type triggers context load mode",
            returns:
              "list - List of matching memory dicts ranked by relevance",
          },
          {
            name: "forget",
            signature: "forget(id)",
            description: "Remove a memory by ID",
            returns: "bool - True if a memory was removed",
          },
          {
            name: "count",
            signature: "count()",
            description: "Return the total number of stored memories",
            returns: "int - Count of all stored memories",
          },
          {
            name: "compact",
            signature: "compact()",
            description:
              "Manually trigger compaction (prune + deduplicate)",
            returns: 'dict - {"removed": int, "remaining": int}',
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.ai.tools",
    description: "AI tools registry for building tool schemas (all environments)",
    functions: [
      {
        name: "Registry",
        signature: "Registry()",
        description: "Create a new tool registry for building AI tool schemas",
        returns: "Registry - Tool registry instance",
        returnType: "Registry",
      },
    ],
  },
  {
    module: "scriptling.runtime",
    description:
      "Runtime utilities for background function execution (Local/Remote environments)",
    functions: [
      {
        name: "background",
        signature: "background(name, handler, *args, **kwargs)",
        description:
          "Run a named handler function in the background. Handler is a 'library.function' string",
        returns: "Promise or None - Promise for tracking completion, or None on error",
        returnType: "Promise",
      },
    ],
    classes: [
      {
        name: "Promise",
        description: "Result from background task execution",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Wait for and return result",
            returns: "any - Function return value",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Wait for completion",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.kv",
    description:
      "Key-value store for runtime state sharing (Local/Remote environments)",
    constants: [
      {
        name: "default",
        description: "Default system-wide KV store instance",
        type: "Storage",
      },
    ],
    functions: [
      {
        name: "open",
        signature: "open(name)",
        description: "Open or reuse a named KV store",
        returns: "Storage - Storage instance",
        returnType: "Storage",
      },
    ],
    classes: [
      {
        name: "Storage",
        description: "Key-value storage with optional TTL support",
        methods: [
          {
            name: "set",
            signature: "set(key, value, ttl=0)",
            description: "Store a value with optional TTL in seconds (0 = no expiry)",
            returns: "None",
          },
          {
            name: "get",
            signature: "get(key, default=None)",
            description: "Get a value from the store",
            returns: "any - Stored value or default",
          },
          {
            name: "incr",
            signature: "incr(key, delta=1)",
            description: "Atomically increment an integer value",
            returns: "int - New value after increment",
          },
          {
            name: "delete",
            signature: "delete(key)",
            description: "Delete a key from the store",
            returns: "None",
          },
          {
            name: "exists",
            signature: "exists(key)",
            description: "Check if a key exists in the store",
            returns: "bool - True if key exists",
          },
          {
            name: "ttl",
            signature: "ttl(key)",
            description: "Get remaining TTL for a key",
            returns: "int - Remaining seconds, or -1 if no TTL, -2 if not found",
          },
          {
            name: "keys",
            signature: 'keys(pattern="*")',
            description: "Get keys matching a glob pattern",
            returns: "list - List of matching key strings",
          },
          {
            name: "clear",
            signature: "clear()",
            description: "Remove all keys from the store",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Release the store reference",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.sync",
    description:
      "Concurrency primitives for thread synchronization (Local/Remote environments)",
    classes: [
      {
        name: "Mutex",
        description: "Mutual exclusion lock",
        methods: [
          {
            name: "lock",
            signature: "lock()",
            description: "Acquire the lock",
            returns: "None",
          },
          {
            name: "unlock",
            signature: "unlock()",
            description: "Release the lock",
            returns: "None",
          },
        ],
      },
      {
        name: "WaitGroup",
        description: "Wait for a collection of goroutines to finish",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Add to the counter",
            returns: "None",
          },
          {
            name: "done",
            signature: "done()",
            description: "Decrement the counter",
            returns: "None",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Block until counter reaches zero",
            returns: "None",
          },
        ],
      },
      {
        name: "Queue",
        description: "Thread-safe queue for passing data between goroutines",
        methods: [
          {
            name: "put",
            signature: "put(item)",
            description: "Add item to the queue (blocks if full)",
            returns: "None",
          },
          {
            name: "get",
            signature: "get()",
            description: "Remove and return an item (blocks if empty)",
            returns: "any - Retrieved item",
          },
          {
            name: "size",
            signature: "size()",
            description: "Get number of items in the queue",
            returns: "int - Queue size",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the queue",
            returns: "None",
          },
        ],
      },
      {
        name: "Atomic",
        description: "Atomic integer for thread-safe counting",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Atomically add delta and return new value",
            returns: "int - New value after addition",
          },
          {
            name: "get",
            signature: "get()",
            description: "Atomically read the current value",
            returns: "int - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Atomically set the value",
            returns: "None",
          },
        ],
      },
      {
        name: "Shared",
        description: "Thread-safe shared value container",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Thread-safe read of the current value",
            returns: "any - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Thread-safe set of the value",
            returns: "None",
          },
          {
            name: "update",
            signature: "update(fn)",
            description: "Atomically read-modify-write using a function",
            returns: "any - New value after update",
          },
        ],
      },
    ],
    functions: [
      {
        name: "Mutex",
        signature: "Mutex()",
        description: "Create a new mutex lock",
        returns: "Mutex - Mutex instance",
        returnType: "Mutex",
      },
      {
        name: "WaitGroup",
        signature: "WaitGroup(name)",
        description: "Get or create a named wait group",
        returns: "WaitGroup - WaitGroup instance",
        returnType: "WaitGroup",
      },
      {
        name: "Queue",
        signature: "Queue(name, maxsize=0)",
        description: "Get or create a named queue (0 = unbounded)",
        returns: "Queue - Queue instance",
        returnType: "Queue",
      },
      {
        name: "Atomic",
        signature: "Atomic(name, initial=0)",
        description: "Get or create a named atomic counter",
        returns: "Atomic - Atomic instance",
        returnType: "Atomic",
      },
      {
        name: "Shared",
        signature: "Shared(name, initial=None)",
        description: "Get or create a named shared variable",
        returns: "Shared - Shared instance",
        returnType: "Shared",
      },
    ],
  },
  {
    module: "scriptling.runtime.sandbox",
    description:
      "Isolated script execution environments (Local/Remote environments)",
    classes: [
      {
        name: "Sandbox",
        description: "An isolated script execution context",
        methods: [
          {
            name: "set",
            signature: "set(name, value)",
            description: "Set a variable in the sandbox",
            returns: "None",
          },
          {
            name: "get",
            signature: "get(name)",
            description: "Get a variable from the sandbox",
            returns: "any - Variable value or None",
          },
          {
            name: "exec",
            signature: "exec(code)",
            description: "Execute script code in the sandbox",
            returns: "None",
          },
          {
            name: "exec_file",
            signature: "exec_file(path)",
            description: "Load and execute a script file in the sandbox",
            returns: "None",
          },
          {
            name: "exit_code",
            signature: "exit_code()",
            description: "Get the exit code from the last execution (0 = success)",
            returns: "int - Exit code",
          },
        ],
      },
    ],
    functions: [
      {
        name: "create",
        signature: "create(capture_output=False)",
        description:
          "Create a new isolated sandbox environment with its own variable scope",
        returns: "Sandbox - Sandbox instance",
        returnType: "Sandbox",
      },
    ],
  },
  {
    module: "scriptling.runtime.http",
    description:
      "HTTP server route registration and response helpers (Local/Remote environments)",
    classes: [
      {
        name: "Request",
        description: "HTTP request object passed to handlers",
        methods: [
          {
            name: "json",
            signature: "json()",
            description: "Parse request body as JSON",
            returns: "dict/list - Parsed JSON or None if body is empty",
          },
        ],
        properties: [
          {
            name: "method",
            description: "HTTP method (GET, POST, etc.)",
          },
          {
            name: "path",
            description: "Request path",
          },
          {
            name: "body",
            description: "Raw request body",
          },
          {
            name: "headers",
            description: "Request headers as dict",
          },
          {
            name: "query",
            description: "Query parameters as dict",
          },
        ],
      },
      {
        name: "WebSocketClient",
        description: "WebSocket connection for route handlers",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description: "Send a text message to the client",
            returns: "None",
          },
          {
            name: "send_binary",
            signature: "send_binary(data)",
            description: "Send binary data to the client",
            returns: "None",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the client",
            returns: "any - Message or None if timeout/closed",
          },
          {
            name: "connected",
            signature: "connected()",
            description: "Check if the connection is still open",
            returns: "bool - True if connected",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the WebSocket connection",
            returns: "None",
          },
        ],
      },
    ],
    functions: [
      {
        name: "get",
        signature: "get(path, handler)",
        description: 'Register a GET route (handler is "library.function" string)',
        returns: "None",
      },
      {
        name: "post",
        signature: "post(path, handler)",
        description: 'Register a POST route (handler is "library.function" string)',
        returns: "None",
      },
      {
        name: "put",
        signature: "put(path, handler)",
        description: 'Register a PUT route (handler is "library.function" string)',
        returns: "None",
      },
      {
        name: "delete",
        signature: "delete(path, handler)",
        description: 'Register a DELETE route (handler is "library.function" string)',
        returns: "None",
      },
      {
        name: "websocket",
        signature: "websocket(path, handler)",
        description:
          "Register a WebSocket route. Handler receives a WebSocketClientConn",
        returns: "None",
      },
      {
        name: "route",
        signature: 'route(path, handler, methods=["GET", "POST", "PUT", "DELETE"])',
        description: "Register a route for multiple HTTP methods",
        returns: "None",
      },
      {
        name: "middleware",
        signature: "middleware(handler)",
        description: "Register middleware for all routes",
        returns: "None",
      },
      {
        name: "static",
        signature: "static(path, directory)",
        description: "Register a static file serving route",
        returns: "None",
      },
      {
        name: "json",
        signature: "json(status_code, data)",
        description: "Create a JSON response",
        returns: "dict - Response object",
      },
      {
        name: "html",
        signature: "html(status_code, content)",
        description: "Create an HTML response",
        returns: "dict - Response object",
      },
      {
        name: "text",
        signature: "text(status_code, content)",
        description: "Create a plain text response",
        returns: "dict - Response object",
      },
      {
        name: "redirect",
        signature: "redirect(location, status=302)",
        description: "Create a redirect response",
        returns: "dict - Response object",
      },
      {
        name: "parse_query",
        signature: "parse_query(query_string)",
        description: "Parse a URL query string into a dict",
        returns: "dict - Parsed key-value pairs",
      },
    ],
  },
  {
    module: "toml",
    description: "TOML parsing and manipulation (all environments)",
    functions: [
      {
        name: "loads",
        signature: "loads(s)",
        description: "Parse TOML string to dict",
        returns: "dict - Parsed TOML data",
      },
      {
        name: "dumps",
        signature: "dumps(obj)",
        description: "Serialize dict to TOML string",
        returns: "str - TOML formatted string",
      },
    ],
  },
  {
    module: "yaml",
    description: "YAML parsing and manipulation (all environments)",
    functions: [
      {
        name: "safe_load",
        signature: "safe_load(s)",
        description: "Parse YAML string to object (safe, no arbitrary code)",
        returns: "object - Parsed YAML data",
      },
      {
        name: "dump",
        signature: "dump(obj)",
        description: "Serialize object to YAML string",
        returns: "str - YAML formatted string",
      },
      {
        name: "load",
        signature: "load(s)",
        description: "Parse YAML string to object",
        returns: "object - Parsed YAML data",
      },
    ],
  },

  // ============================================================================
  // STANDARD LIBRARY MODULES
  // ============================================================================
  {
    module: "json",
    description: "JSON encoding/decoding",
    functions: [
      {
        name: "dumps",
        signature: "dumps(obj)",
        description: "Serialize object to JSON string",
        returns: "str",
      },
      {
        name: "loads",
        signature: "loads(s)",
        description: "Deserialize JSON string to object",
        returns: "object",
      },
    ],
  },
  {
    module: "time",
    description: "Time functions and sleeping",
    functions: [
      {
        name: "sleep",
        signature: "sleep(seconds)",
        description: "Suspend execution for given seconds",
        returns: "None",
      },
      {
        name: "time",
        signature: "time()",
        description: "Current time in seconds since epoch",
        returns: "float",
      },
      {
        name: "perf_counter",
        signature: "perf_counter()",
        description: "Performance counter in seconds (highest resolution)",
        returns: "float",
      },
      {
        name: "localtime",
        signature: "localtime([timestamp_or_datetime])",
        description: "Convert to local time tuple",
        returns: "tuple - Time tuple",
      },
      {
        name: "gmtime",
        signature: "gmtime([timestamp_or_datetime])",
        description: "Convert to UTC time tuple",
        returns: "tuple - Time tuple",
      },
      {
        name: "mktime",
        signature: "mktime(tuple)",
        description: "Convert time tuple to timestamp",
        returns: "float - Unix timestamp",
      },
      {
        name: "strftime",
        signature: "strftime(format[, tuple])",
        description: "Format time as string",
        returns: "str - Formatted time string",
      },
      {
        name: "strptime",
        signature: "strptime(string, format)",
        description: "Parse time from string",
        returns: "datetime - Parsed datetime",
      },
      {
        name: "asctime",
        signature: "asctime([tuple])",
        description: "Convert time tuple to string",
        returns: "str - Time string",
      },
      {
        name: "ctime",
        signature: "ctime([timestamp])",
        description: "Convert timestamp to string",
        returns: "str - Time string",
      },
    ],
  },
  {
    module: "datetime",
    description: "Date and time manipulation",
    functions: [
      {
        name: "datetime",
        signature:
          "datetime(year, month, day, hour=0, minute=0, second=0, microsecond=0)",
        description: "Create datetime instance",
        returns: "datetime - Datetime instance",
        returnType: "datetime",
      },
      {
        name: "date",
        signature: "date(year, month, day)",
        description: "Create date instance",
        returns: "date - Date instance",
        returnType: "date",
      },
      {
        name: "timedelta",
        signature: "timedelta(**kwargs)",
        description:
          "Create timedelta with days, seconds, microseconds, milliseconds, minutes, hours, weeks",
        returns: "timedelta - Timedelta instance",
        returnType: "timedelta",
      },
    ],
    classes: [
      {
        name: "datetime",
        description: "Date and time object",
        methods: [
          {
            name: "strftime",
            signature: "strftime(format)",
            description: "Format datetime as string",
            returns: "str - Formatted string",
          },
          {
            name: "timestamp",
            signature: "timestamp()",
            description: "Return POSIX timestamp",
            returns: "float - Unix timestamp",
          },
          {
            name: "year",
            signature: "year()",
            description: "Get year component",
            returns: "int - Year",
          },
          {
            name: "month",
            signature: "month()",
            description: "Get month component",
            returns: "int - Month (1-12)",
          },
          {
            name: "day",
            signature: "day()",
            description: "Get day component",
            returns: "int - Day",
          },
          {
            name: "hour",
            signature: "hour()",
            description: "Get hour component",
            returns: "int - Hour (0-23)",
          },
          {
            name: "minute",
            signature: "minute()",
            description: "Get minute component",
            returns: "int - Minute (0-59)",
          },
          {
            name: "second",
            signature: "second()",
            description: "Get second component",
            returns: "int - Second (0-59)",
          },
          {
            name: "weekday",
            signature: "weekday()",
            description: "Day of week (Monday=0, Sunday=6)",
            returns: "int - Weekday",
          },
          {
            name: "isoformat",
            signature: "isoformat()",
            description: "Return ISO 8601 formatted string",
            returns: "str - ISO format string",
          },
          {
            name: "replace",
            signature: "replace(**kwargs)",
            description: "Return datetime with replaced fields",
            returns: "datetime - New datetime",
          },
        ],
      },
      {
        name: "date",
        description: "Date object (year, month, day)",
        methods: [
          {
            name: "strftime",
            signature: "strftime(format)",
            description: "Format date as string",
            returns: "str - Formatted string",
          },
          {
            name: "year",
            signature: "year()",
            description: "Get year component",
            returns: "int - Year",
          },
          {
            name: "month",
            signature: "month()",
            description: "Get month component",
            returns: "int - Month (1-12)",
          },
          {
            name: "day",
            signature: "day()",
            description: "Get day component",
            returns: "int - Day",
          },
        ],
      },
    ],
    classAttributes: [
      {
        class: "datetime",
        name: "now",
        description: "Current local datetime",
        signature: "datetime.now()",
      },
      {
        class: "datetime",
        name: "utcnow",
        description: "Current UTC datetime",
        signature: "datetime.utcnow()",
      },
      {
        class: "datetime",
        name: "strptime",
        description: "Parse string to datetime",
        signature: "datetime.strptime(date_string, format)",
      },
      {
        class: "datetime",
        name: "fromtimestamp",
        description: "Create datetime from Unix timestamp",
        signature: "datetime.fromtimestamp(timestamp)",
      },
    ],
  },
  {
    module: "scriptling.glob",
    description: "Unix shell-style wildcards for file path matching (Local/Remote environments)",
    functions: [
      {
        name: "glob",
        signature: "glob(pattern[, root_dir='.''])",
        description:
          "Find all pathnames matching a shell-style pattern. Supports *, ?, [seq] wildcards",
        returns: "list - List of matching paths",
      },
      {
        name: "iglob",
        signature: "iglob(pattern[, root_dir='.''])",
        description: "Memory-efficient iterator version of glob",
        returns: "iterator - Iterator of matching paths",
      },
      {
        name: "escape",
        signature: "escape(pattern)",
        description:
          "Escape special characters (*, ?, [, ]) to treat them as literals",
        returns: "str - Escaped pattern string",
      },
    ],
  },
  {
    module: "html.parser",
    description: "HTML parsing for simple structured data extraction",
    functions: [],
    classes: [
      {
        name: "HTMLParser",
        description:
          "Simple HTML parser with handler methods for different elements",
        methods: [
          {
            name: "__init__",
            signature: "__init__(*, convert_charrefs=True)",
            description: "Initialize parser",
            returns: "HTMLParser",
          },
          {
            name: "feed",
            signature: "feed(data)",
            description: "Feed HTML data to parser",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Force processing of buffered data",
            returns: "None",
          },
          {
            name: "reset",
            signature: "reset()",
            description: "Reset parser instance",
            returns: "None",
          },
          {
            name: "getpos",
            signature: "getpos()",
            description: "Return current (line, offset) position",
            returns: "tuple - (line, offset)",
          },
          {
            name: "handle_starttag",
            signature: "handle_starttag(tag, attrs)",
            description: "Override to handle start tags",
            returns: "None",
          },
          {
            name: "handle_endtag",
            signature: "handle_endtag(tag)",
            description: "Override to handle end tags",
            returns: "None",
          },
          {
            name: "handle_data",
            signature: "handle_data(data)",
            description: "Override to handle text content",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "logging",
    description: "Logging facility for event tracking and debugging",
    functions: [
      {
        name: "getLogger",
        signature: "getLogger([name='scriptling'])",
        description: "Get logger instance",
        returns: "Logger - Logger instance",
        returnType: "Logger",
      },
      {
        name: "debug",
        signature: "debug(msg)",
        description: "Log debug message",
        returns: "None",
      },
      {
        name: "info",
        signature: "info(msg)",
        description: "Log info message",
        returns: "None",
      },
      {
        name: "warning",
        signature: "warning(msg)",
        description: "Log warning message",
        returns: "None",
      },
      {
        name: "warn",
        signature: "warn(msg)",
        description: "Log warning message (alias for warning)",
        returns: "None",
      },
      {
        name: "error",
        signature: "error(msg)",
        description: "Log error message",
        returns: "None",
      },
      {
        name: "critical",
        signature: "critical(msg)",
        description: "Log critical message",
        returns: "None",
      },
    ],
    classes: [
      {
        name: "Logger",
        description: "Logger instance for logging messages",
        methods: [
          {
            name: "debug",
            signature: "debug(msg)",
            description: "Log debug message",
            returns: "None",
          },
          {
            name: "info",
            signature: "info(msg)",
            description: "Log info message",
            returns: "None",
          },
          {
            name: "warning",
            signature: "warning(msg)",
            description: "Log warning message",
            returns: "None",
          },
          {
            name: "warn",
            signature: "warn(msg)",
            description: "Log warning message",
            returns: "None",
          },
          {
            name: "error",
            signature: "error(msg)",
            description: "Log error message",
            returns: "None",
          },
          {
            name: "critical",
            signature: "critical(msg)",
            description: "Log critical message",
            returns: "None",
          },
        ],
      },
    ],
    constants: [
      {
        name: "DEBUG",
        value: "10",
        description: "Debug log level",
      },
      {
        name: "INFO",
        value: "20",
        description: "Info log level",
      },
      {
        name: "WARNING",
        value: "30",
        description: "Warning log level",
      },
      {
        name: "ERROR",
        value: "40",
        description: "Error log level",
      },
      {
        name: "CRITICAL",
        value: "50",
        description: "Critical log level",
      },
    ],
  },
  {
    module: "math",
    description: "Mathematical functions and constants",
    functions: [
      {
        name: "sqrt",
        signature: "sqrt(x)",
        description: "Square root of x",
        returns: "float",
      },
      {
        name: "pow",
        signature: "pow(base, exp)",
        description: "Base raised to the power of exp",
        returns: "float",
      },
      {
        name: "fabs",
        signature: "fabs(x)",
        description: "Absolute value as float",
        returns: "float",
      },
      {
        name: "floor",
        signature: "floor(x)",
        description: "Floor of x",
        returns: "int",
      },
      {
        name: "ceil",
        signature: "ceil(x)",
        description: "Ceiling of x",
        returns: "int",
      },
      {
        name: "sin",
        signature: "sin(x)",
        description: "Sine of x (radians)",
        returns: "float",
      },
      {
        name: "cos",
        signature: "cos(x)",
        description: "Cosine of x (radians)",
        returns: "float",
      },
      {
        name: "tan",
        signature: "tan(x)",
        description: "Tangent of x (radians)",
        returns: "float",
      },
      {
        name: "log",
        signature: "log(x)",
        description: "Natural logarithm",
        returns: "float",
      },
      {
        name: "exp",
        signature: "exp(x)",
        description: "Exponential function e^x",
        returns: "float",
      },
      {
        name: "degrees",
        signature: "degrees(x)",
        description: "Convert radians to degrees",
        returns: "float",
      },
      {
        name: "radians",
        signature: "radians(x)",
        description: "Convert degrees to radians",
        returns: "float",
      },
      {
        name: "fmod",
        signature: "fmod(x, y)",
        description: "Floating-point remainder",
        returns: "float",
      },
      {
        name: "gcd",
        signature: "gcd(a, b)",
        description: "Greatest common divisor",
        returns: "int",
      },
      {
        name: "factorial",
        signature: "factorial(n)",
        description: "Factorial (n <= 20)",
        returns: "int",
      },
      {
        name: "isnan",
        signature: "isnan(x)",
        description: "Check if x is NaN",
        returns: "bool",
      },
      {
        name: "isinf",
        signature: "isinf(x)",
        description: "Check if x is infinity",
        returns: "bool",
      },
      {
        name: "isfinite",
        signature: "isfinite(x)",
        description: "Check if x is finite",
        returns: "bool",
      },
      {
        name: "copysign",
        signature: "copysign(x, y)",
        description: "Copy sign from y to x",
        returns: "float",
      },
      {
        name: "trunc",
        signature: "trunc(x)",
        description: "Truncate toward zero",
        returns: "int",
      },
      {
        name: "log10",
        signature: "log10(x)",
        description: "Base-10 logarithm",
        returns: "float",
      },
      {
        name: "log2",
        signature: "log2(x)",
        description: "Base-2 logarithm",
        returns: "float",
      },
      {
        name: "hypot",
        signature: "hypot(x, y)",
        description: "Euclidean distance sqrt(x*x + y*y)",
        returns: "float",
      },
      {
        name: "asin",
        signature: "asin(x)",
        description: "Arc sine",
        returns: "float",
      },
      {
        name: "acos",
        signature: "acos(x)",
        description: "Arc cosine",
        returns: "float",
      },
      {
        name: "atan",
        signature: "atan(x)",
        description: "Arc tangent",
        returns: "float",
      },
      {
        name: "atan2",
        signature: "atan2(y, x)",
        description: "Arc tangent of y/x",
        returns: "float",
      },
    ],
    constants: [
      {
        name: "pi",
        value: "3.14159...",
        description: "Pi constant",
      },
      {
        name: "e",
        value: "2.71828...",
        description: "Euler's number",
      },
      {
        name: "inf",
        value: "Infinity",
        description: "Positive infinity",
      },
      {
        name: "nan",
        value: "NaN",
        description: "Not a number",
      },
    ],
  },
  {
    module: "random",
    description: "Random number generation",
    functions: [
      {
        name: "seed",
        signature: "seed([a])",
        description: "Initialize random number generator",
        returns: "None",
      },
      {
        name: "random",
        signature: "random()",
        description: "Random float in [0.0, 1.0)",
        returns: "float",
      },
      {
        name: "randint",
        signature: "randint(min, max)",
        description: "Random integer in range [min, max]",
        returns: "int",
      },
      {
        name: "choice",
        signature: "choice(seq)",
        description: "Random element from sequence",
        returns: "any",
      },
      {
        name: "shuffle",
        signature: "shuffle(list)",
        description: "Shuffle list in place",
        returns: "None",
      },
      {
        name: "uniform",
        signature: "uniform(a, b)",
        description: "Random float in [a, b]",
        returns: "float",
      },
      {
        name: "sample",
        signature: "sample(population, k)",
        description: "k unique random elements",
        returns: "list",
      },
      {
        name: "randrange",
        signature: "randrange(stop) or randrange(start, stop[, step])",
        description: "Random from range",
        returns: "int",
      },
      {
        name: "gauss",
        signature: "gauss(mu, sigma)",
        description: "Gaussian distribution",
        returns: "float",
      },
      {
        name: "expovariate",
        signature: "expovariate(lambd)",
        description: "Exponential distribution",
        returns: "float",
      },
    ],
  },
  {
    module: "hashlib",
    description: "Cryptographic hashing",
    functions: [
      {
        name: "sha256",
        signature: "sha256(string)",
        description: "Compute SHA-256 hash",
        returns: "str - Hex digest",
      },
      {
        name: "sha1",
        signature: "sha1(string)",
        description: "Compute SHA-1 hash",
        returns: "str - Hex digest",
      },
      {
        name: "md5",
        signature: "md5(string)",
        description: "Compute MD5 hash",
        returns: "str - Hex digest",
      },
    ],
  },
  {
    module: "base64",
    description: "Base64 encoding/decoding",
    functions: [
      {
        name: "b64encode",
        signature: "b64encode(s)",
        description: "Encode to Base64",
        returns: "str",
      },
      {
        name: "b64decode",
        signature: "b64decode(s)",
        description: "Decode from Base64",
        returns: "str",
      },
    ],
  },
  {
    module: "uuid",
    description: "UUID generation",
    functions: [
      {
        name: "uuid1",
        signature: "uuid1()",
        description: "Generate UUID version 1 (time-based)",
        returns: "str - UUID string",
      },
      {
        name: "uuid4",
        signature: "uuid4()",
        description: "Generate UUID version 4 (random)",
        returns: "str - UUID string",
      },
      {
        name: "uuid7",
        signature: "uuid7()",
        description: "Generate UUID version 7 (timestamp-based, sortable)",
        returns: "str - UUID string",
      },
    ],
  },
  {
    module: "collections",
    description: "Specialized container datatypes",
    functions: [
      {
        name: "Counter",
        signature: "Counter([iterable])",
        description: "Create a Counter for counting hashable objects",
        returns: "Counter - Counter instance",
        returnType: "Counter",
      },
      {
        name: "namedtuple",
        signature: "namedtuple(typename, field_names)",
        description: "Create named tuple class with attribute access",
        returns: "type - Named tuple class",
      },
      {
        name: "OrderedDict",
        signature: "OrderedDict([items])",
        description: "Create ordered dictionary",
        returns: "dict - Ordered dictionary",
      },
      {
        name: "deque",
        signature: "deque([iterable, maxlen])",
        description: "Create double-ended queue",
        returns: "deque - Deque instance",
        returnType: "deque",
      },
      {
        name: "ChainMap",
        signature: "ChainMap(*maps)",
        description: "Group multiple dicts for single lookup",
        returns: "ChainMap - ChainMap instance",
        returnType: "ChainMap",
      },
      {
        name: "DefaultDict",
        signature: "DefaultDict(default_factory)",
        description: "Create dictionary with default factory behavior",
        returns: "DefaultDict - DefaultDict instance",
        returnType: "DefaultDict",
      },
    ],
    classes: [
      {
        name: "Counter",
        description: "Dictionary subclass for counting hashable objects",
        methods: [
          {
            name: "__getitem__",
            signature: "__getitem__(key)",
            description: "Get count for key (supports c[key] syntax)",
            returns: "int - Count",
          },
          {
            name: "most_common",
            signature: "most_common([n])",
            description: "Return n most common elements",
            returns: "list - List of (element, count) tuples",
          },
          {
            name: "elements",
            signature: "elements()",
            description: "Iterator over elements repeating by count",
            returns: "iterator",
          },
        ],
      },
      {
        name: "DefaultDict",
        description: "Dictionary with default factory behavior",
        methods: [
          {
            name: "__getitem__",
            signature: "__getitem__(key)",
            description: "Get value with default creation",
            returns: "any - Value",
          },
          {
            name: "__setitem__",
            signature: "__setitem__(key, value)",
            description: "Set value",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "functools",
    description: "Higher-order functions and operations on callable objects",
    functions: [
      {
        name: "reduce",
        signature: "reduce(function, iterable[, initializer])",
        description: "Apply function cumulatively to items",
        returns: "any - Reduced value",
      },
      {
        name: "partial",
        signature: "partial(func, *args, **kwargs)",
        description: "Create partial function with pre-filled arguments",
        returns: "callable - Partial function",
      },
    ],
  },
  {
    module: "html",
    description: "HTML helper functions",
    functions: [
      {
        name: "escape",
        signature: "escape(s)",
        description: "Escape HTML special characters",
        returns: "str - Escaped string",
      },
      {
        name: "unescape",
        signature: "unescape(s)",
        description: "Unescape HTML entities",
        returns: "str - Unescaped string",
      },
    ],
  },
  {
    module: "itertools",
    description: "Functions creating iterators for efficient looping",
    functions: [
      {
        name: "chain",
        signature: "chain(*iterables)",
        description: "Chain multiple iterables together",
        returns: "iterator",
      },
      {
        name: "repeat",
        signature: "repeat(elem, n)",
        description: "Repeat element n times",
        returns: "iterator",
      },
      {
        name: "cycle",
        signature: "cycle(iterable, n)",
        description: "Cycle through iterable n times",
        returns: "iterator",
      },
      {
        name: "count",
        signature: "count(start, stop[, step])",
        description: "Generate sequence of numbers",
        returns: "iterator",
      },
      {
        name: "islice",
        signature:
          "islice(iterable, stop) or islice(iterable, start, stop[, step])",
        description: "Slice iterable",
        returns: "iterator",
      },
      {
        name: "takewhile",
        signature: "takewhile(predicate, iterable)",
        description: "Take elements while predicate is true",
        returns: "iterator",
      },
      {
        name: "dropwhile",
        signature: "dropwhile(predicate, iterable)",
        description: "Drop elements while predicate is true",
        returns: "iterator",
      },
      {
        name: "zip_longest",
        signature: "zip_longest(*iterables, fillvalue=None)",
        description: "Zip iterables filling shorter ones",
        returns: "iterator",
      },
      {
        name: "product",
        signature: "product(*iterables)",
        description: "Cartesian product",
        returns: "iterator",
      },
      {
        name: "permutations",
        signature: "permutations(iterable[, r])",
        description: "Generate r-length permutations",
        returns: "iterator",
      },
      {
        name: "combinations",
        signature: "combinations(iterable, r)",
        description: "Generate r-length combinations (without repetition)",
        returns: "iterator",
      },
      {
        name: "combinations_with_replacement",
        signature: "combinations_with_replacement(iterable, r)",
        description: "Generate combinations with repetition",
        returns: "iterator",
      },
      {
        name: "groupby",
        signature: "groupby(iterable[, key])",
        description: "Group consecutive elements",
        returns: "iterator",
      },
      {
        name: "accumulate",
        signature: "accumulate(iterable[, func])",
        description: "Running totals/accumulation",
        returns: "iterator",
      },
      {
        name: "filterfalse",
        signature: "filterfalse(predicate, iterable)",
        description: "Filter elements where predicate is false",
        returns: "iterator",
      },
      {
        name: "starmap",
        signature: "starmap(func, iterable)",
        description: "Apply function to argument tuples",
        returns: "iterator",
      },
      {
        name: "compress",
        signature: "compress(data, selectors)",
        description: "Filter data based on selectors",
        returns: "iterator",
      },
      {
        name: "pairwise",
        signature: "pairwise(iterable)",
        description: "Return successive overlapping pairs",
        returns: "iterator",
      },
      {
        name: "batched",
        signature: "batched(iterable, n)",
        description: "Batch elements into tuples of size n",
        returns: "iterator",
      },
    ],
  },
  {
    module: "platform",
    description: "Platform and system information",
    functions: [
      {
        name: "python_version",
        signature: "python_version()",
        description: "Return Python (Scriptling) version",
        returns: "str - Version string",
      },
      {
        name: "scriptling_version",
        signature: "scriptling_version()",
        description: "Return Scriptling version",
        returns: "str - Version string",
      },
      {
        name: "system",
        signature: "system()",
        description: "Return OS name (Darwin, Linux, Windows, etc.)",
        returns: "str - OS name",
      },
      {
        name: "platform",
        signature: "platform()",
        description: "Return platform string",
        returns: "str - Platform",
      },
      {
        name: "architecture",
        signature: "architecture()",
        description: "Return architecture info",
        returns: "str - Architecture",
      },
      {
        name: "machine",
        signature: "machine()",
        description: "Return machine type",
        returns: "str - Machine type",
      },
      {
        name: "processor",
        signature: "processor()",
        description: "Return processor name",
        returns: "str - Processor",
      },
      {
        name: "node",
        signature: "node()",
        description: "Return hostname",
        returns: "str - Hostname",
      },
      {
        name: "release",
        signature: "release()",
        description: "Return release info",
        returns: "str - Release",
      },
      {
        name: "version",
        signature: "version()",
        description: "Return version info",
        returns: "str - Version",
      },
      {
        name: "uname",
        signature: "uname()",
        description: "Return system info dict",
        returns: "dict - System info",
      },
    ],
  },
  {
    module: "re",
    description: "Regular expression operations",
    functions: [
      {
        name: "match",
        signature: "match(pattern, string, flags=0)",
        description: "Match pattern at start of string",
        returns: "Match - Match object or None",
        returnType: "Match",
      },
      {
        name: "search",
        signature: "search(pattern, string, flags=0)",
        description: "Search for pattern anywhere in string",
        returns: "Match - Match object or None",
        returnType: "Match",
      },
      {
        name: "findall",
        signature: "findall(pattern, string, flags=0)",
        description: "Find all matches",
        returns: "list - List of matches",
      },
      {
        name: "finditer",
        signature: "finditer(pattern, string, flags=0)",
        description: "Find all matches as Match objects",
        returns: "iterator - Match objects",
      },
      {
        name: "sub",
        signature: "sub(pattern, repl, string, count=0, flags=0)",
        description: "Replace matches",
        returns: "str - Modified string",
      },
      {
        name: "split",
        signature: "split(pattern, string, maxsplit=0, flags=0)",
        description: "Split string by pattern",
        returns: "list - Split parts",
      },
      {
        name: "compile",
        signature: "compile(pattern, flags=0)",
        description: "Compile regex pattern",
        returns: "Regex - Compiled regex",
        returnType: "Regex",
      },
      {
        name: "escape",
        signature: "escape(pattern)",
        description: "Escape special regex characters",
        returns: "str - Escaped string",
      },
      {
        name: "fullmatch",
        signature: "fullmatch(pattern, string, flags=0)",
        description: "Match entire string",
        returns: "Match - Match object or None",
        returnType: "Match",
      },
    ],
    classes: [
      {
        name: "Regex",
        description: "Compiled regular expression object",
        methods: [
          {
            name: "match",
            signature: "match(string)",
            description: "Match at start of string",
            returns: "Match - Match object or None",
          },
          {
            name: "search",
            signature: "search(string)",
            description: "Search anywhere in string",
            returns: "Match - Match object or None",
          },
          {
            name: "findall",
            signature: "findall(string)",
            description: "Find all matches",
            returns: "list - List of matches",
          },
          {
            name: "finditer",
            signature: "finditer(string)",
            description: "Find all matches as Match objects",
            returns: "iterator - Match objects",
          },
        ],
      },
      {
        name: "Match",
        description: "Regex match result",
        methods: [
          {
            name: "group",
            signature: "group(n=0)",
            description: "Return nth group",
            returns: "str - Matched group",
          },
          {
            name: "groups",
            signature: "groups()",
            description: "Return tuple of all groups (excluding group 0)",
            returns: "tuple - All groups",
          },
          {
            name: "start",
            signature: "start(n=0)",
            description: "Return start position",
            returns: "int - Start index",
          },
          {
            name: "end",
            signature: "end(n=0)",
            description: "Return end position",
            returns: "int - End index",
          },
          {
            name: "span",
            signature: "span(n=0)",
            description: "Return (start, end) tuple",
            returns: "tuple - (start, end)",
          },
        ],
      },
    ],
    constants: [
      {
        name: "IGNORECASE",
        value: "2",
        description: "Case-insensitive matching flag (alias: I)",
      },
      {
        name: "I",
        value: "2",
        description: "Case-insensitive matching (short alias)",
      },
      {
        name: "MULTILINE",
        value: "8",
        description: "^ and $ match at line boundaries (alias: M)",
      },
      {
        name: "M",
        value: "8",
        description: "Multiline mode (short alias)",
      },
      {
        name: "DOTALL",
        value: "16",
        description: ". matches newlines (alias: S)",
      },
      {
        name: "S",
        value: "16",
        description: "Dot matches all (short alias)",
      },
    ],
  },
  {
    module: "statistics",
    description: "Mathematical statistics functions",
    functions: [
      {
        name: "mean",
        signature: "mean(data)",
        description: "Arithmetic mean",
        returns: "float - Average value",
      },
      {
        name: "fmean",
        signature: "fmean(data)",
        description: "Arithmetic mean (fast)",
        returns: "float - Average value",
      },
      {
        name: "geometric_mean",
        signature: "geometric_mean(data)",
        description: "Geometric mean (positive numbers)",
        returns: "float - Geometric mean",
      },
      {
        name: "harmonic_mean",
        signature: "harmonic_mean(data)",
        description: "Harmonic mean (positive numbers)",
        returns: "float - Harmonic mean",
      },
      {
        name: "median",
        signature: "median(data)",
        description: "Median value",
        returns: "float - Median",
      },
      {
        name: "mode",
        signature: "mode(data)",
        description: "Most common value",
        returns: "any - Mode",
      },
      {
        name: "stdev",
        signature: "stdev(data)",
        description: "Sample standard deviation",
        returns: "float - Standard deviation",
      },
      {
        name: "pstdev",
        signature: "pstdev(data)",
        description: "Population standard deviation",
        returns: "float - Standard deviation",
      },
      {
        name: "variance",
        signature: "variance(data)",
        description: "Sample variance",
        returns: "float - Variance",
      },
      {
        name: "pvariance",
        signature: "pvariance(data)",
        description: "Population variance",
        returns: "float - Variance",
      },
    ],
  },
  {
    module: "string",
    description: "String constants and operations",
    functions: [],
    constants: [
      {
        name: "ascii_letters",
        value: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
        description: "ASCII letters",
      },
      {
        name: "ascii_lowercase",
        value: "abcdefghijklmnopqrstuvwxyz",
        description: "ASCII lowercase letters",
      },
      {
        name: "ascii_uppercase",
        value: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
        description: "ASCII uppercase letters",
      },
      {
        name: "digits",
        value: "0123456789",
        description: "Decimal digits",
      },
      {
        name: "hexdigits",
        value: "0123456789abcdefABCDEF",
        description: "Hexadecimal digits",
      },
      {
        name: "octdigits",
        value: "01234567",
        description: "Octal digits",
      },
      {
        name: "punctuation",
        value: "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
        description: "Punctuation characters",
      },
      {
        name: "whitespace",
        value: " \\t\\n\\r\\x0b\\x0c",
        description: "Whitespace characters",
      },
      {
        name: "printable",
        value: "All printable characters",
        description: "Printable characters",
      },
    ],
  },
  {
    module: "textwrap",
    description: "Text wrapping and filling",
    functions: [
      {
        name: "wrap",
        signature: "wrap(text, width=70)",
        description: "Wrap text to specified width",
        returns: "list - List of lines",
      },
      {
        name: "fill",
        signature: "fill(text, width=70)",
        description: "Wrap and return single string",
        returns: "str - Wrapped text",
      },
      {
        name: "dedent",
        signature: "dedent(text)",
        description: "Remove common leading whitespace",
        returns: "str - Dedented text",
      },
      {
        name: "indent",
        signature: "indent(text, prefix)",
        description: "Add prefix to non-empty lines",
        returns: "str - Indented text",
      },
      {
        name: "shorten",
        signature: "shorten(text, width, placeholder='[...]')",
        description: "Truncate to fit width",
        returns: "str - Shortened text",
      },
    ],
  },
  {
    module: "urllib.parse",
    description: "URL parsing and manipulation",
    functions: [
      {
        name: "quote",
        signature: "quote(string, safe='')",
        description: "URL encode string",
        returns: "str - Encoded string",
      },
      {
        name: "quote_plus",
        signature: "quote_plus(string, safe='')",
        description: "URL encode with + for spaces",
        returns: "str - Encoded string",
      },
      {
        name: "unquote",
        signature: "unquote(string)",
        description: "URL decode string",
        returns: "str - Decoded string",
      },
      {
        name: "unquote_plus",
        signature: "unquote_plus(string)",
        description: "URL decode with + as spaces",
        returns: "str - Decoded string",
      },
      {
        name: "urlparse",
        signature: "urlparse(urlstring)",
        description: "Parse URL into components",
        returns: "ParseResult - Parsed URL",
        returnType: "ParseResult",
      },
      {
        name: "urlunparse",
        signature: "urlunparse(components)",
        description: "Construct URL from components",
        returns: "str - URL",
      },
      {
        name: "urljoin",
        signature: "urljoin(base, url)",
        description: "Join base URL with reference",
        returns: "str - Joined URL",
      },
      {
        name: "urlsplit",
        signature: "urlsplit(urlstring)",
        description: "Split URL into components",
        returns: "SplitResult - Split URL",
      },
      {
        name: "urlunsplit",
        signature: "urlunsplit(components)",
        description: "Construct URL from component tuple",
        returns: "str - URL",
      },
      {
        name: "parse_qs",
        signature: "parse_qs(qs)",
        description: "Parse query string as dict",
        returns: "dict - Query parameters",
      },
      {
        name: "parse_qsl",
        signature: "parse_qsl(qs)",
        description: "Parse query string as list of tuples",
        returns: "list - Query tuples",
      },
      {
        name: "urlencode",
        signature: "urlencode(query)",
        description: "Encode dict as query string",
        returns: "str - Query string",
      },
    ],
    classes: [
      {
        name: "ParseResult",
        description: "Parsed URL result",
        methods: [
          {
            name: "geturl",
            signature: "geturl()",
            description: "Reconstruct URL from components",
            returns: "str - URL",
          },
        ],
      },
    ],
  },
  {
    module: "os",
    description: "OS operations and file system access",
    functions: [
      {
        name: "getenv",
        signature: "getenv(key[, default])",
        description: "Get environment variable value",
        returns: "str or default - Environment value or default",
      },
      {
        name: "environ",
        signature: "environ()",
        description: "Get all environment variables",
        returns: "dict - All environment variables as dictionary",
      },
      {
        name: "getcwd",
        signature: "getcwd()",
        description: "Get current working directory",
        returns: "str - Current directory path",
      },
      {
        name: "listdir",
        signature: "listdir[path='.'])",
        description: "List directory contents",
        returns: "list - Directory entries",
      },
      {
        name: "read_file",
        signature: "read_file(path)",
        description: "Read entire file contents as string",
        returns: "str - File contents",
      },
      {
        name: "write_file",
        signature: "write_file(path, content)",
        description: "Write string content to file",
        returns: "None",
      },
      {
        name: "append_file",
        signature: "append_file(path, content)",
        description: "Append content to file",
        returns: "None",
      },
      {
        name: "remove",
        signature: "remove(path)",
        description: "Remove file",
        returns: "None",
      },
      {
        name: "mkdir",
        signature: "mkdir(path)",
        description: "Create directory",
        returns: "None",
      },
      {
        name: "makedirs",
        signature: "makedirs(path)",
        description: "Create directories recursively",
        returns: "None",
      },
      {
        name: "rmdir",
        signature: "rmdir(path)",
        description: "Remove empty directory",
        returns: "None",
      },
      {
        name: "rename",
        signature: "rename(old, new)",
        description: "Rename file or directory",
        returns: "None",
      },
    ],
    constants: [
      {
        name: "sep",
        value: "'/' or '\\'",
        description: "Path separator string",
      },
      {
        name: "linesep",
        value: "'\\n' or '\\r\\n'",
        description: "Line separator string",
      },
      {
        name: "name",
        value: "'posix' or 'nt'",
        description: "OS name identifier",
      },
    ],
  },
  {
    module: "os.path",
    description: "Common pathname manipulations",
    functions: [
      {
        name: "join",
        signature: "join(*paths)",
        description: "Join path components intelligently",
        returns: "str - Combined path",
      },
      {
        name: "exists",
        signature: "exists(path)",
        description: "Check if path exists",
        returns: "bool - True if path exists",
      },
      {
        name: "isfile",
        signature: "isfile(path)",
        description: "Check if path is a file",
        returns: "bool - True if file",
      },
      {
        name: "isdir",
        signature: "isdir(path)",
        description: "Check if path is a directory",
        returns: "bool - True if directory",
      },
      {
        name: "basename",
        signature: "basename(path)",
        description: "Get final path component",
        returns: "str - Base name",
      },
      {
        name: "dirname",
        signature: "dirname(path)",
        description: "Get directory component",
        returns: "str - Directory path",
      },
      {
        name: "split",
        signature: "split(path)",
        description: "Split into (directory, filename) tuple",
        returns: "tuple - (dirname, basename)",
      },
      {
        name: "splitext",
        signature: "splitext(path)",
        description: "Split into (root, extension) tuple",
        returns: "tuple - (root, ext)",
      },
      {
        name: "abspath",
        signature: "abspath(path)",
        description: "Get absolute path",
        returns: "str - Absolute path",
      },
      {
        name: "normpath",
        signature: "normpath(path)",
        description: "Normalize path (collapse redundant separators, etc.)",
        returns: "str - Normalized path",
      },
      {
        name: "relpath",
        signature: "relpath(path[, start])",
        description: "Get relative path",
        returns: "str - Relative path",
      },
      {
        name: "isabs",
        signature: "isabs(path)",
        description: "Check if path is absolute",
        returns: "bool - True if absolute",
      },
      {
        name: "getsize",
        signature: "getsize(path)",
        description: "Get file size in bytes",
        returns: "int - File size",
      },
    ],
  },
  {
    module: "pathlib",
    description: "Object-oriented path manipulation",
    functions: [
      {
        name: "Path",
        signature: "Path(path)",
        description: "Create Path object",
        returns: "Path - Path instance",
        returnType: "Path",
      },
    ],
    classes: [
      {
        name: "Path",
        description: "Object-oriented filesystem path",
        methods: [
          {
            name: "joinpath",
            signature: "joinpath(*other)",
            description: "Combine with other path segments",
            returns: "Path - New combined path",
          },
          {
            name: "exists",
            signature: "exists()",
            description: "Check if path exists",
            returns: "bool - True if exists",
          },
          {
            name: "is_file",
            signature: "is_file()",
            description: "Check if path is a regular file",
            returns: "bool - True if file",
          },
          {
            name: "is_dir",
            signature: "is_dir()",
            description: "Check if path is a directory",
            returns: "bool - True if directory",
          },
          {
            name: "mkdir",
            signature: "mkdir(parents=False)",
            description: "Create directory",
            returns: "None",
          },
          {
            name: "rmdir",
            signature: "rmdir()",
            description: "Remove empty directory",
            returns: "None",
          },
          {
            name: "unlink",
            signature: "unlink(missing_ok=False)",
            description: "Remove file or symlink",
            returns: "None",
          },
          {
            name: "read_text",
            signature: "read_text()",
            description: "Read file contents as string",
            returns: "str - File contents",
          },
          {
            name: "write_text",
            signature: "write_text(data)",
            description: "Write string data to file",
            returns: "None",
          },
        ],
        properties: [
          {
            name: "name",
            description: "Final path component",
          },
          {
            name: "stem",
            description: "Final component without suffix",
          },
          {
            name: "suffix",
            description: "Final component's last suffix",
          },
          {
            name: "parent",
            description: "Logical parent path",
          },
          {
            name: "parts",
            description: "Tuple of path components",
          },
        ],
      },
    ],
  },
  {
    module: "requests",
    description: "HTTP client for making web requests",
    functions: [
      {
        name: "get",
        signature: "get(url, **kwargs)",
        description: "Send GET request",
        returns: "Response - Response object",
        returnType: "Response",
      },
      {
        name: "post",
        signature: "post(url, data=None, **kwargs)",
        description: "Send POST request",
        returns: "Response - Response object",
        returnType: "Response",
      },
      {
        name: "put",
        signature: "put(url, data=None, **kwargs)",
        description: "Send PUT request",
        returns: "Response - Response object",
        returnType: "Response",
      },
      {
        name: "delete",
        signature: "delete(url, **kwargs)",
        description: "Send DELETE request",
        returns: "Response - Response object",
        returnType: "Response",
      },
      {
        name: "patch",
        signature: "patch(url, data=None, **kwargs)",
        description: "Send PATCH request",
        returns: "Response - Response object",
        returnType: "Response",
      },
    ],
    classes: [
      {
        name: "Response",
        description: "HTTP response object",
        methods: [
          {
            name: "json",
            signature: "json()",
            description: "Parse response body as JSON",
            returns: "object - Parsed JSON",
          },
          {
            name: "raise_for_status",
            signature: "raise_for_status()",
            description: "Raise exception if status >= 400",
            returns: "None",
          },
        ],
        properties: [
          {
            name: "status_code",
            description: "HTTP status code (int)",
          },
          {
            name: "text",
            description: "Response body as string",
          },
          {
            name: "headers",
            description: "Response headers as dict",
          },
          {
            name: "body",
            description: "Response body as string (alias for text)",
          },
          {
            name: "url",
            description: "Request URL",
          },
        ],
      },
    ],
  },
  {
    module: "secrets",
    description: "Secure random number generation for secrets and tokens",
    functions: [
      {
        name: "token_bytes",
        signature: "token_bytes([nbytes])",
        description: "Generate nbytes random bytes as list of integers",
        returns: "list[int] - Random bytes",
      },
      {
        name: "token_hex",
        signature: "token_hex([nbytes])",
        description: "Generate random hexadecimal string",
        returns: "str - Hex string",
      },
      {
        name: "token_urlsafe",
        signature: "token_urlsafe([nbytes])",
        description: "Generate URL-safe random text",
        returns: "str - URL-safe random string",
      },
      {
        name: "randbelow",
        signature: "randbelow(n)",
        description: "Random integer in [0, n)",
        returns: "int - Random number",
      },
      {
        name: "randbits",
        signature: "randbits(k)",
        description: "Random integer with k random bits",
        returns: "int - Random number",
      },
      {
        name: "choice",
        signature: "choice(sequence)",
        description: "Random element from string or list",
        returns: "any - Random element",
      },
      {
        name: "compare_digest",
        signature: "compare_digest(a, b)",
        description: "Constant-time string comparison",
        returns: "bool - True if equal",
      },
    ],
  },
  {
    module: "subprocess",
    description: "Subprocess management and command execution",
    functions: [
      {
        name: "run",
        signature: "run(args, **kwargs)",
        description:
          "Run command and wait for completion. kwargs: capture_output, shell, cwd, timeout, check, text, encoding, input, env",
        returns: "CompletedProcess - Process result",
        returnType: "CompletedProcess",
      },
    ],
    classes: [
      {
        name: "CompletedProcess",
        description: "Result of subprocess execution",
        properties: [
          {
            name: "args",
            description: "List of command arguments",
          },
          {
            name: "returncode",
            description: "Exit status code",
          },
          {
            name: "stdout",
            description: "Standard output",
          },
          {
            name: "stderr",
            description: "Standard error",
          },
        ],
        methods: [
          {
            name: "check_returncode",
            signature: "check_returncode()",
            description: "Raises exception if returncode != 0",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "sys",
    description: "System-specific parameters and functions",
    functions: [
      {
        name: "exit",
        signature: "exit([code])",
        description: "Exit from script. Raises SystemExit exception",
        returns: "Never returns",
      },
    ],
    constants: [
      {
        name: "platform",
        value: "'darwin', 'linux', 'win32', etc.",
        description: "Platform identifier",
      },
      {
        name: "version",
        value: "Scriptling version string",
        description: "Scriptling interpreter version",
      },
      {
        name: "maxsize",
        value: "2^63 - 1",
        description: "Maximum integer value",
      },
      {
        name: "path_sep",
        value: "'/' or '\\'",
        description: "Path separator",
      },
      {
        name: "argv",
        value: "List of command line arguments",
        description: "Script arguments",
      },
    ],
  },
  {
    module: "wait_for",
    description: "Wait for resources to become available",
    functions: [
      {
        name: "file",
        signature: "file(path, timeout=30, poll_rate=1)",
        description: "Wait for file to exist",
        returns: "bool - True if file exists",
      },
      {
        name: "dir",
        signature: "dir(path, timeout=30, poll_rate=1)",
        description: "Wait for directory to exist",
        returns: "bool - True if directory exists",
      },
      {
        name: "port",
        signature: "port(host, port, timeout=30, poll_rate=1)",
        description: "Wait for TCP port to be open",
        returns: "bool - True if port is open",
      },
      {
        name: "http",
        signature: "http(url, timeout=30, poll_rate=1, status_code=200)",
        description: "Wait for HTTP endpoint with expected status",
        returns: "bool - True if endpoint responds",
      },
      {
        name: "file_content",
        signature: "file_content(path, content, timeout=30, poll_rate=1)",
        description: "Wait for file to contain content",
        returns: "bool - True if content found",
      },
      {
        name: "process_name",
        signature: "process_name(name, timeout=30, poll_rate=1)",
        description: "Wait for process to be running",
        returns: "bool - True if process is running",
      },
    ],
  },
];

/**
 * Variable type tracking for context-aware autocomplete
 * Maps variable names to their inferred types
 */
const variableTypes = new Map();

/**
 * Patterns to detect variable assignments with known types
 * e.g., "client = sl.ai.Client(...)" -> client is OpenAIClient
 */
const typePatterns = [
  // scriptling.ai.Client returns OpenAIClient
  {
    regex: /(\w+)\s*=\s*scriptling\.ai\.Client\s*\(/,
    type: "OpenAIClient",
  },
  // scriptling.mcp.Client returns MCPClient
  {
    regex: /(\w+)\s*=\s*scriptling\.mcp\.Client\s*\(/,
    type: "MCPClient",
  },
  // sl.ai.completion_stream returns ChatStream
  {
    regex: /(\w+)\s*=\s*(\w+\.)*completion_stream\s*\(/,
    type: "ChatStream",
  },
  // sl.ai.response_stream returns ResponseStream
  {
    regex: /(\w+)\s*=\s*(\w+\.)*response_stream\s*\(/,
    type: "ResponseStream",
  },
  // scriptling.threads.run returns Promise
  {
    regex: /(\w+)\s*=\s*scriptling\.threads\.run\s*\(/,
    type: "Promise",
  },
  // scriptling.runtime.sandbox.create returns Sandbox
  {
    regex: /(\w+)\s*=\s*scriptling\.runtime\.sandbox\.create\s*\(/,
    type: "Sandbox",
  },
  // scriptling.ai.agent.Agent returns Agent
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?ai\.agent\.Agent\s*\(/,
    type: "Agent",
  },
  // scriptling.runtime.background returns Promise
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.background\s*\(/,
    type: "Promise",
  },
  // requests.get/post/put/delete/patch returns Response
  {
    regex: /(\w+)\s*=\s*requests\.(get|post|put|delete|patch)\s*\(/,
    type: "Response",
  },
  // subprocess.run returns CompletedProcess
  {
    regex: /(\w+)\s*=\s*subprocess\.run\s*\(/,
    type: "CompletedProcess",
  },
  // logging.getLogger returns Logger
  {
    regex: /(\w+)\s*=\s*logging\.getLogger\s*\(/,
    type: "Logger",
  },
  // pathlib.Path returns Path
  {
    regex: /(\w+)\s*=\s*(pathlib\.)?Path\s*\(/,
    type: "Path",
  },
  // html.parser.HTMLParser()
  {
    regex: /(\w+)\s*=\s*html\.parser\.HTMLParser\s*\(/,
    type: "HTMLParser",
  },
  // collections.Counter()
  {
    regex: /(\w+)\s*=\s*collections\.Counter\s*\(/,
    type: "Counter",
  },
  // collections.DefaultDict()
  {
    regex: /(\w+)\s*=\s*collections\.DefaultDict\s*\(/,
    type: "DefaultDict",
  },
  // collections.deque()
  {
    regex: /(\w+)\s*=\s*collections\.deque\s*\(/,
    type: "deque",
  },
  // collections.ChainMap()
  {
    regex: /(\w+)\s*=\s*collections\.ChainMap\s*\(/,
    type: "ChainMap",
  },
  // re.compile() returns Regex
  {
    regex: /(\w+)\s*=\s*re\.compile\s*\(/,
    type: "Regex",
  },
  // re.match() returns Match
  {
    regex: /(\w+)\s*=\s*re\.(match|search|fullmatch)\s*\(/,
    type: "Match",
  },
  // urllib.parse.urlparse() returns ParseResult
  {
    regex: /(\w+)\s*=\s*urllib\.parse\.urlparse\s*\(/,
    type: "ParseResult",
  },
  // datetime.datetime() returns datetime
  {
    regex: /(\w+)\s*=\s*datetime\.datetime\s*\(/,
    type: "datetime",
  },
  // datetime.date() returns date
  {
    regex: /(\w+)\s*=\s*datetime\.date\s*\(/,
    type: "date",
  },
  // datetime.timedelta() returns timedelta
  {
    regex: /(\w+)\s*=\s*datetime\.timedelta\s*\(/,
    type: "timedelta",
  },
  // scriptling.ai.memory.new returns MemoryStore
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?ai\.memory\.new\s*\(/,
    type: "MemoryStore",
  },
  // scriptling.websocket.connect returns WebSocketClientConn
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?websocket\.connect\s*\(/,
    type: "WebSocketClientConn",
  },
  // scriptling.messaging.telegram.client returns MessagingClient
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?messaging\.telegram\.client\s*\(/,
    type: "MessagingClient",
  },
  // scriptling.messaging.discord.client returns MessagingClient
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?messaging\.discord\.client\s*\(/,
    type: "MessagingClient",
  },
  // scriptling.messaging.slack.client returns MessagingClient
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?messaging\.slack\.client\s*\(/,
    type: "MessagingClient",
  },
  // scriptling.messaging.console.client returns MessagingClient
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?messaging\.console\.client\s*\(/,
    type: "MessagingClient",
  },
  // scriptling.console.create_panel returns Panel
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?console\.create_panel\s*\(/,
    type: "Panel",
  },
  // scriptling.runtime.kv.open returns Storage
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.kv\.open\s*\(/,
    type: "Storage",
  },
  // scriptling.runtime.sync.WaitGroup returns WaitGroup
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.sync\.WaitGroup\s*\(/,
    type: "WaitGroup",
  },
  // scriptling.runtime.sync.Queue returns Queue
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.sync\.Queue\s*\(/,
    type: "Queue",
  },
  // scriptling.runtime.sync.Atomic returns Atomic
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.sync\.Atomic\s*\(/,
    type: "Atomic",
  },
  // scriptling.runtime.sync.Shared returns Shared
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?runtime\.sync\.Shared\s*\(/,
    type: "Shared",
  },
];

/**
 * Update variable type tracking based on code changes
 * @param {string} code - The current code content
 */
function updateVariableTypes(code) {
  // Clear existing types
  variableTypes.clear();

  // Scan code for variable assignments
  const lines = code.split("\n");
  for (const line of lines) {
    for (const pattern of typePatterns) {
      const match = line.match(pattern.regex);
      if (match) {
        variableTypes.set(match[1], pattern.type);
      }
    }
  }
}

/**
 * Get completions for a specific class type
 * @param {string} className - The class name (e.g., "OpenAIClient")
 * @returns {Array} Ace completion objects
 */
function getClassCompletions(className) {
  const completions = [];

  for (const lib of scriptLibraries) {
    if (lib.classes) {
      for (const cls of lib.classes) {
        if (cls.name === className) {
          for (const method of cls.methods) {
            completions.push({
              caption: method.name,
              value: method.name,
              meta: "method",
              doc: `${method.signature}\n\n${method.description}\n\nReturns: ${method.returns}`,
            });
          }
          return completions;
        }
      }
    }
  }

  return completions;
}

/**
 * Get completions for module functions
 * @param {string} modulePrefix - The module prefix (e.g., "sl.ai")
 * @returns {Array} Ace completion objects
 */
function getModuleCompletions(modulePrefix) {
  const completions = [];
  const moduleName = modulePrefix.replace(/\.\w*$/, ""); // Remove trailing dot if present

  for (const lib of scriptLibraries) {
    if (lib.module === moduleName) {
      // Add module description as a pseudo-completion
      completions.push({
        caption: `${lib.module} module`,
        value: "",
        meta: "module",
        doc: lib.description,
        score: 0, // Show at the top
      });

      // Add functions
      if (lib.functions) {
        for (const func of lib.functions) {
          completions.push({
            caption: func.name,
            value: func.name,
            meta: "function",
            doc: `${func.signature}\n\n${func.description}\n\nReturns: ${func.returns}`,
          });
        }
      }

      // Add classes
      if (lib.classes) {
        for (const cls of lib.classes) {
          completions.push({
            caption: cls.name,
            value: cls.name,
            meta: "class",
            doc: `${cls.name}\n\n${cls.description}`,
          });
        }
      }

      break;
    }
  }

  return completions;
}

/**
 * Get all module name completions (for top-level module access)
 * @returns {Array} Ace completion objects
 */
function getModuleNameCompletions() {
  const completions = [];

  for (const lib of scriptLibraries) {
    completions.push({
      caption: lib.module,
      value: lib.module,
      meta: "module",
      doc: `${lib.module}\n\n${lib.description}`,
    });
  }

  return completions;
}

/**
 * Main getCompletions function for Ace editor
 * Provides context-aware autocomplete based on:
 * - Module name completion (e.g., "sl." -> shows modules)
 * - Module function completion (e.g., "sl.ai." -> shows ai functions)
 * - Instance method completion (e.g., "client." -> shows client methods if client is typed)
 *
 * @param {object} editor - Ace editor instance
 * @param {object} session - Ace session
 * @param {object} pos - Cursor position
 * @param {string} prefix - Current prefix before cursor
 * @param {function} callback - Callback to return completions
 */
function getCompletions(editor, session, pos, prefix, callback) {
  // Update variable type tracking
  updateVariableTypes(session.getValue());

  const line = session.getLine(pos.row);
  const column = pos.column;

  // Get the text before the cursor
  const textBeforeCursor = line.substring(0, column);

  // Check if we're completing after a dot
  const dotMatch = textBeforeCursor.match(/(\w+)\.\s*(\w*)$/);
  if (dotMatch) {
    const leftSide = dotMatch[1];
    const partialName = dotMatch[2];

    // Check if left side is a known variable type
    if (variableTypes.has(leftSide)) {
      const typeName = variableTypes.get(leftSide);
      let completions = getClassCompletions(typeName);

      // Filter by partial name
      if (partialName) {
        completions = completions.filter((c) =>
          c.caption.toLowerCase().startsWith(partialName.toLowerCase()),
        );
      }

      callback(null, completions);
      return;
    }

    // Check if left side is a module (e.g., "sl.ai")
    if (leftSide.includes(".")) {
      let completions = getModuleCompletions(leftSide);

      // Filter by partial name
      if (partialName) {
        completions = completions.filter((c) =>
          c.caption.toLowerCase().startsWith(partialName.toLowerCase()),
        );
      }

      callback(null, completions);
      return;
    }

    // Unknown left side, try to match module prefix
    let completions = getModuleCompletions(leftSide);

    // Filter by partial name
    if (partialName) {
      completions = completions.filter((c) =>
        c.caption.toLowerCase().startsWith(partialName.toLowerCase()),
      );
    }

    callback(null, completions);
    return;
  }

  // No dot - show all available modules and builtins
  let completions = getModuleNameCompletions();

  // Add standard builtins
  const builtins = [
    "abs",
    "all",
    "any",
    "bin",
    "bool",
    "chr",
    "dict",
    "dir",
    "enumerate",
    "filter",
    "float",
    "hex",
    "int",
    "len",
    "list",
    "map",
    "max",
    "min",
    "oct",
    "ord",
    "pow",
    "print",
    "range",
    "reversed",
    "round",
    "set",
    "sorted",
    "str",
    "sum",
    "tuple",
    "type",
    "zip",
  ];
  for (const builtin of builtins) {
    completions.push({
      caption: builtin,
      value: builtin,
      meta: "builtin",
    });
  }

  // Filter by prefix
  if (prefix) {
    completions = completions.filter((c) =>
      c.caption.toLowerCase().startsWith(prefix.toLowerCase()),
    );
  }

  callback(null, completions);
}

export { getCompletions, scriptLibraries };
