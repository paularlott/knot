package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// update the template var on a leaf node
func (s *Session) UpdateTemplateVar(templateVar *model.TemplateVar) {
	// Only send vars that match the location or are global
	if templateVar.Location == "" || templateVar.Location == s.location {
		message := &msg.ClientMessage{
			Command: msg.MSG_UPDATE_TEMPLATEVAR,
			Payload: templateVar,
		}

		s.ch <- message
	}
}

// delete the template var on a leaf node
func (s *Session) DeleteTemplateVar(id string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_DELETE_TEMPLATEVAR,
		Payload: &id,
	}

	s.ch <- message
}

// update the template var on all leaf nodes
func UpdateTemplateVar(templateVar *model.TemplateVar) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		session.UpdateTemplateVar(templateVar)
	}
}

// delete the template var on all leaf nodes
func DeleteTemplateVar(id string) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	// Send the user to all followers
	for _, session := range session {
		session.DeleteTemplateVar(id)
	}
}
