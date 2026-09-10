package mcptools

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

// TestExecute_ListTemplates_MuxClient runs list_templates through the real
// server-side path — MuxClient loopback and the embedded scriptling env —
// rather than the bypass-HTTP harness the other exec tests use.
func TestExecute_ListTemplates_MuxClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/templates", func(w http.ResponseWriter, r *http.Request) {
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{
			"templates": []map[string]any{
				{
					"template_id": "t-1", "name": "basic", "active": true,
					"custom_fields": []map[string]any{
						{"name": "env", "description": "Environment", "type": "select", "required": true, "options": []string{"dev", "prod"}},
						{"name": "team", "description": "Team", "type": "text"},
						{"name": "auto1", "description": "Testing Auto", "type": "autocomplete", "default": "dev", "handler": "plugin.demo-scriptling.field_environment"},
					},
				},
			},
		})
	})
	mux.HandleFunc("GET /api/plugins/field-handlers/plugin.demo-scriptling.field_environment", func(w http.ResponseWriter, r *http.Request) {
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{
			"options": []map[string]any{
				{"key": "dev", "text": "development"},
				{"key": "prod", "text": "production"},
			},
		})
	})
	rest.SetAPIMux(mux)
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})

	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools: %v", err)
	}

	user := &model.User{Id: "u-1", Username: "tester", Active: true}

	done := make(chan struct{})
	var result any
	var err error
	go func() {
		defer close(done)
		result, err = ExecuteTool("list_templates", nil, user)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("list_templates did not return within 20s")
	}

	if err != nil {
		t.Fatalf("list_templates failed: %v", err)
	}

	// Handler-backed fields must come back with their options resolved and
	// the handler id stripped — the LLM can only use option values.
	encoded, ok := result.(string)
	if !ok {
		t.Fatalf("tool returned %T, want string: %+v", result, result)
	}
	if strings.Contains(encoded, "handler") {
		t.Fatalf("handler id leaked into discovery output: %s", encoded)
	}

	var parsed struct {
		Templates []struct {
			CustomFields []struct {
				Name    string   `json:"name"`
				Options []string `json:"options"`
			} `json:"custom_fields"`
		} `json:"templates"`
	}
	if err := json.NewDecoder(strings.NewReader(encoded)).Decode(&parsed); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(parsed.Templates) != 1 || len(parsed.Templates[0].CustomFields) != 3 {
		t.Fatalf("unexpected shape: %s", encoded)
	}
	auto := parsed.Templates[0].CustomFields[2]
	if auto.Name != "auto1" || len(auto.Options) != 2 || auto.Options[0] != "dev" || auto.Options[1] != "prod" {
		t.Fatalf("handler-backed field not resolved: %+v", auto)
	}
}
