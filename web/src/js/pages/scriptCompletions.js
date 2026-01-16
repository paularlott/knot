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
        returns: "list - List of space dicts with name, id, is_running, description",
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
        returns: "str - Script output",
      },
      {
        name: "port_forward",
        signature: "port_forward(source_space, local_port, remote_space, remote_port)",
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
    ],
  },
  {
    module: "knot.ai",
    description: "Knot AI completion functions",
    functions: [
      {
        name: "completion",
        signature: "completion(messages)",
        description: "Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys",
        returns: "str - AI response content",
      },
      {
        name: "response_create",
        signature: "response_create(input, model=None, instructions=None, previous_response_id=None, background=False)",
        description: "Create AI response. Returns response dict by default, or response_id if background=True",
        returns: "dict or str - Response object or response ID",
      },
      {
        name: "response_get",
        signature: "response_get(id)",
        description: "Get response by ID",
        returns: "dict - Response dict with status and result",
      },
      {
        name: "response_wait",
        signature: "response_wait(id, timeout=300)",
        description: "Wait for response completion. timeout is in seconds (default 300)",
        returns: "dict - Response dict",
      },
      {
        name: "response_cancel",
        signature: "response_cancel(id)",
        description: "Cancel in-progress response",
        returns: "bool - True if successful",
      },
      {
        name: "response_delete",
        signature: "response_delete(id)",
        description: "Delete response",
        returns: "bool - True if successful",
      },
    ],
  },
  {
    module: "knot.mcp",
    description: "Knot MCP tool functions - parameter access and tool calling",
    functions: [
      {
        name: "get",
        signature: "get(name, default=None)",
        description: "Get MCP parameter value with automatic type conversion",
        returns: "any - Parameter value or default",
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
        name: "return_error",
        signature: "return_error(message)",
        description: "Return an error message and exit with error code",
        returns: "str - Error message",
      },
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
        signature: "tool_search(query)",
        description: "Search for tools by keyword. Returns list of matching tools",
        returns: "list - List of matching tool dicts",
      },
      {
        name: "execute_tool",
        signature: "execute_tool(name, arguments)",
        description: "Execute a discovered tool. Use full name for namespaced tools",
        returns: "any - Tool response",
      },
    ],
  },

  // ============================================================================
  // SCRIPTLING LIBRARIES (sl.*) - Standalone scriptling libraries
  // ============================================================================
  {
    module: "sl.space",
    description: "Scriptling space management (same as knot.space)",
    functions: [
      {
        name: "start",
        signature: "start(name)",
        description: "Start a space by name",
        returns: "bool",
      },
      {
        name: "stop",
        signature: "stop(name)",
        description: "Stop a space by name",
        returns: "bool",
      },
      {
        name: "restart",
        signature: "restart(name)",
        description: "Restart a space by name",
        returns: "bool",
      },
      {
        name: "delete",
        signature: "delete(name)",
        description: "Delete a space by name",
        returns: "bool",
      },
      {
        name: "list",
        signature: "list()",
        description: "List all spaces for current user",
        returns: "list",
      },
      {
        name: "is_running",
        signature: "is_running(name)",
        description: "Check if a space is running",
        returns: "bool",
      },
      {
        name: "create",
        signature: "create(name, template_name, description='', shell='bash')",
        description: "Create a new space",
        returns: "str - Space ID",
      },
      {
        name: "run",
        signature: "run(space_name, command, args=[], timeout=30, workdir='')",
        description: "Execute command in space",
        returns: "str - Output",
      },
      {
        name: "run_script",
        signature: "run_script(space_name, script_name, *args)",
        description: "Execute a script in a space",
        returns: "str - Output",
      },
      {
        name: "port_forward",
        signature: "port_forward(source_space, local_port, remote_space, remote_port)",
        description: "Forward ports between spaces",
        returns: "bool",
      },
      {
        name: "port_list",
        signature: "port_list(space)",
        description: "List active port forwards",
        returns: "list",
      },
      {
        name: "port_stop",
        signature: "port_stop(space, local_port)",
        description: "Stop a port forward",
        returns: "bool",
      },
    ],
  },
  {
    module: "sl.ai",
    description: "Full AI client with streaming, custom tools, and MCP integration",
    functions: [
      {
        name: "completion",
        signature: "completion(model, messages)",
        description: "Create a chat completion using the specified model and messages",
        returns: "dict - Response with id, choices, usage",
      },
      {
        name: "models",
        signature: "models()",
        description: "List available models",
        returns: "list - List of model dicts",
      },
      {
        name: "response_create",
        signature: "response_create(model, input)",
        description: "Create a Responses API response",
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
        name: "new_client",
        signature: 'new_client(base_url, service="openai", api_key=None)',
        description: "Create a new AI client instance for making API calls to supported services",
        returns: "OpenAIClient - A client instance",
        returnType: "OpenAIClient",
      },
    ],
    classes: [
      {
        name: "OpenAIClient",
        description: "OpenAI-compatible AI client",
        methods: [
          {
            name: "completion",
            signature: "completion(model, messages)",
            description: "Create a chat completion",
            returns: "dict",
          },
          {
            name: "completion_stream",
            signature: "completion_stream(model, messages)",
            description: "Create a streaming chat completion",
            returns: "ChatStream - Stream object with next() method",
            returnType: "ChatStream",
          },
          {
            name: "models",
            signature: "models()",
            description: "List available models",
            returns: "list",
          },
          {
            name: "response_create",
            signature: "response_create(model, input)",
            description: "Create Responses API response",
            returns: "dict",
          },
          {
            name: "response_get",
            signature: "response_get(id)",
            description: "Get response by ID",
            returns: "dict",
          },
          {
            name: "response_cancel",
            signature: "response_cancel(id)",
            description: "Cancel response",
            returns: "dict",
          },
          {
            name: "add_remote_server",
            signature: 'add_remote_server(base_url, namespace="", bearer_token="")',
            description: "Add remote MCP server for AI tool access",
            returns: "None",
          },
          {
            name: "remove_remote_server",
            signature: "remove_remote_server(prefix)",
            description: "Remove remote MCP server",
            returns: "None",
          },
          {
            name: "set_tools",
            signature: "set_tools(tools)",
            description: "Set custom tools for manual execution",
            returns: "None",
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
        ],
      },
    ],
  },
  {
    module: "sl.mcp",
    description: "MCP client for connecting to remote MCP servers",
    functions: [
      {
        name: "decode_response",
        signature: "decode_response(response)",
        description: "Decode a raw MCP tool response into scriptling objects",
        returns: "object - Decoded response",
      },
      {
        name: "new_client",
        signature: 'new_client(base_url, namespace="", bearer_token="")',
        description: "Create a new MCP client for connecting to a remote MCP server",
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
    module: "sl.toon",
    description: "TOON (Token-Oriented Object Notation) encoding/decoding library",
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
    ],
  },
  {
    module: "datetime",
    description: "Date and time manipulation",
    functions: [
      {
        name: "now",
        signature: "datetime.now()",
        description: "Return current local date and time",
        returns: "datetime",
      },
    ],
  },
  {
    module: "math",
    description: "Mathematical functions",
    functions: [
      {
        name: "sqrt",
        signature: "sqrt(x)",
        description: "Square root of x",
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
    ],
  },
  {
    module: "random",
    description: "Random number generation",
    functions: [
      {
        name: "random",
        signature: "random()",
        description: "Random float in [0.0, 1.0)",
        returns: "float",
      },
      {
        name: "randint",
        signature: "randint(a, b)",
        description: "Random integer N where a <= N <= b",
        returns: "int",
      },
    ],
  },
  {
    module: "hashlib",
    description: "Cryptographic hashing",
    functions: [
      {
        name: "md5",
        signature: "md5(data)",
        description: "Return MD5 hash",
        returns: "str - Hex digest",
      },
      {
        name: "sha256",
        signature: "sha256(data)",
        description: "Return SHA-256 hash",
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
        name: "uuid4",
        signature: "uuid4()",
        description: "Generate random UUID",
        returns: "str - UUID string",
      },
    ],
  },
  {
    module: "requests",
    description: "HTTP client",
    functions: [
      {
        name: "get",
        signature: "get(url, params=None, headers=None)",
        description: "Send GET request",
        returns: "Response",
      },
      {
        name: "post",
        signature: "post(url, data=None, json=None, headers=None)",
        description: "Send POST request",
        returns: "Response",
      },
    ],
  },
  {
    module: "secrets",
    description: "Secret management",
    functions: [
      {
        name: "get",
        signature: "get(key)",
        description: "Get secret value",
        returns: "str",
      },
    ],
  },
  {
    module: "subprocess",
    description: "Subprocess execution",
    functions: [
      {
        name: "run",
        signature: "run(command, args=[])",
        description: "Run subprocess command",
        returns: "CompletedProcess",
      },
    ],
  },
  {
    module: "os",
    description: "OS operations",
    functions: [
      {
        name: "getenv",
        signature: "getenv(key, default=None)",
        description: "Get environment variable",
        returns: "str",
      },
      {
        name: "path",
        signature: "os.path.join(*parts)",
        description: "Join path parts",
        returns: "str",
      },
    ],
  },
  {
    module: "pathlib",
    description: "Path manipulation",
    functions: [
      {
        name: "Path",
        signature: "Path(path)",
        description: "Create Path object",
        returns: "Path",
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
 * e.g., "client = sl.ai.new_client(...)" -> client is OpenAIClient
 */
const typePatterns = [
  // sl.ai.new_client returns OpenAIClient
  {
    regex: /(\w+)\s*=\s*sl\.ai\.new_client\s*\(/,
    type: "OpenAIClient",
  },
  // sl.mcp.new_client returns MCPClient
  {
    regex: /(\w+)\s*=\s*sl\.mcp\.new_client\s*\(/,
    type: "MCPClient",
  },
  // sl.ai.completion_stream returns ChatStream
  {
    regex: /(\w+)\s*=\s*(\w+\.)*completion_stream\s*\(/,
    type: "ChatStream",
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
          c.caption.toLowerCase().startsWith(partialName.toLowerCase())
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
          c.caption.toLowerCase().startsWith(partialName.toLowerCase())
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
        c.caption.toLowerCase().startsWith(partialName.toLowerCase())
      );
    }

    callback(null, completions);
    return;
  }

  // No dot - show all available modules and builtins
  let completions = getModuleNameCompletions();

  // Add standard builtins
  const builtins = [
    "abs", "all", "any", "bin", "bool", "chr", "dict", "dir", "enumerate",
    "filter", "float", "hex", "int", "len", "list", "map", "max", "min",
    "oct", "ord", "pow", "print", "range", "reversed", "round", "set",
    "sorted", "str", "sum", "tuple", "type", "zip",
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
      c.caption.toLowerCase().startsWith(prefix.toLowerCase())
    );
  }

  callback(null, completions);
}

export { getCompletions, scriptLibraries };
