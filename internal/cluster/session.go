package cluster

import (
	"time"

	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/log"
)

const (
	leafSessionQueueSize = 100
)

type leafSessionMsg struct {
	Type    leafmsg.MessageType
	Payload interface{}
}

type leafSession struct {
	Id    uuid.UUID
	Zone  string
	ws    *websocket.Conn
	ch    chan *leafSessionMsg
	user  *model.User
	token *model.Token
}

func (s *leafSession) SendMessage(msgType leafmsg.MessageType, payload interface{}) {
	select {
	case s.ch <- &leafSessionMsg{
		Type:    msgType,
		Payload: payload,
	}:

	default:
		log.Error("failed to send message: queue is full", "zone", s.Zone)
	}
}

func (c *Cluster) registerLeaf(ws *websocket.Conn, user *model.User, token *model.Token, zone string) *leafSession {
	newSessionId, err := uuid.NewV7()
	if err != nil {
		c.logger.Fatal("failed to create leaf ID:", "err", err)
	}

	c.leafSessionMux.Lock()
	defer c.leafSessionMux.Unlock()

	session := &leafSession{
		Id:    newSessionId,
		Zone:  zone,
		ws:    ws,
		ch:    make(chan *leafSessionMsg, leafSessionQueueSize),
		user:  user,
		token: token,
	}
	c.leafSessions[newSessionId] = session

	// Start a go routine to listen for messages on ch and send to the leaf
	go func() {
		for message := range session.ch {
			attempts := 3
			for i := 1; i <= attempts; i++ {
				err := leafmsg.WriteMessage(ws, message.Type, message.Payload)
				if err == nil {
					break
				}

				log.Warn("attempt failed to write message to leaf", "error", err, "zone", session.Zone, "i", i)

				time.Sleep(time.Duration(i) * time.Second)

				if i == attempts {
					log.Error("failed to write message to leaf after attempts", "zone", session.Zone, "attempts", attempts)
				}
			}
		}
	}()

	return session
}

func (c *Cluster) unregisterLeaf(session *leafSession) {
	c.leafSessionMux.Lock()
	defer c.leafSessionMux.Unlock()

	delete(c.leafSessions, session.Id)
	close(session.ch)
}

func (c *Cluster) sendToLeafNodes(msgType leafmsg.MessageType, payload interface{}) {
	c.leafSessionMux.RLock()
	defer c.leafSessionMux.RUnlock()

	for _, session := range c.leafSessions {
		// Filter templates and skills by user groups
		if msgType == leafmsg.MessageGossipTemplate {
			templates := payload.(*[]*model.Template)
			filteredTemplates := []*model.Template{}
			for _, template := range *templates {
				// Check if template matches user's groups
				matches := len(template.Groups) == 0
				if !matches {
					for _, groupId := range template.Groups {
						for _, userGroupId := range session.user.Groups {
							if groupId == userGroupId {
								matches = true
								break
							}
						}
						if matches {
							break
						}
					}
				}
				// If matches, send as-is; if doesn't match and not already deleted, mark as deleted
				if matches {
					filteredTemplates = append(filteredTemplates, template)
				} else if !template.IsDeleted {
					// Send as deleted to remove from leaf
					deletedTemplate := *template
					deletedTemplate.IsDeleted = true
					filteredTemplates = append(filteredTemplates, &deletedTemplate)
				}
			}
			if len(filteredTemplates) > 0 {
				session.SendMessage(msgType, &filteredTemplates)
			}
		} else if msgType == leafmsg.MessageGossipSkill {
			skills := payload.(*[]*model.Skill)
			filteredSkills := []*model.Skill{}
			for _, skill := range *skills {
				// Check if skill matches user's groups
				matches := len(skill.Groups) == 0
				if !matches {
					for _, groupId := range skill.Groups {
						for _, userGroupId := range session.user.Groups {
							if groupId == userGroupId {
								matches = true
								break
							}
						}
						if matches {
							break
						}
					}
				}
				// If matches, send as-is; if doesn't match and not already deleted, mark as deleted
				if matches {
					filteredSkills = append(filteredSkills, skill)
				} else if !skill.IsDeleted {
					// Send as deleted to remove from leaf
					deletedSkill := *skill
					deletedSkill.IsDeleted = true
					filteredSkills = append(filteredSkills, &deletedSkill)
				}
			}
			if len(filteredSkills) > 0 {
				session.SendMessage(msgType, &filteredSkills)
			}
		} else if msgType == leafmsg.MessageGossipStackDefinition {
			defs := payload.(*[]*model.StackDefinition)
			filteredDefs := []*model.StackDefinition{}
			for _, def := range *defs {
				// Check if stack definition matches user's groups
				matches := len(def.Groups) == 0
				if !matches {
					for _, groupId := range def.Groups {
						for _, userGroupId := range session.user.Groups {
							if groupId == userGroupId {
								matches = true
								break
							}
						}
						if matches {
							break
						}
					}
				}
				// If matches, send as-is; if doesn't match and not already deleted, mark as deleted
				if matches {
					filteredDefs = append(filteredDefs, def)
				} else if !def.IsDeleted {
					deletedDef := *def
					deletedDef.IsDeleted = true
					filteredDefs = append(filteredDefs, &deletedDef)
				}
			}
			if len(filteredDefs) > 0 {
				session.SendMessage(msgType, &filteredDefs)
			}
		} else {
			session.SendMessage(msgType, payload)
		}
	}
}
