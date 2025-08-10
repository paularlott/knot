package mcp

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux) *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)
	routes.HandleFunc("POST /mcp", middleware.ApiAuth(server.HandleRequest))

	// Groups
	server.RegisterTool(
		mcp.NewTool("list_groups", "Get a list of all the groups available within the system."),
		listGroups,
	)

	// Templates
	server.RegisterTool(
		mcp.NewTool("list_templates", "Get a list of all templates available within the system."),
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
			AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("icon_url", mcp.String, "Icon URL for the template", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Array of group UUIDs (not names) that can use this template. Use list_groups to get available group UUIDs.", false),
		createTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("update_template", "Updates and existing space template from a job specification. IMPORTANT: Do NOT call this tool directly. You must FIRST call get_platform_spec(platform='<platform>') to learn the correct structure for the 'job' and 'volumes' arguments, if the user's request does not contain the full spec; you must retrieve it first.").
			AddParam("template_id", mcp.String, "The ID of the template to update", true).
			AddParam("name", mcp.String, "The name of the template", false).
			AddParam("job", mcp.String, "Job specification", false).
			AddParam("description", mcp.String, "Template description", false).
			AddParam("volumes", mcp.String, "Volume specification", false).
			AddParam("compute_units", mcp.Number, "Compute units required", false).
			AddParam("storage_units", mcp.Number, "Storage units required", false).
			AddParam("with_terminal", mcp.Boolean, "Enable terminal access", false).
			AddParam("with_vscode_tunnel", mcp.Boolean, "Enable VSCode tunnel", false).
			AddParam("with_code_server", mcp.Boolean, "Enable code server", false).
			AddParam("with_ssh", mcp.Boolean, "Enable SSH access", false).
			AddParam("active", mcp.Boolean, "Template active status", false).
			AddParam("group_action", mcp.String, "Action for groups: 'replace', 'add', or 'remove'", false).
			AddParam("groups", mcp.ArrayOf(mcp.String), "Array of group UUIDs (not names) that can use this template. Use list_groups to get available group UUIDs.", false),
		updateTemplate,
	)
	server.RegisterTool(
		mcp.NewTool("delete_template", "Delete a template").
			AddParam("template_id", mcp.String, "The ID of the template to delete", true),
		deleteTemplate,
	)

	// Spaces
	server.RegisterTool(
		mcp.NewTool("list_spaces", "Get a list of all spaces on this server (zone) for the current user. Returns status, sharing and other details about the spaces."),
		listSpaces,
	)
	server.RegisterTool(
		mcp.NewTool("start_space", "Start a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to start", true),
		startSpace,
	)
	server.RegisterTool(
		mcp.NewTool("stop_space", "Stop a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to stop", true),
		stopSpace,
	)
	server.RegisterTool(
		mcp.NewTool("restart_space", "Restart a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to restart", true),
		restartSpace,
	)
	server.RegisterTool(
		mcp.NewTool("share_space", "Share a space with another user").
			AddParam("space_id", mcp.String, "The ID of the space to share", true).
			AddParam("user_id", mcp.String, "The ID of the user to share with", true),
		shareSpace,
	)
	server.RegisterTool(
		mcp.NewTool("stop_sharing_space", "Stop sharing a space").
			AddParam("space_id", mcp.String, "The ID of the space to stop sharing", true),
		stopSharingSpace,
	)
	server.RegisterTool(
		mcp.NewTool("transfer_space", "Transfer ownership of a space to another user").
			AddParam("space_id", mcp.String, "The ID of the space to transfer", true).
			AddParam("user_id", mcp.String, "The ID of the user to transfer to", true),
		transferSpace,
	)
	server.RegisterTool(
		mcp.NewTool("create_space", "Create a new space from a template").
			AddParam("name", mcp.String, "The name of the space", true).
			AddParam("template_id", mcp.String, "The ID of the template to use", true).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL for the space", false),
		createSpace,
	)
	server.RegisterTool(
		mcp.NewTool("update_space", "Update an existing space").
			AddParam("space_id", mcp.String, "The ID of the space to update", true).
			AddParam("name", mcp.String, "The name of the space", false).
			AddParam("description", mcp.String, "Space description", false).
			AddParam("template_id", mcp.String, "The ID of the template to use", false).
			AddParam("shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("icon_url", mcp.String, "Icon URL for the space", false),
		updateSpace,
	)
	server.RegisterTool(
		mcp.NewTool("delete_space", "Delete a space").
			AddParam("space_id", mcp.String, "The ID of the space to delete", true),
		deleteSpace,
	)

	// Specifications
	server.RegisterTool(
		mcp.NewTool("get_platform_spec", "Crucial first step. Retrieves the required job specification schema for a given platform ('nomad', 'docker' or 'podman). This MUST be called before attempting to create a template.").
			AddParam("platform", mcp.String, "Platform type: 'docker', 'podman', or 'nomad'", true),
		getPlatformSpec,
	)

	return server
}
