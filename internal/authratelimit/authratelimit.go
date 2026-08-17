// Package authratelimit implements failed-login blocking. Failed attempts
// are counted per IP and per account email; past the configured threshold
// within the window, auth is refused for the block duration. Successful
// logins reset both keys.
//
// State is in-memory per server; Event/ApplyEvent exist so servers gossip
// failures, blocks and clears over the cluster, making tracking and blocking
// cluster-wide: an attacker spreading failures across servers behind a load
// balancer still trips one shared budget.
package authratelimit

import (
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
)

// Event is a rate-limit state change gossiped between servers. Exactly one
// of the behaviours applies: Clear removes the keys' state; BlockUntil > At
// applies a block; otherwise the event records a failure at At.
type Event struct {
	IP         string    `msgpack:"ip"`
	Email      string    `msgpack:"email"`
	At         time.Time `msgpack:"at"`
	BlockUntil time.Time `msgpack:"block_until"`
	Clear      bool      `msgpack:"clear"`
	// ClearAll wipes every limiter entry (all IPs and emails); the IP and
	// Email fields are ignored when set.
	ClearAll bool `msgpack:"clear_all"`
}

// loginAttempts tracks failed logins for one key (IP or email) and the
// block that trips once enough failures accumulate within the window.
type loginAttempts struct {
	failures     []time.Time
	blockedUntil time.Time
	lastUsed     time.Time
}

var (
	// Rate limit by IP address
	ipLimiters = make(map[string]*loginAttempts)
	ipMutex    sync.Mutex

	// Rate limit by email address
	emailLimiters = make(map[string]*loginAttempts)
	emailMutex    sync.Mutex
)

const (
	cleanupInterval = 10 * time.Minute
	cleanupMaxAge   = 30 * time.Minute

	defaultAuthRateLimitAttempts = 10
	defaultAuthRateLimitWindow   = 60  // seconds
	defaultAuthRateLimitBlock    = 300 // seconds
)

// rateLimitConfig returns the effective limiter settings, falling back to
// the defaults when unset (e.g. in tests without a server config).
func rateLimitConfig() (attempts int, window, block time.Duration) {
	cfg := config.GetServerConfig()
	attempts, windowSec, blockSec := defaultAuthRateLimitAttempts, defaultAuthRateLimitWindow, defaultAuthRateLimitBlock
	if cfg != nil {
		if cfg.AuthRateLimitAttempts > 0 {
			attempts = cfg.AuthRateLimitAttempts
		}
		if cfg.AuthRateLimitWindow > 0 {
			windowSec = cfg.AuthRateLimitWindow
		}
		if cfg.AuthRateLimitBlock > 0 {
			blockSec = cfg.AuthRateLimitBlock
		}
	}
	return attempts, time.Duration(windowSec) * time.Second, time.Duration(blockSec) * time.Second
}

// Blocked reports whether either key is currently blocked, refreshing
// last-use timestamps.
func Blocked(ip, email string) bool {
	return blocked(ipLimiters, &ipMutex, ip) || blocked(emailLimiters, &emailMutex, email)
}

func blocked(limiters map[string]*loginAttempts, mutex *sync.Mutex, key string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	entry, exists := limiters[key]
	if !exists {
		return false
	}
	entry.lastUsed = time.Now()
	return time.Now().Before(entry.blockedUntil)
}

// RecordFailure records a failed login for both keys. Once the configured
// number of failures occur within the window, the keys are blocked for the
// configured duration; the tripped block deadline is returned so callers
// can gossip it cluster-wide.
func RecordFailure(ip, email string) (blockedUntil time.Time, tripped bool) {
	attempts, window, block := rateLimitConfig()
	now := time.Now()

	// Each key is recorded against its own budget. The gossiped deadline is
	// whichever trip happened at all (either key may trip alone), preferring
	// the later deadline when both did.
	until, tripped := record(ipLimiters, &ipMutex, ip, attempts, window, block, now)
	until2, tripped2 := record(emailLimiters, &emailMutex, email, attempts, window, block, now)
	if tripped2 && until2.After(until) {
		until, tripped = until2, true
	}
	return until, tripped
}

func record(limiters map[string]*loginAttempts, mutex *sync.Mutex, key string, attempts int, window, block time.Duration, now time.Time) (time.Time, bool) {
	mutex.Lock()
	defer mutex.Unlock()

	entry, exists := limiters[key]
	if !exists {
		entry = &loginAttempts{}
		limiters[key] = entry
	}
	entry.lastUsed = now

	// Keep only failures inside the window.
	failures := entry.failures[:0]
	for _, t := range entry.failures {
		if now.Sub(t) < window {
			failures = append(failures, t)
		}
	}
	failures = append(failures, now)
	entry.failures = failures

	if len(failures) >= attempts && now.After(entry.blockedUntil) {
		entry.blockedUntil = now.Add(block)
		return entry.blockedUntil, true
	}
	return time.Time{}, false
}

// Clear resets both keys' state — called on successful login so a user who
// eventually authenticates isn't left with a near-tripped counter.
func Clear(ip, email string) {
	ipMutex.Lock()
	delete(ipLimiters, ip)
	ipMutex.Unlock()

	emailMutex.Lock()
	delete(emailLimiters, email)
	emailMutex.Unlock()
}

// ClearAll removes every rate limiter entry (all IPs and emails). It is the
// in-memory "flush all blocks" used by the admin API; blocks expire on
// their own otherwise.
func ClearAll() {
	ipMutex.Lock()
	ipLimiters = make(map[string]*loginAttempts)
	ipMutex.Unlock()

	emailMutex.Lock()
	emailLimiters = make(map[string]*loginAttempts)
	emailMutex.Unlock()
}

// ApplyEvent applies a gossiped state change from another server. Failures
// count towards the shared budget everywhere; blocks are applied as the
// maximum of local and remote deadlines; clears remove state.
func ApplyEvent(evt *Event) {
	if evt == nil {
		return
	}

	if evt.ClearAll {
		ClearAll()
		return
	}

	if evt.Clear {
		Clear(evt.IP, evt.Email)
		return
	}

	if evt.BlockUntil.After(evt.At) {
		applyBlock(ipLimiters, &ipMutex, evt.IP, evt.BlockUntil)
		applyBlock(emailLimiters, &emailMutex, evt.Email, evt.BlockUntil)
		return
	}

	// A plain failure: count it with the remote timestamp.
	attempts, window, block := rateLimitConfig()
	record(ipLimiters, &ipMutex, evt.IP, attempts, window, block, evt.At)
	record(emailLimiters, &emailMutex, evt.Email, attempts, window, block, evt.At)
}

func applyBlock(limiters map[string]*loginAttempts, mutex *sync.Mutex, key string, until time.Time) {
	mutex.Lock()
	defer mutex.Unlock()

	entry, exists := limiters[key]
	if !exists {
		entry = &loginAttempts{}
		limiters[key] = entry
	}
	entry.lastUsed = time.Now()
	if until.After(entry.blockedUntil) {
		entry.blockedUntil = until
	}
}

// StartCleanup periodically drops idle limiter state.
func StartCleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanupTime := time.Now().Add(-cleanupMaxAge)

			ipMutex.Lock()
			for ip, entry := range ipLimiters {
				if entry.lastUsed.Before(cleanupTime) {
					delete(ipLimiters, ip)
				}
			}
			ipMutex.Unlock()

			emailMutex.Lock()
			for email, entry := range emailLimiters {
				if entry.lastUsed.Before(cleanupTime) {
					delete(emailLimiters, email)
				}
			}
			emailMutex.Unlock()

		case <-stop:
			return
		}
	}
}
