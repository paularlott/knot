package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

func (s *Session) UpdateRole(role *model.Role) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_UPDATE_ROLE,
		Payload: role,
	}

	s.ch <- message
}

// delete the role on a leaf node
func (s *Session) DeleteRole(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_DELETE_ROLE,
		Payload: &id,
	}

	s.ch <- message
}

// delete the role on all leaf nodes
func DeleteRole(id string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.DeleteRole(id)
		}
	}
}

// update the role on all leaf nodes
func UpdateRole(role *model.Role, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.UpdateRole(role)
		}
	}
}
