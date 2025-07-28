package cluster

import (
	"time"

	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
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
		log.Error().Str("zone", s.Zone).Msg("failed to send message: queue is full")
	}
}

func (c *Cluster) registerLeaf(ws *websocket.Conn, user *model.User, token *model.Token, zone string) *leafSession {
	newSessionId, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msgf("origin: failed to create leaf ID: %s", err)
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

				log.Warn().Err(err).Str("zone", session.Zone).Msgf("attempt %d failed to write message to leaf", i)

				time.Sleep(time.Duration(i) * time.Second)

				if i == attempts {
					log.Error().Str("zone", session.Zone).Msgf("failed to write message to leaf after %d attempts", attempts)
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
		session.SendMessage(msgType, payload)
	}
}
