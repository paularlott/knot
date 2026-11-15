package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/toolsurface"
)

// Tools reached through /api/chat/tools and the OpenAI compatible endpoints
// have no one to ask, so no tool-call surface survives into them.
func TestMCPServerContextStripsSurface(t *testing.T) {
	var seen toolsurface.Surface = "unset"
	h := MCPServerContext(nil, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = toolsurface.From(r.Context())
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/chat/tools/call", nil)
	r = r.WithContext(toolsurface.With(r.Context(), toolsurface.WebChat))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "" {
		t.Errorf("surface %q reached the handler", seen)
	}
}
