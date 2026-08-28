// GENERATED FILE — do not edit by hand.
// Source of truth: ../scriptling-vscode/stubs (the IntelliSense type stubs).
// Regenerate with: node scripts/generate-scriptling-completions.mjs
//                 (or: task scriptling-completions)
// Provides the scriptling library completions for the ACE editor (imported by
// scriptCompletions.js, which keeps the knot.* entries of its own).

const scriptlingLibraries = [
  {
    module: "glob",
    description: "Scriptling glob Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "glob",
        signature: "glob(pattern, root_dir=\".\", recursive=False, include_hidden=False)",
        description: "Find all pathnames matching a shell-style wildcard pattern.",
        returns: "list[str] - List of matching pathnames as strings (arbitrary order).",
      },
      {
        name: "iglob",
        signature: "iglob(pattern, root_dir=\".\", recursive=False, include_hidden=False)",
        description: "Find all pathnames matching a shell-style wildcard pattern, returned as an iterator. Memory efficient for large result sets. See glob() for pattern syntax and parameter details.",
        returns: "Iterator[str] - Iterator yielding matching pathnames as strings.",
      },
      {
        name: "escape",
        signature: "escape(pattern)",
        description: "Escape all special characters (*, ?, [, ]) in a pattern so they are treated as literal characters rather than wildcards.",
        returns: "str - The escaped pattern.",
      },
    ],
  },
  {
    module: "msgpack",
    description: "MessagePack binary serialisation library — type stubs for IntelliSense.",
    functions: [
      {
        name: "packb",
        signature: "packb(obj)",
        description: "Serialise a Scriptling value (dict, list, str, int, float, bool, None, bytes) to MessagePack bytes.",
        returns: "bytes - bytes: the MessagePack-encoded payload.",
      },
      {
        name: "unpackb",
        signature: "unpackb(packed)",
        description: "Parse MessagePack bytes into a Scriptling value.",
        returns: "Any - dict, list, str, int, float, bool, None, or bytes: the decoded value.",
      },
      {
        name: "pack",
        signature: "pack(obj)",
        description: "Alias for packb().",
        returns: "bytes",
      },
      {
        name: "unpack",
        signature: "unpack(packed)",
        description: "Alias for unpackb().",
        returns: "Any",
      },
    ],
  },
  {
    module: "os",
    description: "Scriptling os library stubs.",
    functions: [
      {
        name: "getenv",
        signature: "getenv(key, default=None)",
        description: "Return the environment variable value, or default/None if unset.",
        returns: "str",
      },
      {
        name: "getcwd",
        signature: "getcwd()",
        description: "Return the current working directory.",
        returns: "str",
      },
      {
        name: "listdir",
        signature: "listdir(path=\".\")",
        description: "Return entry names in a directory.",
        returns: "List[str]",
      },
      {
        name: "read_file",
        signature: "read_file(path)",
        description: "Read an entire file as a string. Use read_bytes() for binary files.",
        returns: "str",
      },
      {
        name: "read_bytes",
        signature: "read_bytes(path)",
        description: "Read an entire file as bytes (preserves binary data).",
        returns: "bytes",
      },
      {
        name: "read_lines",
        signature: "read_lines(path)",
        description: "Iterate over lines in a file lazily (memory-efficient for large files).",
        returns: "\"Iterator[str]\"",
      },
      {
        name: "write_file",
        signature: "write_file(path, content, mode=0o644)",
        description: "Write a string or bytes to a file, creating or overwriting it.",
        returns: "None",
      },
      {
        name: "append_file",
        signature: "append_file(path, content)",
        description: "Append a string or bytes to a file, creating it if needed.",
        returns: "None",
      },
      {
        name: "remove",
        signature: "remove(path)",
        description: "Remove a file.",
        returns: "None",
      },
      {
        name: "chmod",
        signature: "chmod(path, mode)",
        description: "Change file or directory permissions.",
        returns: "None",
      },
      {
        name: "mkdir",
        signature: "mkdir(path, mode=0o777)",
        description: "Create a directory with an optional permission mode.",
        returns: "None",
      },
      {
        name: "makedirs",
        signature: "makedirs(path, mode=0o777, exist_ok=False)",
        description: "Create a directory and all missing parents.",
        returns: "None",
      },
      {
        name: "rmdir",
        signature: "rmdir(path)",
        description: "Remove an empty directory.",
        returns: "None",
      },
      {
        name: "removedirs",
        signature: "removedirs(name)",
        description: "Remove an empty directory and empty parent directories.",
        returns: "None",
      },
      {
        name: "rename",
        signature: "rename(old, new)",
        description: "Rename a file or directory.",
        returns: "None",
      },
      {
        name: "symlink",
        signature: "symlink(src, dst)",
        description: "Create a symbolic link named dst that points to src.",
        returns: "None",
      },
    ],
  },
  {
    module: "os.path",
    description: "Scriptling os.path stubs.",
    functions: [
      {
        name: "join",
        signature: "join(*paths)",
        description: "Join path components.",
        returns: "str",
      },
      {
        name: "exists",
        signature: "exists(path)",
        description: "Return True if path exists.",
        returns: "bool",
      },
      {
        name: "isfile",
        signature: "isfile(path)",
        description: "Return True if path is a regular file.",
        returns: "bool",
      },
      {
        name: "isdir",
        signature: "isdir(path)",
        description: "Return True if path is a directory.",
        returns: "bool",
      },
      {
        name: "islink",
        signature: "islink(path)",
        description: "Return True if path is a symbolic link. Uses Lstat so the link itself is checked, not the target it points to.",
        returns: "bool",
      },
      {
        name: "basename",
        signature: "basename(path)",
        description: "Return the final path component.",
        returns: "str",
      },
      {
        name: "dirname",
        signature: "dirname(path)",
        description: "Return the directory component.",
        returns: "str",
      },
      {
        name: "split",
        signature: "split(path)",
        description: "Split path into directory and filename.",
        returns: "Tuple[str, str]",
      },
      {
        name: "splitext",
        signature: "splitext(path)",
        description: "Split path into root and extension.",
        returns: "Tuple[str, str]",
      },
      {
        name: "abspath",
        signature: "abspath(path)",
        description: "Return an absolute path.",
        returns: "str",
      },
      {
        name: "normpath",
        signature: "normpath(path)",
        description: "Normalize a path.",
        returns: "str",
      },
      {
        name: "relpath",
        signature: "relpath(path, start=\".\")",
        description: "Return a relative path from start.",
        returns: "str",
      },
      {
        name: "isabs",
        signature: "isabs(path)",
        description: "Return True if path is absolute.",
        returns: "bool",
      },
      {
        name: "getsize",
        signature: "getsize(path)",
        description: "Return file size in bytes.",
        returns: "int",
      },
      {
        name: "getmtime",
        signature: "getmtime(path)",
        description: "Return the modification time as a timestamp.",
        returns: "float",
      },
    ],
  },
  {
    module: "pathlib",
    description: "Scriptling pathlib stubs.",
    classes: [
      {
        name: "Path",
        description: "",
        methods: [
          {
            name: "__init__",
            signature: "__init__(path)",
            description: "",
            returns: "None",
          },
          {
            name: "joinpath",
            signature: "joinpath(*other)",
            description: "Combine this path with other path segments.",
            returns: "\"Path\"",
          },
          {
            name: "exists",
            signature: "exists()",
            description: "Return True if the path exists.",
            returns: "bool",
          },
          {
            name: "is_file",
            signature: "is_file()",
            description: "Return True if this path is a regular file.",
            returns: "bool",
          },
          {
            name: "is_dir",
            signature: "is_dir()",
            description: "Return True if this path is a directory.",
            returns: "bool",
          },
          {
            name: "mkdir",
            signature: "mkdir(mode=0o777, parents=False, exist_ok=False)",
            description: "Create this directory.",
            returns: "None",
          },
          {
            name: "chmod",
            signature: "chmod(mode)",
            description: "Change file or directory permissions.",
            returns: "None",
          },
          {
            name: "rmdir",
            signature: "rmdir()",
            description: "Remove this empty directory.",
            returns: "None",
          },
          {
            name: "unlink",
            signature: "unlink(missing_ok=False)",
            description: "Remove this file or symbolic link.",
            returns: "None",
          },
          {
            name: "read_text",
            signature: "read_text()",
            description: "Read the file contents as a string.",
            returns: "str",
          },
          {
            name: "write_text",
            signature: "write_text(data)",
            description: "Write a string to the file.",
            returns: "None",
          },
          {
            name: "read_bytes",
            signature: "read_bytes()",
            description: "Read the file contents as bytes (preserves binary data).",
            returns: "bytes",
          },
          {
            name: "write_bytes",
            signature: "write_bytes(data)",
            description: "Write bytes to the file. Accepts bytes or str (UTF-8 encoded).",
            returns: "None",
          },
          {
            name: "copy",
            signature: "copy(target)",
            description: "Copy this file or directory to the target path.",
            returns: "\"Path\"",
          },
          {
            name: "rename",
            signature: "rename(target)",
            description: "Rename this file or directory to the target path.",
            returns: "\"Path\"",
          },
          {
            name: "iterdir",
            signature: "iterdir()",
            description: "Return a list of Path objects for the directory contents.",
            returns: "List[\"Path\"]",
          },
          {
            name: "glob",
            signature: "glob(pattern)",
            description: "Return a list of Path objects matching the pattern.",
            returns: "List[\"Path\"]",
          },
        ],
        properties: [
        {
          name: "name",
          description: "str",
        },
        {
          name: "stem",
          description: "str",
        },
        {
          name: "suffix",
          description: "str",
        },
        {
          name: "parent",
          description: "str",
        },
        {
          name: "parts",
          description: "Tuple[str, ...]",
        },
        ],
      },
    ],
  },
  {
    module: "requests",
    description: "Scriptling Requests Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "get",
        signature: "get(url, **kwargs)",
        description: "Send a GET request.",
        returns: "Response - Response object",
      },
      {
        name: "post",
        signature: "post(url, data=None, **kwargs)",
        description: "Send a POST request.",
        returns: "Response - Response object",
      },
      {
        name: "put",
        signature: "put(url, data=None, **kwargs)",
        description: "Send a PUT request.",
        returns: "Response - Response object",
      },
      {
        name: "delete",
        signature: "delete(url, **kwargs)",
        description: "Send a DELETE request.",
        returns: "Response - Response object",
      },
      {
        name: "patch",
        signature: "patch(url, data=None, **kwargs)",
        description: "Send a PATCH request.",
        returns: "Response - Response object",
      },
      {
        name: "parallel",
        signature: "parallel(requests, max_parallel=4)",
        description: "Execute multiple HTTP requests in parallel.",
        returns: "list[Response] - List of Response objects in the same order as input. Failed requests return a Response with status_code=0 and the error in body.",
      },
    ],
    classes: [
      {
        name: "Response",
        description: "HTTP response object returned by all request functions.",
        methods: [
          {
            name: "json",
            signature: "json()",
            description: "Parse the response body as JSON.",
            returns: "Any - Parsed JSON data (dict, list, string, number, bool, or None)",
          },
          {
            name: "raise_for_status",
            signature: "raise_for_status()",
            description: "Raise an exception if the response status code indicates an error (>= 400).",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "status_code",
          description: "int",
        },
        {
          name: "content",
          description: "bytes",
        },
        {
          name: "text",
          description: "str",
        },
        {
          name: "body",
          description: "str",
        },
        {
          name: "headers",
          description: "dict[str, str]",
        },
        {
          name: "url",
          description: "str",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling",
    description: "Scriptling root package stubs.",
  },
  {
    module: "scriptling.ai",
    description: "Scriptling AI Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "Client",
        signature: "Client(base_url, provider=\"openai\", api_key=\"\", max_tokens=0, temperature=None, top_p=None, headers=None, remote_servers=None, max_retries=3, retry_backoff=1.0, retry_on_rate_limit=True, retry_on_server_error=True)",
        description: "Create a new AI client.",
        returns: "OpenAIClient - Client instance with methods for API calls",
      },
      {
        name: "extract_thinking",
        signature: "extract_thinking(text)",
        description: "Extract thinking blocks from AI response.",
        returns: "dict[str, Any] - Dict containing 'thinking' (list of extracted blocks) and 'content' (cleaned text)",
      },
      {
        name: "text",
        signature: "text(response)",
        description: "Get text content from response (without thinking blocks).",
        returns: "str - The response text with thinking blocks removed",
      },
      {
        name: "thinking",
        signature: "thinking(response)",
        description: "Get thinking blocks from response.",
        returns: "list[str] - List of thinking block strings (empty if no thinking blocks)",
      },
      {
        name: "tool_calls",
        signature: "tool_calls(response_or_message)",
        description: "Extract normalized tool calls from a completion response, message dict, or tool call list.",
        returns: "list[dict[str, Any]] - List of normalized tool call dicts with id, type, and function fields",
      },
      {
        name: "execute_tool_calls",
        signature: "execute_tool_calls(registry, tool_calls)",
        description: "Execute normalized tool calls using handlers from a ToolRegistry.",
        returns: "list[dict[str, Any]] - List of tool result message dicts with role, tool_call_id, and content",
      },
      {
        name: "collect_stream",
        signature: "collect_stream(stream, chunk_timeout=None, first_chunk_timeout=None, on_event=None)",
        description: "Consume a ChatStream and aggregate content, reasoning, tool calls, and finish status.",
        returns: "dict[str, Any] - Aggregated result dict with content, reasoning, tool_calls, finish_reason, timed_out, assistant_message, and error (only present when timed_out is true)",
      },
      {
        name: "estimate_tokens",
        signature: "estimate_tokens(request, response=None)",
        description: "Estimate token counts for request messages and/or response.",
        returns: "dict[str, int] - Dict with token usage estimates: - prompt_tokens (int): Estimated tokens in the request messages - completion_tokens (int): Estimated tokens in the response - total_tokens (int): Sum of prompt and completion tokens",
      },
    ],
    classes: [
      {
        name: "ToolRegistry",
        description: "Registry for AI tool definitions.",
        methods: [
          {
            name: "add",
            signature: "add(name, description, params, handler)",
            description: "Add a tool to the registry.",
            returns: "None",
          },
          {
            name: "build",
            signature: "build()",
            description: "Build OpenAI-compatible tool schemas.",
            returns: "list[dict[str, Any]] - List of tool schema dicts suitable for passing to completion requests",
          },
          {
            name: "get_handler",
            signature: "get_handler(name)",
            description: "Get tool handler by name.",
            returns: "Callable[[dict[str, Any]], Any] - Tool handler function",
          },
        ],
      },
      {
        name: "ChatStream",
        description: "Stream object for chat completions.",
        methods: [
          {
            name: "next",
            signature: "next()",
            description: "Get the next chunk from the stream.",
            returns: "dict[str, Any] - The next response chunk, or None if the stream is complete",
          },
          {
            name: "next_timeout",
            signature: "next_timeout(timeout)",
            description: "Get the next chunk from the stream, but stop waiting after a timeout.",
            returns: "dict[str, Any] - The next response chunk, {\"timed_out\": True}, or None if the stream is complete",
          },
          {
            name: "err",
            signature: "err()",
            description: "Get any error from the stream.",
            returns: "str - Error message, or None if no error",
          },
          {
            name: "retry",
            signature: "retry()",
            description: "Get retry metadata for the stream.",
            returns: "dict[str, Any] - Dict with attempts (int), rate_limit_hit (bool), and total_backoff (float), or None if no retries occurred",
          },
        ],
      },
      {
        name: "ResponseStream",
        description: "Stream object for Responses API.",
        methods: [
          {
            name: "next",
            signature: "next()",
            description: "Get the next event from the response stream.",
            returns: "dict[str, Any] - The next event dict, or None if the stream is complete",
          },
        ],
      },
      {
        name: "OpenAIClient",
        description: "AI client for making API calls to supported services.",
        methods: [
          {
            name: "completion",
            signature: "completion(model, messages, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None, extra_body=None, timeout=None)",
            description: "Create a chat completion.",
            returns: "dict[str, Any] - Response dict containing id, choices, usage, etc.",
          },
          {
            name: "completion_stream",
            signature: "completion_stream(model, messages, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None, extra_body=None, timeout=None)",
            description: "Create a streaming chat completion.",
            returns: "ChatStream - ChatStream object with a next() method",
          },
          {
            name: "models",
            signature: "models()",
            description: "List available models.",
            returns: "dict[str, Any] - Response dict with object and data fields. data contains the model list.",
          },
          {
            name: "response_create",
            signature: "response_create(model, input, system_prompt=None, background=False, extra_body=None)",
            description: "Create a response using the OpenAI Responses API.",
            returns: "dict[str, Any] - Response object with id, status, output, usage, etc.",
          },
          {
            name: "response_get",
            signature: "response_get(id)",
            description: "Get a response by ID.",
            returns: "dict[str, Any] - Response object with id, status, output, usage, etc.",
          },
          {
            name: "response_cancel",
            signature: "response_cancel(id)",
            description: "Cancel a response.",
            returns: "dict[str, Any] - Cancelled response object",
          },
          {
            name: "response_delete",
            signature: "response_delete(id)",
            description: "Delete a response by ID.",
            returns: "None",
          },
          {
            name: "response_stream",
            signature: "response_stream(model, input, system_prompt=None, extra_body=None)",
            description: "Stream a response using the Responses API.",
            returns: "ResponseStream - ResponseStream object with a next() method",
          },
          {
            name: "response_compact",
            signature: "response_compact(id)",
            description: "Compact a response by removing intermediate reasoning steps.",
            returns: "dict[str, Any] - Compacted response object with reasoning removed",
          },
          {
            name: "embedding",
            signature: "embedding(model, input)",
            description: "Create an embedding vector for the given input text(s).",
            returns: "dict[str, Any] - Response containing data (list of embeddings), model, and usage",
          },
          {
            name: "ask",
            signature: "ask(model, messages, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None)",
            description: "Quick completion that returns text directly (thinking blocks removed).",
            returns: "str - The response text with thinking blocks removed",
          },
          {
            name: "completion_parallel",
            signature: "completion_parallel(model, messages_list, max_parallel=1, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None, extra_body=None, timeout=None)",
            description: "Run multiple chat completions in parallel with adaptive concurrency.",
            returns: "list[dict[str, Any]] - List of response dicts in the same order as messages_list. Each dict may include a \"retry\" key if retries occurred: {\"attempts\": int, \"rate_limit_hit\": bool, \"total_backoff\": float}",
          },
          {
            name: "ask_parallel",
            signature: "ask_parallel(model, messages_list, max_parallel=1, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None, extra_body=None, timeout=None)",
            description: "Run multiple ask completions in parallel with adaptive concurrency.",
            returns: "list[str] - List of response text strings in the same order as messages_list",
          },
          {
            name: "Pipeline",
            signature: "Pipeline(model, max_parallel=1, ask=False, system_prompt=None, tools=None, temperature=None, top_p=None, max_tokens=None, extra_body=None, timeout=None)",
            description: "Create a streaming completion pipeline.",
            returns: "\"Pipeline\" - Pipeline instance with add() and complete() methods",
          },
        ],
      },
      {
        name: "Pipeline",
        description: "AI completion pipeline that processes requests as they are added.",
        methods: [
          {
            name: "add",
            signature: "add(message)",
            description: "Add a message to the pipeline.",
            returns: "None - None",
          },
          {
            name: "complete",
            signature: "complete()",
            description: "Wait for all queued completions and return results.",
            returns: "list[Any] - list[dict] in completion mode, list[str] in ask mode",
          },
        ],
      },
    ],
    constants: [
      {
        name: "OPENAI",
        description: "str",
      },
      {
        name: "CLAUDE",
        description: "str",
      },
      {
        name: "GEMINI",
        description: "str",
      },
      {
        name: "OLLAMA",
        description: "str",
      },
      {
        name: "ZAI",
        description: "str",
      },
      {
        name: "MISTRAL",
        description: "str",
      },
    ],
  },
  {
    module: "scriptling.ai.agent",
    description: "Scriptling AI Agent Library - Type stubs for IntelliSense support.",
    classes: [
      {
        name: "Message",
        description: "Represents a message in the conversation.",
        properties: [
        {
          name: "content",
          description: "Optional[str]",
        },
        {
          name: "role",
          description: "str",
        },
        {
          name: "tool_calls",
          description: "Optional[list[Any]]",
        },
        ],
      },
      {
        name: "Agent",
        description: "AI Agent with tool-calling capabilities.",
        methods: [
          {
            name: "__init__",
            signature: "__init__(client, tools=None, system_prompt=\"\", model=\"\", memory=None, max_tokens=32000, compaction_threshold=80, request_timeout=300, extra_body=None)",
            description: "Initialize an Agent.",
            returns: "None",
          },
          {
            name: "trigger",
            signature: "trigger(message, max_iterations=1)",
            description: "Trigger the agent with a message and run the agentic loop.",
            returns: "Message - The final message from the agent (content field has thinking blocks removed)",
          },
          {
            name: "get_messages",
            signature: "get_messages()",
            description: "Get the current conversation messages.",
            returns: "list[dict[str, Any]] - List of message dicts in the conversation",
          },
          {
            name: "interact",
            signature: "interact(max_iterations=25)",
            description: "Start an interactive terminal session using the TUI console.",
            returns: "None",
          },
          {
            name: "set_messages",
            signature: "set_messages(messages)",
            description: "Set the conversation messages.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "client",
          description: "OpenAIClient",
        },
        {
          name: "tools",
          description: "Optional[ToolRegistry]",
        },
        {
          name: "system_prompt",
          description: "str",
        },
        {
          name: "model",
          description: "str",
        },
        {
          name: "messages",
          description: "list[dict[str, Any]]",
        },
        {
          name: "tool_schemas",
          description: "list[dict[str, Any]]",
        },
        {
          name: "memory",
          description: "Optional[\"MemoryStore\"]",
        },
        {
          name: "max_tokens",
          description: "int",
        },
        {
          name: "compaction_threshold",
          description: "int",
        },
        {
          name: "request_timeout",
          description: "int",
        },
        {
          name: "extra_body",
          description: "Optional[dict[str, Any]]",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.ai.memory",
    description: "Scriptling AI Memory Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "new",
        signature: "new(kv_store, ai_client=None, model=\"\")",
        description: "Create a memory store backed by a kv store.",
        returns: "MemoryStore - Memory store object with remember, recall, forget, list, count, compact methods",
      },
    ],
    classes: [
      {
        name: "Memory",
        description: "A single stored memory entry.",
        properties: [
        {
          name: "id",
          description: "str",
        },
        {
          name: "content",
          description: "str",
        },
        {
          name: "type",
          description: "MemoryType",
        },
        {
          name: "importance",
          description: "float",
        },
        {
          name: "created_at",
          description: "str",
        },
        {
          name: "accessed_at",
          description: "str",
        },
        ],
      },
      {
        name: "MemoryStore",
        description: "Memory store object for storing and recalling memories.",
        methods: [
          {
            name: "remember",
            signature: "remember(content, type=\"note\", importance=0.5)",
            description: "Store a memory for later recall.",
            returns: "Memory - The stored memory dict with id, content, type, importance, created_at, accessed_at",
          },
          {
            name: "recall",
            signature: "recall(query=\"\", limit=10, type=\"\")",
            description: "Search memories by keyword and semantic similarity.",
            returns: "list[Memory] - List of matching memory dicts ranked by relevance",
          },
          {
            name: "forget",
            signature: "forget(id)",
            description: "Remove a memory by ID.",
            returns: "bool - True if a memory was removed, False otherwise",
          },
          {
            name: "count",
            signature: "count()",
            description: "Return the total number of stored memories.",
            returns: "int - Count of all stored memories",
          },
          {
            name: "compact",
            signature: "compact()",
            description: "Manually trigger compaction (prune + deduplicate).",
            returns: "dict[str, int] - Dict with \"removed\" and \"remaining\" counts",
          },
        ],
      },
    ],
    constants: [
      {
        name: "TYPE_FACT",
        description: "str",
      },
      {
        name: "TYPE_PREFERENCE",
        description: "str",
      },
      {
        name: "TYPE_EVENT",
        description: "str",
      },
      {
        name: "TYPE_NOTE",
        description: "str",
      },
    ],
  },
  {
    module: "scriptling.ai.tools",
    description: "Scriptling AI Tools Library - Type stubs for IntelliSense support.",
    classes: [
      {
        name: "Registry",
        description: "Registry for building tool schemas for AI agents.",
        methods: [
          {
            name: "__init__",
            signature: "__init__()",
            description: "Create a new tool registry.",
            returns: "None",
          },
          {
            name: "add",
            signature: "add(name, description, params, handler)",
            description: "Add a tool to the registry.",
            returns: "None",
          },
          {
            name: "build",
            signature: "build()",
            description: "Build OpenAI-compatible tool schemas.",
            returns: "list[dict[str, Any]] - List of tool schema dicts suitable for passing to AI completion requests",
          },
          {
            name: "get_handler",
            signature: "get_handler(name)",
            description: "Get tool handler by name.",
            returns: "Callable[[dict[str, Any]], Any] - Tool handler function",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.console",
    description: "Scriptling Console Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "panel",
        signature: "panel(name=\"main\")",
        description: "Get an existing Panel instance by name.",
        returns: "Panel - Panel instance, or None if not found",
      },
      {
        name: "main_panel",
        signature: "main_panel()",
        description: "Get the main panel.",
        returns: "Panel - The main Panel instance",
      },
      {
        name: "create_panel",
        signature: "create_panel(name=\"\", width=0, height=0, min_width=0, scrollable=False, title=\"\", no_border=False, skip_focus=False)",
        description: "Create a new panel (independent of layout).",
        returns: "Panel - Panel instance",
      },
      {
        name: "add_left",
        signature: "add_left(panel)",
        description: "Add a panel to the left of the main panel.",
        returns: "None",
      },
      {
        name: "add_right",
        signature: "add_right(panel)",
        description: "Add a panel to the right of the main panel.",
        returns: "None",
      },
      {
        name: "clear_layout",
        signature: "clear_layout()",
        description: "Remove the layout tree but keep all panels and their content.",
        returns: "None",
      },
      {
        name: "has_panels",
        signature: "has_panels()",
        description: "Check if multi-panel layout is active.",
        returns: "bool - True if multi-panel layout is active",
      },
      {
        name: "styled",
        signature: "styled(color, text)",
        description: "Apply theme color to text.",
        returns: "str - Styled text string",
      },
      {
        name: "set_status",
        signature: "set_status(left, right)",
        description: "Set both status bar texts.",
        returns: "None",
      },
      {
        name: "set_status_left",
        signature: "set_status_left(text)",
        description: "Set left status bar text.",
        returns: "None",
      },
      {
        name: "set_status_right",
        signature: "set_status_right(text)",
        description: "Set right status bar text.",
        returns: "None",
      },
      {
        name: "set_labels",
        signature: "set_labels(user, assistant, system)",
        description: "Set role labels.",
        returns: "None",
      },
      {
        name: "register_command",
        signature: "register_command(name, description, fn)",
        description: "Register a slash command.",
        returns: "None",
      },
      {
        name: "remove_command",
        signature: "remove_command(name)",
        description: "Remove a registered slash command.",
        returns: "None",
      },
      {
        name: "on_submit",
        signature: "on_submit(fn)",
        description: "Register handler called when user submits input.",
        returns: "None",
      },
      {
        name: "on_escape",
        signature: "on_escape(fn)",
        description: "Register a callback for Esc key.",
        returns: "None",
      },
      {
        name: "spinner_start",
        signature: "spinner_start(text=\"Working\")",
        description: "Show a spinner with text.",
        returns: "None",
      },
      {
        name: "spinner_stop",
        signature: "spinner_stop()",
        description: "Hide the spinner.",
        returns: "None",
      },
      {
        name: "set_progress",
        signature: "set_progress(label, pct)",
        description: "Set progress bar (0.0-1.0, or <0 to clear).",
        returns: "None",
      },
      {
        name: "run",
        signature: "run()",
        description: "Start the console event loop (blocks until exit).",
        returns: "None",
      },
    ],
    classes: [
      {
        name: "Panel",
        description: "A content panel within the TUI layout.",
        methods: [
          {
            name: "write",
            signature: "write(text)",
            description: "Append text to the panel.",
            returns: "None",
          },
          {
            name: "set_content",
            signature: "set_content(text)",
            description: "Replace all panel content.",
            returns: "None",
          },
          {
            name: "clear",
            signature: "clear()",
            description: "Remove all panel content.",
            returns: "None",
          },
          {
            name: "set_title",
            signature: "set_title(title)",
            description: "Set the panel border title.",
            returns: "None",
          },
          {
            name: "set_color",
            signature: "set_color(color)",
            description: "Set the panel border/accent color.",
            returns: "None",
          },
          {
            name: "set_scrollable",
            signature: "set_scrollable(scrollable)",
            description: "Set whether panel content scrolls.",
            returns: "None",
          },
          {
            name: "add_message",
            signature: "add_message(*args, label=\"\", role=\"\")",
            description: "Add a message to the panel.",
            returns: "None",
          },
          {
            name: "stream_start",
            signature: "stream_start(label=\"\", role=\"\")",
            description: "Begin a streaming message in this panel.",
            returns: "None",
          },
          {
            name: "stream_chunk",
            signature: "stream_chunk(text)",
            description: "Append a chunk to the current stream.",
            returns: "None",
          },
          {
            name: "stream_end",
            signature: "stream_end()",
            description: "Finalize the current stream.",
            returns: "None",
          },
          {
            name: "scroll_to_top",
            signature: "scroll_to_top()",
            description: "Scroll to top of panel content.",
            returns: "None",
          },
          {
            name: "scroll_to_bottom",
            signature: "scroll_to_bottom()",
            description: "Scroll to bottom of panel content.",
            returns: "None",
          },
          {
            name: "size",
            signature: "size()",
            description: "Get the panel dimensions.",
            returns: "List[int] - List of [width, height]",
          },
          {
            name: "styled",
            signature: "styled(color, text)",
            description: "Apply theme color to text.",
            returns: "str - Styled text string",
          },
          {
            name: "write_at",
            signature: "write_at(row, col, text)",
            description: "Write text at a specific position (0-indexed).",
            returns: "None",
          },
          {
            name: "clear_line",
            signature: "clear_line(row)",
            description: "Clear a specific line.",
            returns: "None",
          },
          {
            name: "add_row",
            signature: "add_row(panel)",
            description: "Add a child panel as a vertical row (top to bottom).",
            returns: "None",
          },
          {
            name: "add_column",
            signature: "add_column(panel)",
            description: "Add a child panel as a horizontal column (left to right).",
            returns: "None",
          },
        ],
      },
    ],
    constants: [
      {
        name: "PRIMARY",
        description: "str",
      },
      {
        name: "SECONDARY",
        description: "str",
      },
      {
        name: "ERROR",
        description: "str",
      },
      {
        name: "DIM",
        description: "str",
      },
      {
        name: "USER",
        description: "str",
      },
      {
        name: "TEXT",
        description: "str",
      },
    ],
  },
  {
    module: "scriptling.container",
    description: "Scriptling Container Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "runtimes",
        signature: "runtimes()",
        description: "List available container runtimes.",
        returns: "list[str] - list[str]: Subset of [\"docker\", \"podman\", \"apple\"]",
      },
      {
        name: "Client",
        signature: "Client(driver, socket=\"\")",
        description: "Create a container client for the specified runtime driver.",
        returns: "\"ContainerClient\" - ContainerClient: A client instance",
      },
    ],
    classes: [
      {
        name: "ContainerClient",
        description: "Client for managing containers on a specific runtime.",
        methods: [
          {
            name: "driver",
            signature: "driver()",
            description: "Return the name of the active runtime driver.",
            returns: "str - str: \"docker\", \"podman\", or \"apple\"",
          },
          {
            name: "login",
            signature: "login(server, username, password)",
            description: "Authenticate with a container registry.",
            returns: "None",
          },
          {
            name: "image_list",
            signature: "image_list()",
            description: "List locally available images.",
            returns: "list[dict] - list[dict]: List of image info dicts, each with: - id (str): Image ID (digest for Apple, full ID for Docker/Podman) - reference (str): Image reference e.g. \"ubuntu:24.04\" - digest (str): Content digest e.g. \"sha256:abc123...\" - size (int): Manifest size in bytes",
          },
          {
            name: "image_pull",
            signature: "image_pull(image)",
            description: "Pull an image from a registry.",
            returns: "None",
          },
          {
            name: "image_remove",
            signature: "image_remove(image)",
            description: "Remove a local image.",
            returns: "None",
          },
          {
            name: "exec",
            signature: "exec(name_or_id, command, env=[], workdir=\"\", user=\"\")",
            description: "Run a command in a running container and capture output.",
            returns: "dict - dict: Result with keys: - stdout (str): Captured standard output - stderr (str): Captured standard error - exit_code (int): Process exit code",
          },
          {
            name: "exec_stream",
            signature: "exec_stream(name_or_id, command, callback, env=[], workdir=\"\", user=\"\")",
            description: "Run a command in a running container and stream output line by line.",
            returns: "dict - dict: Result with exit_code (int). stdout and stderr are empty strings.",
          },
          {
            name: "run",
            signature: "run(image, name=\"\", ports=[], env=[], volumes=[], command=[], network=\"\", privileged=False)",
            description: "Create and start a container.",
            returns: "str - str: Container ID",
          },
          {
            name: "stop",
            signature: "stop(name_or_id)",
            description: "Stop a running container.",
            returns: "None",
          },
          {
            name: "wait_stopped",
            signature: "wait_stopped(name_or_id, timeout=30)",
            description: "Wait for a container to reach a stopped state.",
            returns: "bool - bool: True if the container is stopped, False if the timeout was reached",
          },
          {
            name: "remove",
            signature: "remove(name_or_id)",
            description: "Remove a stopped container.",
            returns: "None",
          },
          {
            name: "inspect",
            signature: "inspect(name_or_id)",
            description: "Get container details.",
            returns: "dict - dict: Container info with keys: - id (str): Container ID - name (str): Container name - status (str): Current status e.g. \"running\", \"exited\" - image (str): Image reference - running (bool): True if the container is currently running",
          },
          {
            name: "list",
            signature: "list()",
            description: "List all containers (running and stopped).",
            returns: "list[dict] - list[dict]: List of container info dicts, each with: - id (str): Container ID - name (str): Container name - status (str): Current status - image (str): Image reference - running (bool): True if currently running",
          },
          {
            name: "volume_create",
            signature: "volume_create(name, size=\"\")",
            description: "Create a named volume.",
            returns: "None",
          },
          {
            name: "volume_remove",
            signature: "volume_remove(name)",
            description: "Remove a named volume.",
            returns: "None",
          },
          {
            name: "volume_list",
            signature: "volume_list()",
            description: "List named volumes.",
            returns: "list[str] - list[str]: Volume names",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.csv",
    description: "Scriptling CSV Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "loads",
        signature: "loads(content, delimiter=\",\")",
        description: "Parse a CSV string into a list of rows.",
        returns: "list[list[str]] - List of rows, each a list of string values.",
      },
      {
        name: "loads_dict",
        signature: "loads_dict(content, delimiter=\",\")",
        description: "Parse CSV text into a list of dicts. First row is treated as headers.",
        returns: "list[dict] - List of dicts keyed by header names.",
      },
      {
        name: "dumps",
        signature: "dumps(rows, delimiter=\",\")",
        description: "Format a list of lists into CSV text.",
        returns: "str - CSV-formatted text.",
      },
      {
        name: "dumps_dict",
        signature: "dumps_dict(rows, delimiter=\",\", columns=None)",
        description: "Format a list of dicts into CSV text with a header row.",
        returns: "str - CSV-formatted text with a header row.",
      },
    ],
  },
  {
    module: "scriptling.find",
    description: "Scriptling Find Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "path",
        signature: "path(path, recursive=True, type=\"any\", name=\"\", mtime_min=None, mtime_max=None, size_min=None, size_max=None, include_hidden=False, follow_links=False, max_depth=None)",
        description: "Find files and directories under a path by name, type, modification time, and size. Returns matching paths as a list of strings in arbitrary order.",
        returns: "List[str] - List of matching path strings.",
      },
      {
        name: "entries",
        signature: "entries(path, recursive=True, type=\"any\", name=\"\", mtime_min=None, mtime_max=None, size_min=None, size_max=None, include_hidden=False, follow_links=False, max_depth=None, include_metadata=False, include_hash=False, include_symlinks=False)",
        description: "Find files and directories under a path by name, type, modification time, and size, returning a list of dicts carrying each match's path, size, mtime, and is_dir flag. Use this when you need the metadata to compare trees without re-reading bytes; use path() when only the strings are needed, as path() skips the stat in the no-filter common case.",
        returns: "List[FindEntry]",
      },
    ],
    classes: [
      {
        name: "FindEntry",
        description: "A single matching entry returned by find.entries().",
        properties: [
        {
          name: "path",
          description: "str",
        },
        {
          name: "size",
          description: "int",
        },
        {
          name: "mtime",
          description: "float",
        },
        {
          name: "is_dir",
          description: "bool",
        },
        {
          name: "file_perm",
          description: "Optional[int]",
        },
        {
          name: "hash",
          description: "Optional[str]",
        },
        {
          name: "link_target",
          description: "Optional[str]",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.fs",
    description: "Scriptling FS Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "read_bytes",
        signature: "read_bytes(path, offset, length)",
        description: "Read a range of bytes from a file.",
        returns: "str - Raw bytes as a string",
      },
      {
        name: "write_bytes",
        signature: "write_bytes(path, offset, data, mode=0o644)",
        description: "Write raw bytes at an offset. Creates the file if it does not exist.",
        returns: "None - None",
      },
      {
        name: "unpack",
        signature: "unpack(format, data)",
        description: "Unpack binary data using format strings.",
        returns: "List[Union[int, float]] - List of values",
      },
      {
        name: "pack",
        signature: "pack(format, values)",
        description: "Pack values into a binary string. Uses the same format strings as unpack().",
        returns: "str - Binary string",
      },
      {
        name: "byte_at",
        signature: "byte_at(data, index)",
        description: "Return the unsigned byte value (0-255) at the given index.",
        returns: "int - Unsigned byte value (0-255)",
      },
      {
        name: "len",
        signature: "len(data)",
        description: "Return the byte length of a binary string. Unlike the builtin len(), this counts bytes, not Unicode code points.",
        returns: "int - Byte length",
      },
      {
        name: "slice",
        signature: "slice(data, start, end=None)",
        description: "Byte-safe slicing of binary data. Unlike string slicing, this operates on byte offsets, not Unicode code points.",
        returns: "str - Binary string slice",
      },
    ],
  },
  {
    module: "scriptling.grep",
    description: "Scriptling Grep Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "pattern",
        signature: "pattern(regex, path, recursive=False, ignore_case=False, glob=\"\", follow_links=False, max_size=1048576)",
        description: "Search for a regex pattern in a file or directory.",
        returns: "list[dict] - List of match dicts, each with: - file (str): Path to the matched file - line (int): 1-based line number of the match - text (str): Content of the matched line",
      },
      {
        name: "string",
        signature: "string(text, path, recursive=False, ignore_case=False, glob=\"\", follow_links=False, max_size=1048576)",
        description: "Search for a literal string in a file or directory.",
        returns: "list[dict] - List of match dicts, each with: - file (str): Path to the matched file - line (int): 1-based line number of the match - text (str): Content of the matched line",
      },
    ],
  },
  {
    module: "scriptling.hashlib",
    description: "Scriptling Hashlib Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "md5",
        signature: "md5(data=...)",
        description: "Create an MD5 hash object.",
        returns: "Hash - Hash object (digest_size 16, hexdigest length 32)",
      },
      {
        name: "sha1",
        signature: "sha1(data=...)",
        description: "Create a SHA-1 hash object.",
        returns: "Hash - Hash object (digest_size 20, hexdigest length 40)",
      },
      {
        name: "sha256",
        signature: "sha256(data=...)",
        description: "Create a SHA-256 hash object.",
        returns: "Hash - Hash object (digest_size 32, hexdigest length 64)",
      },
    ],
    classes: [
      {
        name: "Hash",
        description: "A hash object returned by hashlib.md5(), hashlib.sha1() or hashlib.sha256().",
        methods: [
          {
            name: "update",
            signature: "update(data)",
            description: "Feed data into the hash. May be called repeatedly to accumulate input.",
            returns: "None - None",
          },
          {
            name: "digest",
            signature: "digest()",
            description: "Return the raw digest as a byte string.",
            returns: "str - The digest bytes as a string",
          },
          {
            name: "hexdigest",
            signature: "hexdigest()",
            description: "Return the digest as a lowercase hexadecimal string.",
            returns: "str - Hex string (length depends on algorithm: md5=32, sha1=40, sha256=64)",
          },
          {
            name: "copy",
            signature: "copy()",
            description: "Return an independent copy of this hash object.",
            returns: "\"Hash\" - A new Hash with the same algorithm and accumulated data",
          },
        ],
        properties: [
        {
          name: "name",
          description: "str",
        },
        {
          name: "digest_size",
          description: "int",
        },
        {
          name: "block_size",
          description: "int",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.hmac",
    description: "Scriptling HMAC Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "new",
        signature: "new(key, msg=..., digestmod=...)",
        description: "Create an HMAC object.",
        returns: "HMAC - HMAC object",
      },
      {
        name: "digest",
        signature: "digest(key, msg, digestmod)",
        description: "One-shot HMAC. Equivalent to hmac.new(key, msg, digestmod).digest().",
        returns: "str - The raw MAC as a byte string",
      },
      {
        name: "compare_digest",
        signature: "compare_digest(a, b)",
        description: "Compare two strings using a constant-time comparison.",
        returns: "bool - True if the strings are equal, False otherwise",
      },
    ],
    classes: [
      {
        name: "HMAC",
        description: "An HMAC object returned by hmac.new().",
        methods: [
          {
            name: "update",
            signature: "update(data)",
            description: "Feed data into the message being authenticated. May be called repeatedly.",
            returns: "None - None",
          },
          {
            name: "digest",
            signature: "digest()",
            description: "Return the raw MAC as a byte string.",
            returns: "str - The MAC bytes as a string",
          },
          {
            name: "hexdigest",
            signature: "hexdigest()",
            description: "Return the MAC as a lowercase hexadecimal string.",
            returns: "str - Hex string",
          },
          {
            name: "copy",
            signature: "copy()",
            description: "Return an independent copy of this HMAC object.",
            returns: "\"HMAC\" - A new HMAC with the same key, algorithm and accumulated data",
          },
        ],
        properties: [
        {
          name: "name",
          description: "str",
        },
        {
          name: "digest_size",
          description: "int",
        },
        {
          name: "block_size",
          description: "int",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.markdown",
    description: "Scriptling Markdown Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "to_html",
        signature: "to_html(markdown_string)",
        description: "Convert a Markdown string to HTML.",
        returns: "str - HTML representation of the Markdown input.",
      },
    ],
  },
  {
    module: "scriptling.math",
    description: "Scriptling Math Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "sqrt",
        signature: "sqrt(x)",
        description: "Return the square root of x.",
        returns: "float - Float square root",
      },
      {
        name: "pow",
        signature: "pow(base, exp)",
        description: "Return base raised to the power exp.",
        returns: "float - Float result",
      },
      {
        name: "fabs",
        signature: "fabs(x)",
        description: "Return the absolute value of x as a float.",
        returns: "float - Float absolute value",
      },
      {
        name: "floor",
        signature: "floor(x)",
        description: "Return the floor of x.",
        returns: "int - Largest integer less than or equal to x",
      },
      {
        name: "ceil",
        signature: "ceil(x)",
        description: "Return the ceiling of x.",
        returns: "int - Smallest integer greater than or equal to x",
      },
      {
        name: "trunc",
        signature: "trunc(x)",
        description: "Truncate x to the nearest integer toward zero.",
        returns: "int - Integer",
      },
      {
        name: "sin",
        signature: "sin(x)",
        description: "Return the sine of x (radians).",
        returns: "float - Float sine value",
      },
      {
        name: "cos",
        signature: "cos(x)",
        description: "Return the cosine of x (radians).",
        returns: "float - Float cosine value",
      },
      {
        name: "tan",
        signature: "tan(x)",
        description: "Return the tangent of x (radians).",
        returns: "float - Float tangent value",
      },
      {
        name: "asin",
        signature: "asin(x)",
        description: "Return the arc sine of x in radians.",
        returns: "float - Float in [-pi/2, pi/2]",
      },
      {
        name: "acos",
        signature: "acos(x)",
        description: "Return the arc cosine of x in radians.",
        returns: "float - Float in [0, pi]",
      },
      {
        name: "atan",
        signature: "atan(x)",
        description: "Return the arc tangent of x in radians.",
        returns: "float - Float in [-pi/2, pi/2]",
      },
      {
        name: "atan2",
        signature: "atan2(y, x)",
        description: "Return the arc tangent of y/x in radians, correctly handling the quadrant.",
        returns: "float - Float in [-pi, pi]",
      },
      {
        name: "tanh",
        signature: "tanh(x)",
        description: "Return the hyperbolic tangent of x.",
        returns: "float - Float in [-1, 1]",
      },
      {
        name: "log",
        signature: "log(x)",
        description: "Return the natural logarithm of x.",
        returns: "float - Float logarithm",
      },
      {
        name: "log10",
        signature: "log10(x)",
        description: "Return the base-10 logarithm of x.",
        returns: "float - Float logarithm",
      },
      {
        name: "log2",
        signature: "log2(x)",
        description: "Return the base-2 logarithm of x.",
        returns: "float - Float logarithm",
      },
      {
        name: "log1p",
        signature: "log1p(x)",
        description: "Return log(1+x) accurately for small x.",
        returns: "float - Float",
      },
      {
        name: "exp",
        signature: "exp(x)",
        description: "Return e raised to the power x.",
        returns: "float - Float",
      },
      {
        name: "expm1",
        signature: "expm1(x)",
        description: "Return exp(x)-1 accurately for small x.",
        returns: "float - Float",
      },
      {
        name: "degrees",
        signature: "degrees(x)",
        description: "Convert radians to degrees.",
        returns: "float - Float in degrees",
      },
      {
        name: "radians",
        signature: "radians(x)",
        description: "Convert degrees to radians.",
        returns: "float - Float in radians",
      },
      {
        name: "hypot",
        signature: "hypot(x, y)",
        description: "Return the Euclidean distance sqrt(x*x + y*y).",
        returns: "float - Float",
      },
      {
        name: "fmod",
        signature: "fmod(x, y)",
        description: "Return the floating-point remainder of x/y.",
        returns: "float - Float remainder",
      },
      {
        name: "gcd",
        signature: "gcd(a, b)",
        description: "Return the greatest common divisor of a and b.",
        returns: "int - Integer GCD",
      },
      {
        name: "factorial",
        signature: "factorial(n)",
        description: "Return n! (n factorial).",
        returns: "int - Integer factorial",
      },
      {
        name: "copysign",
        signature: "copysign(x, y)",
        description: "Return x with the sign of y.",
        returns: "float - Float with magnitude of x and sign of y",
      },
      {
        name: "cbrt",
        signature: "cbrt(x)",
        description: "Return the cube root of x.",
        returns: "float - Float cube root",
      },
      {
        name: "remainder",
        signature: "remainder(x, y)",
        description: "Return the IEEE 754-style remainder of x/y.",
        returns: "float - Float remainder",
      },
      {
        name: "nextafter",
        signature: "nextafter(x, y)",
        description: "Return the next floating-point value after x towards y.",
        returns: "float - Float next value",
      },
      {
        name: "isnan",
        signature: "isnan(x)",
        description: "Check if x is NaN (Not a Number).",
        returns: "bool - True if x is NaN, False otherwise",
      },
      {
        name: "isinf",
        signature: "isinf(x)",
        description: "Check if x is infinite.",
        returns: "bool - True if x is positive or negative infinity",
      },
      {
        name: "isfinite",
        signature: "isfinite(x)",
        description: "Check if x is finite.",
        returns: "bool - True if x is neither NaN nor infinite",
      },
      {
        name: "erf",
        signature: "erf(x)",
        description: "Return the error function of x.",
        returns: "float - Float in [-1, 1]",
      },
      {
        name: "erfc",
        signature: "erfc(x)",
        description: "Return the complementary error function of x.",
        returns: "float - Float in [0, 2]",
      },
      {
        name: "gamma",
        signature: "gamma(x)",
        description: "Return the gamma function of x.",
        returns: "float - Float",
      },
      {
        name: "lgamma",
        signature: "lgamma(x)",
        description: "Return the natural log of the absolute value of the gamma function.",
        returns: "List[float] - List [log_abs_gamma, sign]",
      },
      {
        name: "comb",
        signature: "comb(n, k)",
        description: "Return the number of ways to choose k items from n (binomial coefficient).",
        returns: "int - Integer binomial coefficient",
      },
      {
        name: "perm",
        signature: "perm(n, k=...)",
        description: "Return the number of ways to choose k items from n with order.",
        returns: "int - Integer permutation count",
      },
      {
        name: "prod",
        signature: "prod(iterable, start=...)",
        description: "Return the product of all elements.",
        returns: "Union[int, float] - Integer for all-integer inputs, float otherwise",
      },
      {
        name: "dist",
        signature: "dist(p, q)",
        description: "Return the Euclidean distance between two points.",
        returns: "float - Float Euclidean distance",
      },
      {
        name: "softmax",
        signature: "softmax(x)",
        description: "Return numerically stable softmax of a vector.",
        returns: "Union[List[float], \"FloatArray\"] - Probability distribution summing to 1.0. Returns FloatArray if input is FloatArray.",
      },
      {
        name: "dot",
        signature: "dot(a, b)",
        description: "Return the dot product of two vectors.",
        returns: "float - Float dot product",
      },
      {
        name: "matmul",
        signature: "matmul(a, b)",
        description: "Matrix-matrix multiply. a is (M x K), b is (K x N). Returns (M x N) matrix.",
        returns: "Union[List[List[float]], \"FloatArray\"] - Matrix as list of lists or 2D FloatArray (M x N). Returns FloatArray if either input is FloatArray.",
      },
      {
        name: "transpose",
        signature: "transpose(m)",
        description: "Transpose a 2D matrix. Rows become columns.",
        returns: "Union[List[List[float]], \"FloatArray\"] - New transposed matrix. Returns FloatArray if input is FloatArray.",
      },
      {
        name: "mat_add",
        signature: "mat_add(a, b)",
        description: "Element-wise addition of two matrices.",
        returns: "Union[List[List[float]], \"FloatArray\"] - New matrix with element-wise sums. Returns FloatArray if either input is FloatArray.",
      },
      {
        name: "array",
        signature: "array(data)",
        description: "Create an efficient FloatArray from a list.",
        returns: "\"FloatArray\" - FloatArray",
      },
      {
        name: "shape",
        signature: "shape(a)",
        description: "Return the shape of a FloatArray as a list of ints.",
        returns: "List[int] - List of integers representing dimensions",
      },
    ],
    classes: [
      {
        name: "FloatArray",
        description: "Efficient numerical array with 1D and 2D support.",
      },
    ],
  },
  {
    module: "scriptling.mcp",
    description: "Scriptling MCP Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "Client",
        signature: "Client(target, namespace=\"\", bearer_token=\"\", args=None, env=None)",
        description: "Create a new MCP client, over HTTP or stdio.",
        returns: "MCPClient - Client instance with methods for interacting with the server. For stdio clients, call close() when done to shut the subprocess down.",
      },
      {
        name: "decode_response",
        signature: "decode_response(response)",
        description: "Decode an MCP tool response.",
        returns: "Any - Decoded response (parsed JSON or string)",
      },
    ],
    classes: [
      {
        name: "MCPClient",
        description: "MCP client for connecting to remote MCP servers.",
        methods: [
          {
            name: "tools",
            signature: "tools()",
            description: "List available tools.",
            returns: "list[dict[str, Any]] - List of tool dicts with name, description, input_schema",
          },
          {
            name: "call_tool",
            signature: "call_tool(name, arguments)",
            description: "Execute a tool by name with the provided arguments.",
            returns: "Any - Decoded tool response",
          },
          {
            name: "refresh_tools",
            signature: "refresh_tools()",
            description: "Refresh the tool cache.",
            returns: "None",
          },
          {
            name: "tool_search",
            signature: "tool_search(query, max_results=10)",
            description: "Search for tools using the tool_search MCP tool.",
            returns: "list[dict[str, Any]] - List of matching tool dicts",
          },
          {
            name: "execute_discovered",
            signature: "execute_discovered(name, arguments)",
            description: "Execute a tool by name using the execute_tool MCP tool.",
            returns: "Any - Tool response",
          },
          {
            name: "call_tools_parallel",
            signature: "call_tools_parallel(calls)",
            description: "Execute multiple tools concurrently.",
            returns: "list[dict[str, Any]] - List of dicts with \"name\", \"result\", and \"error\" keys. \"error\" is an empty string on success.",
          },
          {
            name: "execute_discovered_parallel",
            signature: "execute_discovered_parallel(calls)",
            description: "Execute multiple discovered tools concurrently.",
            returns: "list[dict[str, Any]] - List of dicts with \"name\", \"result\", and \"error\" keys. \"error\" is an empty string on success.",
          },
          {
            name: "list_resources",
            signature: "list_resources()",
            description: "List static resources exposed by the server.",
            returns: "list[dict[str, Any]] - List of resource dicts with uri, name, description, mimeType",
          },
          {
            name: "list_resource_templates",
            signature: "list_resource_templates()",
            description: "List resource templates exposed by the server.",
            returns: "list[dict[str, Any]] - List of dicts with uriTemplate, name, description, mimeType",
          },
          {
            name: "read_resource",
            signature: "read_resource(uri)",
            description: "Read a resource by URI (static or expanded from a template).",
            returns: "Any - A content dict (uri, mimeType, text|blob), or a list of them. text is parsed JSON when valid, else a plain string.",
          },
          {
            name: "list_prompts",
            signature: "list_prompts()",
            description: "List prompts exposed by the server.",
            returns: "list[dict[str, Any]] - List of prompt dicts with name, description, and arguments (each argument: name, description, required)",
          },
          {
            name: "get_prompt",
            signature: "get_prompt(name, arguments)",
            description: "Render a prompt by name into messages for the model.",
            returns: "dict[str, Any] - dict with \"description\" and \"messages\" (a list of {\"role\": ..., \"content\": ...})",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the client and release its transport.",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.mcp.tool",
    description: "Scriptling MCP Tool Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "get_int",
        signature: "get_int(name, default=0)",
        description: "Get a parameter as integer.",
        returns: "int",
      },
      {
        name: "get_float",
        signature: "get_float(name, default=0.0)",
        description: "Get a parameter as float.",
        returns: "float",
      },
      {
        name: "get_string",
        signature: "get_string(name, default=\"\")",
        description: "Get a parameter as string.",
        returns: "str",
      },
      {
        name: "get_bool",
        signature: "get_bool(name, default=False)",
        description: "Get a parameter as boolean.",
        returns: "bool",
      },
      {
        name: "get_list",
        signature: "get_list(name, default=None)",
        description: "Get a parameter as list.",
        returns: "list[Any]",
      },
      {
        name: "get_string_list",
        signature: "get_string_list(name, default=None)",
        description: "Get a string array parameter.",
        returns: "list[str]",
      },
      {
        name: "get_int_list",
        signature: "get_int_list(name, default=None)",
        description: "Get an integer array parameter.",
        returns: "list[int]",
      },
      {
        name: "get_float_list",
        signature: "get_float_list(name, default=None)",
        description: "Get a float array parameter.",
        returns: "list[float]",
      },
      {
        name: "get_bool_list",
        signature: "get_bool_list(name, default=None)",
        description: "Get a boolean array parameter.",
        returns: "list[bool]",
      },
      {
        name: "get_request",
        signature: "get_request()",
        description: "Get the HTTP request this tool call is being served for.",
        returns: "Request",
      },
      {
        name: "request_context",
        signature: "request_context()",
        description: "Get the context dict set by the middleware.",
        returns: "dict[str, Any]",
      },
      {
        name: "return_string",
        signature: "return_string(text)",
        description: "Return a string result from the tool and stop execution.",
        returns: "None",
      },
      {
        name: "return_object",
        signature: "return_object(obj)",
        description: "Return an object as JSON from the tool and stop execution.",
        returns: "None",
      },
      {
        name: "return_toon",
        signature: "return_toon(obj)",
        description: "Return an object encoded as TOON from the tool and stop execution.",
        returns: "None",
      },
      {
        name: "return_error",
        signature: "return_error(message)",
        description: "Return an error from the tool and stop execution.",
        returns: "None",
      },
    ],
  },
  {
    module: "scriptling.messaging",
    description: "Scriptling Messaging Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a platform-agnostic button keyboard.",
        returns: "Keyboard - The keyboard (pass-through for type checking)",
      },
    ],
    classes: [
      {
        name: "MessageDict",
        description: "Rich message content structure for reply() and send_message().",
        properties: [
        {
          name: "title",
          description: "str",
        },
        {
          name: "body",
          description: "str",
        },
        {
          name: "color",
          description: "str",
        },
        {
          name: "image",
          description: "str",
        },
        {
          name: "url",
          description: "str",
        },
        ],
      },
      {
        name: "ButtonWithData",
        description: "Button with callback data for handling clicks.",
        properties: [
        {
          name: "text",
          description: "str",
        },
        {
          name: "data",
          description: "str",
        },
        ],
      },
      {
        name: "ButtonWithURL",
        description: "Button that opens a URL when clicked.",
        properties: [
        {
          name: "text",
          description: "str",
        },
        {
          name: "url",
          description: "str",
        },
        ],
      },
      {
        name: "UserDict",
        description: "User information in context.",
        properties: [
        {
          name: "id",
          description: "str",
        },
        {
          name: "name",
          description: "str",
        },
        {
          name: "platform",
          description: "str",
        },
        ],
      },
      {
        name: "FileDict",
        description: "File attachment information in context.",
        properties: [
        {
          name: "id",
          description: "str",
        },
        {
          name: "name",
          description: "str",
        },
        {
          name: "mime",
          description: "str",
        },
        {
          name: "size",
          description: "int",
        },
        {
          name: "url",
          description: "str",
        },
        ],
      },
      {
        name: "ContextDict",
        description: "Context dict passed to handlers.",
        properties: [
        {
          name: "dest",
          description: "str",
        },
        {
          name: "message_id",
          description: "str",
        },
        {
          name: "text",
          description: "str",
        },
        {
          name: "command",
          description: "str",
        },
        {
          name: "is_callback",
          description: "bool",
        },
        {
          name: "callback_id",
          description: "str",
        },
        {
          name: "callback_token",
          description: "str",
        },
        {
          name: "callback_data",
          description: "str",
        },
        {
          name: "args",
          description: "list[str]",
        },
        {
          name: "user",
          description: "UserDict",
        },
        {
          name: "file",
          description: "Optional[FileDict]",
        },
        ],
      },
      {
        name: "MessagingClient",
        description: "Messaging client with bot framework methods.",
        methods: [
          {
            name: "command",
            signature: "command(name, help_text, handler)",
            description: "Register a command handler.",
            returns: "None",
          },
          {
            name: "on_callback",
            signature: "on_callback(handler, prefix=\"\")",
            description: "Register a callback/button handler.",
            returns: "None",
          },
          {
            name: "on_message",
            signature: "on_message(handler)",
            description: "Register default message handler.",
            returns: "None",
          },
          {
            name: "on_file",
            signature: "on_file(handler)",
            description: "Register file attachment handler.",
            returns: "None",
          },
          {
            name: "auth",
            signature: "auth(handler)",
            description: "Register auth handler.",
            returns: "None",
          },
          {
            name: "run",
            signature: "run()",
            description: "Start the bot event loop (blocks until stopped).",
            returns: "None",
          },
          {
            name: "capabilities",
            signature: "capabilities()",
            description: "Get list of platform capability strings.",
            returns: "list[str]",
          },
          {
            name: "send_message",
            signature: "send_message(dest, message, parse_mode=\"\", keyboard=None)",
            description: "Send a message to a destination.",
            returns: "None",
          },
          {
            name: "send_rich_message",
            signature: "send_rich_message(dest, message)",
            description: "Send a rich message.",
            returns: "None",
          },
          {
            name: "edit_message",
            signature: "edit_message(dest, message_id, text)",
            description: "Edit a sent message.",
            returns: "None",
          },
          {
            name: "delete_message",
            signature: "delete_message(dest, message_id)",
            description: "Delete a message.",
            returns: "None",
          },
          {
            name: "send_file",
            signature: "send_file(dest, source, filename=\"\", caption=\"\", base64=False)",
            description: "Send a file.",
            returns: "None",
          },
          {
            name: "typing",
            signature: "typing(dest)",
            description: "Send typing indicator.",
            returns: "None",
          },
          {
            name: "answer_callback",
            signature: "answer_callback(id, text=\"\", token=\"\")",
            description: "Acknowledge a button press.",
            returns: "None",
          },
          {
            name: "download",
            signature: "download(ref)",
            description: "Download a file by ID or URL.",
            returns: "str - Base64-encoded file data",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.messaging.console",
    description: "Scriptling Console Messaging - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "client",
        signature: "client()",
        description: "Create a console bot client.",
        returns: "MessagingClient - MessagingClient instance",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a console keyboard. See scriptling.messaging.keyboard for details.",
        returns: "Keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.discord",
    description: "Scriptling Discord Messaging - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "client",
        signature: "client(token, allowed_users=None)",
        description: "Create a Discord bot client.",
        returns: "MessagingClient - MessagingClient instance",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Discord keyboard. See scriptling.messaging.keyboard for details.",
        returns: "Keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.slack",
    description: "Scriptling Slack Messaging - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "client",
        signature: "client(bot_token, app_token, allowed_users=None)",
        description: "Create a Slack bot client.",
        returns: "MessagingClient - MessagingClient instance",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Slack keyboard. See scriptling.messaging.keyboard for details.",
        returns: "Keyboard",
      },
    ],
  },
  {
    module: "scriptling.messaging.telegram",
    description: "Scriptling Telegram Messaging - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "client",
        signature: "client(token, allowed_users=None)",
        description: "Create a Telegram bot client.",
        returns: "MessagingClient - MessagingClient instance",
      },
      {
        name: "keyboard",
        signature: "keyboard(rows)",
        description: "Build a Telegram keyboard. See scriptling.messaging.keyboard for details.",
        returns: "Keyboard",
      },
    ],
  },
  {
    module: "scriptling.net",
    description: "Scriptling Net Library - Type stubs for IntelliSense support.",
  },
  {
    module: "scriptling.net.gossip",
    description: "Scriptling Gossip Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "create",
        signature: "create(bind_addr=\"127.0.0.1:8000\", node_id=\"\", advertise_addr=\"\", encryption_key=\"\", tags=None, compression=False, bearer_token=\"\", app_version=\"\", transport=\"socket\", compress_min_size=256, gossip_interval=\"5s\", gossip_max_interval=\"20s\", metadata_gossip_interval=\"500ms\", state_gossip_interval=\"45s\", fan_out_multiplier=1.0, ttl_multiplier=1.0, state_exchange_multiplier=0.8, force_reliable_transport=False, prefer_ipv6=False, node_cleanup_interval=\"20s\", node_retention_time=\"1h\", leaving_node_timeout=\"30s\", health_check_interval=\"2s\", suspect_timeout=\"1.5s\", suspect_retry_interval=\"1s\", dead_node_timeout=\"15s\", peer_recovery_interval=\"30s\", insecure_skip_verify=False)",
        description: "Create a gossip cluster node.",
        returns: "Cluster - Cluster object with methods for membership and messaging",
      },
    ],
    classes: [
      {
        name: "NodeDict",
        description: "Node information returned by node queries.",
        properties: [
        {
          name: "id",
          description: "str",
        },
        {
          name: "addr",
          description: "str",
        },
        {
          name: "state",
          description: "str",
        },
        {
          name: "metadata",
          description: "dict[str, str]",
        },
        {
          name: "tags",
          description: "list[str]",
        },
        ],
      },
      {
        name: "MessageDict",
        description: "Message dict passed to message handlers.",
        properties: [
        {
          name: "type",
          description: "int",
        },
        {
          name: "sender",
          description: "NodeDict",
        },
        {
          name: "payload",
          description: "Any",
        },
        ],
      },
      {
        name: "NodeGroup",
        description: "Metadata-criteria-based node group.",
        methods: [
          {
            name: "nodes",
            signature: "nodes()",
            description: "Get all nodes currently in this group.",
            returns: "list[NodeDict] - List of node dicts matching the group criteria",
          },
          {
            name: "contains",
            signature: "contains(node_id)",
            description: "Check if a node is in this group.",
            returns: "bool - True if the node is in the group",
          },
          {
            name: "count",
            signature: "count()",
            description: "Get number of nodes in this group.",
            returns: "int - Number of matching nodes",
          },
          {
            name: "send_to_peers",
            signature: "send_to_peers(message_type, data, reliable=False)",
            description: "Send a message to all peers in the group.",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the node group and release resources.",
            returns: "None",
          },
        ],
      },
      {
        name: "LeaderElection",
        description: "Leader election manager.",
        methods: [
          {
            name: "start",
            signature: "start()",
            description: "Start the leader election process.",
            returns: "None",
          },
          {
            name: "stop",
            signature: "stop()",
            description: "Stop the leader election process.",
            returns: "None",
          },
          {
            name: "is_leader",
            signature: "is_leader()",
            description: "Check if this node is the current leader.",
            returns: "bool - True if this node is the leader",
          },
          {
            name: "has_leader",
            signature: "has_leader()",
            description: "Check if a leader is currently elected.",
            returns: "bool - True if any node is the leader",
          },
          {
            name: "get_leader_id",
            signature: "get_leader_id()",
            description: "Get the current leader's node ID.",
            returns: "str - Leader node UUID, or None if no leader",
          },
          {
            name: "send_to_peers",
            signature: "send_to_peers(message_type, data, reliable=False)",
            description: "Send a message to eligible leader election peers.",
            returns: "None",
          },
          {
            name: "on_event",
            signature: "on_event(event_type, handler)",
            description: "Register a leader election event handler.",
            returns: "None",
          },
        ],
      },
      {
        name: "Cluster",
        description: "Gossip cluster object returned by create().",
        methods: [
          {
            name: "start",
            signature: "start()",
            description: "Start the cluster node.",
            returns: "None",
          },
          {
            name: "join",
            signature: "join(peers)",
            description: "Join an existing cluster by connecting to known peers.",
            returns: "None",
          },
          {
            name: "leave",
            signature: "leave()",
            description: "Gracefully leave the cluster.",
            returns: "None",
          },
          {
            name: "stop",
            signature: "stop()",
            description: "Stop the cluster and clean up resources.",
            returns: "None",
          },
          {
            name: "send",
            signature: "send(message_type, data, reliable=False)",
            description: "Broadcast a message to all cluster nodes.",
            returns: "None",
          },
          {
            name: "send_tagged",
            signature: "send_tagged(tag, message_type, data, reliable=False)",
            description: "Send a tagged message (only delivered to nodes with matching tag).",
            returns: "None",
          },
          {
            name: "send_to",
            signature: "send_to(node_id, message_type, data, reliable=False)",
            description: "Send a direct message to a specific node.",
            returns: "None",
          },
          {
            name: "send_request",
            signature: "send_request(node_id, message_type, data)",
            description: "Send a request to a specific node and wait for a reply.",
            returns: "Any - The reply payload from the target node",
          },
          {
            name: "handle",
            signature: "handle(message_type, handler)",
            description: "Register a message handler for a specific message type.",
            returns: "None",
          },
          {
            name: "handle_with_reply",
            signature: "handle_with_reply(message_type, handler)",
            description: "Register a request/reply message handler.",
            returns: "None",
          },
          {
            name: "unhandle",
            signature: "unhandle(message_type)",
            description: "Remove a previously registered message handler.",
            returns: "bool - True if a handler was removed, False otherwise",
          },
          {
            name: "on_state_change",
            signature: "on_state_change(handler)",
            description: "Register a node state change handler.",
            returns: "None",
          },
          {
            name: "on_metadata_change",
            signature: "on_metadata_change(handler)",
            description: "Register a handler for remote node metadata changes.",
            returns: "None",
          },
          {
            name: "on_gossip_interval",
            signature: "on_gossip_interval(handler)",
            description: "Register a periodic handler called every gossip interval.",
            returns: "None",
          },
          {
            name: "create_node_group",
            signature: "create_node_group(criteria, on_node_added=None, on_node_removed=None)",
            description: "Create a metadata-criteria-based node group.",
            returns: "NodeGroup - NodeGroup object with nodes(), contains(), count(), send_to_peers(), close()",
          },
          {
            name: "create_leader_election",
            signature: "create_leader_election(check_interval=\"1s\", leader_timeout=\"3s\", heartbeat_msg_type=65, min_cluster_size=0, metadata_criteria=None)",
            description: "Create a leader election manager.",
            returns: "LeaderElection - LeaderElection object with start(), stop(), is_leader(), has_leader(), get_leader_id(), on_event()",
          },
          {
            name: "nodes",
            signature: "nodes()",
            description: "Get all known nodes.",
            returns: "list[NodeDict] - List of node dicts with id, addr, state, metadata, tags",
          },
          {
            name: "alive_nodes",
            signature: "alive_nodes()",
            description: "Get all alive nodes.",
            returns: "list[NodeDict] - List of node dicts for nodes in 'alive' state",
          },
          {
            name: "nodes_by_tag",
            signature: "nodes_by_tag(tag)",
            description: "Get all nodes that have a specific tag.",
            returns: "list[NodeDict] - List of node dicts with the matching tag",
          },
          {
            name: "get_node",
            signature: "get_node(node_id)",
            description: "Get a specific node by ID.",
            returns: "NodeDict - Node dict, or None if not found",
          },
          {
            name: "local_node",
            signature: "local_node()",
            description: "Get the local node info.",
            returns: "NodeDict - Node dict with id, addr, state, metadata, tags",
          },
          {
            name: "num_nodes",
            signature: "num_nodes()",
            description: "Get total number of known nodes.",
            returns: "int",
          },
          {
            name: "num_alive",
            signature: "num_alive()",
            description: "Get number of alive nodes.",
            returns: "int",
          },
          {
            name: "num_suspect",
            signature: "num_suspect()",
            description: "Get number of suspect nodes.",
            returns: "int",
          },
          {
            name: "num_dead",
            signature: "num_dead()",
            description: "Get number of dead nodes.",
            returns: "int",
          },
          {
            name: "is_local",
            signature: "is_local(node_id)",
            description: "Check if a node ID refers to the local node.",
            returns: "bool - True if the node ID is the local node",
          },
          {
            name: "candidates",
            signature: "candidates()",
            description: "Get a random subset of nodes for gossiping.",
            returns: "list[NodeDict] - List of node dicts",
          },
          {
            name: "set_metadata",
            signature: "set_metadata(key, value)",
            description: "Set local node metadata (automatically gossiped to other nodes).",
            returns: "None",
          },
          {
            name: "get_metadata",
            signature: "get_metadata(key)",
            description: "Get local node metadata value.",
            returns: "str - The metadata value as a string, or None if not found",
          },
          {
            name: "all_metadata",
            signature: "all_metadata()",
            description: "Get all local node metadata.",
            returns: "dict[str, str] - Dict of all metadata key-value pairs",
          },
          {
            name: "delete_metadata",
            signature: "delete_metadata(key)",
            description: "Delete a metadata key.",
            returns: "None",
          },
          {
            name: "node_id",
            signature: "node_id()",
            description: "Get the local node's unique ID.",
            returns: "str - Node UUID string",
          },
        ],
      },
    ],
    constants: [
      {
        name: "MSG_USER",
        description: "int",
      },
    ],
  },
  {
    module: "scriptling.net.multicast",
    description: "Scriptling Multicast Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "join",
        signature: "join(group_addr, port, interface=\"\", ttl=1)",
        description: "Join a multicast group.",
        returns: "MulticastGroup - Group object with send(), receive(), close() methods and group_addr, port, local_addr properties",
      },
    ],
    classes: [
      {
        name: "ReceiveResult",
        description: "Result from receive() containing data and source address.",
        properties: [
        {
          name: "data",
          description: "bytes",
        },
        {
          name: "source",
          description: "str",
        },
        ],
      },
      {
        name: "MulticastGroup",
        description: "Multicast group object returned by join().",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description: "Send a message to the multicast group.",
            returns: "None",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the multicast group.",
            returns: "ReceiveResult - Dict with \"data\" (bytes — call .decode() for text) and \"source\" keys, or None on timeout",
          },
          {
            name: "close",
            signature: "close()",
            description: "Leave the multicast group and close the connection.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "group_addr",
          description: "str",
        },
        {
          name: "port",
          description: "int",
        },
        {
          name: "local_addr",
          description: "str",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.net.resolve",
    description: "Scriptling DNS Resolve Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "lookup_ip",
        signature: "lookup_ip(host)",
        description: "Resolve a hostname to a list of IP address strings.",
        returns: "list[str] - List of IP address strings",
      },
      {
        name: "lookup_srv",
        signature: "lookup_srv(service)",
        description: "Resolve an SRV record to a list of address dicts.",
        returns: "list[dict[str, object]] - List of dicts with \"ip\" (str) and \"port\" (int) keys",
      },
      {
        name: "resolve_srv_http",
        signature: "resolve_srv_http(uri)",
        description: "Resolve a srv+http(s):// URI to a concrete URL.",
        returns: "str - The resolved URL",
      },
    ],
  },
  {
    module: "scriptling.net.unicast",
    description: "Scriptling Unicast Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "connect",
        signature: "connect(host, port, protocol=\"udp\", timeout=10)",
        description: "Connect to a remote host.",
        returns: "Connection - Connection object with send(), receive(), close(), connected() and local_addr, remote_addr properties",
      },
      {
        name: "listen",
        signature: "listen(host, port=0, protocol=\"tcp\")",
        description: "Listen for incoming connections.",
        returns: "object",
      },
    ],
    classes: [
      {
        name: "Connection",
        description: "Connection object returned by connect().",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description: "Send a message to the remote peer.",
            returns: "None",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the remote peer.",
            returns: "dict - Dict with \"data\" (bytes — call .decode() for text) and \"source\" keys, or None on timeout",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the connection.",
            returns: "None",
          },
          {
            name: "connected",
            signature: "connected()",
            description: "Check if connection is still open.",
            returns: "bool - True if connected, False otherwise",
          },
        ],
        properties: [
        {
          name: "local_addr",
          description: "str",
        },
        {
          name: "remote_addr",
          description: "str",
        },
        ],
      },
      {
        name: "UDPListener",
        description: "UDP listener object returned by listen(protocol=\"udp\").",
        methods: [
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from any sender.",
            returns: "dict - Dict with \"data\" (bytes — call .decode() for text) and \"source\" keys, or None on timeout",
          },
          {
            name: "send_to",
            signature: "send_to(address, message)",
            description: "Send a message to a specific address.",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the listener.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "addr",
          description: "str",
        },
        ],
      },
      {
        name: "TCPListener",
        description: "TCP listener object returned by listen(protocol=\"tcp\").",
        methods: [
          {
            name: "accept",
            signature: "accept(timeout=30)",
            description: "Accept an incoming TCP connection.",
            returns: "Connection - TCP Connection object, or None on timeout",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the listener.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "addr",
          description: "str",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.net.websocket",
    description: "Scriptling WebSocket Client Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "connect",
        signature: "connect(url, timeout=10, headers=None)",
        description: "Connect to a WebSocket server.",
        returns: "WebSocketClientConn - WebSocketClientConn object for sending/receiving messages",
      },
      {
        name: "is_text",
        signature: "is_text(message)",
        description: "Check if a received message is a text message.",
        returns: "bool - True if the message is text, False otherwise",
      },
      {
        name: "is_binary",
        signature: "is_binary(message)",
        description: "Check if a received message is a binary message.",
        returns: "bool - True if the message is binary, False otherwise",
      },
    ],
    classes: [
      {
        name: "WebSocketMessage",
        description: "Base type for received WebSocket messages.",
      },
      {
        name: "WebSocketClientConn",
        description: "WebSocket client connection object.",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description: "Send a text message to the server.",
            returns: "Exception - None on success, or an error/exception if send fails",
          },
          {
            name: "send_binary",
            signature: "send_binary(data)",
            description: "Send binary data to the server.",
            returns: "Exception - None on success, or an error/exception if send fails",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the server.",
            returns: "WebSocketMessage - The received message, or None if timeout or connection closed",
          },
          {
            name: "connected",
            signature: "connected()",
            description: "Check if the connection is still open.",
            returns: "bool - True if connected, False otherwise",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the WebSocket connection.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "remote_addr",
          description: "str",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.nomad",
    description: "Scriptling Nomad Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "Client",
        signature: "Client(addr, token=\"\", insecure=False, timeout=10)",
        description: "Create a Nomad client.",
        returns: "\"NomadClient\" - NomadClient: A client instance",
      },
    ],
    classes: [
      {
        name: "NomadClient",
        description: "Client for a Nomad cluster's HTTP API.",
        methods: [
          {
            name: "csi_volumes_list",
            signature: "csi_volumes_list(namespace=\"*\", plugin_id=\"\")",
            description: "List CSI volumes.",
            returns: "list[dict] - list[dict]: List of volume summary dicts, each with: - id (str): Volume ID - name (str): Volume name - namespace (str): Namespace - plugin_id (str): CSI plugin ID - provider (str): CSI provider name - schedulable (bool): Whether the volume can currently be scheduled - controllers_healthy (int): Number of healthy controller plugins - nodes_healthy (int): Number of healthy node plugins",
          },
          {
            name: "csi_volume_get",
            signature: "csi_volume_get(id, namespace=\"\")",
            description: "Get details for a CSI volume.",
            returns: "dict - dict: Full volume specification and status, as returned by the Nomad API",
          },
          {
            name: "csi_volume_register",
            signature: "csi_volume_register(id, volume, namespace=\"\")",
            description: "Register a pre-existing CSI volume with Nomad.",
            returns: "None",
          },
          {
            name: "csi_volume_create",
            signature: "csi_volume_create(id, volume, namespace=\"\")",
            description: "Create a CSI volume and provision backing storage.",
            returns: "None",
          },
          {
            name: "csi_volume_deregister",
            signature: "csi_volume_deregister(id, namespace=\"\", force=False)",
            description: "Deregister a CSI volume from Nomad without removing the backing storage.",
            returns: "None",
          },
          {
            name: "csi_volume_delete",
            signature: "csi_volume_delete(id, namespace=\"\")",
            description: "Delete a CSI volume and its backing storage.",
            returns: "None",
          },
          {
            name: "host_volumes_list",
            signature: "host_volumes_list(namespace=\"*\", node_id=\"\", node_pool=\"\", plugin_id=\"\")",
            description: "List dynamic host volumes.",
            returns: "list[dict] - list[dict]: List of volume summary dicts, each with: - id (str): Volume ID - name (str): Volume name - namespace (str): Namespace - plugin_id (str): Host volume plugin ID - node_id (str): Node the volume is on - node_pool (str): Node pool - state (str): Volume state",
          },
          {
            name: "host_volume_get",
            signature: "host_volume_get(id, namespace=\"\")",
            description: "Get details for a dynamic host volume.",
            returns: "dict - dict: Full volume specification and status, as returned by the Nomad API",
          },
          {
            name: "host_volume_register",
            signature: "host_volume_register(id, volume, namespace=\"\")",
            description: "Register a pre-existing dynamic host volume with Nomad.",
            returns: "None",
          },
          {
            name: "host_volume_create",
            signature: "host_volume_create(id, volume, namespace=\"\")",
            description: "Create a dynamic host volume via a plugin.",
            returns: "None",
          },
          {
            name: "host_volume_delete",
            signature: "host_volume_delete(id, namespace=\"\")",
            description: "Delete a dynamic host volume.",
            returns: "None",
          },
          {
            name: "jobs_list",
            signature: "jobs_list(namespace=\"*\", prefix=\"\")",
            description: "List jobs.",
            returns: "list[dict] - list[dict]: List of job summary dicts, each with: - id (str): Job ID - name (str): Job name - namespace (str): Namespace - type (str): Job type e.g. \"service\", \"batch\", \"system\" - status (str): Current status e.g. \"running\", \"pending\", \"dead\" - priority (int): Job priority",
          },
          {
            name: "job_get",
            signature: "job_get(id, namespace=\"\")",
            description: "Get the full specification and status for a job.",
            returns: "dict - dict: Job specification and status, as returned by the Nomad API",
          },
          {
            name: "job_register",
            signature: "job_register(job)",
            description: "Register (create or update) a job.",
            returns: "dict - dict: Registration response with keys: - EvalID (str): Evaluation ID created for this registration - EvalCreateIndex (int) - JobModifyIndex (int) - Warnings (str)",
          },
          {
            name: "job_stop",
            signature: "job_stop(id, namespace=\"\", purge=False)",
            description: "Stop a job.",
            returns: "dict - dict: Stop response with keys: EvalID (str), EvalCreateIndex (int), JobModifyIndex (int)",
          },
          {
            name: "wait_job_stopped",
            signature: "wait_job_stopped(id, namespace=\"\", timeout=30)",
            description: "Wait for a job to reach the \"dead\" status.",
            returns: "bool - bool: True if the job is stopped, False if the timeout was reached",
          },
          {
            name: "job_validate",
            signature: "job_validate(job)",
            description: "Validate a job specification without submitting it.",
            returns: "dict - dict: Validation result with keys: DriverConfigValidated (bool), ValidationErrors (list[str]), Warnings (str)",
          },
          {
            name: "job_plan",
            signature: "job_plan(id, job, diff=False)",
            description: "Dry-run a job registration and return the resulting scheduler plan.",
            returns: "dict - dict: Plan result with keys such as JobModifyIndex (int), Annotations (dict), FailedTGAllocs (dict)",
          },
          {
            name: "jobs_parse",
            signature: "jobs_parse(hcl, canonicalize=False)",
            description: "Convert an HCL job specification into Nomad's JSON job format.",
            returns: "dict - dict: Job specification in Nomad's JSON job format, suitable for job_register(), job_validate(), or job_plan()",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.package",
    description: "Scriptling Package Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "names",
        signature: "names()",
        description: "List all loaded package names.",
        returns: "list[str] - list of str: The manifest name of each loaded package.",
      },
      {
        name: "version",
        signature: "version(name)",
        description: "Get the version of a loaded package.",
        returns: "str - Version string (e.g. \"1.0.0\").",
      },
      {
        name: "exists",
        signature: "exists(name)",
        description: "Check if a package is loaded.",
        returns: "bool - True if the package is loaded.",
      },
      {
        name: "file_exists",
        signature: "file_exists(name, path)",
        description: "Check if a file exists in a package.",
        returns: "bool - True if the file exists in the package.",
      },
      {
        name: "read_file",
        signature: "read_file(name, path)",
        description: "Read a file from a package.",
        returns: "str - File contents as a string. Use read_bytes() for binary files.",
      },
      {
        name: "read_bytes",
        signature: "read_bytes(name, path)",
        description: "Read a file from a package as bytes (preserves binary data).",
        returns: "bytes - File contents as bytes.",
      },
      {
        name: "list",
        signature: "list(name, path)",
        description: "List files in a directory within a package.",
        returns: "list[str] - list of str: File and directory names (directories end with /).",
      },
      {
        name: "glob",
        signature: "glob(name, pattern)",
        description: "Find files matching a glob pattern in a package.",
        returns: "list[str] - list of str: Matching file paths relative to the package root.",
      },
    ],
  },
  {
    module: "scriptling.plugin",
    description: "Scriptling plugin control library stubs.",
    functions: [
      {
        name: "list",
        signature: "list()",
        description: "Return metadata for all loaded executables (discovered + runtime-loaded).",
        returns: "list[dict[str, Any]]",
      },
      {
        name: "describe",
        signature: "describe(name)",
        description: "Return metadata for one loaded plugin.",
        returns: "dict[str, Any]",
      },
      {
        name: "call_function",
        signature: "call_function(library, name, *args, **kwargs)",
        description: "Call a function on a loaded executable.",
        returns: "Any",
      },
      {
        name: "batch_call",
        signature: "batch_call(library, calls)",
        description: "Call multiple functions on one executable in a JSON-RPC batch.",
        returns: "list[Any]",
      },
      {
        name: "call_method",
        signature: "call_method(obj, name, *args, **kwargs)",
        description: "Call a method on a remote plugin object.",
        returns: "Any",
      },
      {
        name: "_new_object",
        signature: "_new_object(library, class_name, *args, **kwargs)",
        description: "Internal: construct a remote plugin object. Used by generated wrappers.",
        returns: "Any",
      },
      {
        name: "release",
        signature: "release(obj)",
        description: "Explicitly release a remote plugin object.",
        returns: "None",
      },
      {
        name: "load",
        signature: "load(name, path, scriptling=False, args=None, insecure_skip_tls=False, headers=None)",
        description: "Register a JSON-RPC peer under ``name``.",
        returns: "str - The normalised library name (e.g. \"plugin.widgets\"). The short form (\"widgets\") may be used with :func:`call_function`, :func:`describe`, and :func:`unload`.",
      },
      {
        name: "unload",
        signature: "unload(name)",
        description: "Close a loaded executable and remove it from the registry.",
        returns: "None",
      },
    ],
    classes: [
      {
        name: "BatchCall",
        description: "One call entry for batch_call().",
        properties: [
        {
          name: "name",
          description: "str",
        },
        {
          name: "args",
          description: "list[Any]",
        },
        {
          name: "kwargs",
          description: "dict[str, Any]",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.provision",
    description: "Scriptling Provision Library - Type stubs for IntelliSense support.",
  },
  {
    module: "scriptling.provision.fetch",
    description: "Scriptling Provision Fetch Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "file",
        signature: "file(url, dest, insecure=False, unpack_zip=False, timeout=30, max_bytes=0, mode=0o644, dir_mode=0o755, provides=None)",
        description: "Fetch a file over HTTP or HTTPS.",
        returns: "dict[str, Any] - A dict with status, url, path, bytes, unpacked, and files keys. status is fetch.CREATED, fetch.UPDATED, or fetch.UNCHANGED.",
      },
    ],
    constants: [
      {
        name: "CREATED",
        description: "str",
      },
      {
        name: "UPDATED",
        description: "str",
      },
      {
        name: "UNCHANGED",
        description: "str",
      },
    ],
  },
  {
    module: "scriptling.provision.file",
    description: "Scriptling Provision File Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "ensure",
        signature: "ensure(path, content, mode=0o644, create_only=False)",
        description: "Ensure a file exists with the given content.",
        returns: "str - file.CREATED, file.UPDATED, or file.UNCHANGED",
      },
      {
        name: "absent",
        signature: "absent(path)",
        description: "Ensure a file does not exist.",
        returns: "str - file.REMOVED if the file was deleted, file.ABSENT if the file did not exist",
      },
      {
        name: "ensure_directory",
        signature: "ensure_directory(path, mode=0o755)",
        description: "Ensure a directory exists.",
        returns: "str - file.CREATED if the directory was newly created, file.EXISTS if the directory already existed",
      },
      {
        name: "absent_directory",
        signature: "absent_directory(path)",
        description: "Ensure an empty directory does not exist.",
        returns: "str - file.REMOVED if the directory was deleted, file.ABSENT if the directory did not exist",
      },
      {
        name: "ensure_block",
        signature: "ensure_block(path, content, id=\"managed\", comment=\"#\", position=\"end\", insert_after=\"\", mode=0o644, create_only=False)",
        description: "Maintain a marker-delimited block within a file.",
        returns: "str - file.CREATED, file.UPDATED, or file.UNCHANGED",
      },
      {
        name: "absent_block",
        signature: "absent_block(path, id=\"managed\", comment=\"#\")",
        description: "Remove a managed block.",
        returns: "str - file.REMOVED if the block was deleted, file.UNCHANGED if the block was not present",
      },
    ],
    constants: [
      {
        name: "CREATED",
        description: "str",
      },
      {
        name: "UPDATED",
        description: "str",
      },
      {
        name: "UNCHANGED",
        description: "str",
      },
      {
        name: "REMOVED",
        description: "str",
      },
      {
        name: "ABSENT",
        description: "str",
      },
      {
        name: "EXISTS",
        description: "str",
      },
    ],
  },
  {
    module: "scriptling.random",
    description: "Scriptling Random Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "seed",
        signature: "seed(a=None)",
        description: "Initialize the random number generator.",
        returns: "None - None",
      },
      {
        name: "randint",
        signature: "randint(a, b)",
        description: "Return a random integer N such that a <= N <= b.",
        returns: "int - Random integer in [a, b]",
      },
      {
        name: "randrange",
        signature: "randrange(start, stop=None, step=None)",
        description: "Return a randomly selected element from range(start, stop, step).",
        returns: "int - Random integer from the range",
      },
      {
        name: "random",
        signature: "random()",
        description: "Return a random float in the range [0.0, 1.0).",
        returns: "float - Random float between 0.0 and 1.0",
      },
      {
        name: "uniform",
        signature: "uniform(a, b)",
        description: "Return a random float N such that a <= N <= b.",
        returns: "float - Random float in [a, b]",
      },
      {
        name: "gauss",
        signature: "gauss(mu, sigma)",
        description: "Return a random number from a Gaussian distribution.",
        returns: "float - Random float from Gaussian distribution",
      },
      {
        name: "normalvariate",
        signature: "normalvariate(mu, sigma)",
        description: "Return a random number from a normal distribution. Same as gauss() but provided for compatibility.",
        returns: "float - Random float from normal distribution",
      },
      {
        name: "expovariate",
        signature: "expovariate(lambd)",
        description: "Return a random number from an exponential distribution.",
        returns: "float - Random float from exponential distribution",
      },
      {
        name: "betavariate",
        signature: "betavariate(alpha, beta)",
        description: "Return a random number from a beta distribution.",
        returns: "float - Random float in [0, 1]",
      },
      {
        name: "gammavariate",
        signature: "gammavariate(alpha, beta)",
        description: "Return a random number from a gamma distribution.",
        returns: "float - Random float from gamma distribution",
      },
      {
        name: "triangular",
        signature: "triangular(low, high, mode=None)",
        description: "Return a random number from a triangular distribution.",
        returns: "float - Random float from triangular distribution",
      },
      {
        name: "paretovariate",
        signature: "paretovariate(alpha)",
        description: "Return a random number from a Pareto distribution.",
        returns: "float - Random float from Pareto distribution",
      },
      {
        name: "weibullvariate",
        signature: "weibullvariate(alpha, beta)",
        description: "Return a random number from a Weibull distribution.",
        returns: "float - Random float from Weibull distribution",
      },
      {
        name: "choice",
        signature: "choice(seq)",
        description: "Return a random element from a sequence.",
        returns: "object - Random element from the sequence",
      },
      {
        name: "shuffle",
        signature: "shuffle(list)",
        description: "Shuffle a list in place using the Fisher-Yates algorithm.",
        returns: "None - None",
      },
      {
        name: "sample",
        signature: "sample(population, k)",
        description: "Return k unique random elements from population.",
        returns: "List - List of k unique elements",
      },
      {
        name: "choices",
        signature: "choices(population, weights=None, k=1)",
        description: "Weighted random sampling with replacement.",
        returns: "List - List of k selected items",
      },
    ],
  },
  {
    module: "scriptling.runtime",
    description: "Scriptling Runtime Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "start_server",
        signature: "start_server(wait=True)",
        description: "Signal the server to start accepting requests.",
        returns: "None",
      },
      {
        name: "server_running",
        signature: "server_running()",
        description: "Returns True while the server is running.",
        returns: "bool",
      },
      {
        name: "background",
        signature: "background(name, handler, *args, **kwargs)",
        description: "Register and start a background task.",
        returns: "Promise - Promise object (in script mode) or None (in server mode)",
      },
    ],
    classes: [
      {
        name: "Promise",
        description: "Promise object representing an async operation result.",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Wait for and return the result.",
            returns: "Any - The result of the background task",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Wait for completion and discard the result.",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.http",
    description: "Scriptling Runtime HTTP Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "get",
        signature: "get(path, handler=...)",
        description: "Register a GET route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "post",
        signature: "post(path, handler=...)",
        description: "Register a POST route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "put",
        signature: "put(path, handler=...)",
        description: "Register a PUT route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "patch",
        signature: "patch(path, handler=...)",
        description: "Register a PATCH route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "delete",
        signature: "delete(path, handler=...)",
        description: "Register a DELETE route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "route",
        signature: "route(path, handler=..., methods=...)",
        description: "Register a route for multiple methods, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "middleware",
        signature: "middleware(handler)",
        description: "Register middleware for all routes and protocol endpoints, or use as bare decorator.",
        returns: "F",
      },
      {
        name: "static",
        signature: "static(path, directory)",
        description: "Register a static file serving route.",
        returns: "None",
      },
      {
        name: "json",
        signature: "json(status_code, data)",
        description: "Create a JSON response.",
        returns: "dict[str, Any] - Response object for the server",
      },
      {
        name: "redirect",
        signature: "redirect(location, status=302)",
        description: "Create a redirect response.",
        returns: "dict[str, Any] - Response object for the server",
      },
      {
        name: "html",
        signature: "html(status_code, content)",
        description: "Create an HTML response.",
        returns: "dict[str, Any] - Response object for the server",
      },
      {
        name: "text",
        signature: "text(status_code, content)",
        description: "Create a plain text response.",
        returns: "dict[str, Any] - Response object for the server",
      },
      {
        name: "parse_query",
        signature: "parse_query(query_string)",
        description: "Parse a URL query string.",
        returns: "dict[str, Any] - Parsed key-value pairs",
      },
      {
        name: "not_found",
        signature: "not_found(handler)",
        description: "Register a custom 404 Not Found handler, or use as bare decorator.",
        returns: "F",
      },
      {
        name: "websocket",
        signature: "websocket(path, handler=...)",
        description: "Register a WebSocket route, or use as decorator.",
        returns: "Callable[[F], F]",
      },
    ],
    classes: [
      {
        name: "Request",
        description: "HTTP request object passed to route handlers.",
        methods: [
          {
            name: "path_param",
            signature: "path_param(name, default=None)",
            description: "Get a path parameter captured from a route wildcard.",
            returns: "Any - The captured value, default, or None",
          },
          {
            name: "query_param",
            signature: "query_param(name, default=None)",
            description: "Get a query parameter.",
            returns: "Any - The first value of the parameter, default, or None",
          },
          {
            name: "header",
            signature: "header(name, default=None)",
            description: "Get a request header. Header names are case-insensitive.",
            returns: "Any - The header value, default, or None",
          },
          {
            name: "json",
            signature: "json()",
            description: "Parse request body as JSON.",
            returns: "Any - Parsed JSON as dict or list, or None if body is empty",
          },
        ],
        properties: [
        {
          name: "method",
          description: "str",
        },
        {
          name: "path",
          description: "str",
        },
        {
          name: "body",
          description: "str",
        },
        {
          name: "headers",
          description: "dict[str, str]",
        },
        {
          name: "query",
          description: "dict[str, str]",
        },
        {
          name: "path_params",
          description: "dict[str, str]",
        },
        {
          name: "remote_addr",
          description: "str",
        },
        {
          name: "context",
          description: "dict[str, Any]",
        },
        ],
      },
      {
        name: "WebSocketClient",
        description: "WebSocket client connection passed to server-side WebSocket handlers.",
        methods: [
          {
            name: "send",
            signature: "send(message)",
            description: "Send a text message to the client.",
            returns: "Exception - None on success, or an error/exception if send fails",
          },
          {
            name: "send_binary",
            signature: "send_binary(data)",
            description: "Send binary data to the client.",
            returns: "Exception - None on success, or an error/exception if send fails",
          },
          {
            name: "receive",
            signature: "receive(timeout=30)",
            description: "Receive a message from the client.",
            returns: "Any - The received message (str for text, list for binary), or None if timeout or connection closed",
          },
          {
            name: "connected",
            signature: "connected()",
            description: "Check if the client connection is still open.",
            returns: "bool - True if connected, False otherwise",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the client connection.",
            returns: "None",
          },
        ],
        properties: [
        {
          name: "remote_addr",
          description: "str",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.jsonrpc",
    description: "Scriptling Runtime JSON-RPC Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "method",
        signature: "method(name, handler=...)",
        description: "Register a JSON-RPC method handler, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "notification",
        signature: "notification(name, handler=...)",
        description: "Register a JSON-RPC notification handler, or use as decorator.",
        returns: "Callable[[F], F]",
      },
      {
        name: "error",
        signature: "error(code, message, data=None)",
        description: "Build a structured JSON-RPC error response.",
        returns: "JSONRPCError - A JSONRPCError instance; return it from a method handler to emit a JSON-RPC error response with a custom code.",
      },
      {
        name: "get_request",
        signature: "get_request()",
        description: "Get the HTTP request this call is being served for.",
        returns: "Request",
      },
      {
        name: "request_context",
        signature: "request_context()",
        description: "Get the context dict set by the middleware.",
        returns: "dict[str, Any]",
      },
      {
        name: "transport",
        signature: "transport()",
        description: "How the JSON-RPC server is being served: \"http\", \"stdio\" or None.",
        returns: "str",
      },
    ],
    classes: [
      {
        name: "JSONRPCError",
        description: "JSON-RPC error object produced by runtime.jsonrpc.error().",
        properties: [
        {
          name: "code",
          description: "int",
        },
        {
          name: "message",
          description: "str",
        },
        {
          name: "data",
          description: "Any",
        },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.kv",
    description: "Scriptling Runtime KV Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "open",
        signature: "open(name)",
        description: "Open or reuse a named KV store.",
        returns: "Storage - KV store object with get, set, delete, exists, ttl, keys, clear, close methods.",
      },
    ],
    classes: [
      {
        name: "Storage",
        description: "KV store object with methods for key-value operations.",
        methods: [
          {
            name: "set",
            signature: "set(key, value, ttl=0)",
            description: "Store a value with optional TTL in seconds.",
            returns: "None",
          },
          {
            name: "get",
            signature: "get(key, default=None)",
            description: "Retrieve a value by key.",
            returns: "Any - The stored value, or the default if not found",
          },
          {
            name: "incr",
            signature: "incr(key, delta=1)",
            description: "Atomically increment an integer value, returns new value.",
            returns: "int - New integer value after increment",
          },
          {
            name: "delete",
            signature: "delete(key)",
            description: "Remove a key from the store.",
            returns: "None",
          },
          {
            name: "exists",
            signature: "exists(key)",
            description: "Check if a key exists and is not expired.",
            returns: "bool - True if key exists and is not expired",
          },
          {
            name: "ttl",
            signature: "ttl(key)",
            description: "Get remaining TTL in seconds.",
            returns: "int - Remaining TTL in seconds, -1 if no expiration, -2 if key doesn't exist",
          },
          {
            name: "keys",
            signature: "keys(pattern=\"*\")",
            description: "Get all keys matching a glob pattern.",
            returns: "list[str] - List of matching keys",
          },
          {
            name: "clear",
            signature: "clear()",
            description: "Remove all keys from the store.",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Release this store. No-op on the default store.",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.mcp",
    description: "Scriptling Runtime MCP Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "register_request_tool",
        signature: "register_request_tool(name, handler, description=\"\", params=None, keywords=None, discoverable=False)",
        description: "Register an MCP tool for this request.",
        returns: "None",
      },
      {
        name: "register_request_resource",
        signature: "register_request_resource(uri, handler, name, description=\"\", mime_type=\"\", template=False)",
        description: "Register an MCP resource for this request.",
        returns: "None",
      },
      {
        name: "register_request_prompt",
        signature: "register_request_prompt(name, handler, description=\"\", arguments=None)",
        description: "Register an MCP prompt for this request.",
        returns: "None",
      },
      {
        name: "transport",
        signature: "transport()",
        description: "How the MCP server is being served: \"http\", \"stdio\" or None.",
        returns: "str",
      },
      {
        name: "tool",
        signature: "tool(description, params=None, keywords=None, discoverable=False)",
        description: "Decorator for MCP tools.",
        returns: "Callable[[Callable[..., Any]], Callable[..., Any]]",
      },
    ],
  },
  {
    module: "scriptling.runtime.plugin",
    description: "Scriptling Runtime Plugin Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "serve",
        signature: "serve(name, version=\"\", description=\"\")",
        description: "Declare this script as a Scriptling plugin server.",
        returns: "None",
      },
      {
        name: "register_function",
        signature: "register_function(name, handler=...)",
        description: "Register a function for the plugin server, or use as decorator.",
        returns: "Any",
      },
      {
        name: "register_constant",
        signature: "register_constant(name, value)",
        description: "Register a constant exported by the plugin server.",
        returns: "None",
      },
      {
        name: "register_class",
        signature: "register_class(handler)",
        description: "Register a class exported by the plugin server, or use as bare decorator.",
        returns: "Any",
      },
    ],
  },
  {
    module: "scriptling.runtime.sandbox",
    description: "Scriptling Runtime Sandbox Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "create",
        signature: "create(capture_output=False)",
        description: "Create a new isolated sandbox environment.",
        returns: "Sandbox - Sandbox object with set(), get(), exec(), exec_file(), and exit_code() methods",
      },
    ],
    classes: [
      {
        name: "Sandbox",
        description: "Isolated script execution environment.",
        methods: [
          {
            name: "set",
            signature: "set(name, value)",
            description: "Set a variable in the sandbox.",
            returns: "None",
          },
          {
            name: "get",
            signature: "get(name)",
            description: "Get a variable from the sandbox.",
            returns: "Any - Variable value, or None if not found",
          },
          {
            name: "exec",
            signature: "exec(code)",
            description: "Execute script code in the sandbox.",
            returns: "None",
          },
          {
            name: "exec_file",
            signature: "exec_file(path)",
            description: "Load and execute a script file in the sandbox.",
            returns: "None",
          },
          {
            name: "exit_code",
            signature: "exit_code()",
            description: "Get the exit code from the last execution.",
            returns: "int - Exit code (0 = success, non-zero = error)",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.runtime.sync",
    description: "Scriptling Runtime Sync Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "WaitGroup",
        signature: "WaitGroup(name)",
        description: "Get or create a named wait group.",
        returns: "WaitGroup - WaitGroup instance",
      },
      {
        name: "Queue",
        signature: "Queue(name, maxsize=0)",
        description: "Get or create a named queue.",
        returns: "Queue - Queue instance",
      },
      {
        name: "Atomic",
        signature: "Atomic(name, initial=0)",
        description: "Get or create a named atomic counter.",
        returns: "Atomic - Atomic instance",
      },
      {
        name: "Shared",
        signature: "Shared(name, initial=None)",
        description: "Get or create a named shared variable.",
        returns: "Shared - Shared instance",
      },
    ],
    classes: [
      {
        name: "WaitGroup",
        description: "Go-style synchronization primitive.",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Add to the wait group counter.",
            returns: "None",
          },
          {
            name: "done",
            signature: "done()",
            description: "Decrement the wait group counter.",
            returns: "None",
          },
          {
            name: "wait",
            signature: "wait()",
            description: "Block until counter reaches zero.",
            returns: "None",
          },
        ],
      },
      {
        name: "Queue",
        description: "Thread-safe queue for producer-consumer patterns.",
        methods: [
          {
            name: "put",
            signature: "put(item)",
            description: "Add item to queue (blocks if full, respects context timeout).",
            returns: "None",
          },
          {
            name: "get",
            signature: "get()",
            description: "Remove and return item from queue (blocks if empty, respects context timeout).",
            returns: "Any - The next item from the queue",
          },
          {
            name: "size",
            signature: "size()",
            description: "Return number of items in queue.",
            returns: "int - Current queue size",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the queue.",
            returns: "None",
          },
        ],
      },
      {
        name: "Atomic",
        description: "Atomic integer with lock-free operations.",
        methods: [
          {
            name: "add",
            signature: "add(delta=1)",
            description: "Atomically add delta and return new value.",
            returns: "int - New value after addition",
          },
          {
            name: "get",
            signature: "get()",
            description: "Atomically read the value.",
            returns: "int - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Atomically set the value.",
            returns: "None",
          },
        ],
      },
      {
        name: "Shared",
        description: "Shared variable with thread-safe access.",
        methods: [
          {
            name: "get",
            signature: "get()",
            description: "Get the current value (thread-safe read).",
            returns: "Any - Current value",
          },
          {
            name: "set",
            signature: "set(value)",
            description: "Set the value (thread-safe write).",
            returns: "None",
          },
          {
            name: "update",
            signature: "update(fn)",
            description: "Atomically read-modify-write.",
            returns: "Any - The new value after update",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.secret",
    description: "Secret Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "get",
        signature: "get(alias, path, field=\"\")",
        description: "Resolve a secret through a host-configured provider alias.",
        returns: "str - Resolved secret value as a string",
      },
      {
        name: "list",
        signature: "list(alias, path)",
        description: "List keys at a path through a host-configured provider alias.",
        returns: "list[str] - List of key or item name strings",
      },
    ],
  },
  {
    module: "scriptling.sed",
    description: "Scriptling Sed Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "replace",
        signature: "replace(old, new, path, recursive=False, ignore_case=False, glob=\"\", follow_links=False, max_size=1048576)",
        description: "Replace all occurrences of a literal string in a file or directory.",
        returns: "int - Number of files modified (int)",
      },
      {
        name: "replace_pattern",
        signature: "replace_pattern(regex, new, path, recursive=False, ignore_case=False, glob=\"\", follow_links=False, max_size=1048576)",
        description: "Replace all regex matches in a file or directory.",
        returns: "int - Number of files modified (int)",
      },
      {
        name: "extract",
        signature: "extract(regex, path, recursive=False, ignore_case=False, glob=\"\", follow_links=False, max_size=1048576)",
        description: "Extract regex capture groups from a file or directory.",
        returns: "list[dict] - List of match dicts, each with: - file (str): Path to the matched file - line (int): 1-based line number of the match - text (str): Full content of the matched line - groups (list[str]): Captured group strings (empty if no capture groups)",
      },
    ],
  },
  {
    module: "scriptling.similarity",
    description: "Scriptling Similarity Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "search",
        signature: "search(query, items, max_results=5, threshold=0.5, key=\"name\")",
        description: "Search for fuzzy matches in a list of items.",
        returns: "list[dict[str, Any]] - List of match dictionaries, each with: - id: The matched item's ID - name: The matched item's name - score: Match score (0.0 to 1.0, higher is better)",
      },
      {
        name: "best",
        signature: "best(query, items, entity_type=\"item\", key=\"name\", threshold=0.5)",
        description: "Find the best match with error formatting.",
        returns: "dict[str, Any] - Dictionary with: - found (bool): True if a match was found - id (int or None): The matched item's ID - name (str or None): The matched item's name - score (float): Match score (0 if not found) - error (str or None): Error message with suggestions if not found",
      },
      {
        name: "score",
        signature: "score(s1, s2)",
        description: "Calculate similarity score between two strings.",
        returns: "float - Similarity score (0.0 to 1.0)",
      },
      {
        name: "tokenize",
        signature: "tokenize(text)",
        description: "Split text into lowercase alphanumeric tokens.",
        returns: "list[str] - List of lowercase tokens",
      },
      {
        name: "minhash",
        signature: "minhash(text, num_hashes=64)",
        description: "Compute a MinHash signature for text.",
        returns: "list[int] - MinHash signature (list of integers)",
      },
      {
        name: "minhash_similarity",
        signature: "minhash_similarity(a, b)",
        description: "Compare two MinHash signatures.",
        returns: "float - Similarity score between 0.0 and 1.0",
      },
      {
        name: "cosine_similarity",
        signature: "cosine_similarity(a, b)",
        description: "Compare two numeric vectors using cosine similarity.",
        returns: "float - Cosine similarity score from -1.0 to 1.0",
      },
      {
        name: "most_similar",
        signature: "most_similar(query, vectors, top_k=5)",
        description: "Rank vectors by cosine similarity to a query vector.",
        returns: "list[dict] - List of dicts sorted by descending score: [{\"index\": int, \"score\": float}, ...]",
      },
      {
        name: "vectorize",
        signature: "vectorize(text, dims=256)",
        description: "Generate a vector from text using the feature-hashing trick (CPU-only).",
        returns: "list[float] - Normalised vector of length dims.",
      },
    ],
  },
  {
    module: "scriptling.template.html",
    description: "Scriptling Template HTML Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "Set",
        signature: "Set(left=\"{{\", right=\"}}\")",
        description: "Create a new HTML template set (uses html/template with auto-escaping).",
        returns: "Set - Set: A template set",
      },
    ],
    classes: [
      {
        name: "Set",
        description: "An HTML template set. Add template sources with add(), render with render(). Values are automatically HTML-escaped when rendered.",
        methods: [
          {
            name: "add",
            signature: "add(source)",
            description: "Add a template source to the set.",
            returns: "None",
          },
          {
            name: "render",
            signature: "render(*args)",
            description: "Render a template from the set.",
            returns: "str - Rendered HTML string (values are auto-escaped)",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.template.text",
    description: "Scriptling Template Text Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "Set",
        signature: "Set(left=\"{{\", right=\"}}\")",
        description: "Create a new text template set (uses text/template, no HTML escaping).",
        returns: "Set - Set: A template set",
      },
    ],
    classes: [
      {
        name: "Set",
        description: "A text template set. Add template sources with add(), render with render(). No HTML escaping is applied.",
        methods: [
          {
            name: "add",
            signature: "add(source)",
            description: "Add a template source to the set.",
            returns: "None",
          },
          {
            name: "render",
            signature: "render(*args)",
            description: "Render a template from the set.",
            returns: "str - Rendered string",
          },
        ],
      },
    ],
  },
  {
    module: "scriptling.toon",
    description: "Scriptling TOON Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "encode",
        signature: "encode(data)",
        description: "Encode data to TOON format.",
        returns: "str - TOON formatted string",
      },
      {
        name: "decode",
        signature: "decode(text)",
        description: "Decode TOON format to scriptling objects.",
        returns: "Any - Decoded scriptling value",
      },
      {
        name: "encode_options",
        signature: "encode_options(data, indent, delimiter)",
        description: "Encode data to TOON format with custom options.",
        returns: "str - TOON formatted string",
      },
      {
        name: "decode_options",
        signature: "decode_options(text, strict, indent_size)",
        description: "Decode TOON format with custom options.",
        returns: "Any - Decoded scriptling value",
      },
    ],
  },
  {
    module: "scriptling.wait_for",
    description: "Wait For Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "file",
        signature: "file(path, timeout=30, poll_rate=1.0)",
        description: "Wait for a file to exist.",
        returns: "bool - True if file exists, False if timeout exceeded",
      },
      {
        name: "dir",
        signature: "dir(path, timeout=30, poll_rate=1.0)",
        description: "Wait for a directory to exist.",
        returns: "bool - True if directory exists, False if timeout exceeded",
      },
      {
        name: "port",
        signature: "port(host, port, timeout=30, poll_rate=1.0)",
        description: "Wait for a TCP port to be open.",
        returns: "bool - True if port is open, False if timeout exceeded",
      },
      {
        name: "http",
        signature: "http(url, timeout=30, poll_rate=1.0, status_code=200)",
        description: "Wait for HTTP endpoint.",
        returns: "bool - True if endpoint responds with expected status, False if timeout exceeded",
      },
      {
        name: "file_content",
        signature: "file_content(path, content, timeout=30, poll_rate=1.0)",
        description: "Wait for file to contain content.",
        returns: "bool - True if file contains the content, False if timeout exceeded",
      },
      {
        name: "process_name",
        signature: "process_name(name, timeout=30, poll_rate=1.0)",
        description: "Wait for a process to be running.",
        returns: "bool - True if process is running, False if timeout exceeded",
      },
    ],
  },
  {
    module: "scriptling.xml",
    description: "Scriptling XML Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "loads",
        signature: "loads(content)",
        description: "Parse an XML string into a nested dict.",
        returns: "dict - Nested dict representing the XML document.",
      },
      {
        name: "dumps",
        signature: "dumps(data, indent=\"\")",
        description: "Format a dict into an XML string.",
        returns: "str - XML-formatted text.",
      },
    ],
  },
  {
    module: "shlex",
    description: "Scriptling shlex Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "quote",
        signature: "quote(s)",
        description: "Escape a string for use as a single shell argument.",
        returns: "str - A shell-safe string. Safe characters are returned unchanged; everything else is wrapped in single quotes.",
      },
      {
        name: "split",
        signature: "split(s)",
        description: "Split a string into shell-style tokens.",
        returns: "list[str] - List of parsed tokens.",
      },
      {
        name: "join",
        signature: "join(split_command)",
        description: "Join a list of arguments into a shell-quoted string.",
        returns: "str - A single shell-safe command-line string.",
      },
    ],
  },
  {
    module: "shutil",
    description: "Scriptling shutil Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "copy",
        signature: "copy(src, dst)",
        description: "Copy a file or directory tree. File modes are preserved.",
        returns: "str - The destination path.",
      },
      {
        name: "copy2",
        signature: "copy2(src, dst)",
        description: "Copy a file with metadata. Identical to copy() — file mode is always preserved. Provided for Python compatibility.",
        returns: "str - The destination path.",
      },
      {
        name: "copytree",
        signature: "copytree(src, dst)",
        description: "Recursively copy a directory tree. File modes are preserved.",
        returns: "str - The destination path.",
      },
      {
        name: "rmtree",
        signature: "rmtree(path)",
        description: "Recursively delete a directory tree. Unlike os.removedirs, the directory does not need to be empty.",
        returns: "None",
      },
      {
        name: "move",
        signature: "move(src, dst)",
        description: "Move or rename a file or directory (same as os.rename).",
        returns: "str - The destination path.",
      },
      {
        name: "disk_usage",
        signature: "disk_usage(path)",
        description: "Return disk usage statistics for the file system containing path.",
        returns: "dict - Dict with keys: - total (int): Total space in bytes - used (int): Used space in bytes (includes reserved blocks) - free (int): Free space available to non-privileged users",
      },
    ],
  },
  {
    module: "sys",
    description: "Scriptling sys library stubs.",
    functions: [
      {
        name: "exit",
        signature: "exit(arg=0)",
        description: "Exit the interpreter. An int sets the exit code; a string exits with code 1 and that message. Cannot be caught by try/except (finally blocks still run).",
        returns: "NoReturn",
      },
    ],
    classes: [
      {
        name: "StdinStream",
        description: "Standard input stream (sys.stdin). Iterating yields lines.",
        methods: [
          {
            name: "read",
            signature: "read()",
            description: "Read all remaining data from stdin.",
            returns: "str",
          },
          {
            name: "readline",
            signature: "readline()",
            description: "Read one line from stdin, including the trailing newline.",
            returns: "str",
          },
          {
            name: "__iter__",
            signature: "__iter__()",
            description: "Iterate over lines of stdin.",
            returns: "Iterator[str]",
          },
        ],
      },
      {
        name: "OutputStream",
        description: "Output stream (sys.stdout / sys.stderr).",
        methods: [
          {
            name: "write",
            signature: "write(s)",
            description: "Write string s to the stream; returns the number of characters written.",
            returns: "int",
          },
          {
            name: "writelines",
            signature: "writelines(lines)",
            description: "Write each string in lines to the stream; no separators are added.",
            returns: "None",
          },
          {
            name: "flush",
            signature: "flush()",
            description: "Flush the write buffer; a no-op for unbuffered streams.",
            returns: "None",
          },
          {
            name: "isatty",
            signature: "isatty()",
            description: "Return True if the stream is a terminal.",
            returns: "bool",
          },
          {
            name: "__enter__",
            signature: "__enter__()",
            description: "Return the stream itself for use in a with statement.",
            returns: "OutputStream",
          },
          {
            name: "__exit__",
            signature: "__exit__(*args)",
            description: "Flush the stream; never suppresses exceptions.",
            returns: "bool",
          },
        ],
      },
    ],
  },
  {
    module: "tarfile",
    description: "Scriptling tarfile Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "is_tarfile",
        signature: "is_tarfile(path)",
        description: "Return True if path is a valid TAR archive.",
        returns: "bool",
      },
    ],
    classes: [
      {
        name: "TarFile",
        description: "",
        methods: [
          {
            name: "__init__",
            signature: "__init__(path, mode=\"r\")",
            description: "Open a TAR archive.",
            returns: "None",
          },
          {
            name: "getnames",
            signature: "getnames()",
            description: "Return a list of archive member names.",
            returns: "list[str]",
          },
          {
            name: "read",
            signature: "read(name)",
            description: "Read a member from the archive as a string.",
            returns: "str",
          },
          {
            name: "extract",
            signature: "extract(member, path=\".\")",
            description: "Extract a single member to path. Returns the extracted file path.",
            returns: "str",
          },
          {
            name: "extractall",
            signature: "extractall(path=\".\")",
            description: "Extract all members to path. Returns list of extracted file paths.",
            returns: "list[str]",
          },
          {
            name: "add",
            signature: "add(filename, arcname=\"\")",
            description: "Add a file to the archive (write mode only).",
            returns: "None",
          },
          {
            name: "addstr",
            signature: "addstr(name, data)",
            description: "Write a string as a member in the archive (write mode only).",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the archive.",
            returns: "None",
          },
        ],
      },
    ],
  },
  {
    module: "tempfile",
    description: "Scriptling tempfile Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "mkstemp",
        signature: "mkstemp(suffix=\"\", prefix=\"tmp\", dir=None)",
        description: "Create a temporary file and return its path.",
        returns: "str - The absolute path to the created file.",
      },
      {
        name: "mkdtemp",
        signature: "mkdtemp(suffix=\"\", prefix=\"tmp\", dir=None)",
        description: "Create a temporary directory and return its path.",
        returns: "str - The absolute path to the created directory.",
      },
      {
        name: "gettempdir",
        signature: "gettempdir()",
        description: "Return the default temporary directory.",
        returns: "str - The temp directory path.",
      },
      {
        name: "gettempprefix",
        signature: "gettempprefix()",
        description: "Return the default temporary file name prefix.",
        returns: "str - \"tmp\"",
      },
    ],
  },
  {
    module: "zipfile",
    description: "Scriptling zipfile Library - Type stubs for IntelliSense support.",
    functions: [
      {
        name: "is_zipfile",
        signature: "is_zipfile(path)",
        description: "Return True if path is a valid ZIP archive.",
        returns: "bool",
      },
    ],
    classes: [
      {
        name: "ZipFile",
        description: "",
        methods: [
          {
            name: "__init__",
            signature: "__init__(path, mode=\"r\")",
            description: "Open a ZIP archive.",
            returns: "None",
          },
          {
            name: "namelist",
            signature: "namelist()",
            description: "Return a list of archive member names.",
            returns: "list[str]",
          },
          {
            name: "read",
            signature: "read(name)",
            description: "Read a member from the archive as a string.",
            returns: "str",
          },
          {
            name: "extract",
            signature: "extract(member, path=\".\")",
            description: "Extract a single member to path. Returns the extracted file path.",
            returns: "str",
          },
          {
            name: "extractall",
            signature: "extractall(path=\".\")",
            description: "Extract all members to path. Returns list of extracted file paths.",
            returns: "list[str]",
          },
          {
            name: "write",
            signature: "write(filename, arcname=\"\")",
            description: "Add a file to the archive (write mode only).",
            returns: "None",
          },
          {
            name: "writestr",
            signature: "writestr(name, data)",
            description: "Write a string as a member in the archive (write mode only).",
            returns: "None",
          },
          {
            name: "close",
            signature: "close()",
            description: "Close the archive.",
            returns: "None",
          },
        ],
      },
    ],
  },
];

export { scriptlingLibraries };
