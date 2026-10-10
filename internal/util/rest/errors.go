package rest

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// HTTPError is returned by the REST clients when the server answers with a
// status other than the one the caller expected. It carries the status code,
// the request that failed and the server's own message (the "error" field of
// knot's {"error": "..."} envelope, or the raw body when there is no
// envelope), so callers can branch on the status with errors.As or the
// StatusOf / IsStatus helpers instead of parsing strings.
//
// Error() keeps the historical "unexpected status code: N: message" wording:
// scripts, the agent link and older callers match on that prefix. Use
// Message() for the server's text alone, or Summary() for a phrase fit to show
// a person.
type HTTPError struct {
	StatusCode int
	Method     string
	Path       string
	// Server is the scheme and host the request went to, when known
	// (empty for in-process mux calls).
	Server string
	// ServerMessage is the server's explanation, already unwrapped from the
	// JSON/msgpack error envelope. It may be empty.
	ServerMessage string
}

// NewHTTPError builds an HTTPError from a response body, decoding knot's error
// envelope when present.
func NewHTTPError(statusCode int, method, path string, body []byte) *HTTPError {
	return &HTTPError{
		StatusCode:    statusCode,
		Method:        method,
		Path:          path,
		ServerMessage: strings.TrimSpace(errorMessageFromBody(body)),
	}
}

func (e *HTTPError) Error() string {
	if e.ServerMessage == "" {
		return fmt.Sprintf("unexpected status code: %d", e.StatusCode)
	}
	return fmt.Sprintf("unexpected status code: %d: %s", e.StatusCode, e.ServerMessage)
}

// Message returns the server's explanation, or the standard status text when
// the server gave none.
func (e *HTTPError) Message() string {
	if e.ServerMessage != "" {
		return e.ServerMessage
	}
	if text := http.StatusText(e.StatusCode); text != "" {
		return strings.ToLower(text)
	}
	return fmt.Sprintf("status %d", e.StatusCode)
}

// Summary returns a plain-language description of the failure followed by the
// status code, e.g. "no permission to manage users (403)". When the server gave
// no message a description of the status is used instead.
func (e *HTTPError) Summary() string {
	msg := e.ServerMessage
	if msg == "" {
		msg = defaultStatusMessage(e.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", lowerFirst(msg), e.StatusCode)
}

func defaultStatusMessage(code int) string {
	switch {
	case code == http.StatusUnauthorized:
		return "not signed in or the token has expired"
	case code == http.StatusForbidden:
		return "you don't have permission to do this"
	case code == http.StatusNotFound:
		return "not found"
	case code == http.StatusConflict:
		return "it conflicts with something that already exists or has changed"
	case code == http.StatusPreconditionFailed:
		return "it was changed by someone else since it was read"
	case code == http.StatusTooManyRequests:
		return "too many requests"
	case code == http.StatusServiceUnavailable:
		return "the server is unavailable"
	case code >= 500:
		return "the server hit an internal error"
	}
	if text := http.StatusText(code); text != "" {
		return strings.ToLower(text)
	}
	return fmt.Sprintf("the server returned status %d", code)
}

// lowerFirst lower-cases the first letter of a server message so it reads as
// part of a sentence ("No permission to ..." -> "no permission to ..."), but
// leaves acronyms such as "SSH key ..." or "TOTP ..." alone.
func lowerFirst(s string) string {
	if len(s) < 2 {
		return strings.ToLower(s)
	}
	if s[0] >= 'A' && s[0] <= 'Z' && !(s[1] >= 'A' && s[1] <= 'Z') {
		return string(s[0]+('a'-'A')) + s[1:]
	}
	return s
}

// AsHTTPError returns the HTTPError in err's chain, or nil.
func AsHTTPError(err error) *HTTPError {
	var he *HTTPError
	if errors.As(err, &he) {
		return he
	}
	return nil
}

// StatusOf returns the HTTP status code carried by err, or 0 when err is not
// (or does not wrap) an HTTPError.
func StatusOf(err error) int {
	if he := AsHTTPError(err); he != nil {
		return he.StatusCode
	}
	return 0
}

// IsStatus reports whether err carries any of the given HTTP status codes.
func IsStatus(err error, codes ...int) bool {
	status := StatusOf(err)
	if status == 0 {
		return false
	}
	for _, c := range codes {
		if status == c {
			return true
		}
	}
	return false
}
