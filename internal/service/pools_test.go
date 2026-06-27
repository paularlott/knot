package service

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// poolMemberSpaces returns a pool's live member spaces sorted by member number.
func poolMemberSpaces(t *testing.T, pool *model.PoolDefinition) []*model.Space {
	t.Helper()
	spaces, err := database.GetInstance().GetSpaces()
	if err != nil {
		t.Fatalf("GetSpaces: %v", err)
	}
	return poolMembers(pool, spaces)
}

// ordinalOf parses the member number out of a member name (poolname-N).
func ordinalOf(t *testing.T, poolName string, space *model.Space) int {
	t.Helper()
	n, ok := memberOrdinal(poolName, space)
	if !ok {
		t.Fatalf("name %q is not a numbered member of pool %q", space.Name, poolName)
	}
	return n
}

func newPoolTestUser(id string) *model.User {
	return &model.User{
		Id:    id,
		Roles: []string{model.RoleAdminUUID},
	}
}

func newPoolTestTemplate(t *testing.T, name string) *model.Template {
	t.Helper()
	template := model.NewTemplate(
		name,
		"test template",
		"job",
		"",
		"user-admin",
		nil,
		model.PlatformManual,
		false,
		false,
		false,
		false,
		false,
		false,
		"",
		"",
		0,
		0,
		false,
		nil,
		nil,
		false,
		true,
		0,
		"",
		"",
		nil,
	)
	if err := database.GetInstance().SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	return template
}

func setupPoolTestDB(t *testing.T) {
	t.Helper()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	model.SetRoleCache(nil)
}

func TestPoolNamesAreScopedPerUser(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-scoped")

	userA := newPoolTestUser("pool-user-a")
	userB := newPoolTestUser("pool-user-b")

	poolA := model.NewPoolDefinition("shared-pool-name", template.Id, "", 1, userA.Id)
	poolA.Active = false
	if err := GetPoolService().Create(poolA, userA); err != nil {
		t.Fatalf("Create poolA: %v", err)
	}

	poolB := model.NewPoolDefinition("shared-pool-name", template.Id, "", 1, userB.Id)
	poolB.Active = false
	if err := GetPoolService().Create(poolB, userB); err != nil {
		t.Fatalf("Create poolB with same name for different user: %v", err)
	}

	if _, err := database.GetInstance().GetPoolDefinitionByName(userA.Id, poolA.Name); err != nil {
		t.Fatalf("GetPoolDefinitionByName userA: %v", err)
	}
	if _, err := database.GetInstance().GetPoolDefinitionByName(userB.Id, poolB.Name); err != nil {
		t.Fatalf("GetPoolDefinitionByName userB: %v", err)
	}
}

func TestPoolSetSizeAdjustsDesiredCount(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-update")
	user := newPoolTestUser("pool-user-update")

	pool := model.NewPoolDefinition(fmt.Sprintf("pool-update-%s", template.Id[:8]), template.Id, "", 1, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create pool: %v", err)
	}

	if err := GetPoolService().SetSize(pool, 3, user); err != nil {
		t.Fatalf("SetSize pool: %v", err)
	}

	after, err := database.GetInstance().GetPoolDefinition(pool.Id)
	if err != nil {
		t.Fatalf("GetPoolDefinition: %v", err)
	}
	if after.Name != pool.Name {
		t.Fatalf("Name = %q, want %q", after.Name, pool.Name)
	}
	if after.TemplateId != template.Id {
		t.Fatalf("TemplateId = %q, want %q", after.TemplateId, template.Id)
	}
	if after.DesiredCount != 3 {
		t.Fatalf("DesiredCount = %d, want 3", after.DesiredCount)
	}
}

func TestPoolMembersSortedByOrdinal(t *testing.T) {
	pool := &model.PoolDefinition{Id: "p1", Name: "mypool"}
	spaces := []*model.Space{
		{Id: "c", PoolId: "p1", Name: "mypool-2"},
		{Id: "a", PoolId: "p1", Name: "mypool-0"},
		{Id: "x", PoolId: "other", Name: "mypool-0"},   // different pool, ignored
		{Id: "b", PoolId: "p1", Name: "mypool-1"},
		{Id: "d", PoolId: "p1", Name: "mypool-3", IsDeleted: true}, // deleted, ignored
	}
	members := poolMembers(pool, spaces)
	gotNames := []string{}
	for _, m := range members {
		gotNames = append(gotNames, m.Name)
	}
	if want := []string{"mypool-0", "mypool-1", "mypool-2"}; !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("names = %v, want %v", gotNames, want)
	}
}

func TestMemberOrdinal(t *testing.T) {
	cases := []struct {
		name     string
		poolName string
		spaceNm  string
		want     int
		wantOk   bool
	}{
		{"simple", "pool", "pool-0", 0, true},
		{"double digit", "pool", "pool-12", 12, true},
		{"dashed pool name", "my-pool", "my-pool-3", 3, true},
		{"renamed member", "pool", "something-else", 0, false},
		{"prefix only", "pool", "pool-", 0, false},
		{"non-numeric suffix", "pool", "pool-abc", 0, false},
		{"negative", "pool", "pool--1", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := memberOrdinal(tc.poolName, &model.Space{Name: tc.spaceNm})
			if got != tc.want || ok != tc.wantOk {
				t.Fatalf("memberOrdinal(%q, %q) = (%d, %v), want (%d, %v)", tc.poolName, tc.spaceNm, got, ok, tc.want, tc.wantOk)
			}
		})
	}
}

func TestNextPoolOrdinal(t *testing.T) {
	cases := []struct {
		name string
		ords []int
		want int
	}{
		{"empty", nil, 0},
		{"contiguous", []int{0, 1, 2}, 3},
		{"gap at start", []int{1, 2}, 0},
		{"gap in middle", []int{0, 1, 3}, 2},
		{"unordered with gap", []int{2, 0, 3}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var members []*model.Space
			for _, o := range tc.ords {
				members = append(members, &model.Space{Name: fmt.Sprintf("pool-%d", o)})
			}
			if got := nextPoolOrdinal("pool", members); got != tc.want {
				t.Fatalf("nextPoolOrdinal(%v) = %d, want %d", tc.ords, got, tc.want)
			}
		})
	}
}

// An un-numbered (manually renamed) member doesn't occupy a slot and sorts last.
func TestNextPoolOrdinalIgnoresUnnumbered(t *testing.T) {
	members := []*model.Space{
		{Name: "pool-0"},
		{Name: "renamed-by-user"},
		{Name: "pool-1"},
	}
	if got := nextPoolOrdinal("pool", members); got != 2 {
		t.Fatalf("nextPoolOrdinal = %d, want 2", got)
	}
}

func TestPoolMemberNamesUseOrdinals(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-ordinals")
	user := newPoolTestUser("pool-user-ordinals")

	pool := model.NewPoolDefinition("ordinal-pool", template.Id, "", 3, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create pool: %v", err)
	}

	members := poolMemberSpaces(t, pool)
	if len(members) != 3 {
		t.Fatalf("got %d members, want 3", len(members))
	}
	for i, m := range members {
		wantName := fmt.Sprintf("ordinal-pool-%d", i)
		if m.Name != wantName {
			t.Errorf("member %d name = %q, want %q", i, m.Name, wantName)
		}
	}
}

// Deleting the lowest member frees ordinal 0; the next create reuses it rather
// than allocating a fresh higher number.
func TestPoolCreateReusesLowestFreeOrdinal(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-reuse")
	user := newPoolTestUser("pool-user-reuse")

	pool := model.NewPoolDefinition("reuse-pool", template.Id, "", 3, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create pool: %v", err)
	}

	members := poolMemberSpaces(t, pool)
	zero := members[0]
	if ordinalOf(t, pool.Name, zero) != 0 {
		t.Fatalf("first member = %q, want reuse-pool-0", zero.Name)
	}
	// Mirror the real full-delete path: a number only frees once the member is
	// IsDeleted, and that always coincides with the name being released (renamed
	// to the space UUID). See service.deleteSpace / container helper.
	zero.IsDeleted = true
	zero.Name = zero.Id
	if err := database.GetInstance().SaveSpace(zero, []string{"IsDeleted", "Name"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	if err := GetPoolService().createPoolSpace(pool, user); err != nil {
		t.Fatalf("createPoolSpace: %v", err)
	}

	members = poolMemberSpaces(t, pool)
	if len(members) != 3 {
		t.Fatalf("got %d members, want 3", len(members))
	}
	if members[0].Name != "reuse-pool-0" {
		t.Fatalf("lowest member = %q, want reuse-pool-0 (number reused)", members[0].Name)
	}
	// Numbers stay contiguous 0,1,2 — no gap and no jump to 3.
	for i, m := range members {
		if got := ordinalOf(t, pool.Name, m); got != i {
			t.Fatalf("member %d = %q (number %d), want number %d", i, m.Name, got, i)
		}
	}
}

// Concurrent creates on the leader must never hand the same ordinal to two
// spaces — createMu serializes allocation.
func TestPoolConcurrentCreateUniqueOrdinals(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-concurrent")
	user := newPoolTestUser("pool-user-concurrent")

	pool := model.NewPoolDefinition("concurrent-pool", template.Id, "", 1, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create pool: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := GetPoolService().createPoolSpace(pool, user); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent createPoolSpace: %v", err)
	}

	members := poolMemberSpaces(t, pool)
	if len(members) != n+1 {
		t.Fatalf("got %d members, want %d", len(members), n+1)
	}
	for i, m := range members {
		if got := ordinalOf(t, pool.Name, m); got != i {
			t.Fatalf("member %d = %q (number %d), want contiguous %d", i, m.Name, got, i)
		}
	}
}
