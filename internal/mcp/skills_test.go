package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

// The skills extension surface: knot's database-backed skills served per
// user over skills/list, skills/get and resources/read, with zone, group
// and user-overrides-global rules applied identically on every path, and
// the synthesized SKILL.md agreeing with the listing's frontmatter (what
// conformance checkers compare).

var testRedis *miniredis.Miniredis

// TestMain installs a miniredis-backed database behind database.GetInstance()
// for the whole binary: the driver initializes once, lazily, so the config
// pointing at miniredis must be in place before the first DB-touching test.
func TestMain(m *testing.M) {
	var err error
	testRedis, err = miniredis.Run()
	if err != nil {
		panic(err)
	}
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		LeafNode: true,
		Redis:    config.RedisConfig{Enabled: true, Hosts: []string{testRedis.Addr()}},
	})
	// Latch the redis driver NOW: the driver initializes once, lazily, and
	// other tests in this package install their own configs (plugin_tools
	// enables BadgerDB), so whoever triggers the first GetInstance picks the
	// driver for the whole binary.
	_ = database.GetInstance()
	code := m.Run()
	config.SetServerConfig(prev)
	testRedis.Close()
	os.Exit(code)
}

// adminUser returns a user whose role carries every permission, seeding the
// role cache with it.
func adminUser(id string) *model.User {
	model.SetRoleCache(nil) // seeds the admin role
	return &model.User{Id: id, Username: id, Roles: []string{model.RoleAdminUUID}}
}

// seedSkills clears previous seeds and stores the given skills.
func seedSkills(t *testing.T, skills ...*model.Skill) {
	t.Helper()
	testRedis.FlushAll()
	db := database.GetInstance()
	for _, skill := range skills {
		if err := db.SaveSkill(skill, nil); err != nil {
			t.Fatalf("SaveSkill(%s): %v", skill.Name, err)
		}
	}
}

func TestSkillsProviderServesDatabaseSkills(t *testing.T) {
	retired := model.NewSkill("retired", "gone", "inactive", nil, nil, "", "u1")
	retired.Active = false
	seedSkills(t,
		model.NewSkill("code-review", "How to review changes", "Read the diff twice.", nil, nil, "", "u1"),
		retired,
		model.NewSkill("mine", "Personal runbook", "My own steps.", nil, nil, "user-1", "user-1"),
		model.NewSkill("theirs", "Someone else's", "not visible", nil, nil, "user-2", "user-2"),
	)

	user := adminUser("user-1")
	provider := NewSkillsProvider(user)

	skillsList, err := provider.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skillsList) != 2 {
		t.Fatalf("skills = %+v, want code-review + mine only (retired inactive, theirs another user's)", skillsList)
	}
	byURI := map[string]mcp.Skill{}
	for _, s := range skillsList {
		byURI[s.URI] = s
	}
	cr, ok := byURI["skill://code-review/SKILL.md"]
	if !ok {
		t.Fatalf("missing entry, got %+v", skillsList)
	}
	if cr.Frontmatter["name"] != "code-review" || cr.Frontmatter["description"] != "How to review changes" {
		t.Fatalf("frontmatter = %+v", cr.Frontmatter)
	}

	// The entry's digest and size describe the exact bytes resources/read
	// serves, and the served SKILL.md's frontmatter block carries the same
	// name and description the listing declares.
	resp, err := provider.ReadResource(context.Background(), "skill://code-review/SKILL.md")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	served := resp.Contents[0].Text
	var digestEntry mcp.SkillResource
	for _, r := range cr.Resources {
		digestEntry = r
	}
	sum := sha256.Sum256([]byte(served))
	if digestEntry.Digest != "sha256:"+hex.EncodeToString(sum[:]) || digestEntry.Size != int64(len(served)) {
		t.Fatalf("entry = %+v, served %d bytes", digestEntry, len(served))
	}
	if !strings.Contains(served, "---\nname: code-review\ndescription: How to review changes\n---") {
		t.Fatalf("served SKILL.md must declare the listed frontmatter verbatim:\n%s", served)
	}
	if !strings.Contains(served, "Read the diff twice.") {
		t.Fatalf("served SKILL.md must carry the stored content:\n%s", served)
	}

	// User skill reads back under its own name; other misses fall through.
	if _, err := provider.ReadResource(context.Background(), "skill://mine/SKILL.md"); err != nil {
		t.Fatalf("user skill must read: %v", err)
	}
	for _, uri := range []string{
		"skill://retired/SKILL.md",          // inactive
		"skill://theirs/SKILL.md",           // another user's
		"skill://code-review/references.md", // a database skill has no subfiles
		"file://other.md",                   // not a skill URI at all
	} {
		if _, err := provider.ReadResource(context.Background(), uri); err != mcp.ErrUnknownResource {
			t.Fatalf("ReadResource(%s) = %v, want ErrUnknownResource", uri, err)
		}
	}

	// resources/list carries one descriptor per accessible skill.
	provided, err := provider.GetResources(context.Background())
	if err != nil {
		t.Fatalf("GetResources: %v", err)
	}
	if len(provided.Resources) != 2 {
		t.Fatalf("resources = %+v, want 2", provided.Resources)
	}
}

// The public /mcp endpoint serves knot's own skills over the skills methods,
// scoped to the requesting user.
func TestPublicMCPEndpointSkills(t *testing.T) {
	// LeafNode skips the /mcp permission gate so a roleless user reaches the
	// handler (checked against the role cache otherwise); earlier tests in
	// this package install their own configs without it.
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		LeafNode: true,
		Redis:    config.RedisConfig{Enabled: true, Hosts: []string{testRedis.Addr()}},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	seedSkills(t, model.NewSkill("deploy", "Deployment runbook", "Ship it.", nil, nil, "", "u1"))

	routes := &http.ServeMux{}
	InitializeMCPServer(routes, true, &config.MCPConfig{})

	user := adminUser("u1")
	do := func(body string) string {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), "user", user))
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		return rec.Body.String()
	}

	out := do(`{"jsonrpc":"2.0","id":1,"method":"skills/list"}`)
	if !strings.Contains(out, `"uri":"skill://deploy/SKILL.md"`) || !strings.Contains(out, `"frontmatter"`) {
		t.Fatalf("skills/list must serve the database skill: %s", out)
	}
	out = do(`{"jsonrpc":"2.0","id":2,"method":"skills/get","params":{"uri":"skill://deploy/SKILL.md"}}`)
	if !strings.Contains(out, `"skill"`) || !strings.Contains(out, `"digest":"sha256:`) {
		t.Fatalf("skills/get shape: %s", out)
	}
	out = do(`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"skill://deploy/SKILL.md"}}`)
	if !strings.Contains(out, "Ship it.") {
		t.Fatalf("resources/read must serve the SKILL.md content: %s", out)
	}
	out = do(`{"jsonrpc":"2.0","id":4,"method":"skills/get","params":{"uri":"skill://nope/SKILL.md"}}`)
	if !strings.Contains(out, "-32602") {
		t.Fatalf("unknown skill must be -32602: %s", out)
	}

	// The listing is per user: a request as a roleless user sees none of the
	// admin's global skills, and the skill's resource is not readable either.
	noRole := &model.User{Id: "u2", Username: "nobody"}
	post := func(user *model.User, body string) string {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), "user", user))
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	if out := post(noRole, `{"jsonrpc":"2.0","id":5,"method":"skills/list"}`); !strings.Contains(out, `"skills":[]`) {
		t.Fatalf("roleless user's skills/list must be empty: %s", out)
	}
	if out := post(noRole, `{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"skill://deploy/SKILL.md"}}`); strings.Contains(out, "Ship it.") {
		t.Fatalf("roleless user must not read another user's skill: %s", out)
	}
}

// The chat system prompt lists knot's own skills plus every reachable
// remote server's, llmrouter-style: "- name: description (uri)" with remote
// names carrying the namespace.
func TestBuildSkillsPromptIncludesRemoteSkills(t *testing.T) {
	seedSkills(t, model.NewSkill("code-review", "How to review changes", "Read the diff twice.", nil, nil, "", "u1"))

	remote := mcp.NewServer("remote-skills", "1.0")
	remote.RegisterSkill(mcp.NewSkill("dashboard-ops").
		Description("Operate dashboards").
		File("SKILL.md", []byte("---\nname: dashboard-ops\ndescription: Operate dashboards\n---\nDo things.")))
	// Not deferred-Closed: knot's user-remote clients always enable
	// notifications, so the client opened a subscriptions/listen stream to
	// this server, and httptest.Server.Close would block on it forever. The
	// server simply lives for the rest of the test binary.
	remoteSrv := httptest.NewServer(http.HandlerFunc(remote.HandleRequest))

	routes := &http.ServeMux{}
	InitializeMCPServer(routes, false, &config.MCPConfig{
		RemoteServers: []config.MCPRemoteServerConfig{{
			Namespace: "fed",
			URL:       remoteSrv.URL,
			Token:     "test-token",
		}},
	})

	// A user-configured remote server pointing at the same fake: its skills
	// must appear too, under the user server's own namespace.
	if err := database.GetInstance().SaveMCPServer(&model.MCPServer{
		Id:        "srv-user-1",
		UserId:    "u1",
		Namespace: "myai",
		URL:       remoteSrv.URL,
		Enabled:   true,
	}, nil); err != nil {
		t.Fatalf("SaveMCPServer: %v", err)
	}

	prompt := BuildSkillsPrompt(context.Background(), adminUser("u1"), "Call the get_skill tool:")
	for _, want := range []string{
		"The following skills are available. Call the get_skill tool:",
		"- code-review: How to review changes (skill://code-review/SKILL.md)",
		"- fed/dashboard-ops: Operate dashboards (skill://dashboard-ops/SKILL.md)",
		"- myai/dashboard-ops: Operate dashboards (skill://dashboard-ops/SKILL.md)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}

	// A user with no role sees none of knot's own skills. Operator remote
	// skills stay listed (their tools federate to everyone the same way).
	got := BuildSkillsPrompt(context.Background(), &model.User{Id: "x", Username: "x"}, "")
	if strings.Contains(got, "skill://code-review") {
		t.Fatalf("no-role prompt must not list database skills:\n%s", got)
	}
	if strings.Count(got, "fed/dashboard-ops") != 1 {
		t.Fatalf("remote skill listed once, got:\n%s", got)
	}
}
