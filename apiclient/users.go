package apiclient

import (
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
