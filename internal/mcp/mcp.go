package mcp

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/openai"

	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/discovery"
)

func InitializeMCPServer(routes *http.ServeMux, enableWebEndpoint bool, nativeTools bool) *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)
	server.SetInstructions(`These tools manage spaces, templates, and other resources.

This server uses FULL tool discovery to minimize context usage. ALL tools are discoverable via tool_search.

CRITICAL RULES:
- ALL tools MUST be accessed via tool_search → execute_tool pattern
- NO tools can be called directly
- ALWAYS search for tools first, then execute them

WORKFLOW FOR EVERY OPERATION:
1. tool_search(query="<operation description>")
2. execute_tool(name="<tool_name>", arguments={...})

COMMON OPERATIONS:
- List spaces: tool_search(query="list spaces") → execute_tool(name="list_spaces")
- Start space: tool_search(query="start space") → execute_tool(name="start_space")
- Stop space: tool_search(query="stop space") → execute_tool(name="stop_space")
- Create template: tool_search(query="create template") → find tool → execute_tool
- List users: tool_search(query="list users") → find tool → execute_tool

CRITICAL TEMPLATE CREATION WORKFLOW:
When user asks to create/update a template, you MUST follow this exact sequence:
1. FIRST: tool_search(query="create template") to find the create_template tool
2. SECOND: Call skills(filename='<platform>-spec.md') where platform is nomad, docker, or podman
3. THIRD: Use the specification from step 2 as your guide to construct the job definition
4. FOURTH: execute_tool(name="create_template", arguments={...}) with the properly formatted job

EXAMPLE: For "create a nomad template":
1. tool_search(query="create template") → finds create_template
2. Call skills(filename='nomad-spec.md')
3. Follow the nomad specification format from the response
4. execute_tool(name="create_template", arguments={...})

REMEMBER: NO tools are directly callable. ALWAYS use tool_search → execute_tool for ALL operations.`)

	if enableWebEndpoint {
		// Create handler with request-scoped support for tool discovery
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The authentication middleware has already run and set the user in the context
			// Tool discovery will work with the existing context and user permissions

			// Handle the MCP request with tool discovery support
			server.HandleRequest(w, r)
		})

		// Apply authentication middleware
		routes.HandleFunc("POST /mcp", middleware.ApiAuth(middleware.ApiPermissionUseMCPServer(handler.ServeHTTP)))
	}

	// Create a tool registry for discoverable tools
	var registry *discovery.ToolRegistry
	if !nativeTools {
		registry = discovery.NewToolRegistry()
	}

	// Helper function to register tools either natively or in discovery registry
	registerTool := func(tool *mcp.ToolBuilder, handler mcp.ToolHandler, keywords ...string) {
		if nativeTools {
			server.RegisterTool(tool, handler)
		} else {
			registry.RegisterTool(tool, handler, keywords...)
		}
	}

	// =========================================================================
	// Register ALL tools in discovery registry or natively
	// =========================================================================

	// Spaces - All space operations
	tool := mcp.NewTool("list_spaces", "List all spaces for current user with status and sharing details.",
		mcp.Output(
			mcp.ObjectArray("spaces", "Array of available spaces and their status",
				mcp.String("id", "Space ID"),
				mcp.String("name", "Space name"),
				mcp.String("state", "Space state (stopped, running, pending, deleting)"),
				mcp.String("description", "Space description"),
				mcp.String("note", "Space note"),
				mcp.String("zone", "Zone name"),
				mcp.String("platform", "Platform type"),
				mcp.StringArray("web_ports", "Web port mappings"),
				mcp.StringArray("tcp_ports", "TCP port mappings"),
				mcp.Boolean("ssh", "SSH access available"),
				mcp.Boolean("web_terminal", "Web terminal access available"),
				mcp.ObjectArray("custom_fields", "The list of custom fields and their values",
					mcp.String("name", "Custom field name"),
					mcp.String("value", "Custom field value"),
				),
				mcp.Object("shared_with", "User ID the space is shared with if any",
					mcp.String("user_id", "User ID"),
					mcp.String("username", "Username"),
					mcp.String("email", "Email"),
				),
			),
		),
	)
	registerTool(tool, listSpaces, "spaces", "list", "environments", "development", "deploy", "instances")
	tool = mcp.NewTool("start_space", "Start a space.",
		mcp.String("space_name", "Name of the space to start", mcp.Required()),
	)
	registerTool(tool, startSpace, "start", "space", "run", "launch", "activate", "environment")

	tool = mcp.NewTool("stop_space", "Stop a space.",
		mcp.String("space_name", "Name of the space to stop", mcp.Required()),
	)
	registerTool(tool, stopSpace, "stop", "space", "shutdown", "halt", "deactivate", "environment")

	tool = mcp.NewTool("restart_space", "Restart a space.",
		mcp.String("space_name", "Name of the space to restart", mcp.Required()),
	)
	registerTool(tool, restartSpace, "restart", "space", "reboot", "reload", "environment")

	// Groups
	tool = mcp.NewTool("list_groups", "List all user groups. Use to find group IDs for template restrictions and user management.",
		mcp.Output(
			mcp.ObjectArray("groups", "Array of available groups",
				mcp.String("id", "Group ID"),
				mcp.String("name", "Group name"),
				mcp.Number("max_spaces", "Maximum number of spaces"),
				mcp.Number("compute_units", "Maximum compute units"),
				mcp.Number("storage_units", "Maximum storage units"),
				mcp.Number("max_tunnels", "Maximum number of tunnels"),
			),
		),
	)
	registerTool(tool, listGroups, "groups", "users", "permissions", "restrictions", "rbac", "access", "team", "organization")

	// Templates
	tool = mcp.NewTool("list_templates", "List all space templates. Use to find template IDs or check existing templates.",
		mcp.Boolean("show_all", "Show template from all zones (default: false)"),
		mcp.Boolean("show_inactive", "Show inactive templates (default: false)"),
		mcp.Output(
			mcp.ObjectArray("templates", "Array of available templates",
				mcp.String("id", "Template ID"),
				mcp.String("name", "Template name"),
				mcp.String("description", "Template description"),
				mcp.String("platform", "Platform type"),
				mcp.ObjectArray("groups", "The list of groups that can use this template",
					mcp.String("id", "Group ID"),
					mcp.String("name", "Group name"),
				),
				mcp.Number("compute_units", "Compute units required"),
				mcp.Number("storage_units", "Storage units required"),
				mcp.Boolean("schedule_enabled", "Schedule restrictions enabled"),
				mcp.Boolean("is_managed", "Is managed template"),
				mcp.String("schedule", "Schedule configuration"),
				mcp.StringArray("zones", "Zone names where template appears"),
				mcp.ObjectArray("custom_fields", "The list of custom field definitions",
					mcp.String("name", "Field name"),
					mcp.String("description", "Field description"),
				),
				mcp.Number("max_uptime", "Maximum uptime"),
				mcp.String("max_uptime_unit", "Maximum uptime unit"),
			),
		),
	)
	registerTool(tool, listTemplates, "templates", "list", "blueprint", "definition", "docker", "nomad", "podman", "platform", "job", "specification")
	registerTool(
		mcp.NewTool("create_template", "Create a new space template. MANDATORY: Before calling this, you MUST first call skills(filename='<platform>-spec.md') to get the platform specification and use it as your guide for the job definition.",
			mcp.String("name", "Template name", mcp.Required()),
			mcp.String("platform", "Platform type ('manual', 'docker', 'podman', or 'nomad')", mcp.Required()),
			mcp.String("job", "Job specification (not required for manual platform)"),
			mcp.String("description", "Template description"),
			mcp.String("volumes", "Volume specification"),
			mcp.Number("compute_units", "Compute units required"),
			mcp.Number("storage_units", "Storage units required"),
			mcp.Boolean("with_terminal", "Enable terminal access"),
			mcp.Boolean("with_vscode_tunnel", "Enable VSCode tunnel"),
			mcp.Boolean("with_code_server", "Enable code server"),
			mcp.Boolean("with_ssh", "Enable SSH access"),
			mcp.Boolean("with_run_command", "Enable command execution and file operations (read/write/copy) in the space"),
			mcp.Boolean("active", "Template active status"),
			mcp.String("icon_url", "Icon URL. Use list_icons to find available URLs"),
			mcp.StringArray("groups", "Group IDs that can use this template. Use list_groups for UUIDs"),
			mcp.StringArray("zones", "Zone names where template should be available"),
			mcp.Boolean("schedule_enabled", "Enable schedule restrictions"),
			mcp.ObjectArray("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Times in '3:04pm' format",
				mcp.Boolean("enabled", "Whether this day is enabled"),
				mcp.String("from", "Start time in '3:04pm' format"),
				mcp.String("to", "End time in '3:04pm' format"),
			),
			mcp.ObjectArray("custom_fields", "Array of custom field objects",
				mcp.String("name", "Field name"),
				mcp.String("description", "Field description"),
			),
			mcp.Output(
				mcp.Boolean("status", "True if template creation was successful"),
				mcp.String("id", "Template ID if template creation was successful"),
			),
		),
		createTemplate,
		"create", "template", "new", "docker", "nomad", "podman", "platform", "job", "specification", "blueprint",
	)
	registerTool(
		mcp.NewTool("update_template", "Update an existing template. MANDATORY: Before calling this, you MUST first call skills(filename='<platform>-spec.md') to get the platform specification and use it as your guide.",
			mcp.String("template_name", "Name of the template to update", mcp.Required()),
			mcp.String("name", "New template name"),
			mcp.String("platform", "Platform: 'manual', 'docker', 'podman', or 'nomad'"),
			mcp.String("job", "Job specification"),
			mcp.String("description", "Template description"),
			mcp.String("volumes", "Volume specification"),
			mcp.Number("compute_units", "Compute units required"),
			mcp.Number("storage_units", "Storage units required"),
			mcp.Boolean("with_terminal", "Enable terminal access"),
			mcp.Boolean("with_vscode_tunnel", "Enable VSCode tunnel"),
			mcp.Boolean("with_code_server", "Enable code server"),
			mcp.Boolean("with_ssh", "Enable SSH access"),
			mcp.Boolean("with_run_command", "Enable command execution and file operations (read/write/copy) in the space"),
			mcp.Boolean("active", "Template active status"),
			mcp.String("icon_url", "Icon URL. Use list_icons to find available URLs"),
			mcp.String("group_action", "Group action: 'replace', 'add', or'remove'"),
			mcp.StringArray("groups", "Group UUIDs. Use list_groups for UUIDs"),
			mcp.String("zone_action", "Zone action: 'replace', 'add', or'remove'"),
			mcp.StringArray("zones", "Zone names"),
			mcp.Boolean("schedule_enabled", "Enable schedule restrictions"),
			mcp.ObjectArray("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Times in '3:04pm' format",
				mcp.Boolean("enabled", "Whether this day is enabled"),
				mcp.String("from", "Start time in '3:04pm' format"),
				mcp.String("to", "End time in '3:04pm' format"),
			),
			mcp.String("custom_field_action", "Custom field action: 'replace', 'add', or'remove'"),
			mcp.ObjectArray("custom_fields", "Custom field definitions (for 'remove', only 'name' required)",
				mcp.String("name", "Field name"),
				mcp.String("description", "Field description"),
			),
			mcp.Output(
				mcp.Boolean("status", "True if template creation was successful"),
			),
		),
		updateTemplate,
		"update", "template", "modify", "edit", "docker", "nomad", "podman", "platform", "job", "specification", "blueprint",
	)
	registerTool(
		mcp.NewTool("get_template", "Get detailed template information including configuration and job specification.",
			mcp.String("template_name", "Template name to retrieve", mcp.Required()),
			mcp.Output(
				mcp.String("name", "Template name"),
				mcp.String("job", "Job specification"),
				mcp.String("description", "Template description"),
				mcp.String("volumes", "Volume specification"),
				mcp.Number("usage", "Number of spaces using this template"),
				mcp.String("hash", "Template hash"),
				mcp.Number("deployed", "Number of spaces deployed using this template"),
				mcp.StringArray("groups", "Groups that can use this template"),
				mcp.String("platform", "Platform type"),
				mcp.Boolean("active", "Template active status"),
				mcp.Boolean("is_managed", "Managed template status"),
				mcp.Boolean("with_terminal", "Terminal access enabled"),
				mcp.Boolean("with_vscode_tunnel", "VSCode tunnel enabled"),
				mcp.Boolean("with_code_server", "Code server enabled"),
				mcp.Boolean("with_ssh", "SSH access enabled"),
				mcp.Boolean("with_run_command", "Command execution enabled"),
				mcp.Number("compute_units", "Compute units required"),
				mcp.Number("storage_units", "Storage units required"),
				mcp.Boolean("schedule_enabled", "Schedule restrictions enabled"),
				mcp.Boolean("auto_start", "Auto-start enabled"),
				mcp.ObjectArray("schedule", "Schedule configuration (7 days, Sunday=0 to Saturday=6)",
					mcp.Boolean("enabled", "Whether this day is enabled"),
					mcp.String("from", "Start time"),
					mcp.String("to", "End time"),
				),
				mcp.StringArray("zones", "Zone names where template is available"),
				mcp.Number("max_uptime", "Maximum uptime"),
				mcp.String("max_uptime_unit", "Maximum uptime unit"),
				mcp.String("icon_url", "Icon URL"),
				mcp.ObjectArray("custom_fields", "Custom field definitions",
					mcp.String("name", "Field name"),
					mcp.String("description", "Field description"),
				),
			),
		),
		getTemplate,
		"get", "template", "view", "details", "configuration", "docker", "nomad", "podman", "platform", "job", "specification",
	)
	registerTool(
		mcp.NewTool("delete_template", "Permanently delete a template. Cannot be undone.",
			mcp.String("template_name", "Template name to delete"),
			mcp.Output(
				mcp.Boolean("status", "True if template creation was successful"),
			),
		),
		deleteTemplate,
		"delete", "template", "remove", "destroy", "blueprint", "platform",
	)

	// Additional space management tools
	registerTool(
		mcp.NewTool("get_space", "Get detailed space information including configuration and status.",
			mcp.String("space_name", "Name of the space to retrieve", mcp.Required()),
			mcp.Output(
				mcp.String("user_id", "User ID of the space owner"),
				mcp.String("template_id", "ID of the template the space is using"),
				mcp.String("name", "Name of the space"),
				mcp.String("description", "Description of the space"),
				mcp.String("shell", "Default shell for the space"),
				mcp.String("zone", "The zone where the space is running"),
				mcp.StringArray("alt_names", "Alternate names for the space"),
				mcp.Boolean("is_deployed", "True if the space is deployed"),
				mcp.Boolean("is_pending", "True if the space is pending a state change e.g. to stop"),
				mcp.Boolean("is_deleting", "True if the space is being deleted"),
				mcp.String("started_at", "Time the space was started"),
				mcp.String("created_at", "Time the space was created"),
				mcp.String("created_at_formatted", "Formatted creation time"),
				mcp.String("icon_url", "Icon URL"),
				mcp.ObjectArray("custom_fields", "Custom fields and their values",
					mcp.String("name", "Field name"),
					mcp.String("value", "Field value"),
				),
			),
		),
		getSpace,
		"get", "space", "details", "information", "configuration", "environment",
	)
	registerTool(
		mcp.NewTool("run_command", "Execute a command in a running space and return results.",
			mcp.String("space_name", "Name of the space to run command in", mcp.Required()),
			mcp.String("command", "Command to execute", mcp.Required()),
			mcp.StringArray("arguments", "Command arguments"),
			mcp.Number("timeout", "Timeout in seconds (default: 30)"),
			mcp.String("workdir", "Working directory"),
			mcp.Output(
				mcp.String("output", "Command output"),
				mcp.Boolean("success", "True if command execution was successful"),
				mcp.String("error", "Error message if command execution failed"),
			),
		),
		runCommand,
		"run", "execute", "command", "shell", "terminal", "bash", "sh", "script", "output",
	)
	registerTool(
		mcp.NewTool("read_file", "Read file contents from a running space.",
			mcp.String("space_name", "Name of the space to read the file from", mcp.Required()),
			mcp.String("file_path", "File path in the space of the file to read", mcp.Required()),
			mcp.Output(
				mcp.String("file_path", "File path in the space of the file read"),
				mcp.Boolean("success", "True if file read was successful"),
				mcp.String("error", "Error message if file read failed"),
				mcp.String("content", "File contents read from the file"),
				mcp.Number("size", "Size of the file in bytes"),
			),
		),
		readFile,
		"read", "file", "content", "view", "open", "cat", "text", "document",
	)
	registerTool(
		mcp.NewTool("write_file", "Write content to a file in a running space.",
			mcp.String("space_name", "Name of the space to write to", mcp.Required()),
			mcp.String("file_path", "File path in the space of the file to write", mcp.Required()),
			mcp.String("content", "Content to write", mcp.Required()),
			mcp.Output(
				mcp.String("file_path", "File path in the space of the file read"),
				mcp.Boolean("success", "True if file read was successful"),
				mcp.String("error", "Error message if file read failed"),
				mcp.String("message", "Status message"),
				mcp.Number("bytes_written", "Number of bytes written to the file"),
			),
		),
		writeFile,
		"write", "file", "create", "save", "edit", "content", "text", "document",
	)
	registerTool(
		mcp.NewTool("share_space", "Share a space with another user. Use list_users to find user ID if not provided.",
			mcp.String("space_name", "Name of the space to share", mcp.Required()),
			mcp.String("user_id", "User ID to share the space with", mcp.Required()),
			mcp.Output(
				mcp.Boolean("status", "True if sharing was successful, false otherwise"),
			),
		),
		shareSpace,
		"share", "space", "grant", "access", "collaborate", "permissions",
	)
	registerTool(
		mcp.NewTool("stop_sharing_space", "Stop sharing a space.",
			mcp.String("space_name", "Name of the space to stop sharing", mcp.Required()),
			mcp.Output(
				mcp.Boolean("status", "True if stopping sharing was successful, false otherwise"),
			),
		),
		stopSharingSpace,
		"stop", "sharing", "revoke", "access", "permissions", "private",
	)
	registerTool(
		mcp.NewTool("transfer_space", "Transfer space ownership to another user. Use list_users to find user ID if not provided.",
			mcp.String("space_name", "Name of the space to transfer", mcp.Required()),
			mcp.String("user_id", "User ID to transfer to", mcp.Required()),
			mcp.Output(
				mcp.Boolean("status", "True if transfer was successful, false otherwise"),
			),
		),
		transferSpace,
		"transfer", "space", "ownership", "give", "assign",
	)
	registerTool(
		mcp.NewTool("create_space", "Create a new development space. ONLY use when explicitly asked to create a space. Spaces are created stopped - use start_space to run them.",
			mcp.String("name", "Name of the space", mcp.Required()),
			mcp.String("template_name", "Template name to use", mcp.Required()),
			mcp.String("description", "Space description"),
			mcp.String("shell", "Preferred shell (bash, zsh, fish, sh)"),
			mcp.String("icon_url", "Icon URL. Use list_icons to find available URLs. Leave empty to use the default icon"),
			mcp.ObjectArray("custom_fields", "Custom field values",
				mcp.String("name", "Custom field name", mcp.Required()),
				mcp.String("value", "Custom field value", mcp.Required()),
			),
			mcp.Output(
				mcp.Boolean("status", "True if creating was successful, false otherwise"),
				mcp.String("space_id", "ID of the new space if it was created successfully"),
			),
		),
		createSpace,
		"create", "space", "new", "environment", "development", "deploy", "instance",
	)
	registerTool(
		mcp.NewTool("update_space", "Update an existing space.",
			mcp.String("space_name", "Name of the space to update", mcp.Required()),
			mcp.String("name", "New space name"),
			mcp.String("description", "Space description"),
			mcp.String("template_name", "Template name to use"),
			mcp.String("shell", "Preferred shell (bash, zsh, fish, sh)"),
			mcp.String("icon_url", "Icon URL. Use list_icons to find available URLs. Leave empty to use the exiting icon"),
			mcp.ObjectArray("custom_fields", "Custom field values",
				mcp.String("name", "Custom field name", mcp.Required()),
				mcp.String("value", "Custom field value", mcp.Required()),
			),
			mcp.Output(
				mcp.Boolean("status", "True if update was successful, false otherwise"),
			),
		),
		updateSpace,
		"update", "space", "modify", "edit", "configure",
	)
	registerTool(
		mcp.NewTool("delete_space", "Permanently delete a space and all its data. Cannot be undone.",
			mcp.String("space_name", "Name of the space to delete", mcp.Required()),
			mcp.Output(
				mcp.Boolean("status", "True if space was deleted, false otherwise"),
			),
		),
		deleteSpace,
		"delete", "space", "remove", "destroy", "clean",
	)

	// Users
	registerTool(
		mcp.NewTool("list_users", "List all users details (id, username, email, active, groups). Use to find user IDs for sharing or transfers.",
			mcp.Output(
				mcp.ObjectArray("users", "Array of users within the system",
					mcp.String("id", "User ID"),
					mcp.String("username", "Username"),
					mcp.String("email", "Email address"),
					mcp.Boolean("active", "User active status"),
					mcp.StringArray("groups", "The list of groups the user belongs to"),
				),
			),
		),
		listUsers,
		"users", "list", "accounts", "people", "team", "directory",
	)

	// Icons
	registerTool(
		mcp.NewTool("list_icons", "List all available icons with descriptions and URLs. Use to find icon URLs for templates or spaces.",
			mcp.Output(
				mcp.ObjectArray("icons", "Array of available icons",
					mcp.String("description", "Description of the icon"),
					mcp.String("source", "Source of the icon e.g. built-in"),
					mcp.String("url", "URL of the icon"),
				),
			),
		),
		listIcons,
		"icons", "list", "images", "visuals", "graphics", "symbols",
	)

	// Skills/Knowledge base
	registerTool(
		mcp.NewTool("skills", "Access knowledge base/skills for guides and best practices. Call without filename to list all, or with filename for specific content. First call skills() to see what's available - don't assume filenames. Standard skills: nomad-spec.md, local-container-spec.md (covers docker, podman, and apple containers).",
			mcp.String("filename", "Skill filename to retrieve. Omit to list all skills."),
			mcp.Output(
				mcp.ObjectArray("skills", "Array of available skills",
					mcp.String("filename", "Filename of the skill"),
					mcp.String("description", "Description of the skill"),
				),
				mcp.String("filename", "Filename of the skill"),
				mcp.String("content", "Content of the skill when fetching a specific filename"),
			),
		),
		skills,
		"skills", "knowledge", "guides", "documentation", "specs", "specifications", "nomad", "docker", "podman", "container",
	)

	// =========================================================================
	// Attach registry to server (registers tool_search, execute_tool) or register tools natively
	// =========================================================================
	if !nativeTools {
		registry.Attach(server)
	}

	return server
}

func ToolFilter(user *model.User) openai.ToolFilter {
	return func(toolName string) bool {
		switch toolName {
		// Command execution and file operations
		case "run_command":
			return user.HasPermission(model.PermissionRunCommands)
		case "read_file", "write_file":
			return user.HasPermission(model.PermissionCopyFiles)

		// Space management
		case "list_spaces", "get_space", "start_space", "stop_space", "restart_space", "create_space", "update_space", "delete_space":
			return user.HasPermission(model.PermissionUseSpaces)

		// Space sharing and transfer
		case "share_space", "stop_sharing_space", "transfer_space":
			return user.HasPermission(model.PermissionTransferSpaces)

		// Template management
		case "list_templates", "get_template":
			return user.HasPermission(model.PermissionUseSpaces) // Users need to see templates to create spaces
		case "create_template", "update_template", "delete_template":
			return user.HasPermission(model.PermissionManageTemplates)

		// User management
		case "list_users":
			return user.HasPermission(model.PermissionManageUsers) ||
				user.HasPermission(model.PermissionManageSpaces) ||
				user.HasPermission(model.PermissionTransferSpaces)

		// Group management
		case "list_groups":
			return user.HasPermission(model.PermissionManageGroups) ||
				user.HasPermission(model.PermissionManageTemplates) // Template managers need to see groups for restrictions

		// Icons and skills - generally available to all users with basic permissions
		case "list_icons":
			return user.HasPermission(model.PermissionUseSpaces) ||
				user.HasPermission(model.PermissionManageTemplates)
		case "skills":
			return user.HasPermission(model.PermissionUseSpaces) ||
				user.HasPermission(model.PermissionManageTemplates)

		default:
			// For unknown tools, default to allow as the tools will check for permissions
			return true
		}
	}
}
