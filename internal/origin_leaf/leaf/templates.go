package leaf

import (
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
)

// update the template on a leaf node
func (s *Session) UpdateTemplate(template *model.Template) {
	message := &msg.ClientMessage{
		Command: msg.MSG_UPDATE_TEMPLATE,
		Payload: template,
	}

	s.ch <- message
}

// delete the template on a leaf node
func (s *Session) DeleteTemplate(templateId string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_DELETE_TEMPLATE,
		Payload: &templateId,
	}

	s.ch <- message
}

// update the template on all leaf nodes
func UpdateTemplate(template *model.Template) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	for _, session := range session {
		session.UpdateTemplate(template)
	}
}

// delete the template on all leaf nodes
func DeleteTemplate(templateId string) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	for _, session := range session {
		session.DeleteTemplate(templateId)
	}
}
