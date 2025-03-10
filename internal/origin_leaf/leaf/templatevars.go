package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// update the template var on a leaf node
func (s *Session) UpdateTemplateVar(templateVar *model.TemplateVar) bool {

	// if token given then don't send protected or restricted vars
	if s.token == nil || (!templateVar.Protected && !templateVar.Restricted) {
		// Only send vars that match the location or are global, never local
		if !templateVar.Local && (templateVar.Location == "" || templateVar.Location == s.location) {
			message := &msg.LeafOriginMessage{
				Command: msg.MSG_UPDATE_TEMPLATEVAR,
				Payload: templateVar,
			}

			s.ch <- message
		} else {
			return false
		}

		return true
	} else {
		return false
	}
}

// delete the template var on a leaf node
func (s *Session) DeleteTemplateVar(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_DELETE_TEMPLATEVAR,
		Payload: &id,
	}

	s.ch <- message
}

// update the template var on all leaf nodes
func UpdateTemplateVar(templateVar *model.TemplateVar, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.UpdateTemplateVar(templateVar)
		}
	}
}

// delete the template var on all leaf nodes
func DeleteTemplateVar(id string, skipSession *Session) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		if session != skipSession {
			session.DeleteTemplateVar(id)
		}
	}
}
