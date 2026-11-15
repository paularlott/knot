package mcptools

import (
	"context"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptdocs"
	"github.com/paularlott/knot/internal/service"
)

// stubAIScripts replaces the run and the audit log for the tool tests and
// records what reached them.
func stubAIScripts(t *testing.T) (runs *[]service.AIScriptRequest, audits *[]map[string]interface{}) {
	t.Helper()
	var r []service.AIScriptRequest
	var a []map[string]interface{}
	oldRun, oldAudit := runAIScript, auditAIScriptEntry
	runAIScript = func(ctx context.Context, user *model.User, req service.AIScriptRequest) (*service.AIScriptRun, error) {
		r = append(r, req)
		return &service.AIScriptRun{OK: true, Output: "done", Calls: 3, Mode: req.Mode}, nil
	}
	auditAIScriptEntry = func(actor, actorType, event, details string, properties *map[string]interface{}) error {
		if event != model.AuditEventAIScriptRun || actor != "kai" {
			t.Errorf("audit event %q by %q", event, actor)
		}
		p := *properties
		p["details"] = details
		a = append(a, p)
		return nil
	}
	t.Cleanup(func() { runAIScript, auditAIScriptEntry = oldRun, oldAudit })
	return &r, &a
}

func scriptUser(perms bool) *model.User {
	model.SetRoleCache(nil)
	if perms {
		// Admin alone does not hold Run AI Scripts; a role made for it does.
		role := model.NewRole("AI Scripts", []uint16{model.PermissionRunAIScripts}, "")
		model.SaveRoleToCache(role)
		return &model.User{Id: "u1", Username: "kai", Roles: []string{model.RoleAdminUUID, role.Id}}
	}
	return &model.User{Id: "u2", Username: "kai"}
}

func TestRunScriptToolNeedsPermission(t *testing.T) {
	mustLoadTools(t)
	runs, audits := stubAIScripts(t)
	for _, tool := range []string{RunScriptToolName, ScriptDocsToolName} {
		_, err := ExecuteToolContext(context.Background(), tool, map[string]interface{}{"code": "print(1)", "purpose": "p"}, scriptUser(false))
		if err == nil || !strings.Contains(err.Error(), "Run AI Scripts") {
			t.Errorf("%s without the permission: %v", tool, err)
		}
	}
	if len(*runs) != 0 || len(*audits) != 0 {
		t.Error("a refused call ran or was audited")
	}
}

func TestRunScriptToolReadModeRunsAndAudits(t *testing.T) {
	mustLoadTools(t)
	runs, audits := stubAIScripts(t)
	code := strings.Repeat("x = 1\n", 2000) // 12,000 bytes
	out, err := ExecuteToolContext(context.Background(), RunScriptToolName, map[string]interface{}{
		"code": code, "purpose": " Count spaces ",
	}, scriptUser(true))
	if err != nil {
		t.Fatal(err)
	}
	if run, ok := out.(*service.AIScriptRun); !ok || !run.OK || run.Mode != "read" {
		t.Errorf("result = %#v", out)
	}
	if len(*runs) != 1 || (*runs)[0].Mode != service.AIScriptModeRead || (*runs)[0].Purpose != "Count spaces" {
		t.Errorf("runs = %+v", *runs)
	}
	if len(*audits) != 1 {
		t.Fatalf("audits = %d, want 1", len(*audits))
	}
	a := (*audits)[0]
	kept := a["code"].(string)
	if len(kept) > aiScriptAuditCodeBytes+30 || !strings.HasSuffix(kept, "[... truncated ...]") {
		t.Errorf("audited code is %d bytes, want it capped at %d", len(kept), aiScriptAuditCodeBytes)
	}
	if a["mode"] != "read" || a["purpose"] != "Count spaces" || a["ok"] != true || a["api_calls"] != int64(3) || a["code_bytes"] != len(code) {
		t.Errorf("audit = %v", a)
	}
	if !strings.Contains(a["details"].(string), "Count spaces") {
		t.Errorf("details = %v", a["details"])
	}
}

func TestRunScriptToolRejectsBadRequests(t *testing.T) {
	mustLoadTools(t)
	runs, _ := stubAIScripts(t)
	for _, params := range []map[string]interface{}{
		{"purpose": "p"},
		{"code": "print(1)"},
		{"code": "print(1)", "purpose": "p", "mode": "admin"},
		// Only the exact strings: the web assistant asks the user when the
		// argument is exactly "write", so no variant may run as write.
		{"code": "print(1)", "purpose": "p", "mode": "WRITE"},
		{"code": "print(1)", "purpose": "p", "mode": " write"},
		{"code": "print(1)", "purpose": "p", "mode": "Write"},
		{"code": "print(1)", "purpose": "p", "mode": "write\n"},
		{"code": "print(1)", "purpose": "p", "mode": "READ"},
		{"code": "print(1)", "purpose": "p", "mode": true},
	} {
		if _, err := ExecuteToolContext(context.Background(), RunScriptToolName, params, scriptUser(true)); err == nil {
			t.Errorf("%v accepted", params)
		}
	}
	if len(*runs) != 0 {
		t.Error("a bad request ran")
	}
}

// Write mode runs where someone approves tool calls (the web assistant, an
// MCP client) and is refused where tools run unattended.
func TestRunScriptToolWriteModeApproval(t *testing.T) {
	mustLoadTools(t)
	runs, audits := stubAIScripts(t)
	params := map[string]interface{}{"code": "space.stop('a')", "purpose": "Stop a", "mode": "write"}

	_, err := ExecuteToolContext(context.Background(), RunScriptToolName, params, scriptUser(true))
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Errorf("unattended write mode: %v", err)
	}
	if len(*runs) != 0 {
		t.Fatal("unapproved write mode ran")
	}
	// The refusal is audited, with its reason.
	if len(*audits) != 1 || (*audits)[0]["refused"] != true || (*audits)[0]["ok"] != false ||
		!strings.Contains((*audits)[0]["error"].(string), "approval") || !strings.Contains((*audits)[0]["details"].(string), "Refused") {
		t.Fatalf("refused write audit = %v", *audits)
	}

	for _, surface := range []Surface{SurfaceWebChat, SurfaceMCP} {
		ctx := WithSurface(context.Background(), surface)
		if _, err := ExecuteToolContext(ctx, RunScriptToolName, params, scriptUser(true)); err != nil {
			t.Errorf("write mode from %s: %v", surface, err)
		}
	}
	if len(*runs) != 2 || (*audits)[2]["surface"] != string(SurfaceMCP) || (*audits)[2]["mode"] != "write" || (*audits)[2]["refused"] != false {
		t.Errorf("runs %d, audits %v", len(*runs), *audits)
	}

	// Read mode needs no approval anywhere.
	if _, err := ExecuteToolContext(context.Background(), RunScriptToolName, map[string]interface{}{"code": "x", "purpose": "p"}, scriptUser(true)); err != nil {
		t.Errorf("unattended read mode: %v", err)
	}
}

func TestScriptDocsTool(t *testing.T) {
	mustLoadTools(t)
	user := scriptUser(true)
	got, err := ExecuteToolContext(context.Background(), ScriptDocsToolName, map[string]interface{}{}, user)
	if err != nil || got != scriptdocs.Guide() {
		t.Errorf("no library: err %v", err)
	}
	got, err = ExecuteToolContext(context.Background(), ScriptDocsToolName, map[string]interface{}{"library": "guide, knot.space, pool"}, user)
	if err != nil {
		t.Fatal(err)
	}
	page := got.(string)
	for _, want := range []string{"# Writing scripts for run_script", "# knot.space", "# knot.pool"} {
		if !strings.Contains(page, want) {
			t.Errorf("combined docs missing %q", want)
		}
	}
	if _, err := ExecuteToolContext(context.Background(), ScriptDocsToolName, map[string]interface{}{"library": "nope"}, user); err == nil || !strings.Contains(err.Error(), "knot.space") {
		t.Errorf("unknown library: %v", err)
	}
}

func TestAIScriptToolsLoaded(t *testing.T) {
	mustLoadTools(t)
	for _, name := range []string{RunScriptToolName, ScriptDocsToolName, "run_space_script"} {
		if _, ok := GetTool(name); !ok {
			t.Errorf("%s not loaded", name)
		}
	}
	tool, _ := GetTool(RunScriptToolName)
	if tool.Handler == nil || tool.Metadata.Destructive == nil || !*tool.Metadata.Destructive {
		t.Error("run_script should be a Go tool marked destructive, so MCP clients ask before running it")
	}
	docs, _ := GetTool(ScriptDocsToolName)
	if docs.Metadata.ReadOnly == nil || !*docs.Metadata.ReadOnly {
		t.Error("script_docs should be marked read-only")
	}
	if err := LoadTools("", []string{RunScriptToolName}); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetTool(RunScriptToolName); ok {
		t.Error("run_script could not be disabled")
	}
	if UserCanRunAIScripts(scriptUser(true)) {
		t.Error("UserCanRunAIScripts with run_script disabled")
	}
	mustLoadTools(t)
	if !UserCanRunAIScripts(scriptUser(true)) || UserCanRunAIScripts(scriptUser(false)) {
		t.Error("UserCanRunAIScripts does not follow the permission")
	}
}

// A request authenticated with an API token limited to scopes such as mcp
// may not run write mode, even from an MCP client; sessions, unscoped tokens
// and OAuth tokens (scoped, but issued to an interactive client) may.
func TestRunScriptToolWriteModeRefusedForScopedTokens(t *testing.T) {
	mustLoadTools(t)
	runs, audits := stubAIScripts(t)
	params := map[string]interface{}{"code": "space.stop('a')", "purpose": "Stop a", "mode": "write"}

	scoped := context.WithValue(WithSurface(context.Background(), SurfaceMCP), "access_token", &model.Token{Id: "t1", Scopes: []string{model.ScopeMCP}})
	_, err := ExecuteToolContext(scoped, RunScriptToolName, params, scriptUser(true))
	if err == nil || !strings.Contains(err.Error(), "limited to particular scopes") {
		t.Fatalf("scoped token write mode: %v", err)
	}
	if len(*runs) != 0 || len(*audits) != 1 || (*audits)[0]["refused"] != true {
		t.Fatalf("runs %v, audits %v", *runs, *audits)
	}

	// Read mode still works with the scoped token.
	if _, err := ExecuteToolContext(scoped, RunScriptToolName, map[string]interface{}{"code": "x", "purpose": "p"}, scriptUser(true)); err != nil {
		t.Errorf("scoped token read mode: %v", err)
	}

	unscoped := context.WithValue(WithSurface(context.Background(), SurfaceMCP), "access_token", &model.Token{Id: "t2"})
	if _, err := ExecuteToolContext(unscoped, RunScriptToolName, params, scriptUser(true)); err != nil {
		t.Errorf("unscoped token write mode: %v", err)
	}
	oauth := context.WithValue(WithSurface(context.Background(), SurfaceMCP), "access_token", &model.Token{Id: "t3", Scopes: []string{model.ScopeMCP}, RefreshToken: true})
	if _, err := ExecuteToolContext(oauth, RunScriptToolName, params, scriptUser(true)); err != nil {
		t.Errorf("OAuth token write mode: %v", err)
	}
	if len(*runs) != 3 {
		t.Errorf("runs = %d, want 3", len(*runs))
	}
}

// The error kept in the audit log is capped.
func TestRunScriptToolAuditCapsError(t *testing.T) {
	mustLoadTools(t)
	_, audits := stubAIScripts(t)
	runAIScript = func(ctx context.Context, user *model.User, req service.AIScriptRequest) (*service.AIScriptRun, error) {
		return &service.AIScriptRun{OK: false, Error: strings.Repeat("e", 100_000), Mode: req.Mode}, nil
	}
	if _, err := ExecuteToolContext(context.Background(), RunScriptToolName, map[string]interface{}{"code": "x", "purpose": "p"}, scriptUser(true)); err != nil {
		t.Fatal(err)
	}
	kept := (*audits)[0]["error"].(string)
	if len(kept) > aiScriptAuditErrorBytes+30 || !strings.HasSuffix(kept, "[... truncated ...]") {
		t.Errorf("audited error is %d bytes, want it capped at %d", len(kept), aiScriptAuditErrorBytes)
	}
}
