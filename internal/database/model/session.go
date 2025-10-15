package model

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/knot/internal/log"
)

const (
	WebSessionCookie      = "__KNOT_WEBUI_SESSION"
	SessionExpiryDuration = 2 * time.Hour
)

// Session object
type Session struct {
	Id           string        `json:"session_id" db:"session_id,pk"`
	Ip           string        `json:"ip" db:"ip"`
	UserId       string        `json:"user_id" db:"user_id"`
	UserAgent    string        `json:"user_agent" db:"user_agent"`
	ExpiresAfter time.Time     `json:"expires_after" db:"expires_after"`
	UpdatedAt    hlc.Timestamp `json:"updated_at" db:"updated_at"`
	IsDeleted    bool          `json:"is_deleted" db:"is_deleted"`
}

func NewSession(r *http.Request, userId string) *Session {

	// Get the users IP
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

	// Strip off the port (ipv4 or ipv6)
	ipOnly, _, err := net.SplitHostPort(ip)
	if err != nil && err.(*net.AddrError).Err != "missing port in address" {
		log.Fatal("error parsing ip:", "err", err)
	}
	if ipOnly != "" {
		ip = ipOnly
	}

	id, err := crypt.GenerateAPIKey()
	if err != nil {
		log.Fatal(err.Error())
	}

	now := time.Now()
	expires := now.Add(SessionExpiryDuration)

	session := &Session{
		Id:           id,
		Ip:           ip,
		UserId:       userId,
		UserAgent:    r.UserAgent(),
		ExpiresAfter: expires.UTC(),
		UpdatedAt:    hlc.Now(),
		IsDeleted:    false,
	}

	return session
}
