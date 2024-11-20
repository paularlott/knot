package leaf

import (
	"sync"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

type Session struct {
	ws       *websocket.Conn
	ch       chan *msg.ClientMessage
	location string
	token    *model.Token
}

var (
	sessionMutex sync.RWMutex        = sync.RWMutex{}
	session      map[string]*Session = make(map[string]*Session)
)

func Register(id string, ws *websocket.Conn, location string, token *model.Token) *Session {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	leaf := &Session{
		ws:       ws,
		ch:       make(chan *msg.ClientMessage, 100),
		location: location,
		token:    token,
	}
	session[id] = leaf

	// Start a go routine to listen for messages on ch and send to the follower
	go func() {
		for {
			select {
			case message, ok := <-leaf.ch:
				if !ok {
					// Channel closed, exit
					return
				}

				if err := msg.WriteCommand(ws, message.Command); err != nil {
					log.Error().Msgf("error writing command to leaf (%s): %s", id, err)

					Unregister(id)
					return
				}

				if message.Payload != nil {
					if err := msg.WriteMessage(ws, message.Payload); err != nil {
						log.Error().Msgf("error writing message to leaf (%s): %s", id, err)

						Unregister(id)
						return
					}
				}
			}
		}
	}()

	return leaf
}

func Unregister(id string) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	// Stop the go routine
	close(session[id].ch)
	delete(session, id)
}

func (s *Session) Ping() {
	message := &msg.ClientMessage{
		Command: msg.MSG_PING,
		Payload: nil,
	}

	s.ch <- message
}

func (s *Session) Bootstrap() {
	message := &msg.ClientMessage{
		Command: msg.MSG_BOOTSTRAP,
		Payload: nil,
	}

	s.ch <- message
}
