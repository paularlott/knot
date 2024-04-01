package model

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const WEBUI_SESSION_COOKIE = "__KNOT_WEBUI_SESSION"

// Session object
type Session struct {
	Id              string    `json:"session_id"`
	Ip              string    `json:"ip"`
	UserId          string    `json:"user_id"`
	RemoteSessionId string    `json:"remote_session_id"`
	UserAgent       string    `json:"user_agent"`
	ExpiresAfter    time.Time `json:"expires_after"`
}

func NewSession(r *http.Request, userId string, remoteSessionId string) *Session {

	// Get the users IP
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	session := &Session{
		Id:              id.String(),
		Ip:              ip,
		UserId:          userId,
		RemoteSessionId: remoteSessionId,
		UserAgent:       r.UserAgent(),
	}

	return session
}
