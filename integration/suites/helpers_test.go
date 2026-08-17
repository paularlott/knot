//go:build integration

package suites

import (
	"context"

	"github.com/paularlott/knot/integration/harness"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"testing"
	"time"
)

// testCtx returns a context with a generous per-call timeout.
func testCtx(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

// uniqueName generates a collision-free lowercase name for API objects.
func uniqueName(prefix string) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 6)
	for i := range suffix {
		suffix[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s-%s", prefix, string(suffix))
}

// rawGet performs a GET with a bearer token, for endpoints without an
// apiclient wrapper.
func rawGet(url, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// readBody drains and returns a response body.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

// uniqueVarName generates a name valid for VarName-validated resources
// (letters, digits, underscore; no hyphens).
func uniqueVarName(prefix string) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 6)
	for i := range suffix {
		suffix[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s_%s", prefix, string(suffix))
}

func mustEqual[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func mustContain(t *testing.T, what, haystack, needle string) {
	t.Helper()
	if !contains(haystack, needle) {
		t.Fatalf("%s did not contain %q: %s", what, needle, haystack)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

// statusOf returns the HTTP status for an authenticated GET — for negative
// assertions, because several apiclient Get wrappers do not surface
// non-200 statuses as errors (they decode the error body into an empty
// struct and return nil).
func statusOf(u *harness.User, path string) int {
	resp, err := rawGet(server.BaseURL+path, u.Token)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// waitFor polls cond every ~2s until it returns true or the timeout elapses.
func waitFor(t *testing.T, seconds int, cond func() bool) {
	t.Helper()
	waitForCond(seconds, cond)
}

// waitForCond is waitFor without the testing.T.
func waitForCond(seconds int, cond func() bool) bool {
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return cond()
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
