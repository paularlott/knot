package mcp

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux, enableWebEndpoint bool) *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)

	if enableWebEndpoint {
		routes.HandleFunc("POST /mcp", middleware.ApiAuth(middleware.ApiPermissionUseMCPServer(server.HandleRequest)))
	}

	// Groups
	server.RegisterTool(
		mcp.NewTool("list_groups", "Get a list of all user groups in the system. Use this to find group IDs when creating or updating templates that should be restricted to specific groups."),
		listGroups,
	)

	// Templates
	server.RegisterTool(
		mcp.NewTool("list_templates", "Get a list of all available space templates. Use this to find template IDs when creating spaces, or to see what templates exist before creating new ones."),
		listTemplates,
	)
	server.RegisterTool(
		mcp.NewTool("create_template", "Creates a new space template from a job specification. IMPORTANT: Do NOT call this tool directly. You must FIRST call get_platform_spec(platform='<platform>') to learn the correct structure for the 'job' and 'volumes' arguments, if the user's request does not contain the full spec; you must retrieve it first.").
			AddParam("name", mcp.String, "The name of the template", true).
			AddParam("platform", mcp.String, "Platform type: 'manual', 'docker', 'podman', or 'nomad'", true).
			AddParam("job", mcp.String, "Job specification (not required for manual platform)", false).
			AddParam("description", mcp.String, "Template description", false).
			AddParam("volumes", mcp.String, "Volume specification", false).
			AddParam("compute_units", mcp.Number, "Compute units required", false).
			AddParam("storage_units", mcp.Number, "Storage units required", false).
			AddParam("with_terminal", mcp.Boolean, "Enable terminal access", false).
			AddParam("with_vscode_tunnel", mcp.Boolean, "Enable VSCode tunnel", false).
			AddParam("with_code_server", mcp.Boolean, "Enable code server", false).
			AddParam("with_ssh", mcp.Boolean, "Enable SSH access", false).
			AddParam("with_run_command", mcp.Boolean, "Enable run command in space", false).
			AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("icon_url", mcp.String, "Icon URL for the template. Use list_icons to find available icon URLs.", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Array of group UUIDs (not names) that can use this template. Use list_groups to get available group UUIDs.", false).
			AddParam("zones", mcp.ArrayOf(mcp.String), "Array of zone names that the template should show in", false).
			AddParam("schedule_enabled", mcp.Boolean, "Enable schedule restrictions", false).
			AddArrayObjectParam("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Each day has enabled, from, and to fields. Times must be in format '3:04pm' (e.g., '9:00am', '12:15pm', '11:45pm')", false).
			AddProperty("enabled", mcp.Boolean, "Whether this day is enabled", true).
			AddProperty("from", mcp.String, "Start time in format '3:04pm' (e.g., '9:00am')", true).
			AddProperty("to", mcp.String, "End time in format '3:04pm' (e.g., '5:00pm')", true).
			Done().
			AddArrayObjectParam("custom_fields", "Array of custom field definitions", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("description", mcp.String, "Field description", false).
			Done(),
		createTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("update_template", "Updates and existing space template from a job specification. IMPORTANT: Do NOT call this tool directly. You must FIRST call get_platform_spec(platform='<platform>') to learn the correct structure for the 'job' and 'volumes' arguments, if the user's request does not contain the full spec; you must retrieve it first.").
			AddParam("template_id", mcp.String, "The ID of the template to update", true).
			AddParam("name", mcp.String, "The name of the template", false).
			AddParam("platform", mcp.String, "Platform type: 'manual', 'docker', 'podman', or 'nomad'", false).
			AddParam("job", mcp.String, "Job specification", false).
			AddParam("description", mcp.String, "Template description", false).
			AddParam("volumes", mcp.String, "Volume specification", false).
			AddParam("compute_units", mcp.Number, "Compute units required", false).
			AddParam("storage_units", mcp.Number, "Storage units required", false).
			AddParam("with_terminal", mcp.Boolean, "Enable terminal access", false).
			AddParam("with_vscode_tunnel", mcp.Boolean, "Enable VSCode tunnel", false).
			AddParam("with_code_server", mcp.Boolean, "Enable code server", false).
			AddParam("with_ssh", mcp.Boolean, "Enable SSH access", false).
			AddParam("with_run_command", mcp.Boolean, "Enable run command in space", false).
			AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("icon_url", mcp.String, "Icon URL for the template. Use list_icons to find available icon URLs.", false).
			AddParam("group_action", mcp.String, "Action for groups: 'replace', 'add', or 'remove'", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Array of group UUIDs (not names) that can use this template. Use list_groups to get available group UUIDs.", false).
			AddParam("zone_action", mcp.String, "Action for zones: 'replace', 'add', or 'remove'", false).
			AddParam("zones", mcp.ArrayOf(mcp.String), "Array of zone names that the template should show in", false).
			AddParam("schedule_enabled", mcp.Boolean, "Enable schedule restrictions", false).
			AddArrayObjectParam("schedule", "Array of 7 schedule objects (Sunday=0 to Saturday=6). Each day has enabled, from, and to fields. Times must be in format '3:04pm' (e.g., '9:00am', '12:15pm', '11:45pm')", false).
			AddProperty("enabled", mcp.Boolean, "Whether this day is enabled", true).
			AddProperty("from", mcp.String, "Start time in format '3:04pm' (e.g., '9:00am')", true).
			AddProperty("to", mcp.String, "End time in format '3:04pm' (e.g., '5:00pm')", true).
			Done().
			AddParam("custom_field_action", mcp.String, "Action for custom fields: 'replace', 'add', or 'remove'", false).
			AddArrayObjectParam("custom_fields", "Array of custom field definitions (for 'remove' action, only 'name' property is required)", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("description", mcp.String, "Field description", false).
			Done(),
		updateTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("get_template", "Get detailed information about a specific template including its configuration, job specification, and settings. Use this to understand a template before editing it.").
			AddParam("template_id", mcp.String, "The ID of the template to retrieve", true),
		getTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("delete_template", "Permanently delete a template. This action cannot be undone. Use list_templates first to find the template ID.").
			AddParam("template_id", mcp.String, "The ID of the template to delete", true),
		deleteTemplate,
	)

	// Spaces
	server.RegisterTool(
		mcp.NewTool("list_spaces", "Get a list of all spaces for the current user, including their status (running/stopped), sharing details, and IDs. Use this to find space IDs before performing actions like start/stop/delete, or to check space status."),
		listSpaces,
	)
	server.RegisterTool(
		mcp.NewTool("get_space", "Get detailed information about a specific space including its configuration, template details, custom fields, and current status. Use this to understand a space before editing it.").
			AddParam("space_id", mcp.String, "The ID of the space to retrieve", true),
		getSpace,
	)
	server.RegisterTool(
		mcp.NewTool("run_command", "Execute a command within a running space and return the output").
			AddParam("space_id", mcp.String, "The ID of the space to run the command in, use list_spaces if you need to convert a space name to an ID", true).
			AddParam("command", mcp.String, "The command to execute", true).
			AddParam("arguments", mcp.ArrayOf(mcp.String), "The arguments to pass to the command", false).
			AddParam("timeout", mcp.Number, "Command timeout in seconds (default: 30)", false).
			AddParam("workdir", mcp.String, "Working directory for the command", false),
		runCommand,
	)
	server.RegisterTool(
		mcp.NewTool("copy_file", "Read and write the contents of files within a running space. Provide either (content and dest_path) to write to space, or source_path to read from space.").
			AddParam("space_id", mcp.String, "The ID of the space to copy files to/from, use list_spaces if you need to convert a space name to an ID", true).
			AddParam("content", mcp.String, "Content to write to the space (for writing to space)", false).
			AddParam("dest_path", mcp.String, "Destination path in space (for writing to space)", false).
			AddParam("source_path", mcp.String, "Source path in space (for reading from space)", false),
		copyFile,
	)
	server.RegisterTool(
		mcp.NewTool("manage_space_state", "Start, stop, or restart a space. Use list_spaces first to find the space ID if you only have the space name.").
			AddParam("space_id", mcp.String, "The ID of the space to manage (use list_spaces to find this)", true).
			AddParam("action", mcp.String, "Action to perform: 'start', 'stop', or 'restart'", true),
		manageSpaceState,
	)
	server.RegisterTool(
		mcp.NewTool("manage_space_sharing", "Share, stop sharing, or transfer ownership of a space. IMPORTANT: If the user doesn't provide the ID of the user then FIRST call list_users to find the ID of the user.").
			AddParam("space_id", mcp.String, "The ID of the space to manage", true).
			AddParam("action", mcp.String, "Action to perform: 'share', 'stop_sharing', or 'transfer'", true).
			AddParam("user_id", mcp.String, "The ID of the user (required for share and transfer actions).", false),
		manageSpaceSharing,
	)
	server.RegisterTool(
		mcp.NewTool("create_space", "Create a new space from a template").
			AddParam("name", mcp.String, "The name of the space", true).
			AddParam("template_id", mcp.String, "The ID of the template to use", true).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL for the space. Use list_icons to find available icon URLs.", false).
			AddArrayObjectParam("custom_fields", "Array of custom field values", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("value", mcp.String, "Field value", true).
			Done(),
		createSpace,
	)
	server.RegisterTool(
		mcp.NewTool("update_space", "Update an existing space").
			AddParam("space_id", mcp.String, "The ID of the space to update", true).
			AddParam("name", mcp.String, "The name of the space", false).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("template_id", mcp.String, "The ID of the template to use", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL for the space. Use list_icons to find available icon URLs.", false).
			AddArrayObjectParam("custom_fields", "Array of custom field values", false).
			AddProperty("name", mcp.String, "Field name", true).
			AddProperty("value", mcp.String, "Field value", true).
			Done(),
		updateSpace,
	)
	server.RegisterTool(
		mcp.NewTool("delete_space", "Permanently delete a space and all its data. This action cannot be undone. Use list_spaces first to find the space ID.").
			AddParam("space_id", mcp.String, "The ID of the space to delete", true),
		deleteSpace,
	)

	// Users
	server.RegisterTool(
		mcp.NewTool("list_users", "Get a list of all users in the system with their IDs and details. Use this to find user IDs when sharing spaces or transferring ownership."),
		listUsers,
	)

	// Icons
	server.RegisterTool(
		mcp.NewTool("list_icons", "Get a list of all available icons (built-in and user-supplied) with their descriptions and URLs. Use this when you need to find an icon URL for templates or spaces. The LLM should use this tool to convert icon names/descriptions to URLs when creating or updating templates and spaces."),
		listIcons,
	)

	// Specifications
	server.RegisterTool(
		mcp.NewTool("get_platform_spec", "Crucial first step. Retrieves the required job specification schema for a given platform ('nomad', 'docker' or 'podman). This MUST be called before attempting to create a template.").
			AddParam("platform", mcp.String, "Platform type: 'docker', 'podman', or 'nomad'", true),
		getPlatformSpec,
	)

	// Recipes/Knowledgebase
	server.RegisterTool(
		mcp.NewTool("recipes", "Access the knowledge base/recipes collection for step-by-step guides and best practices. Call without filename to list all available recipes, or with filename to get specific recipe content. Always check recipes first when users request project setup, environment configuration, or similar tasks. Returns empty list if recipes path is not configured.").
			AddParam("filename", mcp.String, "Filename of the recipe to retrieve. Omit to list all recipes.", false),
		recipes,
	)

	return server
}
