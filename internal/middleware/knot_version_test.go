package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/build"
)

func TestKnotVersion(t *testing.T) {
	handler := KnotVersion(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mm := build.MajorMinor(build.Version)
	for _, tc := range []struct {
		userAgent string
		want      int
	}{
		{"knot v" + mm + ".9", http.StatusOK},
		{"knot v" + build.Version + " (agent)", http.StatusOK},
		{"knot v0.1.0", http.StatusBadRequest},
		{"knot v99.0.0", http.StatusBadRequest},
		{"Mozilla/5.0", http.StatusOK},
		{"knot-mux-client", http.StatusOK},
		{"", http.StatusOK},
	} {
		r := httptest.NewRequest("GET", "/api/ping", nil)
		r.Header.Set("User-Agent", tc.userAgent)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("User-Agent %q: status %d, want %d", tc.userAgent, w.Code, tc.want)
		}
		if w.Header().Get(build.VersionHeader) != build.Version {
			t.Errorf("User-Agent %q: version header %q", tc.userAgent, w.Header().Get(build.VersionHeader))
		}
		if tc.want == http.StatusBadRequest && !strings.Contains(w.Body.String(), "use a knot command from version "+mm) {
			t.Errorf("User-Agent %q: body %s", tc.userAgent, w.Body.String())
		}
	}
}
