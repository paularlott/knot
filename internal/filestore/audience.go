package filestore

import (
	"sync"
	"time"
)

// Change notices go only to those who can see the bucket. A bucket that was
// just unshared, transferred or deleted must still reach those who could see
// it before, so their views drop it: its record from before the change is
// kept for a while to answer for them.

// audienceKeep is how long a bucket's earlier record answers MaySee.
const audienceKeep = time.Minute

type audience struct {
	mu   sync.Mutex
	prev map[string][]prevBucket // by bucket id, oldest first
}

type prevBucket struct {
	b  *Bucket
	at time.Time
}

// maxPrev bounds the earlier records kept of one bucket.
const maxPrev = 16

// remember keeps a bucket's record as it was before a change.
func (a *audience) remember(b *Bucket) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prev == nil {
		a.prev = make(map[string][]prevBucket)
	}
	now := time.Now()
	for id, list := range a.prev {
		if id != b.Id && now.Sub(list[len(list)-1].at) >= audienceKeep {
			delete(a.prev, id)
		}
	}
	list := append(fresh(a.prev[b.Id], now), prevBucket{b: b, at: now})
	if len(list) > maxPrev {
		list = list[len(list)-maxPrev:]
	}
	a.prev[b.Id] = list
}

// fresh drops the records too old to answer for.
func fresh(list []prevBucket, now time.Time) []prevBucket {
	for len(list) > 0 && now.Sub(list[0].at) >= audienceKeep {
		list = list[1:]
	}
	return list
}

// sawBefore reports whether p could see the bucket in any of its records of
// the last while.
func (a *audience) sawBefore(p *Principal, bucketId string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pb := range fresh(a.prev[bucketId], time.Now()) {
		if live(pb.b) && pb.b.AccessFor(p) != AccessNone {
			return true
		}
	}
	return false
}

// MaySee reports whether p can see a bucket, or could in the last while
// before it changed: whether to tell them it changed.
func (s *Store) MaySee(p *Principal, bucketId string) bool {
	s.mu.RLock()
	b := s.buckets[bucketId]
	s.mu.RUnlock()
	if live(b) && b.AccessFor(p) != AccessNone {
		return true
	}
	return s.audience.sawBefore(p, bucketId)
}
