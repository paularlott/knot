package apiclient

import (
	"errors"
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
	MaxSpaces       int        `json:"max_spaces"`
	MaxDiskSpace    int        `json:"max_disk_space"`
	SSHPublicKey    string     `json:"ssh_public_key"`
	PreferredShell  string     `json:"preferred_shell"`
	Timezone        string     `json:"timezone"`
	Current         bool       `json:"current"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type userRequest struct {
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	ServicePassword string   `json:"service_password"`
	Email           string   `json:"email"`
	Roles           []string `json:"roles"`
	Groups          []string `json:"groups"`
	Active          bool     `json:"active"`
	MaxSpaces       int      `json:"max_spaces"`
	MaxDiskSpace    int      `json:"max_disk_space"`
	SSHPublicKey    string   `json:"ssh_public_key"`
	PreferredShell  string   `json:"preferred_shell"`
	Timezone        string   `json:"timezone"`
}
type CreateUserRequest = userRequest
type UpdateUserRequest = userRequest

type CreateUserResponse struct {
	Status bool   `json:"status"`
	UserId string `json:"user_id"`
}

type UserInfoResponse struct {
	Id                   string     `json:"user_id"`
	Username             string     `json:"username"`
	Email                string     `json:"email"`
	Roles                []string   `json:"roles"`
	Groups               []string   `json:"groups"`
	Active               bool       `json:"active"`
	MaxSpaces            int        `json:"max_spaces"`
	MaxDiskSpace         int        `json:"max_disk_space"`
	Current              bool       `json:"current"`
	LastLoginAt          *time.Time `json:"last_login_at"`
	NumberSpaces         int        `json:"number_spaces"`
	NumberSpacesDeployed int        `json:"number_spaces_deployed"`
	UsedDiskSpace        int        `json:"used_disk_space"`
}
type UserInfo = UserInfoResponse

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
		Roles:           response.Roles,
		Groups:          response.Groups,
		Active:          response.Active,
		MaxSpaces:       response.MaxSpaces,
		MaxDiskSpace:    response.MaxDiskSpace,
		PreferredShell:  response.PreferredShell,
		Timezone:        response.Timezone,
		LastLoginAt:     response.LastLoginAt,
		CreatedAt:       response.CreatedAt,
		UpdatedAt:       response.UpdatedAt,
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
		Roles:           response.Roles,
		Groups:          response.Groups,
		Active:          response.Active,
		MaxSpaces:       response.MaxSpaces,
		MaxDiskSpace:    response.MaxDiskSpace,
		PreferredShell:  response.PreferredShell,
		Timezone:        response.Timezone,
		LastLoginAt:     response.LastLoginAt,
		CreatedAt:       response.CreatedAt,
		UpdatedAt:       response.UpdatedAt,
	}

	return user, nil
}

func (c *ApiClient) GetUsers(state string) (*[]UserInfo, error) {
	response := []UserInfo{}

	_, err := c.httpClient.Get("/api/v1/users?state="+state, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *ApiClient) UpdateUser(user *model.User) error {
	request := UpdateUserRequest{
		Username:        user.Username,
		Email:           user.Email,
		ServicePassword: user.ServicePassword,
		Roles:           user.Roles,
		Groups:          user.Groups,
		Active:          user.Active,
		MaxSpaces:       user.MaxSpaces,
		MaxDiskSpace:    user.MaxDiskSpace,
		SSHPublicKey:    user.SSHPublicKey,
		PreferredShell:  user.PreferredShell,
		Timezone:        user.Timezone,
	}

	_, err := c.httpClient.Put("/api/v1/users/"+user.Id, &request, nil, 200)
	if err != nil {
		return err
	}

	return nil
}

func (c *ApiClient) DeleteUser(userId string) error {
	_, err := c.httpClient.Delete("/api/v1/users/"+userId, nil, nil, 200)
	if err != nil {
		return err
	}

	return nil
}
