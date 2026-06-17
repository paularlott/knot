package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling/conversion"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

// mockAPIServer returns a test HTTP server that serves canned JSON responses
// for the paths it knows about. Each handler is a simple function over
// (method, path) -> response body.
func mockAPIServer(t *testing.T, handlers map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		h, ok := handlers[key]
		if !ok {
			// Library loader and other internal lookups may hit unhandled paths —
			// return 404 silently rather than logging on every request.
			http.NotFound(w, r)
			return
		}
		// All knot API responses require a valid bearer token
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
}

// capturedRequest holds the body and URL of the most recent request received
// by a capturingHandler.
type capturedRequest struct {
	Body       map[string]interface{}
	BodyString string
	URL        string
	Headers    http.Header
}

// capturingHandler returns a handler that writes the given response and
// captures the incoming request into *capturedRequest for later assertion.
func capturingHandler(response interface{}) (http.HandlerFunc, *capturedRequest) {
	cap := &capturedRequest{Headers: http.Header{}}
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.BodyString = string(body)
		_ = json.Unmarshal(body, &cap.Body)
		cap.URL = r.URL.String()
		cap.Headers = r.Header.Clone()
		if response != nil {
			json.NewEncoder(w).Encode(response)
		}
	}, cap
}

// runTool executes a loaded MCP tool's script against a mock API server.
// Returns the raw response string from scriptlingmcp.RunToolScript.
func runTool(t *testing.T, toolName string, serverURL string, params map[string]interface{}) (string, error) {
	t.Helper()

	// Build a real HTTP client pointed at the mock server (bypasses MuxClient)
	client, err := apiclient.NewClient(serverURL, "test-token", false)
	if err != nil {
		t.Fatalf("apiclient.NewClient failed: %v", err)
	}

	user := &model.User{Id: "u1", Username: "tester", Email: "tester@example.com"}

	// Convert params to scriptling objects
	mcpParams := make(map[string]object.Object, len(params))
	for k, v := range params {
		mcpParams[k] = conversion.FromGo(v)
	}

	// Create the scriptling env exactly as the MCP server would
	env, _, err := service.NewMCPScriptlingEnv(client, mcpParams, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv failed: %v", err)
	}

	// Load tool script
	tool, ok := GetTool(toolName)
	if !ok {
		t.Fatalf("tool %q not loaded", toolName)
	}

	// Execute via scriptling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, _, err := scriptlingmcp.RunToolScript(ctx, env, tool.Script, mcpParams)
	return response, err
}

// mustLoadTools loads the embedded tools once.
func mustLoadTools(t *testing.T) {
	t.Helper()
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools failed: %v", err)
	}
}

// decodeJSON decodes a JSON string into a map for ad-hoc field access in tests.
func decodeJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(s)).Decode(&m); err != nil {
		t.Fatalf("failed to parse response %q as JSON: %v", s, err)
	}
	return m
}

// TestExecute_ListTemplates proves the test harness end-to-end:
//   - mock /api/templates returns 2 templates (one active, one inactive)
//   - the tool filters to active-only (per the recent change to knot.template.list)
//   - the tool returns the active template with its custom fields
func TestExecute_ListTemplates(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools failed: %v", err)
	}

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/templates": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"templates": []map[string]interface{}{
					{
						"template_id": "tpl-active",
						"name":        "ubuntu",
						"description": "Ubuntu dev environment",
						"active":      true,
						"platform":    "linux/amd64",
						"custom_fields": []map[string]string{
							{"name": "VERSION", "description": "Ubuntu version"},
						},
					},
					{
						"template_id": "tpl-inactive",
						"name":        "legacy",
						"description": "Retired template",
						"active":      false,
					},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "list_templates", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}

	// Response should be JSON: {"templates": [...]}
	var result struct {
		Templates []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Active       bool   `json:"active"`
			CustomFields []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"custom_fields"`
		} `json:"templates"`
	}
	if err := json.NewDecoder(strings.NewReader(response)).Decode(&result); err != nil {
		t.Fatalf("failed to parse response %q as JSON: %v", response, err)
	}

	// Should return exactly 1 template (the active one)
	if len(result.Templates) != 1 {
		t.Fatalf("expected 1 active template, got %d: %+v", len(result.Templates), result.Templates)
	}
	tmpl := result.Templates[0]
	if tmpl.Name != "ubuntu" {
		t.Errorf("expected template name %q, got %q", "ubuntu", tmpl.Name)
	}
	if !tmpl.Active {
		t.Errorf("expected template to be active")
	}
	if len(tmpl.CustomFields) != 1 {
		t.Fatalf("expected 1 custom field, got %d", len(tmpl.CustomFields))
	}
	if tmpl.CustomFields[0].Name != "VERSION" {
		t.Errorf("expected custom field name %q, got %q", "VERSION", tmpl.CustomFields[0].Name)
	}

	t.Logf("response: %s", response)
}

// TestExecute_GetSpace proves required-parameter injection works end-to-end:
//   - the "name" parameter is passed through mcpParams
//   - tool.get_string("name") inside the .py picks it up
//   - knot.space.get() calls /api/spaces/{name}
//   - the response is parsed and returned as JSON
func TestExecute_GetSpace(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools failed: %v", err)
	}

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/spaces/my-space": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"space_id":      "space-1",
				"name":          "my-space",
				"description":   "test space",
				"template_id":   "tpl-1",
				"template_name": "ubuntu",
				"is_deployed":   true,
				"shell":         "bash",
				"username":      "tester",
				"custom_fields": []map[string]string{
					{"name": "ENV", "value": "dev"},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "get_space", server.URL, map[string]interface{}{
		"name": "my-space",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}

	var result struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		TemplateID   string `json:"template_id"`
		TemplateName string `json:"template_name"`
		IsRunning    bool   `json:"is_running"`
		Shell        string `json:"shell"`
		Username     string `json:"username"`
	}
	if err := json.NewDecoder(strings.NewReader(response)).Decode(&result); err != nil {
		t.Fatalf("failed to parse response %q as JSON: %v", response, err)
	}

	if result.Name != "my-space" {
		t.Errorf("expected name %q, got %q", "my-space", result.Name)
	}
	if result.ID != "space-1" {
		t.Errorf("expected id %q, got %q", "space-1", result.ID)
	}
	if !result.IsRunning {
		t.Errorf("expected is_running=true (mapped from is_deployed)")
	}
	if result.TemplateName != "ubuntu" {
		t.Errorf("expected template_name %q, got %q", "ubuntu", result.TemplateName)
	}

	t.Logf("response: %s", response)
}

func TestExecute_CreateSpace_NameWithSpace(t *testing.T) {
	mustLoadTools(t)

	// Regression: template_name="ubuntu apple" used to panic the HTTP client
	// because the space broke URL parsing. Now the lib URL-encodes path segments.
	createHandler, createReq := capturingHandler(map[string]interface{}{"space_id": "new-space"})
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		// Note: Go's r.URL.Path is the decoded form, so the handler key uses
		// the literal space even though the wire URL is "ubuntu%20apple".
		"GET /api/templates/ubuntu apple": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"template_id": "tpl-1",
				"name":        "ubuntu apple",
			})
		},
		"POST /api/spaces": createHandler,
	})
	defer server.Close()

	_, err := runTool(t, "create_space", server.URL, map[string]interface{}{
		"name":          "testin101",
		"template_name": "ubuntu apple",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed (regression?): %v", err)
	}
	// Verify template was resolved by name-with-space via the encoded URL
	if createReq.Body["template_id"] != "tpl-1" {
		t.Errorf("template_id = %v, want tpl-1 (template name with space should resolve via encoded URL)", createReq.Body["template_id"])
	}
}

// TestExecute_UrlEncoding verifies that urllib.parse.quote is available and
// works correctly inside the scriptling env. The knot libs depend on this
// for safely interpolating user-provided values (template names, space names)
// into URL paths.
func TestExecute_UrlEncoding(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, nil)
	defer server.Close()

	client, err := apiclient.NewClient(server.URL, "test-token", false)
	if err != nil {
		t.Fatalf("apiclient.NewClient failed: %v", err)
	}
	user := &model.User{Id: "u1", Username: "tester", Email: "tester@example.com"}
	env, _, err := service.NewMCPScriptlingEnv(client, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv failed: %v", err)
	}

	// Verify urllib.parse.quote produces expected encoded output
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, _, err := scriptlingmcp.RunToolScript(ctx, env, `
import urllib.parse
import scriptling.mcp.tool as tool

# Spaces and reserved chars get percent-encoded with safe=''
encoded = urllib.parse.quote("ubuntu apple", safe='')
tool.return_string(encoded)
`, nil)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}
	if strings.TrimSpace(result) != "ubuntu%20apple" {
		t.Errorf("urllib.parse.quote returned %q, want %q", strings.TrimSpace(result), "ubuntu%20apple")
	}
}

// =============================================================================
// Spaces
// =============================================================================

func TestExecute_ListSpaces(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/users/whoami": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"user_id": "u1"})
		},
		"GET /api/spaces": func(w http.ResponseWriter, r *http.Request) {
			// Default (no all_zones) should be current-zone only.
			// all_zones=true opt-in via show_all.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spaces": []map[string]interface{}{
					{
						"space_id":   "s1",
						"name":       "web",
						"is_deployed": true,
						"stack":      "",
						"custom_fields": []map[string]string{
							{"name": "ENV", "value": "prod"},
						},
					},
					{"space_id": "s2", "name": "db", "is_deployed": false, "stack": ""},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "list_spaces", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	spaces, _ := result["spaces"].([]interface{})
	if len(spaces) != 2 {
		t.Fatalf("expected 2 spaces, got %d", len(spaces))
	}
	// Verify custom_fields come through (this was the bug — list endpoint wasn't populating them)
	web, _ := spaces[0].(map[string]interface{})
	cfs, _ := web["custom_fields"].([]interface{})
	if len(cfs) != 1 {
		t.Fatalf("expected 1 custom field on web space, got %d (custom_fields not flowing through list?)", len(cfs))
	}
	cf0, _ := cfs[0].(map[string]interface{})
	if cf0["name"] != "ENV" || cf0["value"] != "prod" {
		t.Errorf("custom_fields[0] = %v, want {ENV prod}", cf0)
	}
}

func TestExecute_CreateSpace(t *testing.T) {
	mustLoadTools(t)

	createHandler, createReq := capturingHandler(map[string]interface{}{"space_id": "new-space"})
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/templates/ubuntu": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"template_id": "tpl-1", "name": "ubuntu"})
		},
		"POST /api/spaces": createHandler,
	})
	defer server.Close()

	response, err := runTool(t, "create_space", server.URL, map[string]interface{}{
		"name":          "my-new-space",
		"template_name": "ubuntu",
		"description":   "test space",
		"custom_fields": []interface{}{"ENV=prod", "ROLE=api"},
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if !strings.Contains(response, "my-new-space") {
		t.Errorf("expected response to mention space name, got: %s", response)
	}

	// Verify request body the tool sent
	if createReq.Body["name"] != "my-new-space" {
		t.Errorf("request body name = %v, want my-new-space", createReq.Body["name"])
	}
	if createReq.Body["template_id"] != "tpl-1" {
		t.Errorf("template_id should be resolved to ID, got %v", createReq.Body["template_id"])
	}
	cfs, _ := createReq.Body["custom_fields"].([]interface{})
	if len(cfs) != 2 {
		t.Fatalf("expected 2 custom fields in request body, got %d", len(cfs))
	}
	cf0, _ := cfs[0].(map[string]interface{})
	if cf0["name"] != "ENV" || cf0["value"] != "prod" {
		t.Errorf("custom_fields[0] = %v, want {ENV prod}", cf0)
	}
}

func TestExecute_UpdateSpace(t *testing.T) {
	mustLoadTools(t)

	updateHandler, updateReq := capturingHandler(nil)
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/spaces/my-space": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"space_id":     "s1",
				"name":         "my-space",
				"description":  "old",
				"shell":        "bash",
				"template_id":  "tpl-1",
				"custom_fields": []map[string]string{},
			})
		},
		"PUT /api/spaces/s1": updateHandler,
	})
	defer server.Close()

	_, err := runTool(t, "update_space", server.URL, map[string]interface{}{
		"name":          "my-space",
		"description":   "new description",
		"custom_fields": []interface{}{"ENV=staging"},
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}

	if updateReq.Body["description"] != "new description" {
		t.Errorf("description = %v, want 'new description'", updateReq.Body["description"])
	}
	cfs, _ := updateReq.Body["custom_fields"].([]interface{})
	if len(cfs) != 1 {
		t.Fatalf("expected 1 custom field in update body, got %d", len(cfs))
	}
}

func TestExecute_DeleteSpace(t *testing.T) {
	mustLoadTools(t)

	deleteHandler, deleteReq := capturingHandler(nil)
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"DELETE /api/spaces/my-space": deleteHandler,
	})
	defer server.Close()

	_, err := runTool(t, "delete_space", server.URL, map[string]interface{}{"name": "my-space"})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if !strings.HasSuffix(deleteReq.URL, "/api/spaces/my-space") {
		t.Errorf("DELETE URL = %q, want suffix /api/spaces/my-space", deleteReq.URL)
	}
}

func TestExecute_StartStopRestartSpace(t *testing.T) {
	mustLoadTools(t)

	for _, tc := range []struct {
		tool        string
		verb        string
		running     bool // current state for the is_running pre-check
		expectError bool // start when running / stop when stopped should error
	}{
		{"start_space", "start", false, false},
		{"stop_space", "stop", true, false},
		{"restart_space", "restart", true, false},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			actionHandler, actionReq := capturingHandler(nil)
			server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				// start_space.py and stop_space.py pre-check via knot.space.is_running,
				// which calls GET /api/spaces/{name}. restart_space.py skips this.
				"GET /api/spaces/my-space": func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"space_id":   "s1",
						"name":       "my-space",
						"is_deployed": tc.running,
					})
				},
				"POST /api/spaces/my-space/" + tc.verb: actionHandler,
			})
			defer server.Close()

			_, err := runTool(t, tc.tool, server.URL, map[string]interface{}{"name": "my-space"})
			if err != nil {
				if !tc.expectError {
					t.Fatalf("RunToolScript failed: %v", err)
				}
				return
			}
			if tc.expectError {
				t.Fatalf("expected error, got success")
			}
			if !strings.Contains(actionReq.URL, "/my-space/"+tc.verb) {
				t.Errorf("URL = %q, expected to contain /my-space/%s", actionReq.URL, tc.verb)
			}
		})
	}
}

// =============================================================================
// Stack Definitions
// =============================================================================

func TestExecute_ListStackDefinitions(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/stack-definitions": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"stack_definitions": []map[string]interface{}{
					{
						"stack_definition_id": "def-1",
						"name":                "lamp",
						"description":         "LAMP stack",
						"active":              true,
						"spaces": []map[string]interface{}{
							{"name": "web", "template_id": "tpl-1"},
							{"name": "db", "template_id": "tpl-2"},
						},
					},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "list_stack_definitions", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	defs, _ := result["stack_definitions"].([]interface{})
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	def0, _ := defs[0].(map[string]interface{})
	if def0["name"] != "lamp" {
		t.Errorf("name = %v, want lamp", def0["name"])
	}
	spaces, _ := def0["spaces"].([]interface{})
	if len(spaces) != 2 {
		t.Errorf("expected 2 components, got %d", len(spaces))
	}
}

// =============================================================================
// Stacks
// =============================================================================

func TestExecute_ListStacks(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/users/whoami": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"user_id": "u1"})
		},
		"GET /api/spaces": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spaces": []map[string]interface{}{
					{"space_id": "s1", "name": "myapp-web", "stack": "myapp", "is_deployed": true},
					{"space_id": "s2", "name": "myapp-db", "stack": "myapp", "is_deployed": false},
					{"space_id": "s3", "name": "standalone", "stack": "", "is_deployed": true},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "list_stacks", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	stacks, _ := result["stacks"].([]interface{})
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack (standalone excluded), got %d: %v", len(stacks), stacks)
	}
	stk0, _ := stacks[0].(map[string]interface{})
	if stk0["name"] != "myapp" {
		t.Errorf("stack name = %v, want myapp", stk0["name"])
	}
	spaces, _ := stk0["spaces"].([]interface{})
	if len(spaces) != 2 {
		t.Errorf("expected 2 spaces in myapp stack, got %d", len(spaces))
	}
}

func TestExecute_CreateStack(t *testing.T) {
	mustLoadTools(t)

	createSpaceHandler, createSpaceReqs := capturingHandler(map[string]interface{}{"space_id": "new-1"})
	// Track how many spaces got created
	createdCount := 0
	wrappedCreate := func(w http.ResponseWriter, r *http.Request) {
		createdCount++
		createSpaceHandler(w, r)
	}

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/stack-definitions/lamp": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"stack_definition_id": "def-1",
				"name":                "lamp",
				"spaces": []map[string]interface{}{
					{"name": "web", "template_id": "tpl-1"},
					{"name": "db", "template_id": "tpl-2"},
				},
			})
		},
		"GET /api/templates/tpl-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"template_id": "tpl-1", "name": "ubuntu"})
		},
		"GET /api/templates/tpl-2": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"template_id": "tpl-2", "name": "postgres"})
		},
		"POST /api/spaces": wrappedCreate,
	})
	defer server.Close()

	response, err := runTool(t, "create_stack", server.URL, map[string]interface{}{
		"definition_name": "lamp",
		"prefix":          "test",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	_ = createSpaceReqs // suppress unused

	if createdCount != 2 {
		t.Errorf("expected 2 POST /api/spaces calls, got %d", createdCount)
	}
	result := decodeJSON(t, response)
	spaces, _ := result["spaces"].(map[string]interface{})
	if len(spaces) != 2 {
		t.Errorf("expected 2 spaces in response, got %d: %v", len(spaces), spaces)
	}
}

func TestExecute_StartStopRestartStack(t *testing.T) {
	mustLoadTools(t)

	for _, tc := range []struct {
		tool   string
		verb   string
	}{
		{"start_stack", "start"},
		{"stop_stack", "stop"},
		{"restart_stack", "restart"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			handler, req := capturingHandler(nil)
			server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
				"POST /api/spaces/stacks/myapp/" + tc.verb: handler,
			})
			defer server.Close()

			_, err := runTool(t, tc.tool, server.URL, map[string]interface{}{
				"stack_name": "myapp",
			})
			if err != nil {
				t.Fatalf("RunToolScript failed: %v", err)
			}
			if !strings.HasSuffix(req.URL, "/stacks/myapp/"+tc.verb) {
				t.Errorf("URL = %q, want suffix .../stacks/myapp/%s", req.URL, tc.verb)
			}
		})
	}
}

func TestExecute_DeleteStack(t *testing.T) {
	mustLoadTools(t)

	deleted := false
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/users/whoami": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"user_id": "u1"})
		},
		"DELETE /api/stacks/myapp": func(w http.ResponseWriter, r *http.Request) {
			deleted = true
		},
	})
	defer server.Close()

	_, err := runTool(t, "delete_stack", server.URL, map[string]interface{}{
		"stack_name": "myapp",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if !deleted {
		t.Errorf("expected DELETE call for stack myapp")
	}
}

// =============================================================================
// Files
// =============================================================================

func TestExecute_ReadFile(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /api/spaces/my-space/files/read": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if body["path"] != "/app/config.json" {
				t.Errorf("request path = %v, want /app/config.json", body["path"])
			}
			json.NewEncoder(w).Encode(map[string]string{"content": `{"key":"value"}`})
		},
	})
	defer server.Close()

	response, err := runTool(t, "read_file", server.URL, map[string]interface{}{
		"name":      "my-space",
		"file_path": "/app/config.json",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if !strings.Contains(response, "key") {
		t.Errorf("expected response to contain file content, got: %s", response)
	}
}

func TestExecute_WriteFile(t *testing.T) {
	mustLoadTools(t)

	writeHandler, writeReq := capturingHandler(nil)
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /api/spaces/my-space/files/write": writeHandler,
	})
	defer server.Close()

	_, err := runTool(t, "write_file", server.URL, map[string]interface{}{
		"name":      "my-space",
		"file_path": "/app/note.txt",
		"content":   "hello world",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if writeReq.Body["path"] != "/app/note.txt" {
		t.Errorf("path = %v, want /app/note.txt", writeReq.Body["path"])
	}
	if writeReq.Body["content"] != "hello world" {
		t.Errorf("content = %v, want 'hello world'", writeReq.Body["content"])
	}
}

// =============================================================================
// Commands
// =============================================================================

func TestExecute_ListScripts(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/scripts": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scripts": []map[string]interface{}{
					{"script_id": "sc1", "name": "deploy", "description": "Deploy app", "active": true, "script_type": "script"},
					{"script_id": "sc2", "name": "test", "description": "Run tests", "active": true, "script_type": "script"},
					{"script_id": "sc3", "name": "inactive-script", "description": "Retired", "active": false, "script_type": "script"},
					{"script_id": "sc4", "name": "ai-tool", "description": "An MCP tool", "active": true, "script_type": "tool"},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "list_scripts", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	scripts, _ := result["scripts"].([]interface{})
	// Should be 2: inactive excluded, tool-type excluded
	if len(scripts) != 2 {
		t.Fatalf("expected 2 active runnable scripts, got %d: %v", len(scripts), scripts)
	}
}

func TestExecute_RunCommand(t *testing.T) {
	mustLoadTools(t)

	cmdHandler, cmdReq := capturingHandler(map[string]interface{}{
		"output": "total 0\ndrwxr-xr-x 2 root root 40 Jan 1 00:00 tmp\n",
	})
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /api/spaces/my-space/run-command": cmdHandler,
	})
	defer server.Close()

	response, err := runTool(t, "run_command", server.URL, map[string]interface{}{
		"name":      "my-space",
		"command":   "ls",
		"arguments": []interface{}{"-la", "/tmp"},
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if cmdReq.Body["command"] != "ls" {
		t.Errorf("command = %v, want ls", cmdReq.Body["command"])
	}
	args, _ := cmdReq.Body["args"].([]interface{})
	if len(args) != 2 || args[0] != "-la" {
		t.Errorf("args = %v, want [-la /tmp]", args)
	}
	if !strings.Contains(response, "tmp") {
		t.Errorf("response should contain output, got: %s", response)
	}
}

func TestExecute_RunScript(t *testing.T) {
	mustLoadTools(t)

	scriptHandler, scriptReq := capturingHandler(map[string]interface{}{
		"output":    "deployed\n",
		"exit_code": 0,
	})
	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /api/spaces/my-space/execute-script": scriptHandler,
	})
	defer server.Close()

	response, err := runTool(t, "run_script", server.URL, map[string]interface{}{
		"name":   "my-space",
		"script": "deploy",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	if scriptReq.Body["script_name"] != "deploy" {
		t.Errorf("script_name = %v, want deploy", scriptReq.Body["script_name"])
	}
	if !strings.Contains(response, "deployed") {
		t.Errorf("response should contain output, got: %s", response)
	}
	if !strings.Contains(response, "success") {
		t.Errorf("response should report success for exit_code 0, got: %s", response)
	}
}

// =============================================================================
// Skills
// =============================================================================

func TestExecute_GetSkill_ByName(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/skill/python-best-practices": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"content": "# Python Best Practices\n\nFollow PEP 8",
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "get_skill", server.URL, map[string]interface{}{
		"name": "python-best-practices",
	})
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	if result["score"] != 1.0 {
		t.Errorf("score = %v, want 1.0", result["score"])
	}
	if !strings.Contains(result["skill"].(string), "PEP 8") {
		t.Errorf("skill content missing expected text")
	}
}

func TestExecute_GetSkill_List(t *testing.T) {
	mustLoadTools(t)

	server := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/skill": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"skills": []map[string]interface{}{
					{"name": "active-skill", "description": "An active skill", "active": true},
					{"name": "inactive-skill", "description": "Retired", "active": false},
				},
			})
		},
	})
	defer server.Close()

	response, err := runTool(t, "get_skill", server.URL, nil)
	if err != nil {
		t.Fatalf("RunToolScript failed: %v", err)
	}
	result := decodeJSON(t, response)
	if result["action"] != "list" {
		t.Errorf("action = %v, want list", result["action"])
	}
	if result["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1 (inactive excluded)", result["count"])
	}
}
