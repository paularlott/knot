package model

import (
	"time"

	"github.com/google/uuid"
)

// Session object
type Token struct {
	Id string `json:"token_id"`
  UserId string `json:"user_id"`
  Name string `json:"name"`
  ExpiresAfter time.Time `json:"expires_after"`
}

func NewToken(name string, userId string) *Token {
  token := &Token{
    Id: uuid.New().String(),
    UserId: userId,
    Name: name,
  }

  return token
}
