// Package devicegrant holds the pending sign-ins of the OAuth 2.0 device
// authorization grant (RFC 8628), which `knot connect` uses: the client gets
// a device code it keeps and a short user code it shows, the user approves
// the user code in the web interface, and the client polls the token
// endpoint with the device code until it gets its API token.
//
// State is in-memory per server; Grant/Apply exist so servers gossip every
// change over the cluster, so the approval and the polls may land on
// different servers behind a load balancer.
package devicegrant

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/util/crypt"
)

const (
	// TTL is how long a device sign-in waits for its user.
	TTL = 10 * time.Minute
	// Interval is the minimum number of seconds between polls.
	Interval = 5

	userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ" // no vowels, so no words and nothing to misread
	userCodeLen      = 8

	maxPending  = 10000 // sign-ins waiting at once, as starting one needs no account
	maxGuesses  = 10    // wrong user codes per user within guessWindow
	guessWindow = 15 * time.Minute
)

type Status int

// The statuses only move forward, so gossip applied in any order converges.
const (
	StatusPending Status = iota
	StatusApproved
	StatusDenied
	StatusUsed
)

// Grant is a device sign-in, gossiped between servers on every change.
type Grant struct {
	DeviceCode string    `msgpack:"device_code"`
	UserCode   string    `msgpack:"user_code"` // normalised, no separator
	TokenName  string    `msgpack:"token_name"`
	ExpiresAt  time.Time `msgpack:"expires_at"`
	Status     Status    `msgpack:"status"`
	UserId     string    `msgpack:"user_id,omitempty"`
	TokenId    string    `msgpack:"token_id,omitempty"` // set on approval
}

func (g *Grant) Expired() bool {
	return time.Now().After(g.ExpiresAt)
}

type store struct {
	mu       sync.Mutex
	byDevice map[string]*Grant
	byUser   map[string]string    // user code -> device code
	lastPoll map[string]time.Time // device code -> last poll, local to this server
	interval map[string]int       // device code -> poll interval in seconds
	guesses  map[string][]time.Time
}

var s = &store{
	byDevice: make(map[string]*Grant),
	byUser:   make(map[string]string),
	lastPoll: make(map[string]time.Time),
	interval: make(map[string]int),
	guesses:  make(map[string][]time.Time),
}

// New starts a device sign-in for a token with the given name.
func New(tokenName string) (*Grant, error) {
	deviceCode, err := crypt.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	if len(s.byDevice) >= maxPending {
		return nil, ErrTooMany
	}

	// A user code is never handed out while another sign-in holds it.
	var userCode string
	for range 5 {
		uc, err := newUserCode()
		if err != nil {
			return nil, err
		}
		if _, taken := s.byUser[uc]; !taken {
			userCode = uc
			break
		}
	}
	if userCode == "" {
		return nil, errNoUserCode
	}

	g := &Grant{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		TokenName:  tokenName,
		ExpiresAt:  time.Now().Add(TTL).UTC(),
	}
	s.putLocked(g)
	c := *g
	return &c, nil
}

// Apply records a grant change from this or another server. A change is
// ignored unless it moves the grant's status forward.
func Apply(g *Grant) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cur, ok := s.byDevice[g.DeviceCode]; ok && cur.Status >= g.Status {
		return
	}
	s.cleanupLocked()
	c := *g
	s.putLocked(&c)
}

func (s *store) putLocked(g *Grant) {
	s.byDevice[g.DeviceCode] = g
	if g.Status == StatusPending {
		s.byUser[g.UserCode] = g.DeviceCode
	} else {
		// Once decided, the user code stops working.
		delete(s.byUser, g.UserCode)
	}
}

// ByUserCode finds the pending, unexpired sign-in a user typed the code of.
// Wrong codes count against userId; once it has made too many in the last
// 15 minutes, every code is refused and limited is true.
func ByUserCode(userId, typed string) (g *Grant, limited bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	recent := s.guesses[userId][:0]
	for _, t := range s.guesses[userId] {
		if now.Sub(t) < guessWindow {
			recent = append(recent, t)
		}
	}
	s.guesses[userId] = recent
	if len(recent) >= maxGuesses {
		return nil, true
	}

	if code := NormUserCode(typed); code != "" {
		if dc, ok := s.byUser[code]; ok {
			if cur := s.byDevice[dc]; cur != nil && cur.Status == StatusPending && !cur.Expired() {
				c := *cur
				return &c, false
			}
		}
	}

	s.guesses[userId] = append(recent, now)
	return nil, false
}

// ByDeviceCode returns the sign-in of a device code, for a poll of the token
// endpoint. slowDown is true when the poll came sooner than the device's
// interval, which then grows by 5 seconds (RFC 8628 section 3.5).
func ByDeviceCode(deviceCode string) (g *Grant, slowDown bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.byDevice[deviceCode]
	if !ok || cur.Expired() {
		return nil, false
	}

	now := time.Now()
	interval := s.interval[deviceCode]
	if interval == 0 {
		interval = Interval
	}
	if last, ok := s.lastPoll[deviceCode]; ok && now.Sub(last) < time.Duration(interval)*time.Second {
		slowDown = true
		s.interval[deviceCode] = interval + 5
	}
	s.lastPoll[deviceCode] = now

	c := *cur
	return &c, slowDown
}

func (s *store) cleanupLocked() {
	for dc, g := range s.byDevice {
		if g.Expired() {
			delete(s.byDevice, dc)
			delete(s.byUser, g.UserCode)
			delete(s.lastPoll, dc)
			delete(s.interval, dc)
		}
	}
}

// newUserCode returns a random user code, normalised (no separator).
func newUserCode() (string, error) {
	b := make([]byte, userCodeLen)
	n := big.NewInt(int64(len(userCodeAlphabet)))
	for i := range b {
		v, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = userCodeAlphabet[v.Int64()]
	}
	return string(b), nil
}

// FormatUserCode shows a normalised code as two groups: WDJB-MJHT.
func FormatUserCode(c string) string {
	if len(c) != userCodeLen {
		return c
	}
	return c[:4] + "-" + c[4:]
}

// NormUserCode turns what someone typed into a normalised code: case, spaces
// and dashes do not matter. It returns "" for anything that cannot be a code.
func NormUserCode(typed string) string {
	if len(typed) > 32 {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		switch {
		case r == '-' || r == ' ' || r == '\t':
		case strings.ContainsRune(userCodeAlphabet, r):
			b.WriteRune(r)
		default:
			return ""
		}
	}
	if b.Len() != userCodeLen {
		return ""
	}
	return b.String()
}

var (
	errNoUserCode = errors.New("no free user code")
	// ErrTooMany is returned by New when too many sign-ins are waiting.
	ErrTooMany = errors.New("too many device sign-ins waiting")
)
