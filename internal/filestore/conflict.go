package filestore

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// A transaction that read a key another committed meanwhile fails with
// badger.ErrConflict and is run again. Writes of the same content all change
// its reference count, so many at once (empty files, folder markers, the
// copies of one licence in a vendor tree) collide on one key: each round only
// one wins, and the rest, retried at once, collide again while the winner's
// commit waits for its disk sync. A short random wait, growing with each
// attempt, lets them take turns.
const (
	// conflictPatience is how long a transaction keeps taking its turn: far
	// longer than writers of one key take to clear, even dozens at once.
	conflictPatience = 10 * time.Second
	conflictWaitMax  = 20 * time.Millisecond
)

// conflictRetry tracks one transaction's attempts.
type conflictRetry struct {
	attempt int
	until   time.Time
}

// again reports whether a transaction that failed with err should be run
// again, after waiting its turn.
func (r *conflictRetry) again(err error) bool {
	if !errors.Is(err, badger.ErrConflict) {
		return false
	}
	now := time.Now()
	if r.until.IsZero() {
		r.until = now.Add(conflictPatience)
	} else if now.After(r.until) {
		return false
	}
	wait := min(time.Duration(r.attempt+1)*200*time.Microsecond, conflictWaitMax)
	r.attempt++
	time.Sleep(wait/2 + rand.N(wait/2+1))
	return true
}

// update runs fn in a read-write transaction, again while it conflicts with
// another. fn may run several times, so it must reset what it records.
func (s *Store) update(fn func(txn *badger.Txn) error) error {
	var retry conflictRetry
	for {
		err := s.db.Update(fn)
		if !retry.again(err) {
			return err
		}
	}
}
