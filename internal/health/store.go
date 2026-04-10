package health

import (
	"sync"
	"time"
)

// Status holds the in-memory health status for a space.
type Status struct {
	Healthy             bool      `json:"healthy"`
	Reason              string    `json:"reason"`
	LastCheckedAt       time.Time `json:"last_checked_at"`
	ConsecutiveFailures uint32    `json:"consecutive_failures"`
}

var (
	mu    sync.RWMutex
	store = make(map[string]*Status)
)

// Get returns the current health status for a space, or nil if unknown.
func Get(spaceId string) *Status {
	mu.RLock()
	defer mu.RUnlock()
	return store[spaceId]
}

// Set updates the health status for a space.
func Set(spaceId string, healthy bool, reason string, consecutiveFailures uint32) {
	mu.Lock()
	store[spaceId] = &Status{
		Healthy:             healthy,
		Reason:              reason,
		LastCheckedAt:       time.Now().UTC(),
		ConsecutiveFailures: consecutiveFailures,
	}
	mu.Unlock()
}

// Delete removes the health status for a space.
func Delete(spaceId string) {
	mu.Lock()
	delete(store, spaceId)
	mu.Unlock()
}
