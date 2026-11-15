package mcptools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

// A request that fails its checks is audited as refused, with the reason
// and what it asked for, capped.
func TestRunScriptToolAuditsBadRequests(t *testing.T) {
	mustLoadTools(t)
	runs, audits := stubAIScripts(t)
	for _, params := range []map[string]interface{}{
		{"code": "print(1)", "purpose": "p", "mode": "WRITE"},
		{"purpose": "no code"},
		{"code": "print(1)", "purpose": strings.Repeat("p", 5000), "mode": strings.Repeat("w", 500)},
	} {
		if _, err := ExecuteToolContext(context.Background(), RunScriptToolName, params, scriptUser(true)); err == nil {
			t.Fatalf("%v accepted", params)
		}
	}
	if len(*runs) != 0 {
		t.Fatal("a bad request ran")
	}
	if len(*audits) != 3 {
		t.Fatalf("audits = %d, want 3", len(*audits))
	}
	if a := (*audits)[0]; a["refused"] != true || a["mode"] != "WRITE" || !strings.Contains(a["error"].(string), "exactly") {
		t.Errorf("WRITE audit = %v", a)
	}
	if a := (*audits)[1]; a["refused"] != true || !strings.Contains(a["error"].(string), "code is required") {
		t.Errorf("missing code audit = %v", a)
	}
	a := (*audits)[2]
	if len(a["purpose"].(string)) > aiScriptAuditPurposeBytes+30 || len(a["mode"].(string)) > aiScriptAuditModeBytes {
		t.Errorf("audit not capped: purpose %d bytes, mode %d bytes", len(a["purpose"].(string)), len(a["mode"].(string)))
	}
}

// A run refused at the concurrency caps is audited as refused.
func TestRunScriptToolAuditsBusyRefusal(t *testing.T) {
	mustLoadTools(t)
	_, audits := stubAIScripts(t)
	runAIScript = func(ctx context.Context, user *model.User, req service.AIScriptRequest) (*service.AIScriptRun, error) {
		return nil, fmt.Errorf("%w: try again", service.ErrAIScriptBusy)
	}
	_, err := ExecuteToolContext(context.Background(), RunScriptToolName, map[string]interface{}{"code": "print(1)", "purpose": "p"}, scriptUser(true))
	if err == nil || !strings.Contains(err.Error(), "try again") {
		t.Fatalf("err = %v", err)
	}
	if len(*audits) != 1 || (*audits)[0]["refused"] != true {
		t.Fatalf("audits = %v", *audits)
	}
}

// Tools that need the user's approval are published for the model only, so
// an MCP Apps view cannot call them (lmchatkit refuses an app's call to a
// tool whose _meta.ui.visibility leaves out "app").
func TestApprovalToolsAreModelOnly(t *testing.T) {
	mustLoadTools(t)
	visibility := map[string][]string{}
	for _, tool := range GetAllMCPTools() {
		switch ui := tool.Meta["ui"].(type) {
		case mcp.UIToolMeta:
			visibility[tool.Name] = ui.Visibility
		case nil:
		default:
			t.Errorf("%s: _meta.ui is %T", tool.Name, ui)
		}
	}
	for _, name := range []string{"run_script", "run_command", "delete_space", "write_file"} {
		if v := visibility[name]; len(v) != 1 || v[0] != "model" {
			t.Errorf("%s visibility = %v, want [model]", name, v)
		}
	}
	for _, name := range []string{"list_spaces", "script_docs"} {
		if v, ok := visibility[name]; ok {
			t.Errorf("%s visibility = %v, want unset", name, v)
		}
	}
}
