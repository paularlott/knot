package model

// Permissions
const (
  PermissionManageUsers = iota // Can Manage Users
  PermissionManageTemplates    // Can Manage Templates
  PermissionManageSpaces       // Can Manage Spaces
)

// Roles
const (
  RoleAdmin            = "00000000-0000-0000-0000-000000000000"
  RoleUserManager      = "00000000-0000-0000-0000-000000000001"
  RoleTemplateManager  = "00000000-0000-0000-0000-000000000002"
  RoleSpaceManager     = "00000000-0000-0000-0000-000000000003"
)

// Mapping of permissions to roles
var rolePermissions = map[string][]int{
  RoleAdmin           : {PermissionManageUsers, PermissionManageTemplates, PermissionManageSpaces},
  RoleUserManager     : {PermissionManageUsers},
  RoleTemplateManager : {PermissionManageTemplates},
  RoleSpaceManager    : {PermissionManageSpaces},
}
