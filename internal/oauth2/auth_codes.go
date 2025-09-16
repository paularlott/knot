package oauth2

import (
	"sync"
	"time"

	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/rs/zerolog/log"
)

const (
	AuthCodeExpiry = 10 * time.Minute // OAuth2 spec recommends max 10 minutes
)

type AuthCode struct {
	Code        string
	UserId      string
	ClientId    string
	RedirectURI string
	Scope       string
	ExpiresAt   time.Time
}

type AuthCodeStore struct {
	codes map[string]*AuthCode
	mutex sync.RWMutex
}

var authCodeStore = &AuthCodeStore{
	codes: make(map[string]*AuthCode),
}

func GetAuthCodeStore() *AuthCodeStore {
	return authCodeStore
}

func (s *AuthCodeStore) CreateAuthCode(userId, clientId, redirectURI, scope string) (*AuthCode, error) {
	code, err := crypt.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	authCode := &AuthCode{
		Code:        code,
		UserId:      userId,
		ClientId:    clientId,
		RedirectURI: redirectURI,
		Scope:       scope,
		ExpiresAt:   time.Now().Add(AuthCodeExpiry),
	}

	s.mutex.Lock()
	s.codes[code] = authCode
	s.mutex.Unlock()

	// Clean up expired codes
	go s.cleanupExpired()

	return authCode, nil
}

func (s *AuthCodeStore) ConsumeAuthCode(code string) (*AuthCode, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	authCode, exists := s.codes[code]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(authCode.ExpiresAt) {
		delete(s.codes, code)
		return nil, false
	}

	// Remove the code (single use only)
	delete(s.codes, code)
	return authCode, true
}

func (s *AuthCodeStore) cleanupExpired() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	for code, authCode := range s.codes {
		if now.After(authCode.ExpiresAt) {
			delete(s.codes, code)
			log.Debug().Msgf("oauth2: cleaned up expired auth code")
		}
	}
}
