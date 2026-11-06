package agent_client

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// AgentClientLogger is a logger.Logger implementation that writes to the
// space's log, through the agent client's log stream, and to the agent's own
// output (stderr, the container's log) as the agent's other records go.
type AgentClientLogger struct {
	client  *AgentClient
	service string
	group   string
	fields  []any // added with With and WithError, to every message
	// spaceOnly sends only to the space's log: the copy log.Tee makes of
	// records the agent already wrote to its own output.
	spaceOnly bool
}

// NewSpaceLogCopy is a logger that writes only to the space's log, for
// log.Tee to copy the agent's own records there.
func NewSpaceLogCopy(client *AgentClient, service string) logger.Logger {
	return &AgentClientLogger{client: client, service: service, spaceOnly: true}
}

// NewAgentClientLogger creates a new logger that writes to the agent client log stream
func NewAgentClientLogger(client *AgentClient, service string) logger.Logger {
	return &AgentClientLogger{
		client:  client,
		service: service,
		group:   "",
	}
}

// Trace logs a trace message
func (l *AgentClientLogger) Trace(message string, keysAndValues ...any) {
	l.local().Trace(message, keysAndValues...)
	l.log(msg.LogLevelDebug, message, keysAndValues...)
}

// Debug logs a debug message
func (l *AgentClientLogger) Debug(message string, keysAndValues ...any) {
	l.local().Debug(message, keysAndValues...)
	l.log(msg.LogLevelDebug, message, keysAndValues...)
}

// Info logs an info message
func (l *AgentClientLogger) Info(message string, keysAndValues ...any) {
	l.local().Info(message, keysAndValues...)
	l.log(msg.LogLevelInfo, message, keysAndValues...)
}

// Warn logs a warning message
func (l *AgentClientLogger) Warn(message string, keysAndValues ...any) {
	l.local().Warn(message, keysAndValues...)
	l.log(msg.LogLevelInfo, message, keysAndValues...)
}

// Error logs an error message
func (l *AgentClientLogger) Error(message string, keysAndValues ...any) {
	l.local().Error(message, keysAndValues...)
	l.log(msg.LogLevelError, message, keysAndValues...)
}

// Fatal logs a fatal message; it does not exit, as the agent carries on.
func (l *AgentClientLogger) Fatal(message string, keysAndValues ...any) {
	l.local().Error(message, keysAndValues...)
	l.log(msg.LogLevelError, message, keysAndValues...)
}

// local is the agent's own logger, without the copy to the space's log
// (this logger sends that itself), for the record's service, group and
// fields.
func (l *AgentClientLogger) local() logger.Logger {
	if l.spaceOnly {
		return logger.NewNullLogger()
	}
	lg := log.Local().With("service", l.service)
	if l.group != "" {
		lg = lg.WithGroup(l.group)
	}
	for i := 0; i+1 < len(l.fields); i += 2 {
		lg = lg.With(fmt.Sprint(l.fields[i]), l.fields[i+1])
	}
	return lg
}

// With adds a key-value pair to every message the logger sends.
func (l *AgentClientLogger) With(key string, value any) logger.Logger {
	c := *l
	c.fields = append(append([]any{}, l.fields...), key, value)
	return &c
}

// WithError adds an error to every message the logger sends.
func (l *AgentClientLogger) WithError(err error) logger.Logger {
	if err == nil {
		return l
	}
	return l.With("error", err.Error())
}

// WithGroup adds a group to the logger
func (l *AgentClientLogger) WithGroup(group string) logger.Logger {
	c := *l
	c.group = group
	return &c
}

// log formats and sends a log message
func (l *AgentClientLogger) log(level msg.LogLevel, message string, keysAndValues ...any) {
	if l.client == nil {
		return
	}

	// Format the message with keys and values, those added with With first
	if len(l.fields) > 0 {
		keysAndValues = append(append([]any{}, l.fields...), keysAndValues...)
	}
	if len(keysAndValues) > 0 {
		message = l.formatMessage(message, keysAndValues...)
	}

	// Add group prefix if set
	if l.group != "" {
		message = fmt.Sprintf("[%s] %s", l.group, message)
	}

	_ = l.client.SendLogMessage(l.service, level, message)
}

// formatMessage formats a log message with keys and values
func (l *AgentClientLogger) formatMessage(msg string, keysAndValues ...any) string {
	if len(keysAndValues) == 0 {
		return msg
	}

	var builder strings.Builder
	builder.WriteString(msg)

	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			value := fmt.Sprintf("%v", keysAndValues[i+1])
			builder.WriteString(fmt.Sprintf(" %s=%s", key, value))
		}
	}

	return builder.String()
}
