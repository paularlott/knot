package leaf

import (
	"sync"
	"time"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/msg"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type Session struct {
	Id       string
	ws       *websocket.Conn
	ch       chan *msg.LeafOriginMessage
	location string
	token    *model.Token
	expires  time.Time
}

const (
	SESSION_MAX_AGE = 30 * time.Second
	SESSION_GC_RATE = 15 * time.Second
	SESSION_Q_LEN   = 100
)

var (
	sessionMutex sync.RWMutex        = sync.RWMutex{}
	session      map[string]*Session = make(map[string]*Session)
)

func Gc() {
	for {
		time.Sleep(SESSION_GC_RATE)

		sessionMutex.Lock()

		now := time.Now().UTC()
		for id, leaf := range session {
			if leaf.expires.Before(now) {
				log.Debug().Msgf("origin: removing expired session %s", id)
				leaf.Destroy()
				delete(session, id)
			}
		}

		sessionMutex.Unlock()
	}
}

func Register(id string, ws *websocket.Conn, location string, token *model.Token) *Session {
	var leaf *Session

	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	now := time.Now().UTC()

	// Check if the session already exists
	var ok bool
	if leaf, ok = session[id]; ok {
		log.Debug().Msgf("origin: reusing session %s", id)

		// Update the expiration time
		leaf.expires = now.Add(SESSION_MAX_AGE)
		leaf.ws = ws
		leaf.location = location
		leaf.token = token
	} else {
		newSessionId, err := uuid.NewV7()
		if err != nil {
			log.Fatal().Msgf("origin: failed to create leaf ID: %s", err)
		}
		id = newSessionId.String()

		log.Debug().Msgf("origin: creating new session %s", id)

		// Creating a new session and adding it to the map
		leaf = &Session{
			Id:       id,
			ws:       ws,
			ch:       make(chan *msg.LeafOriginMessage, SESSION_Q_LEN),
			location: location,
			token:    token,
			expires:  now.Add(SESSION_MAX_AGE),
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

					attempts := 3
					for i := 1; i <= attempts; i++ {
						err := msg.WriteMessage(ws, message.Command, message.Payload)
						if err == nil {
							break
						}

						log.Warn().Msgf("attempt %d failed to write message to leaf (%s): %s", i, id, err)

						time.Sleep(time.Duration(i) * time.Second)

						if i == attempts {
							log.Error().Msgf("failed to write message to leaf (%s) after %d attempts: %s", id, attempts, err)
							Destroy(id)
							return
						}
					}
				}
			}
		}()
	}

	return leaf
}

func (s *Session) KeepAlive() {
	now := time.Now().UTC()
	s.expires = now.Add(SESSION_MAX_AGE)
}

func (s *Session) Destroy() {
	close(s.ch)
	s.ws.Close()
}

func Destroy(id string) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	if leaf, ok := session[id]; ok {
		log.Debug().Msgf("origin: destroying session %s", id)
		leaf.Destroy()
		delete(session, id)
	}
}

func (s *Session) GetLocation() string {
	return s.location
}

func (s *Session) Bootstrap() {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_BOOTSTRAP,
		Payload: nil,
	}

	s.ch <- message
}
