package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// update the user on a leaf node
func (s *Session) UpdateUser(user *model.User, updateFields []string) {
	if s.token == nil || s.token.UserId == user.Id {

		// Don't send the password or TOTP secret to leaf nodes
		user.Password = ""
		user.TOTPSecret = ""

		message := &msg.LeafOriginMessage{
			Command: msg.MSG_UPDATE_USER,
			Payload: &msg.UpdateUser{
				User:         *user,
				UpdateFields: updateFields,
			},
		}

		s.ch <- message
	}
}

// delete the user on a leaf node
func (s *Session) DeleteUser(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_DELETE_USER,
		Payload: &id,
	}

	s.ch <- message
}

// update the user on all leaf nodes
func UpdateUser(user *model.User, updateFields []string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.UpdateUser(user, updateFields)
		}
	}
}

// delete the user on all leaf nodes
func DeleteUser(id string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.DeleteUser(id)
		}
	}
}
