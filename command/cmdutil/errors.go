package cmdutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/util/rest"
)

// ErrNoServer is returned when a command needs a server but none was given on
// the command line, in the environment or in a saved connection.
var ErrNoServer = errors.New("no server configured")

// FormatError turns an error from a command into the single line shown on the
// terminal: "Error: <what failed>: <why>. <what to do next>". It recognises
// the API client's HTTPError (status code and server message) and the usual
// network failures (connection refused, unknown host, certificate problems,
// timeouts) anywhere in the error chain and rewrites them in plain words,
// keeping whatever context the command wrapped around them.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return "Error: " + Describe(err)
}

// Describe is FormatError without the "Error: " prefix, for warnings and
// per-item failures printed while a command carries on.
func Describe(err error) string {
	if err == nil {
		return ""
	}

	msg := strings.TrimSpace(err.Error())
	var hint string

	if he := apiclient.AsHTTPError(err); he != nil {
		msg = replaceOrAppend(msg, he.Error(), he.Message(), he.Summary())
		hint = httpHint(he)
	} else if target, reason, h, raw, ok := describeNetError(err); ok {
		phrase := "can't reach the server"
		if target != "" {
			phrase += " at " + target
		}
		phrase += ": " + reason
		msg = replaceOrAppend(msg, raw, "", phrase)
		hint = h
	} else if errors.Is(err, ErrNoServer) {
		hint = "Run `knot connect <server>` first, pass --server and --token, or pick a saved connection with --alias"
	} else if errors.Is(err, context.Canceled) {
		msg = replaceOrAppend(msg, context.Canceled.Error(), "", "cancelled")
	}

	msg = cleanMessage(msg)
	if hint != "" {
		msg += ". " + hint
	}
	return msg
}

// replaceOrAppend swaps the raw text of the underlying error for its friendly
// form. Commands sometimes pass a cleaned form (the server message alone)
// rather than the full error text, so that is tried too; when neither
// appears the friendly form is appended.
func replaceOrAppend(msg, raw, alt, friendly string) string {
	if raw != "" && strings.Contains(msg, raw) {
		return strings.Replace(msg, raw, friendly, 1)
	}
	if alt != "" {
		if i := strings.LastIndex(msg, alt); i >= 0 {
			return msg[:i] + friendly + msg[i+len(alt):]
		}
	}
	if msg == "" {
		return friendly
	}
	return msg + ": " + friendly
}

// cleanMessage normalises a message for display after "Error: ": no repeated
// "error:" prefixes, a lower-case first letter (acronyms kept) and no
// trailing full stop or newline.
func cleanMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	for {
		lower := strings.ToLower(msg)
		if strings.HasPrefix(lower, "error:") {
			msg = strings.TrimSpace(msg[len("error:"):])
			continue
		}
		break
	}
	msg = strings.TrimRight(msg, ". \n\t")
	return lowerFirst(msg)
}

func lowerFirst(s string) string {
	if len(s) < 2 {
		return strings.ToLower(s)
	}
	if s[0] >= 'A' && s[0] <= 'Z' && !(s[1] >= 'A' && s[1] <= 'Z') {
		return string(s[0]+('a'-'A')) + s[1:]
	}
	return s
}

func httpHint(he *apiclient.HTTPError) string {
	switch code := he.StatusCode; {
	case code == http.StatusUnauthorized:
		server := he.Server
		if server == "" {
			server = "<server>"
		}
		return fmt.Sprintf("Run `knot connect %s` to sign in again, or check --token", server)
	case code == http.StatusForbidden:
		return "Ask an administrator for the permission this needs"
	case code == http.StatusPreconditionFailed:
		return "It changed while this was running; try again"
	case code == http.StatusTooManyRequests:
		return "Wait a moment and try again"
	case code >= 500:
		return "Try again, and check the server logs if it keeps failing"
	}
	return ""
}

// describeNetError recognises a failure to talk to the server at all. It
// returns the server address, the reason in plain words, a hint and the raw
// text of the error to replace.
func describeNetError(err error) (target, reason, hint, raw string, ok bool) {
	var ue *url.Error
	var oe *net.OpError
	switch {
	case errors.As(err, &ue):
		raw = ue.Error()
		if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
			target = (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
		}
	case errors.As(err, &oe) && oe.Op == "dial":
		raw = oe.Error()
		if oe.Addr != nil {
			target = oe.Addr.String()
		}
	default:
		// A bare DNS or certificate error without a wrapper.
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) && !isCertError(err) {
			return "", "", "", "", false
		}
	}

	const checkServer = "Check --server (or the saved connection) and your network"

	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case strings.Contains(err.Error(), "unsupported protocol scheme"):
		return target, "the server address is missing or doesn't start with http:// or https://",
			"Run `knot connect <server>` first, or pass --server with a full URL", raw, true
	case isCertError(err):
		return target, "its TLS certificate can't be verified (" + certDetail(err) + ")",
			"If you trust this server, pass --tls-skip-verify", raw, true
	case errors.As(err, &dnsErr):
		return target, fmt.Sprintf("the host name %q can't be found", dnsErr.Name), checkServer, raw, true
	case errors.Is(err, syscall.ECONNREFUSED):
		return target, "connection refused (is the server running?)", checkServer, raw, true
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return target, "the connection was closed unexpectedly", "Try again; if it persists, check the server is healthy", raw, true
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return target, "the request timed out", checkServer, raw, true
	case errors.Is(err, context.Canceled):
		return target, "the request was cancelled", "", raw, true
	case strings.Contains(err.Error(), "server gave HTTP response to HTTPS client"):
		return target, "the server speaks HTTP, not HTTPS", "Use an http:// address for --server", raw, true
	}
	if ue != nil || oe != nil {
		inner := err
		if ue != nil {
			inner = ue.Err
		} else if oe.Err != nil {
			inner = oe.Err
		}
		return target, inner.Error(), checkServer, raw, true
	}
	return "", "", "", "", false
}

func isCertError(err error) bool {
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verify *tls.CertificateVerificationError
	return errors.As(err, &unknownAuth) || errors.As(err, &hostErr) ||
		errors.As(err, &invalid) || errors.As(err, &verify)
}

func certDetail(err error) string {
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	switch {
	case errors.As(err, &unknownAuth):
		return "signed by an unknown authority"
	case errors.As(err, &hostErr):
		return "it isn't valid for " + hostErr.Host
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return "expired or not yet valid"
		}
		return "the certificate is invalid"
	}
	return "verification failed"
}

// WebSocketError describes a failed websocket dial. When the server answered
// the handshake with an error status it becomes an HTTPError, so the status
// is reported (and hinted) like any other API failure.
func WebSocketError(what string, resp *http.Response, err error) error {
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		var body []byte
		if resp.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
		}
		he := rest.NewHTTPError(resp.StatusCode, http.MethodGet, "", body)
		if resp.Request != nil && resp.Request.URL != nil {
			he.Path = resp.Request.URL.Path
			scheme := strings.Replace(resp.Request.URL.Scheme, "ws", "http", 1)
			he.Server = (&url.URL{Scheme: scheme, Host: resp.Request.URL.Host}).String()
		}
		return fmt.Errorf("%s: %w", what, he)
	}
	return fmt.Errorf("%s: %w", what, err)
}
