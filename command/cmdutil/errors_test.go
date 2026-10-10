package cmdutil

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

func httpErr(code int, msg string) error {
	return &apiclient.HTTPError{StatusCode: code, Method: "POST", Path: "/api/x", Server: "https://knot.example.com", ServerMessage: msg}
}

func TestFormatError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"403 with server message",
			fmt.Errorf("couldn't start space %q: %w", "dev", httpErr(403, "No permission to access this space")),
			`Error: couldn't start space "dev": no permission to access this space (403). Ask an administrator for the permission this needs`,
		},
		{
			"401 suggests connect",
			fmt.Errorf("couldn't list spaces: %w", httpErr(401, "")),
			"Error: couldn't list spaces: not signed in or the token has expired (401). Run `knot connect https://knot.example.com` to sign in again, or check --token",
		},
		{
			"404",
			fmt.Errorf("couldn't delete script %q: %w", "build", httpErr(404, "script not found")),
			`Error: couldn't delete script "build": script not found (404)`,
		},
		{
			"409",
			fmt.Errorf("couldn't create space %q: %w", "dev", httpErr(409, "")),
			`Error: couldn't create space "dev": it conflicts with something that already exists or has changed (409)`,
		},
		{
			"500",
			fmt.Errorf("couldn't stop pool %q: %w", "p", httpErr(500, "database unavailable")),
			`Error: couldn't stop pool "p": database unavailable (500). Try again, and check the server logs if it keeps failing`,
		},
		{
			"429",
			httpErr(429, ""),
			"Error: too many requests (429). Wait a moment and try again",
		},
		{
			"cleaned message keeps the type",
			fmt.Errorf("b:k: %w", CleanErr(httpErr(403, "No permission to write files"))),
			"Error: b:k: no permission to write files (403). Ask an administrator for the permission this needs",
		},
		{
			"connection refused",
			fmt.Errorf("couldn't list spaces: %w", &url.Error{Op: "Get", URL: "https://knot.example.com:3000/api/spaces?x=1",
				Err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}}),
			"Error: couldn't list spaces: can't reach the server at https://knot.example.com:3000: connection refused (is the server running?). Check --server (or the saved connection) and your network",
		},
		{
			"dns",
			&url.Error{Op: "Get", URL: "https://nope.invalid/api/ping", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Name: "nope.invalid", Err: "no such host", IsNotFound: true}}},
			`Error: can't reach the server at https://nope.invalid: the host name "nope.invalid" can't be found. Check --server (or the saved connection) and your network`,
		},
		{
			"certificate",
			&url.Error{Op: "Get", URL: "https://knot.local/api/ping", Err: x509.UnknownAuthorityError{}},
			"Error: can't reach the server at https://knot.local: its TLS certificate can't be verified (signed by an unknown authority). If you trust this server, pass --tls-skip-verify",
		},
		{
			"timeout",
			&url.Error{Op: "Get", URL: "https://knot.local/api/ping", Err: context.DeadlineExceeded},
			"Error: can't reach the server at https://knot.local: the request timed out. Check --server (or the saved connection) and your network",
		},
		{
			"no server",
			ErrNoServer,
			"Error: no server configured. Run `knot connect <server>` first, pass --server and --token, or pick a saved connection with --alias",
		},
		{
			"plain error tidied",
			errors.New("Error: Something went wrong.\n"),
			"Error: something went wrong",
		},
		{
			"acronym kept",
			errors.New("TOTP code required"),
			"Error: TOTP code required",
		},
	}
	for _, c := range cases {
		if got := FormatError(c.err); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
	if FormatError(nil) != "" {
		t.Error("nil should format as empty")
	}
}

func TestCleanAPIError(t *testing.T) {
	err := fmt.Errorf("listing: %w", httpErr(400, "bad prefix"))
	if got := CleanAPIError(err); got != "listing: bad prefix" {
		t.Errorf("CleanAPIError = %q", got)
	}
	if got := CleanAPIError(errors.New("unexpected status code: 400: legacy")); got != "legacy" {
		t.Errorf("legacy CleanAPIError = %q", got)
	}
	ce := CleanErr(httpErr(403, "nope"))
	if ce.Error() != "nope" || !apiclient.IsForbidden(ce) {
		t.Errorf("CleanErr = %q, forbidden=%v", ce.Error(), apiclient.IsForbidden(ce))
	}
}
