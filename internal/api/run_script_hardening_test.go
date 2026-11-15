package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// An update that leaves out the value keeps the stored one only while the
// variable stays protected, or when it was never protected; un-protecting
// without a new value clears it, so the next read cannot show what
// protection hid.
func TestApplyTemplateVarValue(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name          string
		wasProtected  bool
		value         *string
		protected     bool
		wantValue     string
		wantProtected bool
	}{
		{"stays protected, no value", true, nil, true, "stored", true},
		{"un-protected, no value", true, nil, false, "", false},
		{"un-protected with a value", true, str("new"), false, "new", false},
		{"stays protected with a value", true, str("new"), true, "new", true},
		{"unprotected, no value", false, nil, false, "stored", false},
		{"protected now, no value", false, nil, true, "stored", true},
		{"explicit empty value", true, str(""), true, "", true},
	}
	for _, c := range cases {
		tv := &model.TemplateVar{Value: "stored", Protected: c.wasProtected}
		applyTemplateVarValue(tv, &updateTemplateVarRequest{Name: "x", Value: c.value, Protected: c.protected})
		if tv.Value != c.wantValue || tv.Protected != c.wantProtected {
			t.Errorf("%s: value %q protected %v, want %q %v", c.name, tv.Value, tv.Protected, c.wantValue, c.wantProtected)
		}
	}
}

// A user MCP server must have a namespace, on create as on update: without
// one its tool names would be knot's own.
func TestValidateMCPServerNamespace(t *testing.T) {
	if err := validateMCPServerNamespace(""); err == nil {
		t.Error("empty namespace accepted")
	}
	for _, bad := range []string{"   ", " gh", "a__b"} {
		if err := validateMCPServerNamespace(bad); err == nil {
			t.Errorf("namespace %q accepted", bad)
		}
	}
	if err := validateMCPServerNamespace("github"); err != nil {
		t.Errorf("valid namespace refused: %v", err)
	}

	// Create stops at a missing namespace (it used to write the error and
	// carry on).
	r := httptest.NewRequest("POST", "/api/mcp-servers", strings.NewReader(`{"namespace":"","url":"https://example.com/mcp"}`))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), "user", &model.User{Id: "u1"}))
	w := httptest.NewRecorder()
	HandleCreateMCPServer(w, r)
	if w.Code != http.StatusBadRequest || strings.Count(w.Body.String(), "error") != 1 {
		t.Errorf("create without a namespace: %d %s", w.Code, w.Body.String())
	}
}

// Method timeouts are capped, and a batch's total wait with them.
func TestMethodTimeouts(t *testing.T) {
	for in, want := range map[int]int{0: 30, -5: 30, 10: 10, 300: 300, 100000: maxMethodTimeoutSeconds} {
		if got := methodTimeout(in); got != want {
			t.Errorf("methodTimeout(%d) = %d, want %d", in, got, want)
		}
	}
	for _, c := range []struct{ timeout, items int }{{30, 1}, {30, 100}, {100000, 100}, {300, 2}, {1, 100}, {0, 0}} {
		per := batchTimeout(c.timeout, c.items)
		items := c.items
		if items < 1 {
			items = 1
		}
		limit := maxBatchTimeoutSeconds
		if mt := methodTimeout(c.timeout); mt > limit {
			limit = mt
		}
		if per < 1 || per > methodTimeout(c.timeout) || per*items > limit+items {
			t.Errorf("batchTimeout(%d, %d) = %d: total %d over %d", c.timeout, c.items, per, per*items, limit)
		}
	}
	if got := batchTimeout(30, 2); got != 30 {
		t.Errorf("a small batch keeps the method's timeout: got %d", got)
	}
}

// The handlers that wait on a space's agent stop waiting when the request
// ends (a script at its time limit): every wait goes through awaitAgent.
func TestAgentWaitsHonourTheRequest(t *testing.T) {
	bareReceive := regexp.MustCompile(`\n\s*\w+ := <-(ch|responseChannel|respChan)\b`)
	for _, file := range []string{"space_file_ops.go", "space_files.go", "methods.go", "space_run_command.go", "scripts_execution.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if m := bareReceive.Find(src); m != nil {
			t.Errorf("%s waits on an agent without the request's context: %s", file, m)
		}
	}
	src, _ := os.ReadFile("methods.go")
	if strings.Contains(string(src), "SendCallMethod(callReq, entry.Server.Timeout)") || !strings.Contains(string(src), "awaitAgent(r.Context(), done)") {
		t.Error("a single method call does not honour the request")
	}
}
