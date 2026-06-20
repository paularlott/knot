package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/methods"
)

// --------------------------------------------------------------------
// jsonHasField — exhaustive
// --------------------------------------------------------------------

func TestJsonHasField(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		field  string
		expect bool
	}{
		{"present string id", `{"id":"abc","method":"x"}`, "id", true},
		{"present int id", `{"id":1,"method":"x"}`, "id", true},
		{"present null id", `{"id":null,"method":"x"}`, "id", true},
		{"absent id (notification)", `{"method":"x","params":{}}`, "id", false},
		{"empty object", `{}`, "id", false},
		{"different field present", `{"jsonrpc":"2.0","method":"x"}`, "method", true},
		{"different field absent", `{"jsonrpc":"2.0"}`, "method", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonHasField(json.RawMessage(tt.raw), tt.field)
			if got != tt.expect {
				t.Errorf("jsonHasField(%q, %q) = %v, want %v", tt.raw, tt.field, got, tt.expect)
			}
		})
	}
}

// --------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------

func setupTestRegistry(t *testing.T) (*model.User, *model.User, func()) {
	t.Helper()
	registry := methods.DefaultRegistry()

	owner := &model.User{Id: "owner-1", Username: "paul"}
	caller := &model.User{Id: "caller-1", Username: "alice"}

	role := &model.Role{Id: "role-methods", Permissions: []uint16{model.PermissionUseMethods}}
	model.SetRoleCache([]*model.Role{role})
	caller.Roles = []string{"role-methods"}

	space := &model.Space{Id: "space-1", Name: "test", UserId: owner.Id}
	reg := &methods.Registration{
		Server: methods.ServerConfig{Type: methods.ServerTypeStdio, Command: "./test", Timeout: 5},
		Methods: []methods.MethodDefinition{
			{Name: "search", LocalName: "search", Description: "Search", Scope: methods.ScopeShared},
			{Name: "admin", LocalName: "admin", Description: "Admin only", Scope: methods.ScopePrivate},
		},
	}
	if err := registry.Register(space, owner, reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	return owner, caller, func() { registry.UnregisterSpace("space-1") }
}

// postToHandler runs HandleCallMethod with the given body and user,
// returning the httptest.ResponseRecorder for assertions.
func postToHandler(t *testing.T, user *model.User, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/methods/call", bytes.NewReader([]byte(body)))
	req = req.WithContext(context.WithValue(req.Context(), "user", user))
	rr := httptest.NewRecorder()
	HandleCallMethod(rr, req)
	return rr
}

func decodeResponses(t *testing.T, rr *httptest.ResponseRecorder) []methods.JSONRPCResponse {
	t.Helper()
	var responses []methods.JSONRPCResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &responses); err != nil {
		t.Fatalf("unmarshal response array: %v (body: %s)", err, rr.Body.String())
	}
	return responses
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder) methods.JSONRPCResponse {
	t.Helper()
	var resp methods.JSONRPCResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (body: %s)", err, rr.Body.String())
	}
	return resp
}

// --------------------------------------------------------------------
// Single request tests
// --------------------------------------------------------------------

func TestSingleRequest_MethodNotFound(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `{"jsonrpc":"2.0","method":"nope","id":1}`)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNotFound)
	}
	resp := decodeResponse(t, rr)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Errorf("expected -32601, got %+v", resp.Error)
	}
}

func TestSingleRequest_ValidMethodNoServer(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	rr := postToHandler(t, caller, `{"jsonrpc":"2.0","method":"user.paul.search","params":{},"id":1}`)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
	resp := decodeResponse(t, rr)
	if resp.Error == nil {
		t.Error("expected error response")
	}
}

func TestSingleRequest_OwnerPrivateMethodNoServer(t *testing.T) {
	owner, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	rr := postToHandler(t, owner, `{"jsonrpc":"2.0","method":"admin","id":1}`)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d (method found, no server)", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestSingleRequest_PermissionDenied(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Caller tries to use the owner's private method by canonical name.
	rr := postToHandler(t, caller, `{"jsonrpc":"2.0","method":"admin","id":1}`)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestSingleRequest_ParseError(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `not valid json`)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSingleRequest_MissingMethod(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `{"jsonrpc":"2.0","id":1}`)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, rr)
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Errorf("expected -32600, got %+v", resp.Error)
	}
}

// --------------------------------------------------------------------
// Single notification tests
// --------------------------------------------------------------------

func TestSingleNotification_ValidMethodNoServer(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Notification (no id) with a valid method but no server. The single
	// handler checks session==nil before the notification branch → 503.
	rr := postToHandler(t, caller, `{"jsonrpc":"2.0","method":"user.paul.search","params":{}}`)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestSingleNotification_MethodNotFound(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `{"jsonrpc":"2.0","method":"nope","params":{}}`)

	// Routing fails before the notification check fires.
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// --------------------------------------------------------------------
// Batch request tests
// --------------------------------------------------------------------

func TestBatch_AllNotFound(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `[
		{"jsonrpc":"2.0","method":"a","id":1},
		{"jsonrpc":"2.0","method":"b","id":2}
	]`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
	responses := decodeResponses(t, rr)
	if len(responses) != 2 {
		t.Fatalf("expected 2, got %d", len(responses))
	}
	for _, r := range responses {
		if r.Error == nil || r.Error.Code != -32601 {
			t.Errorf("expected -32601, got %+v", r.Error)
		}
	}
}

func TestBatch_MixedErrors(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	rr := postToHandler(t, caller, `[
		{"jsonrpc":"2.0","method":"user.paul.search","params":{},"id":1},
		{"jsonrpc":"2.0","method":"unknown","id":2},
		{"jsonrpc":"2.0","method":"user.paul.admin","id":3}
	]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 3 {
		t.Fatalf("expected 3, got %d", len(responses))
	}

	// Item 1: valid shared method, no server → -32000
	if responses[0].Error == nil || responses[0].Error.Code != -32000 {
		t.Errorf("item 1: expected -32000, got %+v", responses[0].Error)
	}
	// Item 2: unknown method → -32601
	if responses[1].Error == nil || responses[1].Error.Code != -32601 {
		t.Errorf("item 2: expected -32601, got %+v", responses[1].Error)
	}
	// Item 3: private method, caller not owner → -32601 (permission)
	if responses[2].Error == nil || responses[2].Error.Code != -32601 {
		t.Errorf("item 3: expected -32601, got %+v", responses[2].Error)
	}
}

func TestBatch_ResponseOrdering(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `[
		{"jsonrpc":"2.0","method":"alpha","id":10},
		{"jsonrpc":"2.0","method":"beta","id":20},
		{"jsonrpc":"2.0","method":"gamma","id":30}
	]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 3 {
		t.Fatalf("expected 3, got %d", len(responses))
	}
	expectedIDs := []float64{10, 20, 30}
	for i, want := range expectedIDs {
		got, ok := responses[i].ID.(float64)
		if !ok {
			t.Errorf("response %d: id type %T, want float64", i, responses[i].ID)
		} else if got != want {
			t.Errorf("response %d: id %v, want %v", i, got, want)
		}
	}
}

func TestBatch_IdNullIsRequest(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `[{"jsonrpc":"2.0","method":"x","id":null}]`)

	// id:null is a request, not a notification → must produce a response.
	if rr.Code == http.StatusNoContent {
		t.Fatal("expected a response for id:null, got 204")
	}
	responses := decodeResponses(t, rr)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
}

func TestBatch_ParseErrorItem(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `[
		{"jsonrpc":"2.0","method":"x","id":1},
		"garbage",
		{"jsonrpc":"2.0","method":"y","id":3}
	]`)

	responses := decodeResponses(t, rr)
	// All 3 produce entries (parse error for garbage, routing errors for x and y).
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}
	// The garbage item should be a parse error.
	// Responses are ordered; items 0 and 2 are routing errors, item 1 is parse error.
	if responses[1].Error == nil || responses[1].Error.Code != -32700 {
		t.Errorf("item 1: expected parse error -32700, got %+v", responses[1].Error)
	}
}

func TestBatch_MissingMethodField(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `[{"jsonrpc":"2.0","id":1}]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 1 {
		t.Fatalf("expected 1, got %d", len(responses))
	}
	if responses[0].Error == nil || responses[0].Error.Code != -32600 {
		t.Errorf("expected -32600 (method required), got %+v", responses[0].Error)
	}
}

func TestBatch_TooLarge(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}

	// Build a batch with maxBatchSize+1 items.
	var items []byte
	items = append(items, '[')
	for i := 0; i <= maxBatchSize; i++ {
		if i > 0 {
			items = append(items, ',')
		}
		items = append(items, `{"jsonrpc":"2.0","method":"x","id":0}`...)
	}
	items = append(items, ']')

	rr := postToHandler(t, user, string(items))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusBadRequest)
	}
	resp := decodeResponse(t, rr)
	if resp.Error == nil || resp.Error.Code != -32600 {
		t.Errorf("expected -32600 (batch too large), got %+v", resp.Error)
	}
}

func TestBatch_ParseErrorCorrectCode(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	rr := postToHandler(t, user, `["garbage"]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 1 {
		t.Fatalf("expected 1, got %d", len(responses))
	}
	if responses[0].Error == nil || responses[0].Error.Code != -32700 {
		t.Errorf("expected -32700 (parse error), got %+v", responses[0].Error)
	}
}

// --------------------------------------------------------------------
// Batch notification tests
// --------------------------------------------------------------------

func TestBatch_NotificationExcludedFromResponses(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Item 1: notification targeting valid method but no server → skipped.
	// Item 2: unknown method request → error response.
	// Net: 1 response.
	rr := postToHandler(t, caller, `[
		{"jsonrpc":"2.0","method":"user.paul.search","params":{}},
		{"jsonrpc":"2.0","method":"unknown.x","id":1}
	]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response (notification excluded), got %d", len(responses))
	}
}

func TestBatch_AllNotificationsNoServer(t *testing.T) {
	_, caller, cleanup := setupTestRegistry(t)
	defer cleanup()

	// All notifications targeting a valid method with no server.
	// Notifications with session==nil are skipped → no responses → 204.
	rr := postToHandler(t, caller, `[
		{"jsonrpc":"2.0","method":"user.paul.search","params":{}},
		{"jsonrpc":"2.0","method":"user.paul.search","params":{"q":"b"}}
	]`)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestBatch_AllNotificationsMethodNotFound(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	// Notifications targeting unknown methods → routing fails, skipped → 204.
	rr := postToHandler(t, user, `[
		{"jsonrpc":"2.0","method":"nope1","params":{}},
		{"jsonrpc":"2.0","method":"nope2","params":{}}
	]`)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestBatch_NotificationWithUnknownMethod(t *testing.T) {
	_, _, cleanup := setupTestRegistry(t)
	defer cleanup()

	user := &model.User{Id: "u1", Username: "x"}
	// Mix: unknown method notification + unknown method request.
	// Notification produces no entry; request produces error entry.
	rr := postToHandler(t, user, `[
		{"jsonrpc":"2.0","method":"nope","params":{}},
		{"jsonrpc":"2.0","method":"also.nope","id":1}
	]`)

	responses := decodeResponses(t, rr)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
}
