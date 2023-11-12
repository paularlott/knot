package model

import (
	"time"

	"github.com/google/uuid"
)

const WEBUI_SESSION_COOKIE = "__WEBUI_SESSION"

// Session object
type Session struct {
	Id string `json:"session_id"`
  Ip string `json:"ip"`
  UserId string `json:"user_id"`
	Values  map[string]interface{} `json:"data"`
  ExpiresAfter time.Time `json:"expires_after"`
}

func NewSession(ip string, userId string) *Session {
  session := &Session{
    Id: uuid.New().String(),
    Ip: ip,
    UserId: userId,
    Values: make(map[string]interface{}),
  }

  return session
}
