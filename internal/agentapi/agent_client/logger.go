package agent_client

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/logger"
)

// AgentClientLogger is a logger.Logger implementation that writes to the agent client log stream
type AgentClientLogger struct {
	client  *AgentClient
	service string
	group   string
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
	l.log(msg.LogLevelDebug, message, keysAndValues...)
}

// Debug logs a debug message
func (l *AgentClientLogger) Debug(message string, keysAndValues ...any) {
	l.log(msg.LogLevelDebug, message, keysAndValues...)
}

// Info logs an info message
func (l *AgentClientLogger) Info(message string, keysAndValues ...any) {
	l.log(msg.LogLevelInfo, message, keysAndValues...)
}

// Warn logs a warning message
func (l *AgentClientLogger) Warn(message string, keysAndValues ...any) {
	l.log(msg.LogLevelInfo, message, keysAndValues...)
}

// Error logs an error message
func (l *AgentClientLogger) Error(message string, keysAndValues ...any) {
	l.log(msg.LogLevelError, message, keysAndValues...)
}

// Fatal logs a fatal message and exits
func (l *AgentClientLogger) Fatal(message string, keysAndValues ...any) {
	l.log(msg.LogLevelError, message, keysAndValues...)
}

// With adds a key-value pair to the logger
func (l *AgentClientLogger) With(key string, value any) logger.Logger {
	return l
}

// WithError adds an error to the logger
func (l *AgentClientLogger) WithError(err error) logger.Logger {
	return l
}

// WithGroup adds a group to the logger
func (l *AgentClientLogger) WithGroup(group string) logger.Logger {
	return &AgentClientLogger{
		client:  l.client,
		service: l.service,
		group:   group,
	}
}

// log formats and sends a log message
func (l *AgentClientLogger) log(level msg.LogLevel, message string, keysAndValues ...any) {
	if l.client == nil {
		return
	}

	// Format the message with keys and values
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
