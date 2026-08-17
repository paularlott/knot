//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestVolumesCRUD(t *testing.T) {
	harness.Feature(t, "volumes")
	ctx, cancel := testCtx(120)
	defer cancel()

	name := uniqueName("it-vol")
	resp, code, err := admin.Client.CreateVolume(ctx, &apiclient.VolumeCreateRequest{
		Name:       name,
		Definition: "volumes:\n  " + name + ":\n    driver: local\n",
		Platform:   "container",
	})
	if err != nil {
		t.Fatalf("create volume: %v (status %d)", err, code)
	}

	vol, code, err := admin.Client.GetVolume(ctx, resp.VolumeId)
	if err != nil {
		t.Fatalf("get volume: %v (status %d)", err, code)
	}
	mustEqual(t, "volume name", vol.Name, name)

	list, code, err := admin.Client.GetVolumes(ctx)
	if err != nil {
		t.Fatalf("list volumes: %v (status %d)", err, code)
	}
	found := false
	for _, v := range list.Volumes {
		if v.Id == resp.VolumeId {
			found = true
		}
	}
	if !found {
		t.Fatal("volume missing from list")
	}

	if code, err := admin.Client.UpdateVolume(ctx, resp.VolumeId, &apiclient.VolumeUpdateRequest{
		Name: name, Definition: "volumes:\n  " + name + ":\n    driver: local\n", Platform: "container",
	}); err != nil {
		t.Fatalf("update volume: %v (status %d)", err, code)
	}

	if code, err := admin.Client.DeleteVolume(ctx, resp.VolumeId); err != nil {
		t.Fatalf("delete volume: %v (status %d)", err, code)
	}
}

func TestPoolLifecycle(t *testing.T) {
	harness.Feature(t, "pools")
	ctx, cancel := testCtx(60)
	defer cancel()

	resp, code, err := admin.Client.CreatePool(ctx, &apiclient.PoolRequest{
		Name:         uniqueName("it-pool"),
		TemplateId:   templateId,
		DesiredCount: 1,
		Active:       true,
	})
	if err != nil {
		t.Fatalf("create pool: %v (status %d)", err, code)
	}
	poolId := resp.Id
	t.Cleanup(func() {
		ctx, cancel := testCtx(60)
		admin.Client.DeletePool(ctx, poolId)
		cancel()
	})

	// The pool provisions a member space; wait for it to come alive.
	deadline := time.Now().Add(harnessSpaceTimeout())
	for time.Now().Before(deadline) {
		ctx, cancel := testCtx(15)
		pool, _, err := admin.Client.GetPool(ctx, poolId)
		cancel()
		if err == nil && pool.AliveMembers >= 1 {
			break
		}
		time.Sleep(3 * time.Second)
	}
	ctx2, cancel2 := testCtx(15)
	defer cancel2()
	pool, _, err := admin.Client.GetPool(ctx2, poolId)
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if pool.AliveMembers < 1 {
		t.Fatalf("pool has no alive members (desired 1): %+v", pool)
	}

	// Scale to zero (set-size requires >= 1, so stop the pool instead).
	if code, err := admin.Client.StopPool(ctx2, poolId); err != nil {
		t.Fatalf("stop pool: %v (status %d)", err, code)
	}
	deadline = time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := testCtx(15)
		pool, _, err := admin.Client.GetPool(ctx, poolId)
		cancel()
		if err == nil && pool.AliveMembers == 0 {
			break
		}
		time.Sleep(3 * time.Second)
	}
	pool, _, _ = admin.Client.GetPool(ctx2, poolId)
	if pool.AliveMembers != 0 {
		t.Fatalf("pool still has alive members after scale-down: %+v", pool)
	}

	if code, err := admin.Client.DeletePool(ctx2, poolId); err != nil {
		t.Fatalf("delete pool: %v (status %d)", err, code)
	}
}

func harnessSpaceTimeout() time.Duration {
	if cfg != nil {
		return time.Duration(cfg.SpaceReadyTimeoutSeconds) * time.Second
	}
	return 600 * time.Second
}

func TestScriptsSkillsCommandsCRUD(t *testing.T) {
	harness.Feature(t, "scripts")
	ctx, cancel := testCtx(60)
	defer cancel()

	// Scripts.
	scriptResp, err := admin.Client.CreateScript(ctx, apiclient.ScriptCreateRequest{
		Name:        uniqueVarName("it_script"),
		Description: "integration test script",
		Content:     "print('hello from script')",
		Active:      true,
		ScriptType:  "python",
	})
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	scripts, err := admin.Client.GetScripts(ctx)
	if err != nil {
		t.Fatalf("list scripts: %v", err)
	}
	if len(scripts.Scripts) == 0 {
		t.Fatal("no scripts listed after creation")
	}
	if err := admin.Client.DeleteScript(ctx, scriptResp.Id); err != nil {
		t.Fatalf("delete script: %v", err)
	}

	// Skills.
	harness.Feature(t, "skills")
	skillResp, err := admin.Client.CreateSkill(ctx, &apiclient.SkillCreateRequest{
		Content: "---\nname: it-skill\ndescription: integration test skill\n---\n\nbody",
		Active:  true,
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	skills, err := admin.Client.GetSkills(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills.Skills) == 0 {
		t.Fatal("no skills listed after creation")
	}
	if err := admin.Client.DeleteSkill(ctx, skillResp.Id); err != nil {
		t.Fatalf("delete skill: %v", err)
	}

	// Slash commands.
	harness.Feature(t, "commands")
	cmdResp, err := admin.Client.CreateCommand(ctx, &apiclient.CommandCreateRequest{
		Content: "---\nname: it-cmd\ndescription: integration test command\n---\n\nbody",
		Active:  true,
	})
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	cmds, err := admin.Client.GetCommands(ctx)
	if err != nil {
		t.Fatalf("list commands: %v", err)
	}
	if len(cmds.Commands) == 0 {
		t.Fatal("no commands listed after creation")
	}
	if err := admin.Client.DeleteCommand(ctx, cmdResp.Id); err != nil {
		t.Fatalf("delete command: %v", err)
	}
}

func TestStackDefinitionsAndTemplateVars(t *testing.T) {
	harness.Feature(t, "stack-definitions")
	ctx, cancel := testCtx(60)
	defer cancel()

	stackName := uniqueName("it-stackdef")
	stackId, code, err := admin.Client.CreateStackDefinition(ctx, &apiclient.StackDefinitionRequest{
		Name:        stackName,
		Description: "integration stack",
		Active:      true,
		Scope:       "global",
		Spaces: []apiclient.StackDefSpace{
			{Name: "web", TemplateId: templateId, Shell: "bash"},
		},
	})
	if err != nil {
		t.Fatalf("create stack definition: %v (status %d)", err, code)
	}

	byName, err := admin.Client.GetStackDefinitionByName(ctx, stackName)
	if err != nil {
		t.Fatalf("get stack def by name: %v", err)
	}
	mustEqual(t, "stack def id", byName.Id, stackId)

	// Validation endpoint accepts the definition.
	validation, code, err := admin.Client.ValidateStackDefinition(ctx, &apiclient.StackDefinitionRequest{
		Name: stackName, Active: true, Scope: "global",
		Spaces: []apiclient.StackDefSpace{{Name: "web", TemplateId: templateId, Shell: "bash"}},
	})
	if err != nil {
		t.Fatalf("validate stack definition: %v (status %d)", err, code)
	}
	_ = validation

	if code, err := admin.Client.DeleteStackDefinition(ctx, stackId); err != nil {
		t.Fatalf("delete stack definition: %v (status %d)", err, code)
	}

	harness.Feature(t, "template-vars")
	varName := uniqueVarName("it_var")
	varId, code, err := admin.Client.CreateTemplateVar(ctx, &apiclient.TemplateVarValue{
		Name:  varName,
		Value: "it-value",
	})
	if err != nil {
		t.Fatalf("create template var: %v (status %d)", err, code)
	}
	vars, _, err := admin.Client.GetTemplateVars(ctx)
	if err != nil {
		t.Fatalf("list template vars: %v", err)
	}
	found := false
	for _, v := range vars.TemplateVar {
		if v.Id == varId {
			found = true
		}
	}
	if !found {
		t.Fatal("template var missing from list")
	}
	if code, err := admin.Client.DeleteTemplateVar(ctx, varId); err != nil {
		t.Fatalf("delete template var: %v (status %d)", err, code)
	}
}

func TestMCPServersCRUD(t *testing.T) {
	harness.Feature(t, "mcp")
	ctx, cancel := testCtx(60)
	defer cancel()

	resp, err := admin.Client.CreateMCPServer(ctx, &apiclient.MCPServerCreateRequest{
		Namespace: "it-mcp-" + uniqueName("x"),
		URL:       "http://127.0.0.1:9/mcp",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	servers, err := admin.Client.GetMCPServers(ctx, admin.Id)
	if err != nil {
		t.Fatalf("list mcp servers: %v", err)
	}
	if len(servers.Servers) == 0 {
		t.Fatal("no mcp servers listed")
	}

	if err := admin.Client.DeleteMCPServer(ctx, resp.Id); err != nil {
		t.Fatalf("delete mcp server: %v", err)
	}
}

func TestSearch(t *testing.T) {
	harness.Feature(t, "search")
	ctx, cancel := testCtx(30)
	defer cancel()

	results, err := admin.Client.Search(ctx, "it-ubuntu")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if results == nil {
		t.Fatal("nil search results")
	}
}

func TestSpaceUsageHistory(t *testing.T) {
	harness.Feature(t, "usage-history")
	id := workspace(t)

	// Cause some filesystem activity so the usage sample isn't empty.
	harness.RunCommand(t, user1.Client, id, 30, "for i in 1 2 3 4 5; do echo $i > usage-$i.txt; done; sync")

	var resp struct {
		SpaceId string `json:"space_id"`
		Points  []struct {
			BucketStart time.Time `json:"bucket_start"`
		} `json:"points"`
	}
	ctx, cancel := testCtx(150)
	defer cancel()
	// The agent samples usage periodically; keep writing files until a
	// sample lands.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		harness.RunCommand(t, user1.Client, id, 30,
			"for i in $(seq 1 20); do echo $i > usage-$i.txt; done; sync")
		resp.Points = nil
		if _, err := admin.Client.Do(ctx, "GET", "/api/spaces/"+id+"/usage/history?range=1h", nil, &resp); err != nil {
			t.Fatalf("usage history: %v", err)
		}
		if len(resp.Points) > 0 {
			return
		}
		time.Sleep(10 * time.Second)
	}
	t.Fatal("no usage history points returned")
}
