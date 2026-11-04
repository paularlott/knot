package nfsserver

import (
	"strings"
	"sync"
	"time"
)

// moves remembers, for a while, where files and folders renamed here went,
// and which were removed here, so a handle made before still finds its
// file, or is known to be of one removed. Handles carry
// the path; a client that keeps using one after renaming the file is told
// it is stale, as NFS lets a server say, but the Mac's client does not look
// the new name up for a file it renamed itself: it renames a file removed
// while open to ".nfs..." and goes on writing, unlocking and closing it by
// the old handle. Refused, a lock it held was never released, and every
// process waiting for that lock waited forever.
type moves struct {
	mu   sync.Mutex
	from map[string][]move // by the path moved from
	n    int
	// seen is the inode number a handle last found at a path, and when
	// (see sawRecently).
	seen map[string]seenIno
}

type seenIno struct {
	ino uint64
	at  time.Time
}

type move struct {
	to  string // "" for removed
	ino uint64 // the file's inode number, 0 for none
	at  time.Time
}

const (
	// movesKept is how long a move is remembered: a file renamed while
	// open is used by its old handle until it is closed.
	movesKept = 15 * time.Minute
	// movesMax bounds the moves remembered.
	movesMax = 16384
	// movesHops bounds following a file renamed again and again.
	movesHops = 16
)

func cleanPath(p string) string { return strings.Trim(p, "/") }

// add records that what was at from, with inode number ino, is now at to,
// or was removed when to is "".
func (m *moves) add(from, to string, ino uint64, now time.Time) {
	from = cleanPath(from)
	if to != "" {
		to = cleanPath(to)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.from == nil {
		m.from = map[string][]move{}
	}
	if m.n >= movesMax {
		m.sweepLocked(now)
	}
	if m.n >= movesMax {
		// All recent: forget the lot rather than grow; handles made
		// before these renames are stale, as they were before.
		m.from, m.n = map[string][]move{}, 0
	}
	m.from[from] = append(m.from[from], move{to: to, ino: ino, at: now})
	m.n++
}

func (m *moves) sweepLocked(now time.Time) {
	for from, list := range m.from {
		kept := list[:0]
		for _, mv := range list {
			if now.Sub(mv.at) < movesKept {
				kept = append(kept, mv)
			}
		}
		m.n -= len(list) - len(kept)
		if len(kept) == 0 {
			delete(m.from, from)
		} else {
			m.from[from] = kept
		}
	}
}

// find returns where the file at path with inode number ino (0 for not
// known) went, following later renames, and of the folders above it: "" if
// it was removed here. found is false when nothing here moved or removed it.
func (m *moves) find(path string, ino uint64, now time.Time) (to string, found bool) {
	path = cleanPath(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	for hop := 0; hop < movesHops; hop++ {
		next, ok := m.stepLocked(path, ino, now)
		if !ok {
			break
		}
		if next == "" {
			return "", true
		}
		path, found = next, true
	}
	return path, found
}

// stepLocked finds one move of path itself, or of a folder above it.
func (m *moves) stepLocked(path string, ino uint64, now time.Time) (string, bool) {
	if mv, ok := m.latestLocked(path, ino, now); ok {
		return mv.to, true
	}
	for i := strings.LastIndexByte(path, '/'); i > 0; i = strings.LastIndexByte(path[:i], '/') {
		// A folder's number is not in its files' handles: its latest
		// move is the one.
		if mv, ok := m.latestLocked(path[:i], 0, now); ok {
			if mv.to == "" {
				return "", true
			}
			return mv.to + path[i:], true
		}
	}
	return "", false
}

// latestLocked is the latest recent move from path, of the file with ino
// when it is known and the moves say.
func (m *moves) latestLocked(path string, ino uint64, now time.Time) (move, bool) {
	list := m.from[path]
	for i := len(list) - 1; i >= 0; i-- {
		mv := list[i]
		if now.Sub(mv.at) >= movesKept {
			break
		}
		if ino == 0 || mv.ino == 0 || mv.ino == ino {
			return mv, true
		}
	}
	return move{}, false
}

// saw records that a handle found the file with inode number ino at path.
func (m *moves) saw(path string, ino uint64, now time.Time) {
	path = cleanPath(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.seen[path]; ok && s.ino == ino && now.Sub(s.at) < time.Minute {
		return // seen lately enough: no need to write
	}
	if m.seen == nil {
		m.seen = map[string]seenIno{}
	}
	if len(m.seen) >= movesMax {
		for p, s := range m.seen {
			if now.Sub(s.at) >= movesKept {
				delete(m.seen, p)
			}
		}
		if len(m.seen) >= movesMax {
			clear(m.seen)
		}
	}
	m.seen[path] = seenIno{ino: ino, at: now}
}

// sawRecently reports whether a handle found the file with inode number
// ino at path lately. Such a file, at its path with another number now and
// neither removed nor renamed here (which moves would say), was replaced by
// another server or client: it is the same file changed, as a file
// overwritten in place keeps its inode, and a client writing it as another
// server's version arrives goes on writing it, last write winning, rather
// than find its handle stale mid-write. A handle not used lately is taken,
// as before, to be of another file.
func (m *moves) sawRecently(path string, ino uint64, now time.Time) bool {
	path = cleanPath(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.seen[path]
	return ok && s.ino == ino && now.Sub(s.at) < movesKept
}
