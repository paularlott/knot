package leaf

import (
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// delete the space on a leaf node
func (s *Session) DeleteToken(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_DELETE_TOKEN,
		Payload: &id,
	}

	s.ch <- message
}

// delete the space on all leaf nodes
func DeleteToken(id string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.DeleteToken(id)
		}
	}
}
