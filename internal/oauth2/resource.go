package oauth2

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/config"
)

// Resource indicators (RFC 8707), as the MCP authorization specification
// requires: a client names the resource it wants a token for with the
// resource parameter at /authorize and /token, knot only issues tokens for
// its own MCP endpoint, and the token records that resource as its audience
// so /mcp can refuse a token issued for something else.

// ErrorInvalidTarget is the OAuth error for a resource the authorization
// server will not issue a token for (RFC 8707 section 2).
const ErrorInvalidTarget = "invalid_target"

// MCPResource returns the canonical resource identifier of knot's MCP
// endpoint, the same value the protected resource metadata advertises.
func MCPResource(r *http.Request) string {
	return BaseURL(r) + "/mcp"
}

// MatchesMCPResource reports whether uri names this server's MCP endpoint.
//
// The comparison is the one the protected resource (mcp.ResourceServer) uses
// for audiences: scheme and host compare case-insensitively, a trailing slash
// is ignored, and a query, fragment or userinfo never matches.
//
// When server.url is configured the scheme must match too. Without it,
// BaseURL takes the scheme from the request, which behind a TLS-terminating
// proxy is http while the client saw https, so the scheme is not compared
// and either http or https is accepted: only host and path decide. Configure
// server.url on a server behind a proxy for the strict comparison.
func MatchesMCPResource(r *http.Request, uri string) bool {
	return sameResource(uri, MCPResource(r), config.GetServerConfig().URL != "")
}

// checkResourceParams validates the resource parameters of an authorization
// or token request. It returns the canonical MCP resource when at least one
// was given, "" when none was, and ok false when any is not an absolute URI
// without a fragment or does not name this server's MCP endpoint. RFC 8707
// allows the parameter to repeat; knot has a single resource, so every value
// must name it.
func checkResourceParams(r *http.Request, values []string) (resource, desc string, ok bool) {
	for _, v := range values {
		if !validResourceIndicator(v) {
			return "", "resource must be an absolute URI without a fragment", false
		}
		if !MatchesMCPResource(r, v) {
			return "", "this server only issues tokens for " + MCPResource(r), false
		}
		resource = MCPResource(r)
	}
	return resource, "", true
}

// validResourceIndicator reports whether s is an absolute URI without a
// fragment, as RFC 8707 section 2 requires of a resource parameter.
func validResourceIndicator(s string) bool {
	if s == "" || strings.Contains(s, "#") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.IsAbs() && u.Host != ""
}

// sameResource compares two resource identifiers as described on
// MatchesMCPResource; strictScheme says whether the schemes must agree.
func sameResource(a, b string, strictScheme bool) bool {
	ua, errA := url.Parse(strings.TrimSuffix(a, "/"))
	ub, errB := url.Parse(strings.TrimSuffix(b, "/"))
	if errA != nil || errB != nil || ua.Host == "" || ub.Host == "" {
		return false
	}
	if ua.RawQuery != "" || ua.Fragment != "" || ua.User != nil || ua.ForceQuery {
		return false
	}
	if strictScheme {
		if !strings.EqualFold(ua.Scheme, ub.Scheme) {
			return false
		}
	} else if !isHTTPScheme(ua.Scheme) || !isHTTPScheme(ub.Scheme) {
		return false
	}
	return strings.EqualFold(ua.Host, ub.Host) && ua.Path == ub.Path
}

func isHTTPScheme(s string) bool {
	return strings.EqualFold(s, "http") || strings.EqualFold(s, "https")
}
