// Package startlog keeps a short, in-memory log of each space's most recent
// start: what the server did before the space's agent connected (preparing
// volumes, pulling the image, creating and starting the container) and why a
// start failed. The space's log window shows it while the space is starting,
// until the agent's own log stream takes over.
//
// It is node-local and never persisted or gossiped: no database or wire
// format is involved.
package startlog

import (
	"fmt"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

const (
	// Service is the service name shown against start-up log lines.
	Service = "knot"

	maxEntries = 200
	// A start-up log nobody has written to for this long is dropped.
	maxAge = 2 * time.Hour
)

type spaceLog struct {
	entries   []*msg.LogMessage
	listeners map[int]chan *msg.LogMessage
	updated   time.Time
}

var (
	mu     sync.Mutex
	logs   = map[string]*spaceLog{}
	nextId int
)

func get(spaceId string) *spaceLog {
	l := logs[spaceId]
	if l == nil {
		l = &spaceLog{listeners: map[int]chan *msg.LogMessage{}}
		logs[spaceId] = l
	}
	return l
}

// sweep drops old logs that nobody is watching. Called with mu held.
func sweep(now time.Time) {
	for id, l := range logs {
		if len(l.listeners) == 0 && now.Sub(l.updated) > maxAge {
			delete(logs, id)
		}
	}
}

// Begin starts a new start-up log for the space, discarding the previous one.
func Begin(spaceId, format string, args ...any) {
	mu.Lock()
	now := time.Now()
	sweep(now)
	l := get(spaceId)
	l.entries = l.entries[:0]
	mu.Unlock()
	add(spaceId, msg.LogLevelInfo, fmt.Sprintf(format, args...))
}

// Info records a step of the space's start.
func Info(spaceId, format string, args ...any) {
	add(spaceId, msg.LogLevelInfo, fmt.Sprintf(format, args...))
}

// Error records why the space's start failed.
func Error(spaceId, format string, args ...any) {
	add(spaceId, msg.LogLevelError, fmt.Sprintf(format, args...))
}

func add(spaceId string, level msg.LogLevel, text string) {
	if spaceId == "" {
		return
	}
	entry := &msg.LogMessage{Level: level, Service: Service, Message: text, Date: time.Now()}

	mu.Lock()
	defer mu.Unlock()
	l := get(spaceId)
	l.updated = entry.Date
	l.entries = append(l.entries, entry)
	if len(l.entries) > maxEntries {
		l.entries = l.entries[len(l.entries)-maxEntries:]
	}
	for _, c := range l.listeners {
		select {
		case c <- entry:
		default: // a slow reader misses lines rather than blocking a start
		}
	}
}

// Subscribe returns the space's start-up log so far and a channel carrying
// the lines added after it. Call cancel when done; it closes the channel.
func Subscribe(spaceId string) (history []*msg.LogMessage, lines <-chan *msg.LogMessage, cancel func()) {
	mu.Lock()
	defer mu.Unlock()
	l := get(spaceId)
	l.updated = time.Now()
	history = append([]*msg.LogMessage(nil), l.entries...)
	id := nextId
	nextId++
	c := make(chan *msg.LogMessage, 100)
	l.listeners[id] = c

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			if cur := logs[spaceId]; cur != nil {
				delete(cur.listeners, id)
			}
			close(c)
		})
	}
	return history, c, cancel
}
