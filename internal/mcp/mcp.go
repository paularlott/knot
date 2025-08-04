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

	// Permissions
	server.RegisterTool(
		mcp.NewTool("list_permissions", "Get a list of all the permissions available within the system."),
		listPermissions,
	)

	// Groups
	server.RegisterTool(
		mcp.NewTool("list_groups", "Get a list of all the groups available within the system."),
		listGroups,
	)
	server.RegisterTool(
		mcp.NewTool("create_group", "Create a new group").
			AddParam("name", mcp.String, "The name of the group", true).
			AddParam("max_spaces", mcp.Number, "Maximum number of spaces (default: 0)", false).
			AddParam("compute_units", mcp.Number, "Compute units limit (default: 0)", false).
			AddParam("storage_units", mcp.Number, "Storage units limit (default: 0)", false).
			AddParam("max_tunnels", mcp.Number, "Maximum number of tunnels (default: 0)", false),
		createGroup,
	)
	server.RegisterTool(
		mcp.NewTool("update_group", "Update an existing group").
			AddParam("group_id", mcp.String, "The ID of the group to update", true).
			AddParam("name", mcp.String, "The name of the group", true).
			AddParam("max_spaces", mcp.Number, "Maximum number of spaces (default: 0)", false).
			AddParam("compute_units", mcp.Number, "Compute units limit (default: 0)", false).
			AddParam("storage_units", mcp.Number, "Storage units limit (default: 0)", false).
			AddParam("max_tunnels", mcp.Number, "Maximum number of tunnels (default: 0)", false),
		updateGroup,
	)
	server.RegisterTool(
		mcp.NewTool("delete_group", "Delete a group").
			AddParam("group_id", mcp.String, "The ID of the group to delete", true),
		deleteGroup,
	)

	// Roles
	server.RegisterTool(
		mcp.NewTool("list_roles", "Get a list of all the roles available within the system."),
		listRoles,
	)
	server.RegisterTool(
		mcp.NewTool("create_role", "Create a new role with specified permissions. IMPORTANT: Use list_permissions tool first to get the numeric permission IDs.").
			AddParam("name", mcp.String, "The name of the role", true).
			AddParam("permissions", mcp.Array, "Array of permission IDs as integers (e.g., [1, 2, 5]). Do NOT use permission names - use the numeric 'id' field from list_permissions.", false),
		createRole,
	)
	server.RegisterTool(
		mcp.NewTool("update_role", "Update an existing role. IMPORTANT: Use list_permissions tool first to get the numeric permission IDs.").
			AddParam("role_id", mcp.String, "The ID of the role to update", true).
			AddParam("name", mcp.String, "The name of the role", false).
			AddParam("permission_action", mcp.String, "Action to perform on permissions: 'replace', 'add', or 'remove'", false).
			AddParam("permissions", mcp.Array, "Array of permission IDs as integers (e.g., [1, 2, 5]). Do NOT use permission names - use the numeric 'id' field from list_permissions.", false),
		updateRole,
	)
	server.RegisterTool(
		mcp.NewTool("delete_role", "Delete a role").
			AddParam("role_id", mcp.String, "The ID of the role to delete", true),
		deleteRole,
	)
	server.RegisterTool(
		mcp.NewTool("get_role", "Get details of a specific role including permissions").
			AddParam("role_id", mcp.String, "The ID of the role to retrieve", true),
		getRole,
	)

	// Templates
	server.RegisterTool(
		mcp.NewTool("list_templates", "Get a list of all templates available within the system."),
		listTemplates,
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

	// Tokens
	server.RegisterTool(
		mcp.NewTool("list_tokens", "Get a list of all API tokens for the current user."),
		listTokens,
	)
	server.RegisterTool(
		mcp.NewTool("create_token", "Create a new API token").
			AddParam("name", mcp.String, "The name of the token", true),
		createToken,
	)
	server.RegisterTool(
		mcp.NewTool("delete_token", "Delete an API token").
			AddParam("token_id", mcp.String, "The ID of the token to delete", true),
		deleteToken,
	)

	// Tunnels
	server.RegisterTool(
		mcp.NewTool("list_tunnels", "Get a list of all tunnels for the current user."),
		listTunnels,
	)
	server.RegisterTool(
		mcp.NewTool("delete_tunnel", "Delete a tunnel").
			AddParam("tunnel_name", mcp.String, "The name of the tunnel to delete (format: username--tunnelname)", true),
		deleteTunnel,
	)

	// Users
	server.RegisterTool(
		mcp.NewTool("list_users", "Get a list of all users in the system."),
		listUsers,
	)
	server.RegisterTool(
		mcp.NewTool("create_user", "Create a new user").
			AddParam("username", mcp.String, "The username", true).
			AddParam("email", mcp.String, "The email address", true).
			AddParam("password", mcp.String, "The password", true).
			AddParam("preferred_shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("timezone", mcp.String, "User timezone", false).
			AddParam("ssh_public_key", mcp.String, "SSH public key", false).
			AddParam("github_username", mcp.String, "GitHub username", false).
			AddParam("max_spaces", mcp.Number, "Maximum spaces allowed", false).
			AddParam("compute_units", mcp.Number, "Compute units limit", false).
			AddParam("storage_units", mcp.Number, "Storage units limit", false).
			AddParam("max_tunnels", mcp.Number, "Maximum tunnels allowed", false).
			AddParam("roles", mcp.Array, "Array of role IDs", false).
			AddParam("groups", mcp.Array, "Array of group IDs", false),
		createUser,
	)
	server.RegisterTool(
		mcp.NewTool("update_user", "Update an existing user").
			AddParam("user_id", mcp.String, "The ID of the user to update", true).
			AddParam("email", mcp.String, "The email address", false).
			AddParam("password", mcp.String, "The password", false).
			AddParam("preferred_shell", mcp.String, "Preferred shell (bash, zsh, fish, sh)", false).
			AddParam("timezone", mcp.String, "User timezone", false).
			AddParam("ssh_public_key", mcp.String, "SSH public key", false).
			AddParam("github_username", mcp.String, "GitHub username", false).
			AddParam("active", mcp.Boolean, "User active status (admin only)", false).
			AddParam("max_spaces", mcp.Number, "Maximum spaces allowed (admin only)", false).
			AddParam("compute_units", mcp.Number, "Compute units limit (admin only)", false).
			AddParam("storage_units", mcp.Number, "Storage units limit (admin only)", false).
			AddParam("max_tunnels", mcp.Number, "Maximum tunnels allowed (admin only)", false).
			AddParam("role_action", mcp.String, "Action for roles: 'replace', 'add', or 'remove' (admin only)", false).
			AddParam("roles", mcp.Array, "Array of role IDs for the specified action (admin only)", false).
			AddParam("group_action", mcp.String, "Action for groups: 'replace', 'add', or 'remove' (admin only)", false).
			AddParam("groups", mcp.Array, "Array of group IDs for the specified action (admin only)", false),
		updateUser,
	)
	server.RegisterTool(
		mcp.NewTool("delete_user", "Delete a user").
			AddParam("user_id", mcp.String, "The ID of the user to delete", true),
		deleteUser,
	)

	// Specifications
	server.RegisterTool(
		mcp.NewTool("get_docker_podman_spec", "Get the complete Docker/Podman job specification documentation in markdown format"),
		getContainerSpec,
	)

	return server
}
