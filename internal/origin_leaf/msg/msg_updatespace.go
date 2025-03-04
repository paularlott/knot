package msg

import "github.com/paularlott/knot/database/model"

// message sent from server to a follower to update a space
type UpdateSpace struct {
	Space        model.Space
	UpdateFields []string
}

type SyncUserSpaces struct {
	UserId   string
	Existing []string
}
