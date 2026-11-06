package log

import (
	"strings"
	"sync/atomic"

	"github.com/paularlott/logger"
)

// copyTo is where Tee copies records to; nil until it is called.
var copyTo atomic.Pointer[logger.Logger]

// copyMin is the lowest level copied: the configured level.
var copyMin atomic.Int32

// Tee has a copy of every record the logger takes at or above the
// configured level sent to l too: a space's agent copies its records to the
// space's log. Every logger the package gives copies, whenever it was taken,
// with its fields and group. A later Tee replaces the copy; Tee(nil) stops
// it.
func Tee(l logger.Logger) {
	if l == nil {
		copyTo.Store(nil)
		return
	}
	copyTo.Store(&l)
}

// Local is the logger without what Tee added: for the code that delivers
// the copies, which would otherwise copy its own complaints about failing
// to deliver them, without end.
func Local() logger.Logger {
	return localLogger
}

const (
	rankTrace = iota
	rankDebug
	rankInfo
	rankWarn
	rankError
	rankFatal
)

func levelRank(l string) int {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "trace":
		return rankTrace
	case "debug":
		return rankDebug
	case "warn", "warning":
		return rankWarn
	case "error":
		return rankError
	case "fatal":
		return rankFatal
	}
	return rankInfo
}

// teeOp is a With, WithError or WithGroup to make on the copy, as on the
// logger itself.
type teeOp struct {
	kind  byte // 'w' With, 'e' WithError, 'g' WithGroup
	key   string
	value any
	err   error
}

// teeLogger logs to main and, once Tee is called, copies to its target what
// is at or above the configured level.
type teeLogger struct {
	main logger.Logger
	ops  []teeOp
}

// copy is the logger to copy a record of rank to, nil for none.
func (t teeLogger) copy(rank int) logger.Logger {
	p := copyTo.Load()
	if p == nil || rank < int(copyMin.Load()) {
		return nil
	}
	l := *p
	for _, o := range t.ops {
		switch o.kind {
		case 'w':
			l = l.With(o.key, o.value)
		case 'e':
			l = l.WithError(o.err)
		case 'g':
			l = l.WithGroup(o.key)
		}
	}
	return l
}

func (t teeLogger) Trace(msg string, kv ...any) {
	t.main.Trace(msg, kv...)
	if c := t.copy(rankTrace); c != nil {
		c.Trace(msg, kv...)
	}
}

func (t teeLogger) Debug(msg string, kv ...any) {
	t.main.Debug(msg, kv...)
	if c := t.copy(rankDebug); c != nil {
		c.Debug(msg, kv...)
	}
}

func (t teeLogger) Info(msg string, kv ...any) {
	t.main.Info(msg, kv...)
	if c := t.copy(rankInfo); c != nil {
		c.Info(msg, kv...)
	}
}

func (t teeLogger) Warn(msg string, kv ...any) {
	t.main.Warn(msg, kv...)
	if c := t.copy(rankWarn); c != nil {
		c.Warn(msg, kv...)
	}
}

func (t teeLogger) Error(msg string, kv ...any) {
	t.main.Error(msg, kv...)
	if c := t.copy(rankError); c != nil {
		c.Error(msg, kv...)
	}
}

// Fatal copies first: main exits.
func (t teeLogger) Fatal(msg string, kv ...any) {
	if c := t.copy(rankFatal); c != nil {
		c.Error(msg, kv...)
	}
	t.main.Fatal(msg, kv...)
}

func (t teeLogger) with(main logger.Logger, o teeOp) teeLogger {
	return teeLogger{main: main, ops: append(append(make([]teeOp, 0, len(t.ops)+1), t.ops...), o)}
}

func (t teeLogger) With(key string, value any) logger.Logger {
	return t.with(t.main.With(key, value), teeOp{kind: 'w', key: key, value: value})
}

func (t teeLogger) WithError(err error) logger.Logger {
	return t.with(t.main.WithError(err), teeOp{kind: 'e', err: err})
}

func (t teeLogger) WithGroup(group string) logger.Logger {
	return t.with(t.main.WithGroup(group), teeOp{kind: 'g', key: group})
}
