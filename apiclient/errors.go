package apiclient

import (
	"net/http"

	"github.com/paularlott/knot/internal/util/rest"
)

// HTTPError is the error returned when the server answers a request with an
// unexpected status. Use errors.As, AsHTTPError or the Is* helpers to inspect
// it rather than matching on the error text.
type HTTPError = rest.HTTPError

// AsHTTPError returns the HTTPError in err's chain, or nil.
func AsHTTPError(err error) *HTTPError { return rest.AsHTTPError(err) }

// StatusCode returns the HTTP status carried by err, or 0 when err did not
// come from a server response (e.g. a connection failure).
func StatusCode(err error) int { return rest.StatusOf(err) }

// IsUnauthorized reports a 401: missing, invalid or expired credentials.
func IsUnauthorized(err error) bool { return rest.IsStatus(err, http.StatusUnauthorized) }

// IsForbidden reports a 403: signed in but not allowed.
func IsForbidden(err error) bool { return rest.IsStatus(err, http.StatusForbidden) }

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return rest.IsStatus(err, http.StatusNotFound) }

// IsConflict reports a 409 Conflict or 412 Precondition Failed.
func IsConflict(err error) bool {
	return rest.IsStatus(err, http.StatusConflict, http.StatusPreconditionFailed)
}

// IsTooManyRequests reports a 429.
func IsTooManyRequests(err error) bool { return rest.IsStatus(err, http.StatusTooManyRequests) }

// IsServerError reports any 5xx status.
func IsServerError(err error) bool {
	code := rest.StatusOf(err)
	return code >= 500 && code <= 599
}

// newStatusError builds an HTTPError for callers that only have the status
// code (the body was decoded or discarded before the status was checked).
func newStatusError(statusCode int, method, path string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Method: method, Path: path}
}
