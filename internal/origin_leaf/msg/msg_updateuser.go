package msg

import "github.com/paularlott/knot/database/model"

// message sent from server to a left to update a user
type UpdateUser struct {
	User         model.User
	UpdateFields []string
}
