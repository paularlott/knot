package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

type forwardFixture struct {
	owner    *model.User // owns the service space and pool
	client   *model.User // forwards to the owner's targets
	template *model.Template
}

// newForwardFixture creates two users and a template with one private port
// (8080) and one public port (5432).
func newForwardFixture(t *testing.T, name string) *forwardFixture {
	t.Helper()
	restorePoolDeps(t)
	setServerZone(t, "fwd-"+name)

	owner := newPoolTestUser("fwd-owner-" + uuid.NewString())
	owner.Username = "owner" + uuid.NewString()[:8]
	owner.Email = owner.Username + "@test.local"
	if err := database.GetInstance().SaveUser(owner, nil); err != nil {
		t.Fatalf("SaveUser owner: %v", err)
	}
	client := newPoolTestUser("fwd-client-" + uuid.NewString())
	client.Username = "client" + uuid.NewString()[:8]
	client.Email = client.Username + "@test.local"
	if err := database.GetInstance().SaveUser(client, nil); err != nil {
		t.Fatalf("SaveUser client: %v", err)
	}

	template := newPoolTestTemplate(t, "fwd-tmpl-"+uuid.NewString()[:8])
	template.Ports = []model.TemplatePort{
		{Name: "private", Port: 8080, Protocol: "tcp"},
		{Name: "public", Port: 5432, Protocol: "tcp", Public: true},
	}
	if err := database.GetInstance().SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	return &forwardFixture{owner: owner, client: client, template: template}
}

func (f *forwardFixture) space(t *testing.T, owner *model.User, name string) *model.Space {
	t.Helper()
	space := model.NewSpace(name, "", owner.Id, f.template.Id, "bash", &[]model.AltNameEntry{}, "fwd", "", nil)
	space.IsDeployed = true
	if err := database.GetInstance().SaveSpace(space, nil); err != nil {
		t.Fatalf("SaveSpace %s: %v", name, err)
	}
	return space
}

func (f *forwardFixture) pool(t *testing.T, owner *model.User, name string, members int) *model.PoolDefinition {
	t.Helper()
	pool := model.NewPoolDefinition(name, f.template.Id, "", members, owner.Id)
	pool.Zone = "fwd"
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	for i := 0; i < members; i++ {
		member := model.NewSpace(name+"-m"+string(rune('a'+i)), "", owner.Id, f.template.Id, "bash", &[]model.AltNameEntry{}, "fwd", "", nil)
		member.PoolId = pool.Id
		member.IsDeployed = true
		if err := database.GetInstance().SaveSpace(member, nil); err != nil {
			t.Fatalf("SaveSpace member: %v", err)
		}
	}
	return pool
}

// refreshPoolService returns the production pool service with clean
// round-robin state; routing state must not leak between tests.
func refreshPoolService() *PoolService {
	svc := GetPoolService()
	svc.mu.Lock()
	svc.rrCounters = make(map[string]int)
	svc.mu.Unlock()
	return svc
}

func TestSplitForwardRef(t *testing.T) {
	cases := []struct {
		ref  string
		user string
		name string
		ok   bool
	}{
		{ref: "paul--svc", user: "paul", name: "svc", ok: true},
		{ref: "paul--svc--x", user: "", name: "", ok: false}, // names never contain --
		{ref: "9x--svc", user: "", name: "", ok: false},      // usernames start with a letter
		{ref: "paul--1svc", user: "", name: "", ok: false},   // names start with a letter
		{ref: "svc", user: "", name: "", ok: false},          // bare name
		{ref: "paul-svc", user: "", name: "", ok: false},     // single dash is a bare name
	}
	for _, tc := range cases {
		user, name, ok := SplitForwardRef(tc.ref)
		if ok != tc.ok || (ok && (user != tc.user || name != tc.name)) {
			t.Errorf("SplitForwardRef(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.ref, user, name, ok, tc.user, tc.name, tc.ok)
		}
	}
}

func TestValidForwardRef(t *testing.T) {
	// "svc-" matches knot's own name rules (validate.Name allows a
	// trailing hyphen), so refs stay consistent with space names.
	valid := []string{"svc", "paul--svc", "01a0eaa0-1f64-723a-a052-337b211bb07b", "a-1", "svc-"}
	invalid := []string{"", "paul--svc--x", "-svc", "paul--", "--svc"}
	for _, ref := range valid {
		if !ValidForwardRef(ref) {
			t.Errorf("ValidForwardRef(%q) = false, want true", ref)
		}
	}
	for _, ref := range invalid {
		if ValidForwardRef(ref) {
			t.Errorf("ValidForwardRef(%q) = true, want false", ref)
		}
	}
}

func TestAuthorizeForwardTargetOwnSpace(t *testing.T) {
	f := newForwardFixture(t, "own-space")
	svc := f.space(t, f.owner, "svc"+uuid.NewString()[:6])

	target, ferr := AuthorizeForwardTarget(f.owner, svc.Name, 8080)
	if ferr != nil {
		t.Fatalf("own space any port: %v", ferr.Message)
	}
	if target.Space == nil || target.Space.Id != svc.Id {
		t.Fatal("own space did not resolve to the space")
	}
	if target.Ref != svc.Name {
		t.Fatalf("own space Ref = %q, want bare name", target.Ref)
	}
	if target.IsPool {
		t.Fatal("own space misreported as pool")
	}
}

func TestAuthorizeForwardTargetCrossUserSpace(t *testing.T) {
	f := newForwardFixture(t, "cross-space")
	svc := f.space(t, f.owner, "svc"+uuid.NewString()[:6])
	ref := f.owner.Username + "--" + svc.Name

	// Public port: allowed, ref stays qualified.
	target, ferr := AuthorizeForwardTarget(f.client, ref, 5432)
	if ferr != nil {
		t.Fatalf("public port denied: %v", ferr.Message)
	}
	if target.Ref != ref {
		t.Fatalf("Ref = %q, want %q", target.Ref, ref)
	}

	// Private port: 403 with the port in the message.
	_, ferr = AuthorizeForwardTarget(f.client, ref, 8080)
	if ferr == nil || ferr.Status != http.StatusForbidden {
		t.Fatalf("private port allowed: %+v", ferr)
	}

	// The owner reaches their own space by the qualified form too.
	if _, ferr = AuthorizeForwardTarget(f.owner, ref, 8080); ferr != nil {
		t.Fatalf("owner via qualified ref denied: %v", ferr.Message)
	}

	// Unknown user or space: 404.
	if _, ferr = AuthorizeForwardTarget(f.client, "nobody--"+svc.Name, 5432); ferr == nil || ferr.Status != http.StatusNotFound {
		t.Fatalf("unknown user: %+v", ferr)
	}
	if _, ferr = AuthorizeForwardTarget(f.client, f.owner.Username+"--nosuchspace", 5432); ferr == nil || ferr.Status != http.StatusNotFound {
		t.Fatalf("unknown space: %+v", ferr)
	}
}

func TestAuthorizeForwardTargetUUIDRef(t *testing.T) {
	f := newForwardFixture(t, "uuid")
	svc := f.space(t, f.owner, "svc"+uuid.NewString()[:6])

	// Owner by UUID.
	target, ferr := AuthorizeForwardTarget(f.owner, svc.Id, 8080)
	if ferr != nil {
		t.Fatalf("owner by uuid: %v", ferr.Message)
	}
	if target.Ref != svc.Id {
		t.Fatalf("uuid Ref = %q, want %q", target.Ref, svc.Id)
	}

	// Another user by UUID: public port allowed, private denied. This is
	// the form legacy stored entries use, so the rule must hold here too.
	if _, ferr := AuthorizeForwardTarget(f.client, svc.Id, 5432); ferr != nil {
		t.Fatalf("uuid public port denied: %v", ferr.Message)
	}
	if _, ferr := AuthorizeForwardTarget(f.client, svc.Id, 8080); ferr == nil || ferr.Status != http.StatusForbidden {
		t.Fatalf("uuid private port allowed: %+v", ferr)
	}
}

func TestAuthorizeForwardTargetOwnPool(t *testing.T) {
	f := newForwardFixture(t, "own-pool")
	refreshPoolService()
	poolName := "pg" + uuid.NewString()[:6]
	f.pool(t, f.owner, poolName, 2)

	// Authorize doesn't need a live member — any port for the owner.
	target, ferr := AuthorizeForwardTarget(f.owner, poolName, 8080)
	if ferr != nil {
		t.Fatalf("own pool: %v", ferr.Message)
	}
	if !target.IsPool || target.Space != nil {
		t.Fatalf("own pool resolution: IsPool=%v Space=%v", target.IsPool, target.Space)
	}
	if target.Ref != poolName {
		t.Fatalf("Ref = %q, want %q", target.Ref, poolName)
	}

	// Resolve picks a member round-robin.
	resolved, ferr := ResolveForwardTarget(f.owner, poolName, 8080)
	if ferr != nil {
		t.Fatalf("own pool resolve: %v", ferr.Message)
	}
	if resolved.Space == nil || resolved.Space.PoolId == "" {
		t.Fatalf("pool resolve returned no member: %+v", resolved)
	}
}

func TestAuthorizeForwardTargetCrossUserPool(t *testing.T) {
	f := newForwardFixture(t, "cross-pool")
	refreshPoolService()
	poolName := "pg" + uuid.NewString()[:6]
	f.pool(t, f.owner, poolName, 1)
	ref := f.owner.Username + "--" + poolName

	// Public port allowed and resolves to a member.
	target, ferr := ResolveForwardTarget(f.client, ref, 5432)
	if ferr != nil {
		t.Fatalf("cross-user pool public port: %v", ferr.Message)
	}
	if target.Space == nil || target.Space.PoolId == "" {
		t.Fatal("cross-user pool resolve returned no member")
	}
	if target.Ref != ref || !target.IsPool {
		t.Fatalf("Ref = %q IsPool = %v", target.Ref, target.IsPool)
	}

	// Private port denied before any member is picked.
	if _, ferr = AuthorizeForwardTarget(f.client, ref, 8080); ferr == nil || ferr.Status != http.StatusForbidden {
		t.Fatalf("cross-user pool private port allowed: %+v", ferr)
	}
}

func TestResolveForwardTargetEmptyPoolIs503(t *testing.T) {
	f := newForwardFixture(t, "empty-pool")
	refreshPoolService()
	poolName := "pg" + uuid.NewString()[:6]
	f.pool(t, f.owner, poolName, 0)

	// Authorization succeeds with no members; resolution reports 503.
	if _, ferr := AuthorizeForwardTarget(f.owner, poolName, 8080); ferr != nil {
		t.Fatalf("empty pool authorize: %v", ferr.Message)
	}
	_, ferr := ResolveForwardTarget(f.owner, poolName, 8080)
	if ferr == nil || ferr.Status != http.StatusServiceUnavailable {
		t.Fatalf("empty pool resolve: %+v", ferr)
	}
}

func TestResolveForwardTargetLeasedMemberSkipped(t *testing.T) {
	f := newForwardFixture(t, "leased")
	refreshPoolService()
	poolName := "pg" + uuid.NewString()[:6]
	pool := f.pool(t, f.owner, poolName, 1)

	// Lease the only member exclusively.
	members, err := database.GetInstance().GetSpacesByPoolId(pool.Id)
	if err != nil || len(members) != 1 {
		t.Fatalf("GetSpacesByPoolId: %v (%d members)", err, len(members))
	}
	m := members[0]
	future := time.Now().Add(time.Hour)
	m.LeaseId = "lease-1"
	m.LeaseExpiresAt = &future
	if err := database.GetInstance().SaveSpace(m, []string{"LeaseId", "LeaseExpiresAt"}); err != nil {
		t.Fatalf("SaveSpace lease: %v", err)
	}

	_, ferr := ResolveForwardTarget(f.owner, poolName, 5432)
	if ferr == nil || ferr.Status != http.StatusServiceUnavailable {
		t.Fatalf("leased member still routed: %+v", ferr)
	}
}

func TestSpaceShadowsPoolOfSameName(t *testing.T) {
	f := newForwardFixture(t, "shadow")
	refreshPoolService()
	name := "dual" + uuid.NewString()[:6]
	svc := f.space(t, f.owner, name)
	f.pool(t, f.owner, name, 1)

	target, ferr := ResolveForwardTarget(f.owner, name, 5432)
	if ferr != nil {
		t.Fatalf("shadowed resolution: %v", ferr.Message)
	}
	if target.IsPool || target.Space == nil || target.Space.Id != svc.Id {
		t.Fatalf("space did not shadow pool: %+v", target)
	}
}

func TestIsPortPublic(t *testing.T) {
	template := &model.Template{Ports: []model.TemplatePort{
		{Name: "private", Port: 8080, Protocol: "tcp"},
		{Name: "public", Port: 5432, Protocol: "tcp", Public: true},
	}}
	if template.IsPortPublic(5432) != true {
		t.Error("declared public port not reported public")
	}
	if template.IsPortPublic(8080) != false {
		t.Error("private port reported public")
	}
	if template.IsPortPublic(6379) != false {
		t.Error("undeclared port reported public")
	}
}
