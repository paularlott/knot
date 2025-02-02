package apiclient

import (
	"errors"
	"net/url"
	"time"

	"github.com/paularlott/knot/database/model"
)

type UserResponse struct {
	Id              string     `json:"user_id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	ServicePassword string     `json:"service_password"`
	Roles           []string   `json:"roles"`
	Groups          []string   `json:"groups"`
	Active          bool       `json:"active"`
	MaxSpaces       uint32     `json:"max_spaces"`
	ComputeUnits    uint32     `json:"compute_units"`
	StorageUnits    uint32     `json:"storage_units"`
	MaxTunnels      uint32     `json:"max_tunnels"`
	SSHPublicKey    string     `json:"ssh_public_key"`
	GitHubUsername  string     `json:"github_username"`
	PreferredShell  string     `json:"preferred_shell"`
	Timezone        string     `json:"timezone"`
	Current         bool       `json:"current"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	TOTPSecret      string     `json:"totp_secret"`
}

type userRequest struct {
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	ServicePassword string   `json:"service_password"`
	Email           string   `json:"email"`
	Roles           []string `json:"roles"`
	Groups          []string `json:"groups"`
	Active          bool     `json:"active"`
	MaxSpaces       uint32   `json:"max_spaces"`
	ComputeUnits    uint32   `json:"compute_units"`
	StorageUnits    uint32   `json:"storage_units"`
	MaxTunnels      uint32   `json:"max_tunnels"`
	SSHPublicKey    string   `json:"ssh_public_key"`
	GitHubUsername  string   `json:"github_username"`
	PreferredShell  string   `json:"preferred_shell"`
	Timezone        string   `json:"timezone"`
	TOTPSecret      string   `json:"totp_secret"`
}
type CreateUserRequest = userRequest
type UpdateUserRequest = userRequest

type CreateUserResponse struct {
	Status bool   `json:"status"`
	UserId string `json:"user_id"`
}

type UserInfo struct {
	Id                             string     `json:"user_id"`
	Username                       string     `json:"username"`
	Email                          string     `json:"email"`
	Roles                          []string   `json:"roles"`
	Groups                         []string   `json:"groups"`
	Active                         bool       `json:"active"`
	MaxSpaces                      uint32     `json:"max_spaces"`
	ComputeUnits                   uint32     `json:"compute_units"`
	StorageUnits                   uint32     `json:"storage_units"`
	MaxTunnels                     uint32     `json:"max_tunnels"`
	Current                        bool       `json:"current"`
	LastLoginAt                    *time.Time `json:"last_login_at"`
	NumberSpaces                   int        `json:"number_spaces"`
	NumberSpacesDeployed           int        `json:"number_spaces_deployed"`
	NumberSpacesDeployedInLocation int        `json:"number_spaces_deployed_in_location"`
	UsedComputeUnits               uint32     `json:"used_compute_units"`
	UsedStorageUnits               uint32     `json:"used_storage_units"`
}
type UserInfoList struct {
	Count int        `json:"count"`
	Users []UserInfo `json:"users"`
}

type UserQuota struct {
	MaxSpaces            uint32 `json:"max_spaces"`
	ComputeUnits         uint32 `json:"compute_units"`
	StorageUnits         uint32 `json:"storage_units"`
	MaxTunnels           uint32 `json:"max_tunnels"`
	NumberSpaces         int    `json:"number_spaces"`
	NumberSpacesDeployed int    `json:"number_spaces_deployed"`
	UsedComputeUnits     uint32 `json:"used_compute_units"`
	UsedStorageUnits     uint32 `json:"used_storage_units"`
	UsedTunnels          uint32 `json:"used_tunnels"`
}

func (c *ApiClient) CreateUser(request *CreateUserRequest) (string, int, error) {
	response := CreateUserResponse{}

	code, err := c.httpClient.Post("/api/v1/users", request, &response, 201)
	if err != nil {
		return "", code, err
	}

	return response.UserId, code, nil
}

func (c *ApiClient) GetUser(userId string) (*model.User, error) {
	response := UserResponse{}

	code, err := c.httpClient.Get("/api/v1/users/"+userId, &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	user := &model.User{
		Id:              response.Id,
		Username:        response.Username,
		Email:           response.Email,
		ServicePassword: response.ServicePassword,
		SSHPublicKey:    response.SSHPublicKey,
		GitHubUsername:  response.GitHubUsername,
		Roles:           response.Roles,
		Groups:          response.Groups,
		Active:          response.Active,
		MaxSpaces:       response.MaxSpaces,
		ComputeUnits:    response.ComputeUnits,
		StorageUnits:    response.StorageUnits,
		MaxTunnels:      response.MaxTunnels,
		PreferredShell:  response.PreferredShell,
		Timezone:        response.Timezone,
		LastLoginAt:     response.LastLoginAt,
		CreatedAt:       response.CreatedAt,
		UpdatedAt:       response.UpdatedAt,
		TOTPSecret:      response.TOTPSecret,
	}

	return user, nil
}

func (c *ApiClient) WhoAmI() (*model.User, error) {
	response := UserResponse{}

	_, err := c.httpClient.Get("/api/v1/users/whoami", &response)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Id:              response.Id,
		Username:        response.Username,
		Email:           response.Email,
		ServicePassword: response.ServicePassword,
		SSHPublicKey:    response.SSHPublicKey,
		GitHubUsername:  response.GitHubUsername,
		Roles:           response.Roles,
		Groups:          response.Groups,
		Active:          response.Active,
		MaxSpaces:       response.MaxSpaces,
		ComputeUnits:    response.ComputeUnits,
		StorageUnits:    response.StorageUnits,
		MaxTunnels:      response.MaxTunnels,
		PreferredShell:  response.PreferredShell,
		Timezone:        response.Timezone,
		LastLoginAt:     response.LastLoginAt,
		CreatedAt:       response.CreatedAt,
		UpdatedAt:       response.UpdatedAt,
	}

	return user, nil
}

func (c *ApiClient) GetUsers(state string, location string) (*UserInfoList, error) {
	response := UserInfoList{}

	stateEncoded := url.QueryEscape(state)
	locationEncoded := url.QueryEscape(location)

	_, err := c.httpClient.Get("/api/v1/users?state="+stateEncoded+"&location="+locationEncoded, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *ApiClient) UpdateUser(user *model.User) error {
	request := UpdateUserRequest{
		Username:        user.Username,
		Password:        user.Password,
		Email:           user.Email,
		ServicePassword: user.ServicePassword,
		Roles:           user.Roles,
		Groups:          user.Groups,
		Active:          user.Active,
		MaxSpaces:       user.MaxSpaces,
		ComputeUnits:    user.ComputeUnits,
		StorageUnits:    user.StorageUnits,
		MaxTunnels:      user.MaxTunnels,
		SSHPublicKey:    user.SSHPublicKey,
		GitHubUsername:  user.GitHubUsername,
		PreferredShell:  user.PreferredShell,
		Timezone:        user.Timezone,
		TOTPSecret:      user.TOTPSecret,
	}

	_, err := c.httpClient.Put("/api/v1/users/"+user.Id, &request, nil, 200)
	if err != nil {
		return err
	}

	return nil
}

func (c *ApiClient) DeleteUser(userId string) error {
	_, err := c.httpClient.Delete("/api/v1/users/"+userId, nil, nil, 200)
	return err
}

func (c *ApiClient) GetUserQuota(userId string) (*UserQuota, error) {
	response := UserQuota{}

	code, err := c.httpClient.Get("/api/v1/users/"+userId+"/quota", &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	return &response, nil
}
