package model

// Permissions
const (
  PermissionManageUsers = iota
  PermissionManageTemplates
  PermissionManageWorkspaces
)

// Roles
const (
  RoleAdmin            = "00000000-0000-0000-0000-000000000000"
  RoleUserManager      = "00000000-0000-0000-0000-000000000001"
  RoleTemplateManager  = "00000000-0000-0000-0000-000000000002"
  RoleWorkspaceManager = "00000000-0000-0000-0000-000000000003"
)

// Mapping of permissions to roles
var rolePermissions = map[string][]int{
  RoleAdmin           : {PermissionManageUsers, PermissionManageTemplates, PermissionManageWorkspaces},
  RoleUserManager     : {PermissionManageUsers},
  RoleTemplateManager : {PermissionManageTemplates},
  RoleWorkspaceManager: {PermissionManageWorkspaces},
}
