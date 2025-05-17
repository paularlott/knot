package model

import (
	"time"

	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/rs/zerolog/log"
)

// Session object
type Token struct {
	Id           string    `json:"token_id" db:"token_id,pk"`
	UserId       string    `json:"user_id" db:"user_id"`
	SessionId    string    `json:"session_id" db:"session_id"`
	Name         string    `json:"name" db:"name"`
	ExpiresAfter time.Time `json:"expires_after" db:"expires_after"`
}

func NewToken(name string, userId string) *Token {
	id, err := crypt.GenerateAPIKey()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	token := &Token{
		Id:        id,
		UserId:    userId,
		SessionId: "",
		Name:      name,
	}

	return token
}
