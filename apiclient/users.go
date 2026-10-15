package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

type LinkedUserInfo struct {
	Id       string `json:"user_id" msgpack:"user_id"`
	Username string `json:"username" msgpack:"username"`
}

type UserResponse struct {
	Id                         string           `json:"user_id" msgpack:"user_id"`
	Username                   string           `json:"username" msgpack:"username"`
	SpaceId                    string           `json:"space_id,omitempty" msgpack:"space_id,omitempty"` // whoami from a space's own credential: that space
	Email                      string           `json:"email" msgpack:"email"`
	ServicePassword            string           `json:"service_password" msgpack:"service_password"`
	Roles                      []string         `json:"roles" msgpack:"roles"`
	Groups                     []string         `json:"groups" msgpack:"groups"`
	LinkedUsers                []LinkedUserInfo `json:"linked_users" msgpack:"linked_users"`
	Active                     bool             `json:"active" msgpack:"active"`
	MaxSpaces                  uint32           `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits               uint32           `json:"compute_units" msgpack:"compute_units"`
	StorageUnits               uint32           `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels                 uint32           `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB              uint32           `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets                 uint32           `json:"max_buckets" msgpack:"max_buckets"`
	SSHPublicKey               string           `json:"ssh_public_key" msgpack:"ssh_public_key"`
	SSHPrivateKey              string           `json:"ssh_private_key" msgpack:"ssh_private_key"`
	GitHubUsername             string           `json:"github_username" msgpack:"github_username"`
	PreferredShell             string           `json:"preferred_shell" msgpack:"preferred_shell"`
	Timezone                   string           `json:"timezone" msgpack:"timezone"`
	Current                    bool             `json:"current" msgpack:"current"`
	LastLoginAt                *time.Time       `json:"last_login_at" msgpack:"last_login_at"`
	CreatedAt                  time.Time        `json:"created_at" msgpack:"created_at"`
	UpdatedAt                  time.Time        `json:"updated_at" msgpack:"updated_at"`
	TOTPSecret                 string           `json:"totp_secret" msgpack:"totp_secret"`
	NumberSpaces               int              `json:"number_spaces" msgpack:"number_spaces"`
	NumberSpacesDeployed       int              `json:"number_spaces_deployed" msgpack:"number_spaces_deployed"`
	NumberSpacesDeployedInZone int              `json:"number_spaces_deployed_in_zone" msgpack:"number_spaces_deployed_in_zone"`
	UsedComputeUnits           uint32           `json:"used_compute_units" msgpack:"used_compute_units"`
	UsedStorageUnits           uint32           `json:"used_storage_units" msgpack:"used_storage_units"`
	UsedTunnels                uint32           `json:"used_tunnels" msgpack:"used_tunnels"`
}

type CreateUserRequest struct {
	Username       string   `json:"username" msgpack:"username"`
	Password       string   `json:"password" msgpack:"password"`
	Email          string   `json:"email" msgpack:"email"`
	Roles          []string `json:"roles" msgpack:"roles"`
	Groups         []string `json:"groups" msgpack:"groups"`
	Active         bool     `json:"active" msgpack:"active"`
	MaxSpaces      uint32   `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits   uint32   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits   uint32   `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels     uint32   `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB  uint32   `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets     uint32   `json:"max_buckets" msgpack:"max_buckets"`
	SSHPublicKey   string   `json:"ssh_public_key" msgpack:"ssh_public_key"`
	GitHubUsername string   `json:"github_username" msgpack:"github_username"`
	PreferredShell string   `json:"preferred_shell" msgpack:"preferred_shell"`
	Timezone       string   `json:"timezone" msgpack:"timezone"`
	TOTPSecret     string   `json:"totp_secret" msgpack:"totp_secret"`
}

type UpdateUserRequest struct {
	Username        string   `json:"username" msgpack:"username"`
	Password        string   `json:"password" msgpack:"password"`
	ServicePassword string   `json:"service_password" msgpack:"service_password"`
	Email           string   `json:"email" msgpack:"email"`
	Roles           []string `json:"roles" msgpack:"roles"`
	Groups          []string `json:"groups" msgpack:"groups"`
	Active          bool     `json:"active" msgpack:"active"`
	MaxSpaces       uint32   `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits    uint32   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits    uint32   `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels      uint32   `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB   uint32   `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets      uint32   `json:"max_buckets" msgpack:"max_buckets"`
	SSHPublicKey    string   `json:"ssh_public_key" msgpack:"ssh_public_key"`
	GitHubUsername  string   `json:"github_username" msgpack:"github_username"`
	PreferredShell  string   `json:"preferred_shell" msgpack:"preferred_shell"`
	Timezone        string   `json:"timezone" msgpack:"timezone"`
	TOTPSecret      string   `json:"totp_secret" msgpack:"totp_secret"`
}

type CreateUserResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	UserId string `json:"user_id" msgpack:"user_id"`
}

type UserInfo struct {
	Id                         string     `json:"user_id" msgpack:"user_id"`
	Username                   string     `json:"username" msgpack:"username"`
	Email                      string     `json:"email" msgpack:"email"`
	Roles                      []string   `json:"roles" msgpack:"roles"`
	Groups                     []string   `json:"groups" msgpack:"groups"`
	Active                     bool       `json:"active" msgpack:"active"`
	MaxSpaces                  uint32     `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits               uint32     `json:"compute_units" msgpack:"compute_units"`
	StorageUnits               uint32     `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels                 uint32     `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB              uint32     `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets                 uint32     `json:"max_buckets" msgpack:"max_buckets"`
	Current                    bool       `json:"current" msgpack:"current"`
	HasLinkedUsers             bool       `json:"has_linked_users" msgpack:"has_linked_users"`
	LastLoginAt                *time.Time `json:"last_login_at" msgpack:"last_login_at"`
	NumberSpaces               int        `json:"number_spaces" msgpack:"number_spaces"`
	NumberSpacesDeployed       int        `json:"number_spaces_deployed" msgpack:"number_spaces_deployed"`
	NumberSpacesDeployedInZone int        `json:"number_spaces_deployed_in_zone" msgpack:"number_spaces_deployed_in_zone"`
	UsedComputeUnits           uint32     `json:"used_compute_units" msgpack:"used_compute_units"`
	UsedStorageUnits           uint32     `json:"used_storage_units" msgpack:"used_storage_units"`
	UsedTunnels                uint32     `json:"used_tunnels" msgpack:"used_tunnels"`
	UsedFileStorageMB          float64    `json:"used_file_storage_mb" msgpack:"used_file_storage_mb"`
	UsedBuckets                int        `json:"used_buckets" msgpack:"used_buckets"`
}
type UserInfoList struct {
	Count int        `json:"count" msgpack:"count"`
	Users []UserInfo `json:"users" msgpack:"users"`
}

type UserQuota struct {
	MaxSpaces            uint32  `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits         uint32  `json:"compute_units" msgpack:"compute_units"`
	StorageUnits         uint32  `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels           uint32  `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB        uint32  `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets           uint32  `json:"max_buckets" msgpack:"max_buckets"`
	NumberSpaces         int     `json:"number_spaces" msgpack:"number_spaces"`
	NumberSpacesDeployed int     `json:"number_spaces_deployed" msgpack:"number_spaces_deployed"`
	UsedComputeUnits     uint32  `json:"used_compute_units" msgpack:"used_compute_units"`
	UsedStorageUnits     uint32  `json:"used_storage_units" msgpack:"used_storage_units"`
	UsedTunnels          uint32  `json:"used_tunnels" msgpack:"used_tunnels"`
	UsedFileStorageMB    float64 `json:"used_file_storage_mb" msgpack:"used_file_storage_mb"`
	UsedBuckets          int     `json:"used_buckets" msgpack:"used_buckets"`
}

type UserPermissions struct {
	Permissions       []uint16 `json:"permissions" msgpack:"permissions"`
	PluginPermissions []string `json:"plugin_permissions,omitempty" msgpack:"plugin_permissions,omitempty"`
}

type UserHasPermission struct {
	HasPermission bool `json:"has_permission" msgpack:"has_permission"`
}

type UpdateOwnSSHPublicKeyRequest struct {
	SSHPublicKey   string `json:"ssh_public_key" msgpack:"ssh_public_key"`
	GitHubUsername string `json:"github_username" msgpack:"github_username"`
}

type UpdateOwnSSHPrivateKeyRequest struct {
	SSHPrivateKey string `json:"ssh_private_key" msgpack:"ssh_private_key"`
}

// ValidNavURLs is the set of core sidebar URLs that may be pinned to the top
// of the navigation. The server validates preference updates against it so
// clients can't store arbitrary strings in the preferences blob; plugin menu
// URLs are validated separately against the menus the user can currently
// see (plugins.Registry.VisibleMenuURLs).
var ValidNavURLs = map[string]bool{
	"/spaces": true, "/tunnels": true, "/api-tokens": true, "/volumes": true,
	"/files":     true,
	"/templates": true, "/variables": true, "/stacks": true, "/scripts": true,
	"/events": true, "/skills": true, "/commands": true, "/mcp-servers": true,
	"/users": true, "/groups": true, "/roles": true, "/audit-logs": true,
	"/cluster-info": true,
}

// UpdateOwnNavPreferencesRequest replaces the current user's pinned sidebar
// ordering. URLs are validated against ValidNavURLs and de-duplicated, so the
// stored order may differ from the request. An empty slice clears the
// preference and returns the sidebar to its default layout.
type UpdateOwnNavPreferencesRequest struct {
	Starred []string `json:"starred" msgpack:"starred"`
}

// NavPreferences is the pinned-sidebar state for the current user.
type NavPreferences struct {
	Starred []string `json:"starred" msgpack:"starred"`
}

func (c *ApiClient) CreateUser(ctx context.Context, request *CreateUserRequest) (string, int, error) {
	response := CreateUserResponse{}

	code, err := c.httpClient.Post(ctx, "/api/users", request, &response, 201)
	if err != nil {
		return "", code, err
	}

	return response.UserId, code, nil
}

func (c *ApiClient) GetUser(ctx context.Context, userId string) (*UserResponse, error) {
	response := UserResponse{}

	code, err := c.httpClient.Get(ctx, "/api/users/"+userId, &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	return &response, nil
}

func (c *ApiClient) WhoAmI(ctx context.Context) (*UserResponse, error) {
	response := UserResponse{}

	_, err := c.httpClient.Get(ctx, "/api/users/whoami", &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *ApiClient) GetUsers(ctx context.Context, state string, zone string) (*UserInfoList, error) {
	response := UserInfoList{}

	stateEncoded := url.QueryEscape(state)
	zoneEncoded := url.QueryEscape(zone)

	_, err := c.httpClient.Get(ctx, "/api/users?state="+stateEncoded+"&zone="+zoneEncoded, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *ApiClient) UpdateUser(ctx context.Context, userId string, user *UpdateUserRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/users/"+userId, &user, nil, 200)
	if err != nil {
		return err
	}

	return nil
}

func (c *ApiClient) UpdateOwnSSHPublicKey(ctx context.Context, request *UpdateOwnSSHPublicKeyRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/users/whoami/ssh-public-key", request, nil, 200)
	return err
}

func (c *ApiClient) UpdateOwnSSHPrivateKey(ctx context.Context, request *UpdateOwnSSHPrivateKeyRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/users/whoami/ssh-private-key", request, nil, 200)
	return err
}

// GetOwnNavPreferences returns the current user's pinned sidebar URLs in display order.
func (c *ApiClient) GetOwnNavPreferences(ctx context.Context) (*NavPreferences, error) {
	response := NavPreferences{}
	_, err := c.httpClient.Get(ctx, "/api/users/preferences/nav", &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// UpdateOwnNavPreferences replaces the current user's pinned sidebar ordering.
// The returned NavPreferences reflects the canonical (validated, de-duplicated)
// order stored by the server.
func (c *ApiClient) UpdateOwnNavPreferences(ctx context.Context, request *UpdateOwnNavPreferencesRequest) (*NavPreferences, error) {
	response := NavPreferences{}
	_, err := c.httpClient.Put(ctx, "/api/users/preferences/nav", request, &response, 200)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *ApiClient) DeleteUser(ctx context.Context, userId string) error {
	_, err := c.httpClient.Delete(ctx, "/api/users/"+userId, nil, nil, 200)
	return err
}

func (c *ApiClient) GetUserQuota(ctx context.Context, userId string) (*UserQuota, error) {
	response := UserQuota{}

	code, err := c.httpClient.Get(ctx, "/api/users/"+userId+"/quota", &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	return &response, nil
}

// GetUserPermissionsFull returns both halves of a user's effective
// permissions: the built-in ids resolved from roles, and the qualified
// plugin grants.
func (c *ApiClient) GetUserPermissionsFull(ctx context.Context, userId string) (*UserPermissions, error) {
	response := UserPermissions{}

	code, err := c.httpClient.Get(ctx, "/api/users/"+userId+"/permissions", &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return &response, nil
}

func (c *ApiClient) GetUserPermissions(ctx context.Context, userId string) ([]uint16, error) {
	response := UserPermissions{}

	code, err := c.httpClient.Get(ctx, "/api/users/"+userId+"/permissions", &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	return response.Permissions, nil
}

func (c *ApiClient) HasUserPermission(ctx context.Context, userId string, permission uint16) (bool, error) {
	response := UserHasPermission{}

	code, err := c.httpClient.Get(ctx, "/api/users/"+userId+"/has-permission?permission="+url.QueryEscape(fmt.Sprintf("%d", permission)), &response)
	if err != nil {
		if code == 404 {
			return false, errors.New("user not found")
		} else {
			return false, err
		}
	}

	return response.HasPermission, nil
}
