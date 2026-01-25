package scriptling

import (
	"context"
	"fmt"
	"strconv"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetUsersLibrary returns the users management library for scriptling
func GetUsersLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.user", "Knot user management functions")

	builder.FunctionWithHelp("get_me", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userGetMe(ctx, client)
	}, "get_me() - Get current user details as a dict")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userGet(ctx, client, args...)
	}, "get(user_id) - Get user by ID or username")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userList(ctx, client, kwargs)
	}, "list(state='', zone='') - List all users with optional state/zone filter")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userCreate(ctx, client, kwargs, args...)
	}, "create(username, email, password, ...) - Create a new user")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userUpdate(ctx, client, kwargs, args...)
	}, "update(user_id, ...) - Update user properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userDelete(ctx, client, args...)
	}, "delete(user_id) - Delete a user")

	builder.FunctionWithHelp("get_quota", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userGetQuota(ctx, client, args...)
	}, "get_quota(user_id) - Get user quota and usage as a dict")

	builder.FunctionWithHelp("list_permissions", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userListPermissions(ctx, client, args...)
	}, "list_permissions(user_id) - List all permissions for a user")

	builder.FunctionWithHelp("has_permission", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return userHasPermission(ctx, client, args...)
	}, "has_permission(user_id, permission_id) - Check if user has specific permission")

	return builder.Build()
}

// userGetMe returns current user details
func userGetMe(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Users not available - API client not configured"}
	}

	user, err := client.WhoAmI(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get current user: %v", err)}
	}

	return userToDict(user)
}

// userGet returns user by ID
func userGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	user, apiErr := client.GetUser(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get user: %v", apiErr)}
	}

	return userToDict(user)
}

// userList returns all users
func userList(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs) object.Object {
	if client == nil {
		return &object.Error{Message: "Users not available - API client not configured"}
	}

	state, _ := kwargs.GetString("state", "")
	zone, _ := kwargs.GetString("zone", "")

	users, err := client.GetUsers(ctx, state, zone)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list users: %v", err)}
	}

	elements := make([]object.Object, len(users.Users))
	for i, user := range users.Users {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: user.Id}}
		pairs["username"] = object.DictPair{Key: &object.String{Value: "username"}, Value: &object.String{Value: user.Username}}
		pairs["email"] = object.DictPair{Key: &object.String{Value: "email"}, Value: &object.String{Value: user.Email}}
		pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: user.Active}}
		pairs["number_spaces"] = object.DictPair{Key: &object.String{Value: "number_spaces"}, Value: &object.Integer{Value: int64(user.NumberSpaces)}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// userCreate creates a new user
func userCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 3); err != nil {
		return err
	}

	username, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("username", err)
	}

	email, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("email", err)
	}

	password, err := args[2].AsString()
	if err != nil {
		return errors.ParameterError("password", err)
	}

	request := &apiclient.CreateUserRequest{
		Username: username,
		Email:    email,
		Password: password,
		Active:   true,
	}

	// Optional parameters
	if active, errObj := kwargs.GetBool("active", true); errObj == nil {
		request.Active = active
	}
	if maxSpaces, errObj := kwargs.GetInt("max_spaces", 0); errObj == nil {
		request.MaxSpaces = uint32(maxSpaces)
	}
	if computeUnits, errObj := kwargs.GetInt("compute_units", 0); errObj == nil {
		request.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, errObj := kwargs.GetInt("storage_units", 0); errObj == nil {
		request.StorageUnits = uint32(storageUnits)
	}
	if maxTunnels, errObj := kwargs.GetInt("max_tunnels", 0); errObj == nil {
		request.MaxTunnels = uint32(maxTunnels)
	}

	userId, _, apiErr := client.CreateUser(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create user: %v", apiErr)}
	}

	return &object.String{Value: userId}
}

// userUpdate updates a user
func userUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	// Get current user to build request
	currentUser, apiErr := client.GetUser(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get user: %v", apiErr)}
	}

	request := &apiclient.UpdateUserRequest{
		Username:       currentUser.Username,
		Email:          currentUser.Email,
		ServicePassword: currentUser.ServicePassword,
		Roles:          currentUser.Roles,
		Groups:         currentUser.Groups,
		Active:         currentUser.Active,
		MaxSpaces:      currentUser.MaxSpaces,
		ComputeUnits:   currentUser.ComputeUnits,
		StorageUnits:   currentUser.StorageUnits,
		MaxTunnels:     currentUser.MaxTunnels,
		SSHPublicKey:   currentUser.SSHPublicKey,
		GitHubUsername: currentUser.GitHubUsername,
		PreferredShell: currentUser.PreferredShell,
		Timezone:       currentUser.Timezone,
		TOTPSecret:     currentUser.TOTPSecret,
	}

	// Update with provided kwargs
	if username, errObj := kwargs.GetString("username", ""); errObj == nil && username != "" {
		request.Username = username
	}
	if email, errObj := kwargs.GetString("email", ""); errObj == nil && email != "" {
		request.Email = email
	}
	if password, errObj := kwargs.GetString("password", ""); errObj == nil && password != "" {
		request.Password = password
	}
	if active, errObj := kwargs.GetBool("active", currentUser.Active); errObj == nil {
		request.Active = active
	}
	if maxSpaces, errObj := kwargs.GetInt("max_spaces", int64(currentUser.MaxSpaces)); errObj == nil {
		request.MaxSpaces = uint32(maxSpaces)
	}

	apiErr = client.UpdateUser(ctx, userId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update user: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// userDelete deletes a user
func userDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	apiErr := client.DeleteUser(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete user: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// userGetQuota returns user quota and usage
func userGetQuota(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	quota, apiErr := client.GetUserQuota(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get user quota: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["max_spaces"] = object.DictPair{Key: &object.String{Value: "max_spaces"}, Value: &object.Integer{Value: int64(quota.MaxSpaces)}}
	pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(quota.ComputeUnits)}}
	pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(quota.StorageUnits)}}
	pairs["max_tunnels"] = object.DictPair{Key: &object.String{Value: "max_tunnels"}, Value: &object.Integer{Value: int64(quota.MaxTunnels)}}
	pairs["number_spaces"] = object.DictPair{Key: &object.String{Value: "number_spaces"}, Value: &object.Integer{Value: int64(quota.NumberSpaces)}}
	pairs["number_spaces_deployed"] = object.DictPair{Key: &object.String{Value: "number_spaces_deployed"}, Value: &object.Integer{Value: int64(quota.NumberSpacesDeployed)}}
	pairs["used_compute_units"] = object.DictPair{Key: &object.String{Value: "used_compute_units"}, Value: &object.Integer{Value: int64(quota.UsedComputeUnits)}}
	pairs["used_storage_units"] = object.DictPair{Key: &object.String{Value: "used_storage_units"}, Value: &object.Integer{Value: int64(quota.UsedStorageUnits)}}
	pairs["used_tunnels"] = object.DictPair{Key: &object.String{Value: "used_tunnels"}, Value: &object.Integer{Value: int64(quota.UsedTunnels)}}

	return &object.Dict{Pairs: pairs}
}

// userListPermissions returns all permissions for a user
func userListPermissions(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	permissions, apiErr := client.GetUserPermissions(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get user permissions: %v", apiErr)}
	}

	elements := make([]object.Object, len(permissions))
	for i, perm := range permissions {
		elements[i] = &object.Integer{Value: int64(perm)}
	}

	return &object.List{Elements: elements}
}

// userHasPermission checks if user has specific permission
func userHasPermission(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 2); err != nil {
		return err
	}

	userId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	var permissionId uint16
	switch arg := args[1].(type) {
	case *object.Integer:
		permissionId = uint16(arg.Value)
	case *object.String:
		// Try to parse as integer
		val, parseErr := strconv.ParseInt(arg.Value, 10, 16)
		if parseErr != nil {
			return &object.Error{Message: "invalid permission_id: must be integer or integer string"}
		}
		permissionId = uint16(val)
	default:
		return &object.Error{Message: "invalid permission_id type"}
	}

	hasPermission, apiErr := client.HasUserPermission(ctx, userId, permissionId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to check permission: %v", apiErr)}
	}

	return &object.Boolean{Value: hasPermission}
}

// userToDict converts a UserResponse to a scriptling Dict
func userToDict(user *apiclient.UserResponse) object.Object {
	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: user.Id}}
	pairs["username"] = object.DictPair{Key: &object.String{Value: "username"}, Value: &object.String{Value: user.Username}}
	pairs["email"] = object.DictPair{Key: &object.String{Value: "email"}, Value: &object.String{Value: user.Email}}
	pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: user.Active}}
	pairs["max_spaces"] = object.DictPair{Key: &object.String{Value: "max_spaces"}, Value: &object.Integer{Value: int64(user.MaxSpaces)}}
	pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(user.ComputeUnits)}}
	pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(user.StorageUnits)}}
	pairs["max_tunnels"] = object.DictPair{Key: &object.String{Value: "max_tunnels"}, Value: &object.Integer{Value: int64(user.MaxTunnels)}}
	pairs["preferred_shell"] = object.DictPair{Key: &object.String{Value: "preferred_shell"}, Value: &object.String{Value: user.PreferredShell}}
	pairs["timezone"] = object.DictPair{Key: &object.String{Value: "timezone"}, Value: &object.String{Value: user.Timezone}}
	pairs["github_username"] = object.DictPair{Key: &object.String{Value: "github_username"}, Value: &object.String{Value: user.GitHubUsername}}
	pairs["number_spaces"] = object.DictPair{Key: &object.String{Value: "number_spaces"}, Value: &object.Integer{Value: int64(user.NumberSpaces)}}
	pairs["number_spaces_deployed"] = object.DictPair{Key: &object.String{Value: "number_spaces_deployed"}, Value: &object.Integer{Value: int64(user.NumberSpacesDeployed)}}
	pairs["used_compute_units"] = object.DictPair{Key: &object.String{Value: "used_compute_units"}, Value: &object.Integer{Value: int64(user.UsedComputeUnits)}}
	pairs["used_storage_units"] = object.DictPair{Key: &object.String{Value: "used_storage_units"}, Value: &object.Integer{Value: int64(user.UsedStorageUnits)}}
	pairs["used_tunnels"] = object.DictPair{Key: &object.String{Value: "used_tunnels"}, Value: &object.Integer{Value: int64(user.UsedTunnels)}}
	pairs["current"] = object.DictPair{Key: &object.String{Value: "current"}, Value: &object.Boolean{Value: user.Current}}

	// Convert roles list
	roleElements := make([]object.Object, len(user.Roles))
	for i, role := range user.Roles {
		roleElements[i] = &object.String{Value: role}
	}
	pairs["roles"] = object.DictPair{Key: &object.String{Value: "roles"}, Value: &object.List{Elements: roleElements}}

	// Convert groups list
	groupElements := make([]object.Object, len(user.Groups))
	for i, group := range user.Groups {
		groupElements[i] = &object.String{Value: group}
	}
	pairs["groups"] = object.DictPair{Key: &object.String{Value: "groups"}, Value: &object.List{Elements: groupElements}}

	return &object.Dict{Pairs: pairs}
}
