package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

// AI script modes. Read mode may only GET (or HEAD); write mode may also
// change things, within the user's role and outside the deny list.
const (
	AIScriptModeRead  = "read"
	AIScriptModeWrite = "write"
)

// aiScriptRoute is one entry of the AI script deny list: a path prefix (on
// whole segments, "*" matching any one segment) and the methods it covers.
// An empty methods list covers every method, reads included.
type aiScriptRoute struct {
	pattern string
	methods []string
	reason  string
}

// writes is every method that changes something.
var writes = []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// aiScriptDenied is refused to AI-written scripts in either mode, whatever
// the user's role: credentials, sign-in and sessions, backups, account
// switching, role, permission, group and user changes, the endpoints that
// would reach the network or another tool from inside the sandbox, and
// those that store or run code outside the script's guard (stored scripts,
// space jobs, plugin field handlers; see also aiScriptBodyCheck). A script
// that needs one of these must be done by the user in the web UI or with a
// dedicated tool.
var aiScriptDenied = []aiScriptRoute{
	{"/api/tokens", nil, "API tokens"},
	{"/api/auth", nil, "sign-in"},
	{"/api/sessions", nil, "sign-in sessions and devices"},
	{"/api/users/whoami/passkeys", nil, "passkeys"},
	{"/api/users/*/passkeys", nil, "passkeys"},
	{"/api/users/*/linked-users", nil, "linked users (switching accounts)"},
	{"/api/users/*/auth-provider", nil, "sign-in providers"},
	{"/api/users/whoami/ssh-private-key", nil, "SSH keys"},
	{"/api/users/whoami/ssh-public-key", nil, "SSH keys"},
	{"/api/users/*/onepassword-token", nil, "secrets"},
	{"/api/users/*/secrets", nil, "secrets"},
	{"/api/secrets", nil, "secrets"},
	{"/api/onepassword", nil, "secrets"},
	{"/api/vault", nil, "secrets"},
	{"/api/backup", nil, "backups"},
	{"/api/restore", nil, "restores"},
	{"/api/roles", writes, "role and permission changes"},
	{"/api/groups", writes, "group changes"},
	{"/api/users", writes, "creating, changing or deleting users"},
	{"/api/chat", nil, "calling other tools"},
	{"/api/events", []string{http.MethodGet}, "event streams"},
	{"/api/spaces/*/execute-script-stream", nil, "streamed script runs"},
	{"/api/mcp-servers", nil, "MCP server configuration (it holds credentials, and listing tools reaches the remote server)"},
	{"/api/scripts", writes, "stored scripts (they run later with the user's full access, outside the script's guard)"},
	{"/api/spaces/*/jobs", writes, "space jobs (stored commands that run later)"},
	{"/api/plugins/field-handlers/*", nil, "plugin field handlers (they run plugin code outside the script's guard)"},
}

// aiScriptAllowed are exceptions to aiScriptDenied: the user's own
// navigation preferences live under /api/users but are not account changes.
var aiScriptAllowed = []string{"/api/users/preferences/nav"}

// AIScriptPolicy guards every API call an AI-written script makes: the mode,
// the deny list and the call budget. It is safe for concurrent use.
type AIScriptPolicy struct {
	Mode     string
	MaxCalls int64
	calls    atomic.Int64
}

// Calls returns the API calls made so far, refused ones over the budget not
// counted twice.
func (p *AIScriptPolicy) Calls() int64 {
	n := p.calls.Load()
	if p.MaxCalls > 0 && n > p.MaxCalls {
		return p.MaxCalls
	}
	return n
}

// Check is run before each API call that has no body to inspect. It returns
// an error for the script (and the model) to act on when the call is not
// allowed.
func (p *AIScriptPolicy) Check(method, requestPath string) error {
	return p.CheckRequest(method, requestPath, nil)
}

// Precheck is the part of CheckRequest that needs only the method and path:
// the path rules, the mode and the deny list. It counts no call. The
// transport runs it before building a request's body, so a refused call
// costs nothing to build.
func (p *AIScriptPolicy) Precheck(method, requestPath string) (string, error) {
	method = strings.ToUpper(method)
	clean, err := aiScriptPath(requestPath)
	if err != nil {
		return "", err
	}
	if p.Mode != AIScriptModeWrite && method != http.MethodGet && method != http.MethodHead {
		return "", fmt.Errorf("%s %s refused: the script runs in read mode, which can only read. Run it with mode \"write\" if the user asked for this change", method, clean)
	}
	if route, denied := aiScriptDeniedRoute(method, clean); denied {
		return "", fmt.Errorf("%s %s refused: AI-written scripts cannot touch %s, in either mode. Ask the user to do this in the knot web UI", method, clean, route.reason)
	}
	return clean, nil
}

// CheckRequest is run before each API call: Precheck, the request body
// (aiScriptBodyCheck) and the call budget, which a batch of method calls
// draws on once per call in it.
func (p *AIScriptPolicy) CheckRequest(method, requestPath string, body interface{}) error {
	method = strings.ToUpper(method)
	clean, err := p.Precheck(method, requestPath)
	if err != nil {
		return err
	}
	calls := int64(1)
	if body != nil {
		n, err := aiScriptBodyCheck(method, clean, body)
		if err != nil {
			return err
		}
		calls = n
	}
	if n := p.calls.Add(calls); p.MaxCalls > 0 && n > p.MaxCalls {
		return fmt.Errorf("%s %s refused: a script may make at most %d API calls per run (a batch of method calls counts each call). Fetch lists once and work on them in the script, or narrow the question", method, clean, p.MaxCalls)
	}
	return nil
}

// aiScriptBodyCheck inspects the body of a request the deny list let
// through, for what only the body shows: code to run or store, and
// redacted values written back. It returns the API calls the request counts
// as: one, or each call of a batch sent to /api/methods/call. The server
// decodes bodies with encoding/json, which matches keys to fields whatever
// their case, so keys are matched here the same way, and a body with two
// keys differing only in case (which one the server takes is not this
// check's to guess) is refused.
func aiScriptBodyCheck(method, cleanPath string, body interface{}) (int64, error) {
	generic, err := genericJSON(method, cleanPath, body)
	if err != nil {
		return 0, err
	}
	if where, dup := caseDuplicateKey(generic, ""); dup {
		return 0, fmt.Errorf("%s %s refused: the request body has keys that differ only in case (%s); send each field once", method, cleanPath, where)
	}
	segments := strings.Split(strings.Trim(strings.ToLower(cleanPath), "/"), "/")
	isEventSink := len(segments) >= 2 && segments[1] == "event-sinks"
	if isEventSink {
		// A sink read back has its webhook secret redacted; the server keeps
		// the stored secret when it is sent empty.
		if m, ok := generic.(map[string]interface{}); ok {
			if webhook, ok := foldGet(m, "webhook").(map[string]interface{}); ok {
				if key, ok := foldKey(webhook, "secret"); ok && webhook[key] == redactedValue {
					webhook[key] = ""
				}
			}
		}
	}
	if containsRedacted(generic) {
		return 0, fmt.Errorf("%s %s refused: the request carries the %q placeholder that stands for a value scripts cannot see, and it could not be matched to the stored value. Write back a field exactly as it was read, in a PUT to the path it was read from, or set a new value, or ask the user to set it in the knot web UI", method, cleanPath, redactedValue)
	}

	switch {
	case len(segments) == 4 && segments[1] == "spaces" && segments[3] == "execute-script":
		// Running a named script the user may already run is fine; running
		// code the script wrote would step outside its guard.
		if m, ok := generic.(map[string]interface{}); ok && nonEmptyString(foldGet(m, "content")) {
			return 0, fmt.Errorf("%s %s refused: AI-written scripts cannot run their own code in a space, in either mode; run an existing script by script_name or script_id instead", method, cleanPath)
		}
	case isEventSink && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch):
		if m, ok := generic.(map[string]interface{}); ok {
			sinkType, _ := foldGet(m, "sink_type").(string)
			if strings.EqualFold(strings.TrimSpace(sinkType), "script") || nonEmptyString(foldGet(m, "script_id")) {
				return 0, fmt.Errorf("%s %s refused: AI-written scripts cannot create or change script event sinks (the script would run later with full access), in either mode. Ask the user to do this in the knot web UI", method, cleanPath)
			}
		}
	case len(segments) == 3 && segments[1] == "methods" && segments[2] == "call":
		if batch, ok := generic.([]interface{}); ok && len(batch) > 1 {
			return int64(len(batch)), nil
		}
	}
	return 1, nil
}

// genericJSON returns a request body as decoded JSON (maps, lists and
// scalars), as the server will see it.
func genericJSON(method, cleanPath string, body interface{}) (interface{}, error) {
	switch b := body.(type) {
	case nil:
		return nil, nil
	case map[string]interface{}, []interface{}:
		return b, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s %s refused: the request body could not be encoded as JSON: %v", method, cleanPath, err)
	}
	var generic interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("%s %s refused: the request body could not be read back as JSON: %v", method, cleanPath, err)
	}
	return generic, nil
}

// foldKey returns the key of m matching name whatever its case, the exact
// key first.
func foldKey(m map[string]interface{}, name string) (string, bool) {
	if _, ok := m[name]; ok {
		return name, true
	}
	for k := range m {
		if strings.EqualFold(k, name) {
			return k, true
		}
	}
	return "", false
}

// foldGet returns the value of m's key matching name whatever its case.
func foldGet(m map[string]interface{}, name string) interface{} {
	if k, ok := foldKey(m, name); ok {
		return m[k]
	}
	return nil
}

// caseDuplicateKey reports an object, at any depth, with two keys that
// differ only in case, and where. The values of "env" objects are
// environment variables, whose names are case sensitive, and are left out.
func caseDuplicateKey(v interface{}, where string) (string, bool) {
	switch value := v.(type) {
	case map[string]interface{}:
		seen := make(map[string]string, len(value))
		for k := range value {
			lower := strings.ToLower(k)
			if other, ok := seen[lower]; ok {
				return fmt.Sprintf("%q and %q%s", other, k, at(where)), true
			}
			seen[lower] = k
		}
		for k, item := range value {
			if strings.EqualFold(k, "env") {
				continue
			}
			if w, dup := caseDuplicateKey(item, where+"."+k); dup {
				return w, true
			}
		}
	case []interface{}:
		for i, item := range value {
			if w, dup := caseDuplicateKey(item, fmt.Sprintf("%s[%d]", where, i)); dup {
				return w, true
			}
		}
	}
	return "", false
}

// at describes where in a body a key is, "" at the top.
func at(where string) string {
	if where == "" {
		return ""
	}
	return " in " + strings.TrimPrefix(where, ".")
}

// nonEmptyString reports whether v is a string with something in it.
func nonEmptyString(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

// containsRedacted reports whether a decoded JSON value holds the redaction
// placeholder anywhere.
func containsRedacted(v interface{}) bool {
	switch value := v.(type) {
	case string:
		return stringHasRedacted(value)
	case map[string]interface{}:
		for _, item := range value {
			if containsRedacted(item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range value {
			if containsRedacted(item) {
				return true
			}
		}
	}
	return false
}

// stringHasRedacted reports whether s holds the placeholder, which inside a
// URL is percent-encoded.
func stringHasRedacted(s string) bool {
	return strings.Contains(s, redactedValue) || strings.Contains(strings.ToLower(s), "%5bredacted%5d")
}

// aiScriptPath returns the clean path of an API request, refusing anything
// but a plain /api/... path: no scheme or host, no dot segments, no encoded
// slashes, and nothing the router could read differently from this check (a
// fragment, which url.Parse drops but the in-process router keeps, path
// parameters, backslashes, encoded backslashes or double encoding). The
// encoding rules apply to the path only: query values are decoded once, by
// the handler, so an encoded slash there (a file key's prefix, a delimiter)
// is ordinary data. A # is refused anywhere.
func aiScriptPath(requestPath string) (string, error) {
	if strings.Contains(requestPath, "#") {
		return "", fmt.Errorf("%q refused: an API path may not contain #", requestPath)
	}
	for _, r := range requestPath {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%q refused: an API path may not contain control characters", requestPath)
		}
	}
	pathPart := requestPath
	if i := strings.IndexByte(pathPart, '?'); i >= 0 {
		pathPart = pathPart[:i]
	}
	if strings.ContainsAny(pathPart, ";\\") {
		return "", fmt.Errorf("%q refused: an API path may not contain ; or backslashes", requestPath)
	}
	lowerPath := strings.ToLower(pathPart)
	for _, encoded := range []string{"%23", "%3b", "%5c", "%25", "%2f", "%00"} {
		if strings.Contains(lowerPath, encoded) {
			return "", fmt.Errorf("%q refused: an API path may not contain encoded #, ;, %%, slashes, backslashes or NULs before its query string", requestPath)
		}
	}
	ref, err := url.Parse(requestPath)
	if err != nil {
		return "", fmt.Errorf("invalid API path %q: %v", requestPath, err)
	}
	if ref.Scheme != "" || ref.Host != "" || ref.Opaque != "" {
		return "", fmt.Errorf("%q refused: scripts call the knot API by path (/api/...), not by URL", requestPath)
	}
	if ref.Fragment != "" || strings.Contains(ref.Path, "\\") {
		return "", fmt.Errorf("%q refused: use a plain API path", requestPath)
	}
	p := path.Clean(ref.Path)
	if ref.Path == "" || (p != ref.Path && p+"/" != ref.Path) {
		return "", fmt.Errorf("%q refused: use a plain API path without . or .. segments", requestPath)
	}
	if p != "/api" && !strings.HasPrefix(p, "/api/") {
		return "", fmt.Errorf("%q refused: scripts can only call the knot API (/api/...)", requestPath)
	}
	return p, nil
}

// aiScriptDeniedRoute reports the deny list entry a request falls under.
func aiScriptDeniedRoute(method, cleanPath string) (aiScriptRoute, bool) {
	lower := strings.ToLower(cleanPath)
	for _, allowed := range aiScriptAllowed {
		if lower == allowed {
			return aiScriptRoute{}, false
		}
	}
	segments := strings.Split(strings.Trim(lower, "/"), "/")
	for _, route := range aiScriptDenied {
		if !routeMatches(route.pattern, segments) {
			continue
		}
		if len(route.methods) == 0 {
			return route, true
		}
		for _, m := range route.methods {
			if m == method || (m == http.MethodGet && method == http.MethodHead) {
				return route, true
			}
		}
	}
	return aiScriptRoute{}, false
}

// routeMatches reports whether the path segments start with the pattern's.
func routeMatches(pattern string, segments []string) bool {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(segments) < len(parts) {
		return false
	}
	for i, part := range parts {
		if part != "*" && part != segments[i] {
			return false
		}
	}
	return true
}

// aiScriptClient is the REST client an AI-written script's knot.apiclient
// uses: every call is checked against the policy before it reaches the
// user's own client, so the user's role and the policy both apply.
type aiScriptClient struct {
	inner  rest.RESTClient
	policy *AIScriptPolicy
}

func newAIScriptClient(inner rest.RESTClient, policy *AIScriptPolicy) *aiScriptClient {
	return &aiScriptClient{inner: inner, policy: policy}
}

func (c *aiScriptClient) Get(ctx context.Context, p string, response interface{}) (int, error) {
	if err := c.policy.Check(http.MethodGet, p); err != nil {
		return 0, err
	}
	code, err := c.inner.Get(ctx, p, response)
	redactSecrets(response)
	return code, err
}

func (c *aiScriptClient) GetJSON(ctx context.Context, p string, response interface{}) (int, error) {
	if err := c.policy.Check(http.MethodGet, p); err != nil {
		return 0, err
	}
	code, err := c.inner.GetJSON(ctx, p, response)
	redactSecrets(response)
	return code, err
}

// PrecheckRequest lets knot.apiclient refuse a call before it converts the
// call's body (knotscriptling.RequestPrechecker).
func (c *aiScriptClient) PrecheckRequest(method, p string) error {
	_, err := c.policy.Precheck(method, p)
	return err
}

// prepareWrite checks a request with a body and returns the body to send:
// redacted values written back in a PUT are first restored from the stored
// object (restoreRedacted).
func (c *aiScriptClient) prepareWrite(ctx context.Context, method, p string, request interface{}) (interface{}, error) {
	clean, err := c.policy.Precheck(method, p)
	if err != nil {
		return nil, err
	}
	body, err := genericJSON(method, clean, request)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPut && containsRedacted(body) {
		if body, err = c.restoreRedacted(ctx, clean, body); err != nil {
			return nil, err
		}
	}
	if err := c.policy.CheckRequest(method, p, body); err != nil {
		return nil, err
	}
	return body, nil
}

// restoreRedacted puts back the stored values behind the placeholders a PUT
// carries, so a script can read an object, change some fields and write it
// back (event_sink.update(id, active=False), say) without wiping the values
// it was shown redacted. It reads the object at the same path, unredacted,
// and redacts a copy as a script would see it: each string holding the
// placeholder must be exactly what the script was shown at the same place
// in that object, and is replaced by the stored value there. Anything else
// holding the placeholder is refused. So is a request that restores a value
// and also changes a URL (url or *_url, at any depth), since the restored
// credentials would then go to the new address.
func (c *aiScriptClient) restoreRedacted(ctx context.Context, clean string, body interface{}) (interface{}, error) {
	if route, denied := aiScriptDeniedRoute(http.MethodGet, clean); denied {
		return nil, fmt.Errorf("PUT %s refused: the request carries the %q placeholder, and the stored values it stands for cannot be read (%s)", clean, redactedValue, route.reason)
	}
	var stored interface{}
	code, err := c.inner.GetJSON(ctx, clean, &stored)
	if err != nil || code >= http.StatusMultipleChoices {
		if err == nil {
			err = fmt.Errorf("HTTP %d", code)
		}
		return nil, fmt.Errorf("PUT %s refused: the request carries the %q placeholder, and the stored object could not be read to restore the values it stands for: %v", clean, redactedValue, err)
	}
	shown := redactValue(copyJSON(stored))
	r := &redactionRestorer{}
	restored, err := r.walk("", "", body, shown, stored)
	if err != nil {
		return nil, fmt.Errorf("PUT %s refused: %v. Write back a field exactly as it was read, set a new value, or ask the user to change it in the knot web UI", clean, err)
	}
	if r.restored > 0 && len(r.urlChanged) > 0 {
		return nil, fmt.Errorf("PUT %s refused: the request keeps redacted values while changing %s, which would send the stored credentials to the new address. Send the new URL with new values for the redacted fields (such as the webhook headers), or ask the user to change it in the knot web UI", clean, strings.Join(r.urlChanged, ", "))
	}
	return restored, nil
}

// redactionRestorer walks a request body beside the stored object (shown:
// as the script saw it; stored: as it is).
type redactionRestorer struct {
	restored   int
	urlChanged []string
}

func (r *redactionRestorer) walk(where, key string, body, shown, stored interface{}) (interface{}, error) {
	switch b := body.(type) {
	case string:
		lower := strings.ToLower(key)
		if lower == "url" || strings.HasSuffix(lower, "_url") {
			if s, ok := shown.(string); !ok || s != b {
				r.urlChanged = append(r.urlChanged, strings.TrimPrefix(where, "."))
			}
		}
		if !stringHasRedacted(b) {
			return b, nil
		}
		if s, ok := shown.(string); ok && s == b {
			r.restored++
			return stored, nil
		}
		return nil, fmt.Errorf("%s holds the %q placeholder but is not the value read from there", strings.TrimPrefix(where, "."), redactedValue)
	case map[string]interface{}:
		sm, _ := shown.(map[string]interface{})
		rm, _ := stored.(map[string]interface{})
		for k, item := range b {
			v, err := r.walk(where+"."+k, k, item, sm[k], rm[k])
			if err != nil {
				return nil, err
			}
			b[k] = v
		}
	case []interface{}:
		sl, _ := shown.([]interface{})
		rl, _ := stored.([]interface{})
		for i, item := range b {
			var sv, rv interface{}
			if i < len(sl) && i < len(rl) {
				sv, rv = sl[i], rl[i]
			}
			v, err := r.walk(fmt.Sprintf("%s[%d]", where, i), key, item, sv, rv)
			if err != nil {
				return nil, err
			}
			b[i] = v
		}
	}
	return body, nil
}

// copyJSON deep-copies a decoded JSON value.
func copyJSON(v interface{}) interface{} {
	switch value := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(value))
		for k, item := range value {
			out[k] = copyJSON(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(value))
		for i, item := range value {
			out[i] = copyJSON(item)
		}
		return out
	}
	return v
}

func (c *aiScriptClient) Post(ctx context.Context, p string, request, response interface{}, successCode int) (int, error) {
	body, err := c.prepareWrite(ctx, http.MethodPost, p, request)
	if err != nil {
		return 0, err
	}
	code, err := c.inner.Post(ctx, p, body, response, successCode)
	redactSecrets(response)
	return code, err
}

func (c *aiScriptClient) PostJSON(ctx context.Context, p string, request, response interface{}, successCode int) (int, error) {
	body, err := c.prepareWrite(ctx, http.MethodPost, p, request)
	if err != nil {
		return 0, err
	}
	code, err := c.inner.PostJSON(ctx, p, body, response, successCode)
	redactSecrets(response)
	return code, err
}

func (c *aiScriptClient) Put(ctx context.Context, p string, request, response interface{}, successCode int) (int, error) {
	body, err := c.prepareWrite(ctx, http.MethodPut, p, request)
	if err != nil {
		return 0, err
	}
	code, err := c.inner.Put(ctx, p, body, response, successCode)
	redactSecrets(response)
	return code, err
}

func (c *aiScriptClient) PutJSON(ctx context.Context, p string, request, response interface{}, successCode int) (int, error) {
	body, err := c.prepareWrite(ctx, http.MethodPut, p, request)
	if err != nil {
		return 0, err
	}
	code, err := c.inner.PutJSON(ctx, p, body, response, successCode)
	redactSecrets(response)
	return code, err
}

func (c *aiScriptClient) Delete(ctx context.Context, p string, request, response interface{}, successCode int) (int, error) {
	body, err := c.prepareWrite(ctx, http.MethodDelete, p, request)
	if err != nil {
		return 0, err
	}
	code, err := c.inner.Delete(ctx, p, body, response, successCode)
	redactSecrets(response)
	return code, err
}

// DoRaw carries the byte transfers (get_bytes, put_bytes, delete with
// headers); it is refused when the client underneath cannot do them.
func (c *aiScriptClient) DoRaw(ctx context.Context, method string, p string, body io.Reader, size int64, headers map[string]string) (*http.Response, error) {
	raw, ok := c.inner.(rest.RawClient)
	if !ok {
		return nil, fmt.Errorf("raw transfers are not available here")
	}
	if err := c.policy.Check(method, p); err != nil {
		return nil, err
	}
	// Raw transfers carry file storage objects (knot.files); anything else
	// would come back unredacted, so it goes through the JSON calls.
	if clean, _ := aiScriptPath(p); !strings.HasPrefix(clean, "/api/files/") {
		return nil, fmt.Errorf("%s %s refused: byte transfers are only for file storage (/api/files/...); use apiclient.get and friends", method, p)
	}
	if size > AIScriptMaxValueBytes {
		return nil, fmt.Errorf("%s %s refused: an AI-written script may send at most %d MB at a time", method, p, AIScriptMaxValueBytes>>20)
	}
	// The in-process client refuses a response over the run's limit
	// (rest.WithResponseLimit) before it is buffered whole.
	return raw.DoRaw(ctx, method, p, body, size, headers)
}

// The configuration methods change nothing a script can reach: the client
// is fixed to the user it was made for.
func (c *aiScriptClient) SetTimeout(time.Duration) rest.RESTClient                    { return c }
func (c *aiScriptClient) SetContentType(string) rest.RESTClient                       { return c }
func (c *aiScriptClient) SetAccept(...string) rest.RESTClient                         { return c }
func (c *aiScriptClient) SetUserAgent(string) rest.RESTClient                         { return c }
func (c *aiScriptClient) AppendUserAgent(string) rest.RESTClient                      { return c }
func (c *aiScriptClient) SetResponseCheck(func(*http.Response) error) rest.RESTClient { return c }
func (c *aiScriptClient) SetAuthToken(string) rest.RESTClient                         { return c }
func (c *aiScriptClient) SetTokenKey(string) rest.RESTClient                          { return c }
func (c *aiScriptClient) SetTokenFormat(string) rest.RESTClient                       { return c }
func (c *aiScriptClient) SetHeader(string, string) rest.RESTClient                    { return c }
func (c *aiScriptClient) DeleteHeader(string) rest.RESTClient                         { return c }
func (c *aiScriptClient) ClearHeaders() rest.RESTClient                               { return c }
func (c *aiScriptClient) GetBaseURL() string                                          { return c.inner.GetBaseURL() }
func (c *aiScriptClient) GetAuthToken() string                                        { return "" }
func (c *aiScriptClient) SetBaseUrl(string) (rest.RESTClient, error)                  { return c, nil }

// redactedFields are response fields whose values never reach a script:
// credentials the API returns to their owner (the service password, the SSH
// private key, the TOTP secret) and tokens other responses may carry. Beyond
// these, redactValue blanks the values (not the names) of every "headers"
// object, such as an event sink's webhook headers, which carry
// Authorization and API keys; the user name and password of any URL (a
// field named url or ending _url); and, of a webhook's URL, the query
// string and every path segment after the first, where services put their
// keys (Slack's /services/T/B/<token>, Discord's /api/webhooks/<id>/<token>).
// A script writing an object back gets the stored values put back for it
// (aiScriptClient.restoreRedacted).
var redactedFields = map[string]bool{
	"service_password":    true,
	"ssh_private_key":     true,
	"totp_secret":         true,
	"password":            true,
	"token":               true,
	"access_token":        true,
	"refresh_token":       true,
	"oauth_access_token":  true,
	"oauth_refresh_token": true,
	"client_secret":       true,
	"secret":              true,
	"private_key":         true,
}

// redactedValue replaces a redacted field's value.
const redactedValue = "[redacted]"

// redactSecrets blanks the redacted fields, at any depth, of a response
// decoded into generic maps and lists (what knot.apiclient decodes into).
func redactSecrets(response interface{}) {
	if ptr, ok := response.(*interface{}); ok && ptr != nil {
		*ptr = redactValue(*ptr)
		return
	}
	if m, ok := response.(*map[string]interface{}); ok && m != nil {
		redactValue(*m)
	}
}

func redactValue(v interface{}) interface{} {
	switch value := v.(type) {
	case map[string]interface{}:
		for k, item := range value {
			lower := strings.ToLower(k)
			if redactedFields[lower] {
				if s, isString := item.(string); !isString || s != "" {
					value[k] = redactedValue
				}
				continue
			}
			if lower == "headers" {
				if headers, ok := item.(map[string]interface{}); ok {
					for name := range headers {
						headers[name] = redactedValue
					}
					continue
				}
			}
			if lower == "webhook" {
				if webhook, ok := item.(map[string]interface{}); ok {
					if u, ok := webhook["url"].(string); ok {
						webhook["url"] = redactURL(u, true)
					}
				}
			}
			if s, ok := item.(string); ok && (lower == "url" || strings.HasSuffix(lower, "_url")) {
				value[k] = redactURL(s, false)
				continue
			}
			value[k] = redactValue(item)
		}
	case []interface{}:
		for i, item := range value {
			value[i] = redactValue(item)
		}
	}
	return v
}

// redactURL blanks the user name and password of a URL and, for a webhook,
// the values of its query string and its path after the first segment
// (where services put a webhook's key), keeping the scheme and host so a
// script can still tell sinks apart. Anything that does not parse as a URL
// with a host is left alone.
func redactURL(raw string, webhook bool) string {
	if raw == "" || raw == redactedValue {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if err != nil && strings.Contains(raw, "@") {
			return redactedValue
		}
		return raw
	}
	changed := false
	if u.User != nil {
		u.User = url.User(redactedValue)
		changed = true
	}
	if webhook && u.RawQuery != "" {
		values := u.Query()
		for name := range values {
			values[name] = []string{redactedValue}
		}
		u.RawQuery = values.Encode()
		changed = true
	}
	if webhook {
		tail := strings.TrimPrefix(u.Path, "/")
		if i := strings.IndexByte(tail, '/'); i >= 0 && strings.Trim(tail[i:], "/") != "" && tail[i+1:] != redactedValue {
			u.Path = "/" + tail[:i+1] + redactedValue
			u.RawPath = ""
			changed = true
		}
	}
	if u.Fragment != "" && webhook {
		u.Fragment = ""
		changed = true
	}
	if !changed {
		return raw
	}
	return u.String()
}
