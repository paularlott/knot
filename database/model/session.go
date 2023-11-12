package model

import (
	"time"

	"github.com/google/uuid"
)

const WEBUI_SESSION_COOKIE = "__WEBUI_SESSION"

// Session object
type Session struct {
	Id string `json:"session_id"`
	Values  map[string]interface{} `json:"data"`
  ExpiresAfter time.Time `json:"expires_after"`
}

func NewSession() *Session {
  session := &Session{
    Id: uuid.New().String(),
    Values: make(map[string]interface{}),
  }

  return session
}
