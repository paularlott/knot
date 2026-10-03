package model

import (
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/knot/internal/log"
)

const (
	MaxTokenAge = 14 * 24 * time.Hour // 2 weeks

	// ScopeMethods allows a token to reach /api/methods* endpoints.
	// Scopes are narrowing: an empty/nil Scopes slice means unrestricted
	// (the token inherits the user's full authenticated surface, matching
	// pre-scopes behaviour). A non-empty slice restricts the token to only
	// the endpoint groups named by the listed scopes.
	ScopeMethods = "methods"
	ScopeMCP     = "mcp"
	// ScopeTunnels allows a token to reach the tunnel endpoints only:
	// the web/port tunnel websockets (/tunnel/*) and the user's tunnel
	// management API (/api/tunnels*). A tunnels-only key can create,
	// list and delete tunnels and nothing else, apart from reading its
	// own identity at /api/users/whoami (credential fields withheld),
	// which every scoped token may do.
	ScopeTunnels = "tunnels"
	// ScopeFiles allows a token to reach file storage: the files API
	// (/api/files*) and, on Knot Pro, the S3 endpoint with the token as the
	// secret key.
	ScopeFiles = "files"
)

// KnownTokenScopes is the authoritative list of valid scope strings.
var KnownTokenScopes = []string{ScopeMethods, ScopeMCP, ScopeTunnels, ScopeFiles}

// IsKnownTokenScope reports whether s is a valid scope string.
func IsKnownTokenScope(s string) bool {
	for _, k := range KnownTokenScopes {
		if k == s {
			return true
		}
	}
	return false
}

// Session object
type Token struct {
	Id           string        `json:"token_id" db:"token_id,pk"`
	UserId       string        `json:"user_id" db:"user_id"`
	Name         string        `json:"name" db:"name"`
	ExpiresAfter time.Time     `json:"expires_after" db:"expires_after"`
	UpdatedAt    hlc.Timestamp `json:"updated_at" db:"updated_at"`
	IsDeleted    bool          `json:"is_deleted" db:"is_deleted"`
	// Scopes restricts which endpoint groups this token can reach.
	// nil/empty = unrestricted (backward compatible with pre-scopes tokens).
	// Non-empty = token may only reach endpoints covered by the listed scopes.
	Scopes []string `json:"scopes,omitempty" db:"scopes,json"`
	// RefreshToken marks tokens issued via the OAuth2 authorization-code
	// flow; only these may be extended through the /token refresh grant.
	RefreshToken bool `json:"refresh_token,omitempty" db:"refresh_token"`
}

// TokenPrefix starts every API token, so a token is recognisable and never
// begins with "-", which a command line would read as another flag.
const TokenPrefix = "tk_"

func NewToken(name string, userId string) *Token {
	key, err := crypt.GenerateAPIKey()
	if err != nil {
		log.Fatal(err.Error())
	}
	id := TokenPrefix + key

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
