package nfs

import "sync"

// knot: making a name is checked and done under a lock on that name, so of
// several clients making the same folder at once one makes it and the
// others are told it exists. Upstream's NFSv4 CREATE answered every one of
// them "made".
var createLocks = struct {
	sync.Mutex
	names map[string]*createLock
}{names: map[string]*createLock{}}

type createLock struct {
	sync.Mutex
	users int
}

// lockName locks the name p, and returns its unlock.
func lockName(p string) func() {
	createLocks.Lock()
	l := createLocks.names[p]
	if l == nil {
		l = &createLock{}
		createLocks.names[p] = l
	}
	l.users++
	createLocks.Unlock()
	l.Lock()
	return func() {
		l.Unlock()
		createLocks.Lock()
		if l.users--; l.users == 0 {
			delete(createLocks.names, p)
		}
		createLocks.Unlock()
	}
}
