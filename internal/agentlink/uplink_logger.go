package agentlink

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/logger"
	logslog "github.com/paularlott/logger/slog"
)

// uplinkLink owns a single persistent connection to the agent daemon's command
// socket used exclusively for streaming log lines. All derived UplinkLogger
// instances (created via WithGroup) share the same link.
type uplinkLink struct {
	mu   sync.Mutex
	conn net.Conn
}

// send writes one LogRequest to the uplink. It lazily dials the command socket
// on first use and reconnects once if an in-flight write fails. Returns false
// if the line could not be delivered, in which case the caller should fall back
// to stderr.
func (l *uplinkLink) send(level msg.LogLevel, service, message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		path := commandSocketFile()
		if path == "" {
			return false
		}
		c, err := net.Dial("unix", path)
		if err != nil {
			return false
		}
		l.conn = c
	}

	req := LogRequest{Service: service, Level: byte(level), Message: message}
	if err := sendMsg(l.conn, CommandLog, req); err != nil {
		// Broken connection: drop it so the next call retries once.
		l.conn.Close()
		l.conn = nil
		return false
	}
	return true
}

func (l *uplinkLink) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		l.conn.Close()
		l.conn = nil
	}
}

// UplinkLogger is a logger.Logger that forwards log lines from a CLI
// sub-process (e.g. `knot run-script`) to the agent daemon over the command
// socket; the daemon then ships them upstream to the knot server via the agent
// client's log channel. If the uplink is unavailable, lines are written to
// stderr so logs are never silently lost.
type UplinkLogger struct {
	link     *uplinkLink
	service  string
	group    string
	fallback logger.Logger
}

// NewUplinkLogger returns a logger that streams to the agent daemon. Callers
// should generally use NewScriptLogger, which falls back to a plain stderr
// logger when no agent is running.
func NewUplinkLogger(service string) logger.Logger {
	return &UplinkLogger{
		link:     &uplinkLink{},
		service:  service,
		fallback: logslog.New(logslog.Config{Level: "info", Format: "console", Writer: os.Stderr}),
	}
}

// NewScriptLogger returns the appropriate logger for an interactive CLI
// sub-process running inside a space: an UplinkLogger (with stderr fallback)
// when the agent daemon is reachable, otherwise a plain stderr logger for
// standalone use.
func NewScriptLogger(service string) logger.Logger {
	if !IsAgentRunning() {
		return logslog.New(logslog.Config{Level: "info", Format: "console", Writer: os.Stderr})
	}
	return NewUplinkLogger(service)
}

func (l *UplinkLogger) Trace(message string, keysAndValues ...any) {
	l.emit(msg.LogLevelDebug, message, keysAndValues, l.fallback.Debug)
}

func (l *UplinkLogger) Debug(message string, keysAndValues ...any) {
	l.emit(msg.LogLevelDebug, message, keysAndValues, l.fallback.Debug)
}

func (l *UplinkLogger) Info(message string, keysAndValues ...any) {
	l.emit(msg.LogLevelInfo, message, keysAndValues, l.fallback.Info)
}

func (l *UplinkLogger) Warn(message string, keysAndValues ...any) {
	l.emit(msg.LogLevelInfo, message, keysAndValues, l.fallback.Warn)
}

func (l *UplinkLogger) Error(message string, keysAndValues ...any) {
	l.emit(msg.LogLevelError, message, keysAndValues, l.fallback.Error)
}

func (l *UplinkLogger) Fatal(message string, keysAndValues ...any) {
	// Match AgentClientLogger: report at error level without exiting.
	l.emit(msg.LogLevelError, message, keysAndValues, l.fallback.Error)
}

func (l *UplinkLogger) With(key string, value any) logger.Logger { return l }

func (l *UplinkLogger) WithError(err error) logger.Logger { return l }

func (l *UplinkLogger) WithGroup(group string) logger.Logger {
	return &UplinkLogger{
		link:     l.link,
		service:  l.service,
		group:    group,
		fallback: l.fallback,
	}
}

// emit formats, tags with the current group, and ships a single log line. If
// the uplink rejects it, the formatted line is written via the fallback logger.
func (l *UplinkLogger) emit(level msg.LogLevel, message string, keysAndValues []any, fallback func(string, ...any)) {
	if len(keysAndValues) > 0 {
		message = formatLogKV(message, keysAndValues...)
	}
	if l.group != "" {
		message = "[" + l.group + "] " + message
	}
	if !l.link.send(level, l.service, message) {
		fallback(message)
	}
}

func formatLogKV(msg string, keysAndValues ...any) string {
	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		fmt.Fprintf(&b, " %v=%v", keysAndValues[i], keysAndValues[i+1])
	}
	return b.String()
}
