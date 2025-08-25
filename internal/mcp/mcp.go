package mcp

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux, enableWebEndpoint bool) *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)
	server.SetInstructions(`These tools manage spaces, templates, and other resources.

TEMPLATE OPERATIONS:
- When user asks to create/update a template, get the platform spec with recipes(filename='<platform>-spec.md') then execute immediately
- Do NOT ask for confirmation on template operations - execute directly

SPACE OPERATIONS:
- When user asks to create spaces or environments, check recipes() first for guidance
- Follow recipe instructions when available

NEVER create templates or spaces unless explicitly requested.`)

	if enableWebEndpoint {
		routes.HandleFunc("POST /mcp", middleware.ApiAuth(middleware.ApiPermissionUseMCPServer(server.HandleRequest)))
	}

	// Groups
	server.RegisterTool(
		mcp.NewTool("list_groups", "List all user groups. Use to find group IDs for template restrictions."),
		listGroups,
	)

	// Templates
	server.RegisterTool(
		mcp.NewTool("list_templates", "List all space templates. Use to find template IDs or check existing templates.").
			AddParam("show_all", mcp.Boolean, "Show templates from all zones (default: false)", false).
			AddParam("show_inactive", mcp.Boolean, "Show inactive templates (default: false)", false),
		listTemplates,
	)
	server.RegisterTool(
		mcp.NewTool("create_template", "Create a new space template immediately when user requests it. Get platform spec first with recipes(filename='<platform>-spec.md'), then create the template directly.").
			AddParam("name", mcp.String, "Template name", true).
			AddParam("platform", mcp.String, "Platform: 'manual', 'docker', 'podman', or 'nomad'", true).
			AddParam("job", mcp.String, "Job specification (not required for manual platform)", false).
			AddParam("description", mcp.String, "Template description", false).
			AddParam("volumes", mcp.String, "Volume specification", false).
			AddParam("compute_units", mcp.Number, "Compute units required", false).
			AddParam("storage_units", mcp.Number, "Storage units required", false).
			AddParam("with_terminal", mcp.Boolean, "Enable terminal access", false).
			AddParam("with_vscode_tunnel", mcp.Boolean, "Enable VSCode tunnel", false).
			AddParam("with_code_server", mcp.Boolean, "Enable code server", false).
			AddParam("with_ssh", mcp.Boolean, "Enable SSH access", false).
			AddParam("with_run_command", mcp.Boolean, "Enable command execution and file operations (read/write/copy) in the space", false).AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("icon_url", mcp.String, "Icon URL. Use list_icons to find available URLs.", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Group UUIDs that can use this template. Use list_groups for UUIDs.", false).
			AddParam("zones", mcp.ArrayOf(mcp.String), "Zone names where template should appear", false).
			AddParam("schedule_enabled", mcp.Boolean, "Enable schedule restrictions", false).
			AddArrayObjectParam("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Times in '3:04pm' format.", false).
			AddProperty("enabled", mcp.Boolean, "Whether this day is enabled", true).
			AddProperty("from", mcp.String, "Start time in '3:04pm' format", true).
			AddProperty("to", mcp.String, "End time in '3:04pm' format", true).
			Done().
			AddArrayObjectParam("custom_fields", "Custom field definitions", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("description", mcp.String, "Field description", false).
			Done(),
		createTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("update_template", "Update an existing template. ONLY use when explicitly asked to UPDATE A TEMPLATE. First call recipes(filename='<platform>-spec.md') to learn the structure.").
			AddParam("template_name", mcp.String, "Name of template to update", true).
			AddParam("name", mcp.String, "New template name", false).
			AddParam("platform", mcp.String, "Platform: 'manual', 'docker', 'podman', or 'nomad'", false).
			AddParam("job", mcp.String, "Job specification", false).
			AddParam("description", mcp.String, "Template description", false).
			AddParam("volumes", mcp.String, "Volume specification", false).
			AddParam("compute_units", mcp.Number, "Compute units required", false).
			AddParam("storage_units", mcp.Number, "Storage units required", false).
			AddParam("with_terminal", mcp.Boolean, "Enable terminal access", false).
			AddParam("with_vscode_tunnel", mcp.Boolean, "Enable VSCode tunnel", false).
			AddParam("with_code_server", mcp.Boolean, "Enable code server", false).
			AddParam("with_ssh", mcp.Boolean, "Enable SSH access", false).
			AddParam("with_run_command", mcp.Boolean, "Enable command execution and file operations (read/write/copy) in the space", false).AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("icon_url", mcp.String, "Icon URL. Use list_icons to find available URLs.", false).
			AddParam("group_action", mcp.String, "Group action: 'replace', 'add', or 'remove'", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Group UUIDs. Use list_groups for UUIDs.", false).
			AddParam("zone_action", mcp.String, "Zone action: 'replace', 'add', or 'remove'", false).
			AddParam("zones", mcp.ArrayOf(mcp.String), "Zone names", false).
			AddParam("schedule_enabled", mcp.Boolean, "Enable schedule restrictions", false).
			AddArrayObjectParam("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Times in '3:04pm' format.", false).
			AddProperty("enabled", mcp.Boolean, "Whether this day is enabled", true).
			AddProperty("from", mcp.String, "Start time in '3:04pm' format", true).
			AddProperty("to", mcp.String, "End time in '3:04pm' format", true).
			Done().
			AddParam("custom_field_action", mcp.String, "Custom field action: 'replace', 'add', or 'remove'", false).
			AddArrayObjectParam("custom_fields", "Custom field definitions (for 'remove', only 'name' required)", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("description", mcp.String, "Field description", false).
			Done(),
		updateTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("get_template", "Get detailed template information including configuration and job specification.").
			AddParam("template_name", mcp.String, "Template name to retrieve", true),
		getTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("delete_template", "Permanently delete a template. Cannot be undone.").
			AddParam("template_name", mcp.String, "Template name to delete", true),
		deleteTemplate,
	)

	// Spaces
	server.RegisterTool(
		mcp.NewTool("list_spaces", "List all spaces for current user with status and sharing details."),
		listSpaces,
	)
	server.RegisterTool(
		mcp.NewTool("get_space", "Get detailed space information including configuration and status.").
			AddParam("space_name", mcp.String, "Space name to retrieve", true),
		getSpace,
	)
	server.RegisterTool(
		mcp.NewTool("run_command", "Execute a command in a running space and return results.").
			AddParam("space_name", mcp.String, "Space name to run command in", true).
			AddParam("command", mcp.String, "Command to execute", true).
			AddParam("arguments", mcp.ArrayOf(mcp.String), "Command arguments", false).
			AddParam("timeout", mcp.Number, "Timeout in seconds (default: 30)", false).
			AddParam("workdir", mcp.String, "Working directory", false),
		runCommand,
	)
	server.RegisterTool(
		mcp.NewTool("read_file", "Read file contents from a running space.").
			AddParam("space_name", mcp.String, "Space name to read from", true).
			AddParam("file_path", mcp.String, "File path in the space", true),
		readFile,
	)
	server.RegisterTool(
		mcp.NewTool("write_file", "Write content to a file in a running space.").
			AddParam("space_name", mcp.String, "Space name to write to", true).
			AddParam("file_path", mcp.String, "File path in the space", true).
			AddParam("content", mcp.String, "Content to write", true),
		writeFile,
	)
	server.RegisterTool(
		mcp.NewTool("start_space", "Start a space.").
			AddParam("space_name", mcp.String, "Space name to start", true),
		startSpace,
	)
	server.RegisterTool(
		mcp.NewTool("stop_space", "Stop a space.").
			AddParam("space_name", mcp.String, "Space name to stop", true),
		stopSpace,
	)
	server.RegisterTool(
		mcp.NewTool("restart_space", "Restart a space.").
			AddParam("space_name", mcp.String, "Space name to restart", true),
		restartSpace,
	)
	server.RegisterTool(
		mcp.NewTool("share_space", "Share a space with another user. Use list_users to find user ID if not provided.").
			AddParam("space_name", mcp.String, "Space name to share", true).
			AddParam("user_id", mcp.String, "User ID to share with", true),
		shareSpace,
	)
	server.RegisterTool(
		mcp.NewTool("stop_sharing_space", "Stop sharing a space.").
			AddParam("space_name", mcp.String, "Space name to stop sharing", true),
		stopSharingSpace,
	)
	server.RegisterTool(
		mcp.NewTool("transfer_space", "Transfer space ownership to another user. Use list_users to find user ID if not provided.").
			AddParam("space_name", mcp.String, "Space name to transfer", true).
			AddParam("user_id", mcp.String, "User ID to transfer to", true),
		transferSpace,
	)
	server.RegisterTool(
		mcp.NewTool("create_space", "Create a new development space. ONLY use when explicitly asked to create a space. Spaces are created stopped - use start_space to run them.").
			AddParam("name", mcp.String, "Space name", true).
			AddParam("template_name", mcp.String, "Template name to use", true).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL. Use list_icons to find available URLs.", false).
			AddArrayObjectParam("custom_fields", "Custom field values", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("value", mcp.String, "Field value", true).
			Done(),
		createSpace,
	)
	server.RegisterTool(
		mcp.NewTool("update_space", "Update an existing space.").
			AddParam("space_name", mcp.String, "Space name to update", true).
			AddParam("name", mcp.String, "New space name", false).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("template_name", mcp.String, "Template name to use", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL. Use list_icons to find available URLs.", false).
			AddArrayObjectParam("custom_fields", "Custom field values", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("value", mcp.String, "Field value", true).
			Done(),
		updateSpace,
	)
	server.RegisterTool(
		mcp.NewTool("delete_space", "Permanently delete a space and all its data. Cannot be undone.").
			AddParam("space_name", mcp.String, "Space name to delete", true),
		deleteSpace,
	)

	// Users
	server.RegisterTool(
		mcp.NewTool("list_users", "List all users with IDs and details. Use to find user IDs for sharing or transfers."),
		listUsers,
	)

	// Icons
	server.RegisterTool(
		mcp.NewTool("list_icons", "List all available icons with descriptions and URLs. Use to find icon URLs for templates or spaces."),
		listIcons,
	)

	// Recipes/Knowledge base
	server.RegisterTool(
		mcp.NewTool("recipes", "Access knowledge base/recipes for guides and best practices. Call without filename to list all, or with filename for specific content. First call recipes() to see what's available - don't assume filenames. Standard recipes: nomad-spec.md, docker-spec.md, podman-spec.md").
			AddParam("filename", mcp.String, "Recipe filename to retrieve. Omit to list all recipes.", false),
		recipes,
	)

	return server
}
