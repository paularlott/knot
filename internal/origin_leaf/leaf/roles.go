package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

func (s *Session) UpdateRole(role *model.Role) {
	message := &msg.ClientMessage{
		Command: msg.MSG_UPDATE_ROLE,
		Payload: role,
	}

	s.ch <- message
}

// delete the role on a leaf node
func (s *Session) DeleteRole(id string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_DELETE_ROLE,
		Payload: &id,
	}

	s.ch <- message
}

// delete the role on all leaf nodes
func DeleteRole(id string) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		session.DeleteRole(id)
	}
}

// update the role on all leaf nodes
func UpdateRole(role *model.Role) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		session.UpdateRole(role)
	}
}
