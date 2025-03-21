package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// update the space on a leaf node
func (s *Session) UpdateSpace(space *model.Space, updateFields []string) bool {
	if s.token == nil || space.UserId == s.token.UserId {
		message := &msg.LeafOriginMessage{
			Command: msg.MSG_UPDATE_SPACE,
			Payload: &msg.UpdateSpace{
				Space:        *space,
				UpdateFields: updateFields,
			},
		}

		s.ch <- message

		return true
	} else {
		return false
	}
}

// delete the space on a leaf node
func (s *Session) DeleteSpace(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_DELETE_SPACE,
		Payload: &id,
	}

	s.ch <- message
}

// update the space on all leaf nodes
func UpdateSpace(space *model.Space, updateFields []string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the space to all followers
	for _, session := range session {
		if session != skipSession {
			session.UpdateSpace(space, updateFields)
		}
	}
}

// delete the space on all leaf nodes
func DeleteSpace(id string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.DeleteSpace(id)
		}
	}
}
