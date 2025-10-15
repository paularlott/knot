package model

import (
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/knot/internal/log"
)

const (
	MaxTokenAge = 14 * 24 * time.Hour // 2 weeks
)

// Session object
type Token struct {
	Id           string        `json:"token_id" db:"token_id,pk"`
	UserId       string        `json:"user_id" db:"user_id"`
	Name         string        `json:"name" db:"name"`
	ExpiresAfter time.Time     `json:"expires_after" db:"expires_after"`
	UpdatedAt    hlc.Timestamp `json:"updated_at" db:"updated_at"`
	IsDeleted    bool          `json:"is_deleted" db:"is_deleted"`
}

func NewToken(name string, userId string) *Token {
	id, err := crypt.GenerateAPIKey()
	if err != nil {
		log.Fatal(err.Error())
	}

	now := time.Now().UTC()
	expiresAfter := now.Add(MaxTokenAge)

	token := &Token{
		Id:           id,
		UserId:       userId,
		Name:         name,
		UpdatedAt:    hlc.Now(),
		ExpiresAfter: expiresAfter.UTC(),
		IsDeleted:    false,
	}

	return token
}
