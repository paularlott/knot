// Auto-completion data for Scriptling in Knot
// Provides intelligent autocomplete with context-aware suggestions


import { scriptlingLibraries } from "./scriptlingCompletions.js";

/**
 * Rich library definitions with signatures, return types, and class methods
 * Each library has:
 * - module: Module name
 * - description: Module description
 * - functions: Array of function definitions with signature, description, returns
 * - classes: Array of class definitions with methods
 */
// knot.* library completions are generated from the knot-vscode stubs
// (the IntelliSense source of truth) — regenerate with
// node scripts/generate-knot-completions.mjs
import { knotLibraries } from "./knotCompletions.js";

// Scriptling library completions are generated from the scriptling-vscode
// stubs (the IntelliSense source of truth) — regenerate with
// node scripts/generate-scriptling-completions.mjs
const scriptlingModuleNames = new Set(scriptlingLibraries.map((lib) => lib.module));
const scriptLibraries = [
  // Where knot already hand-wrote a module the vscode stubs also define, the
  // stub version wins — the stubs are the single source of truth.
  ...knotLibraries.filter((lib) => !scriptlingModuleNames.has(lib.module)),
  ...scriptlingLibraries,
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
  // scriptling.container.Client returns ContainerClient
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?container\.Client\s*\(/,
    type: "ContainerClient",
  },
  // scriptling.net.websocket.connect returns WebSocketClientConn
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?net\.websocket\.connect\s*\(/,
    type: "WebSocketClientConn",
  },
  // scriptling.net.multicast.join returns MulticastGroup
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?net\.multicast\.join\s*\(/,
    type: "MulticastGroup",
  },
  // scriptling.net.unicast.connect returns Connection
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?net\.unicast\.connect\s*\(/,
    type: "Connection",
  },
  // scriptling.net.gossip.create returns Cluster
  {
    regex: /(\w+)\s*=\s*(scriptling\.)?net\.gossip\.create\s*\(/,
    type: "Cluster",
  },
  // scriptling.ai.Pipeline returns Pipeline
  {
    regex: /(\w+)\s*=\s*(\w+\.)*Pipeline\s*\(/,
    type: "Pipeline",
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
  // HTTP handler/middleware definitions take a Request parameter
  {
    regex: /def\s+\w+\s*\(\s*(request)\s*[,)]/,
    type: "Request",
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
    "input",
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
    "yield_now",
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
