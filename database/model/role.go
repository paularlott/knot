package model

// Permissions
const (
	PermissionManageUsers     = iota // Can Manage Users
	PermissionManageTemplates        // Can Manage Templates
	PermissionManageSpaces           // Can Manage Spaces
	PermissionManageVolumes          // Can Manage Volumes
	PermissionViewLogs               // Can View Logs
)

// Roles
const (
	RoleAdmin           = "00000000-0000-0000-0000-000000000000"
	RoleUserManager     = "00000000-0000-0000-0000-000000000001"
	RoleTemplateManager = "00000000-0000-0000-0000-000000000002"
	RoleSpaceManager    = "00000000-0000-0000-0000-000000000003"
	RoleVolumeManager   = "00000000-0000-0000-0000-000000000004"
	RoleLogViewer       = "00000000-0000-0000-0000-000000000005"
)

// Mapping of role IDs to names
type RoleName struct {
	RoleID   string `json:"id_role"`
	RoleName string `json:"role_name"`
}

var RoleNames = []RoleName{
	{RoleAdmin, "Admin"},
	{RoleUserManager, "User Manager"},
	{RoleTemplateManager, "Template Manager"},
	{RoleSpaceManager, "Space Manager"},
	{RoleVolumeManager, "Volume Manager"},
	{RoleLogViewer, "Log Viewer"},
}

// Mapping of permissions to roles
var rolePermissions = map[string][]int{
	RoleAdmin:           {PermissionManageUsers, PermissionManageTemplates, PermissionManageSpaces, PermissionManageVolumes, PermissionViewLogs},
	RoleUserManager:     {PermissionManageUsers},
	RoleTemplateManager: {PermissionManageTemplates},
	RoleSpaceManager:    {PermissionManageSpaces},
	RoleVolumeManager:   {PermissionManageVolumes},
	RoleLogViewer:       {PermissionViewLogs},
}

func RoleExists(roleId string) bool {
	for _, role := range RoleNames {
		if role.RoleID == roleId {
			return true
		}
	}

	return false
}
