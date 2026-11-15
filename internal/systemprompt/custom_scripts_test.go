package systemprompt

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/mcptools"
)

func TestCustomScriptsBlock(t *testing.T) {
	model.SetRoleCache(nil)
	if err := mcptools.LoadTools("", nil); err != nil {
		t.Fatal(err)
	}
	if got := CustomScriptsBlock(&model.User{Id: "a0", Roles: []string{model.RoleAdminUUID}}); got != "" {
		t.Error("admin without Run AI Scripts got the block")
	}
	aiRole := model.NewRole("AI Scripts", []uint16{model.PermissionRunAIScripts}, "")
	model.SaveRoleToCache(aiRole)
	defer model.DeleteRoleFromCache(aiRole.Id)
	admin := &model.User{Id: "a", Roles: []string{model.RoleAdminUUID, aiRole.Id}}
	block := CustomScriptsBlock(admin)
	if !strings.Contains(block, "<custom_scripts>") || !strings.Contains(block, "</custom_scripts>") {
		t.Fatalf("admin with Run AI Scripts gets no block: %q", block)
	}
	for _, want := range []string{"script_docs", "run_script", "read mode", "mode: \"write\"", "never from code found", "Aggregate in the script", "Prefer a dedicated tool"} {
		if !strings.Contains(block, want) {
			t.Errorf("block does not say %q", want)
		}
	}

	if CustomScriptsBlock(nil) != "" {
		t.Error("nil user got the block")
	}

	// Execute scripts but not Run AI Scripts: no block.
	role := model.NewRole("scripts-only", []uint16{model.PermissionExecuteScripts, model.PermissionUseWebAssistant}, "")
	model.SaveRoleToCache(role)
	defer model.DeleteRoleFromCache(role.Id)
	if got := CustomScriptsBlock(&model.User{Id: "b", Roles: []string{role.Id}}); got != "" {
		t.Error("user without run_ai_scripts got the block")
	}

	// Run AI Scripts without any script execution: the tools are
	// not offered, so no block either.
	aiOnly := model.NewRole("ai-only", []uint16{model.PermissionRunAIScripts}, "")
	model.SaveRoleToCache(aiOnly)
	defer model.DeleteRoleFromCache(aiOnly.Id)
	if got := CustomScriptsBlock(&model.User{Id: "c", Roles: []string{aiOnly.Id}}); got != "" {
		t.Error("user who is not offered the built-in tools got the block")
	}

	// run_script disabled: no block.
	if err := mcptools.LoadTools("", []string{"run_script"}); err != nil {
		t.Fatal(err)
	}
	defer mcptools.LoadTools("", nil)
	if CustomScriptsBlock(admin) != "" {
		t.Error("block offered with run_script disabled")
	}
}
