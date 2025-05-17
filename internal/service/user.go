package service

import (
	"github.com/paularlott/knot/internal/database/model"
)

// UserService defines operations that can be performed on users
type UserService interface {
	// User deletion operations
	DeleteUser(user *model.User) error
	RemoveUsersSessions(user *model.User)
	RemoveUsersTokens(user *model.User)

	// SSH key and space management
	UpdateUserSpaces(user *model.User)
	UpdateSpacesSSHKey(user *model.User)
	UpdateSpaceSSHKeys(space *model.Space, user *model.User)
}

var userService UserService

func SetUserService(service UserService) {
	userService = service
}

func GetUserService() UserService {
	return userService
}
