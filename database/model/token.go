package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Session object
type Token struct {
	Id            string    `json:"token_id"`
	UserId        string    `json:"user_id"`
	RemoteTokenId string    `json:"remote_token_id"`
	Name          string    `json:"name"`
	ExpiresAfter  time.Time `json:"expires_after"`
}

func NewToken(name string, userId string) *Token {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	token := &Token{
		Id:            id.String(),
		UserId:        userId,
		RemoteTokenId: "",
		Name:          name,
	}

	return token
}
