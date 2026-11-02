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
	Id string `json:"session_id" db:"session_id,pk" msgpack:"session_id"`
	Ip string `json:"ip" db:"ip" msgpack:"ip"`
	// UserId is the account the session currently runs as. Switching user
	// rewrites it; OriginalUserId keeps the account that actually
	// authenticated so the session can always flick back — fast user
	// switching, with no reverse link granted.
	UserId         string        `json:"user_id" db:"user_id" msgpack:"user_id"`
	OriginalUserId string        `json:"original_user_id" db:"original_user_id" msgpack:"original_user_id"`
	UserAgent      string        `json:"user_agent" db:"user_agent" msgpack:"user_agent"`
	ExpiresAfter   time.Time     `json:"expires_after" db:"expires_after" msgpack:"expires_after"`
	UpdatedAt      hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
	IsDeleted      bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	// AuthProvider is the sign-in provider that authenticated the session,
	// and ProviderToken the grant it gave, encrypted, so knot can check
	// the sign-in is still good (Pro). ProviderTokenAt orders copies of the
	// grant apart from the rest of the session: the provider may replace
	// it on each check, and a request saving an older copy of the session
	// mustn't bring the replaced grant back.
	AuthProvider    string        `json:"auth_provider,omitempty" db:"auth_provider" msgpack:"auth_provider,omitempty"`
	ProviderToken   string        `json:"provider_token,omitempty" db:"provider_token" msgpack:"provider_token,omitempty"`
	ProviderTokenAt hlc.Timestamp `json:"provider_token_at,omitempty" db:"provider_token_at" msgpack:"provider_token_at,omitempty"`
}

// KeepNewerProviderToken carries stored's provider grant into s when
// stored's is newer, for saving s over stored.
func (s *Session) KeepNewerProviderToken(stored *Session) {
	if stored != nil && stored.ProviderTokenAt.After(s.ProviderTokenAt) {
		s.AuthProvider = stored.AuthProvider
		s.ProviderToken = stored.ProviderToken
		s.ProviderTokenAt = stored.ProviderTokenAt
	}
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
