package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/util/rest"
)

// TestUpdateTemplateVarRequestOptionalValue pins that an update which
// leaves out "value" decodes with no value, which keeps the stored one:
// reads blank a protected variable's value, so a read-modify-write client
// must be able to omit it. An explicit "" still clears the value.
func TestUpdateTemplateVarRequestOptionalValue(t *testing.T) {
	decode := func(body string) updateTemplateVarRequest {
		t.Helper()
		r := httptest.NewRequest("PUT", "/api/templatevars/x", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var req updateTemplateVarRequest
		if err := rest.DecodeRequestBody(httptest.NewRecorder(), r, &req); err != nil {
			t.Fatal(err)
		}
		return req
	}
	if req := decode(`{"name":"x","protected":true}`); req.Value != nil {
		t.Fatalf("omitted value decoded as %q", *req.Value)
	}
	if req := decode(`{"name":"x","value":""}`); req.Value == nil || *req.Value != "" {
		t.Fatalf("explicit empty value = %v", req.Value)
	}
	if req := decode(`{"name":"x","value":"v"}`); req.Value == nil || *req.Value != "v" {
		t.Fatalf("value = %v", req.Value)
	}
}
