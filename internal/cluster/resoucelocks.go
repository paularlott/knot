package cluster

import (
	"time"

	"github.com/paularlott/gossip/hlc"
)

const (
	// ResourceLockTTL is how long a resource lock is held before it expires.
	// In cluster mode it is the TTL given to the distributed lock (generous,
	// so container operations that pull images comfortably fit inside it); in
	// single-process mode it bounds how long an abandoned local lock can wedge
	// its resource.
	ResourceLockTTL        = 5 * time.Minute
	ResourceLockGCInterval = 30 * time.Second
)

// ResourceLock is the local, single-process lock record used when the cluster
// is not running (leaf nodes, single-server zones). Cluster-mode locking is
// handled by the distributed lock pool in the gossip lock package; see
// Cluster.LockResource.
type ResourceLock struct {
	Id           string
	UnlockToken  string
	IsDeleted    bool
	ExpiresAfter time.Time
	UpdatedAt    hlc.Timestamp
}
