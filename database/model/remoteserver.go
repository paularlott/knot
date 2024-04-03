package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	REMOTE_SERVER_PING_INTERVAL = 10 * time.Second
	REMOTE_SERVER_GC_INTERVAL   = 20 * time.Second
	REMOTE_SERVER_TIMEOUT       = 30 * time.Second
)

// Struct holding the state of a remote server
type RemoteServer struct {
	Id           string    `json:"server_id"`
	Url          string    `json:"url"`
	ExpiresAfter time.Time `json:"expires_after"`
}

func NewRemoteServer(url string) *RemoteServer {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	server := &RemoteServer{
		Id:           id.String(),
		Url:          url,
		ExpiresAfter: time.Now().UTC().Add(AGENT_STATE_TIMEOUT),
	}

	return server
}
