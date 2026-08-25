package log

import (
	"io"
	"os"

	"github.com/paularlott/logger"
	logslog "github.com/paularlott/logger/slog"
)

var (
	defaultLogger  logger.Logger
	pipelineLogger logger.Logger

	// externalWriter holds the httpWriter when external logging is
	// configured, so Flush can drain it.
	externalWriter io.Writer
)

func init() {
	// Initialize with default configuration
	defaultLogger = logslog.New(logslog.Config{
		Level:  "info",
		Format: "console",
		Writer: os.Stderr,
	})
	// The pipeline logger carries records promised to the external service
	// (audit events, forwarded space logs, tunnel requests). It shares the
	// main logger's writer but is never level-filtered: log.level filters
	// server diagnostics, not data the operator asked to be delivered.
	pipelineLogger = defaultLogger
}

// Configure sets up the logger with the given settings
// Call this early in your main() function
func Configure(level, format string, writer io.Writer) {
	if writer == nil {
		writer = os.Stderr
	}

	defaultLogger = logslog.New(logslog.Config{
		Level:  level,
		Format: format,
		Writer: writer,
	})
	pipelineLogger = logslog.New(logslog.Config{
		Level:  "info",
		Format: format,
		Writer: writer,
	})
	externalWriter = nil
}

// ConfigureWithHTTP sets up the logger to send JSON-formatted records to the
// given HTTP endpoint. When a URL is configured stderr output is suppressed.
//
// Optional authentication:
//   - username + password: sent as HTTP Basic Auth on every request.
//   - token: sent as "Authorization: Bearer <token>"; takes precedence over
//     basic auth when both are configured.
func ConfigureWithHTTP(level, url, format, stream, username, password, token string) {
	if format == "" {
		format = "ndjson"
	}
	if stream == "" {
		stream = "knot"
	}

	// One writer instance shared by both loggers: a single batching, retry,
	// spool and degraded-mode pipeline for everything the external service
	// receives.
	writer := newHTTPWriter(url, format, stream, nil, username, password, token)
	externalWriter = writer

	defaultLogger = logslog.New(logslog.Config{
		Level:  level,
		Format: "json",
		Writer: writer,
	})
	pipelineLogger = logslog.New(logslog.Config{
		Level:  "info",
		Format: "json",
		Writer: writer,
	})
}

// Flush drains the external log writer when one is configured. A no-op
// otherwise.
func Flush() {
	if w, ok := externalWriter.(*httpWriter); ok {
		w.flush()
	}
}

// GetLogger returns the configured logger instance
// Use this when passing to libraries
func GetLogger() logger.Logger {
	return defaultLogger
}

// Package-level convenience functions
func Trace(msg string, keysAndValues ...any) {
	defaultLogger.Trace(msg, keysAndValues...)
}

func Debug(msg string, keysAndValues ...any) {
	defaultLogger.Debug(msg, keysAndValues...)
}

func Info(msg string, keysAndValues ...any) {
	defaultLogger.Info(msg, keysAndValues...)
}

// Pipeline emits a record that must reach the external logging service
// regardless of log.level — audit events, forwarded space logs, tunnel
// requests. Level filtering is for server diagnostics; these are data the
// operator asked to be delivered.
func Pipeline(msg string, keysAndValues ...any) {
	pipelineLogger.Info(msg, keysAndValues...)
}

func Warn(msg string, keysAndValues ...any) {
	defaultLogger.Warn(msg, keysAndValues...)
}

func Error(msg string, keysAndValues ...any) {
	defaultLogger.Error(msg, keysAndValues...)
}

func Fatal(msg string, keysAndValues ...any) {
	defaultLogger.Fatal(msg, keysAndValues...)
}

func With(key string, value any) logger.Logger {
	return defaultLogger.With(key, value)
}

func WithError(err error) logger.Logger {
	return defaultLogger.WithError(err)
}

func WithGroup(group string) logger.Logger {
	return defaultLogger.WithGroup(group)
}
