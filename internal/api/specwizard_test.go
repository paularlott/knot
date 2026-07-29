package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// TestHandleGetCapabilities checks the shape the wizard's capability picker
// consumes: a capabilities array of {name, description} with canonical names.
func TestHandleGetCapabilities(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/capabilities", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	HandleGetCapabilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp apiclient.CapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
	}
	if len(resp.Capabilities) == 0 {
		t.Fatal("no capabilities returned")
	}
	found := false
	for _, c := range resp.Capabilities {
		if !strings.HasPrefix(c.Name, "CAP_") {
			t.Errorf("capability %q is not in canonical form", c.Name)
		}
		if strings.TrimSpace(c.Description) == "" {
			t.Errorf("capability %q has no description", c.Name)
		}
		if c.Name == "CAP_NET_ADMIN" {
			found = true
		}
	}
	if !found {
		t.Error("CAP_NET_ADMIN missing from the catalog response")
	}
}
