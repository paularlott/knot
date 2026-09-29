package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// countingDriver is a minimal stand-in for a real driver: only the methods
// the caching layer overrides are implemented, each counting calls. Any
// other method panics via the nil embedded interface, which no test here
// should hit.
type countingDriver struct {
	DbDriver

	templates   map[string]*model.Template
	spaces      map[string]*model.Space
	poolsByName map[string]*model.PoolDefinition

	// Users are pluggable: tests install funcs to vary responses over time
	// (renames, refreshes) and the counters track cache effectiveness.
	getUser             func(id string) (*model.User, error)
	getUserByUsernameFn func(username string) (*model.User, error)

	getTemplateCalls       int
	getTemplateByNameCalls int
	saveTemplateCalls      int
	deleteTemplateCalls    int
	getPoolSpacesCalls     int
	saveSpaceCalls         int
	deleteSpaceCalls       int
	getPoolByNameCalls     int
	getUserCalls           int
	getUserByUsernameCalls int
}

func newCountingDriver() *countingDriver {
	return &countingDriver{
		templates:   make(map[string]*model.Template),
		spaces:      make(map[string]*model.Space),
		poolsByName: make(map[string]*model.PoolDefinition),
	}
}

func (d *countingDriver) GetTemplate(id string) (*model.Template, error) {
	d.getTemplateCalls++
	t, ok := d.templates[id]
	if !ok {
		return nil, fmt.Errorf("template not found")
	}
	return t, nil
}

func (d *countingDriver) GetTemplateByName(name string) (*model.Template, error) {
	d.getTemplateByNameCalls++
	for _, t := range d.templates {
		if t.Name == name {
			return t, nil
		}
	}
	return nil, fmt.Errorf("template not found")
}

func (d *countingDriver) SaveTemplate(template *model.Template, updateFields []string) error {
	d.saveTemplateCalls++
	stored := *template
	d.templates[template.Id] = &stored
	return nil
}

func (d *countingDriver) DeleteTemplate(template *model.Template) error {
	d.deleteTemplateCalls++
	delete(d.templates, template.Id)
	return nil
}

func (d *countingDriver) GetSpacesByPoolId(poolId string) ([]*model.Space, error) {
	d.getPoolSpacesCalls++
	var spaces []*model.Space
	for _, s := range d.spaces {
		if s.PoolId == poolId {
			copy := *s
			spaces = append(spaces, &copy)
		}
	}
	return spaces, nil
}

func (d *countingDriver) SaveSpace(space *model.Space, updateFields []string) error {
	d.saveSpaceCalls++
	stored := *space
	d.spaces[space.Id] = &stored
	return nil
}

func (d *countingDriver) DeleteSpace(space *model.Space) error {
	d.deleteSpaceCalls++
	delete(d.spaces, space.Id)
	return nil
}

func (d *countingDriver) GetPoolDefinitionByName(userId, name string) (*model.PoolDefinition, error) {
	d.getPoolByNameCalls++
	pool, ok := d.poolsByName[userId+"/"+name]
	if !ok {
		return nil, fmt.Errorf("pool not found")
	}
	stored := *pool
	return &stored, nil
}

func (d *countingDriver) GetUser(id string) (*model.User, error) {
	d.getUserCalls++
	if d.getUser != nil {
		return d.getUser(id)
	}
	return nil, fmt.Errorf("user not found")
}

func (d *countingDriver) GetUserByUsername(username string) (*model.User, error) {
	d.getUserByUsernameCalls++
	if d.getUserByUsernameFn != nil {
		return d.getUserByUsernameFn(username)
	}
	return nil, fmt.Errorf("user not found")
}

func (d *countingDriver) SaveUser(user *model.User, updateFields []string) error {
	return nil
}

func (d *countingDriver) DeleteUser(user *model.User) error {
	return nil
}

func (d *countingDriver) SavePoolDefinition(pool *model.PoolDefinition, updateFields []string) error {
	stored := *pool
	d.poolsByName[pool.CreatedUserId+"/"+pool.Name] = &stored
	return nil
}

func (d *countingDriver) DeletePoolDefinition(pool *model.PoolDefinition) error {
	delete(d.poolsByName, pool.CreatedUserId+"/"+pool.Name)
	return nil
}

func testTemplate(id, name string) *model.Template {
	return &model.Template{Id: id, Name: name}
}

func newTestCachingDriver(t *testing.T) (*cachingDriver, *countingDriver, *time.Time) {
	t.Helper()
	inner := newCountingDriver()
	d := newCachingDriver(inner)
	current := time.Now()
	d.now = func() time.Time { return current }
	return d, inner, &current
}

func TestCachingTemplateReadThrough(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.templates["t1"] = testTemplate("t1", "web")

	first, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("first GetTemplate: %v", err)
	}
	second, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("second GetTemplate: %v", err)
	}
	if inner.getTemplateCalls != 1 {
		t.Fatalf("inner GetTemplate called %d times, want 1", inner.getTemplateCalls)
	}
	if first.Id != "t1" || second.Id != "t1" {
		t.Fatal("cached template mismatch")
	}
}

func TestCachingTemplateCopyOnRead(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.templates["t1"] = &model.Template{
		Id:   "t1",
		Name: "web",
		Ports: []model.TemplatePort{
			{Name: "db", Port: 5432, Protocol: "shared"},
		},
	}

	got, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	// Mutate everything the caller could reach.
	got.Name = "mutated"
	got.Ports[0].Protocol = "tcp"
	inner.templates["t1"].Name = "changed-behind-cache"

	again, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("second GetTemplate: %v", err)
	}
	if again.Name != "web" {
		t.Fatalf("cache corrupted by caller mutation: name = %q", again.Name)
	}
	if again.Ports[0].Protocol != "shared" {
		t.Fatalf("cache corrupted by caller mutation: protocol = %q", again.Ports[0].Protocol)
	}
}

func TestCachingTemplateEvictOnSave(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.templates["t1"] = testTemplate("t1", "web")

	if _, err := d.GetTemplate("t1"); err != nil {
		t.Fatalf("first read: %v", err)
	}

	updated := testTemplate("t1", "web-v2")
	inner.templates["t1"] = updated
	if err := d.SaveTemplate(updated, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	got, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("read after save: %v", err)
	}
	if got.Name != "web-v2" {
		t.Fatalf("stale template served after save: %q", got.Name)
	}
	if inner.getTemplateCalls != 2 {
		t.Fatalf("inner GetTemplate called %d times, want 2", inner.getTemplateCalls)
	}
}

func TestCachingTemplateNameIndexClearedOnSave(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.templates["t1"] = testTemplate("t1", "web")

	if _, err := d.GetTemplateByName("web"); err != nil {
		t.Fatalf("read by name: %v", err)
	}
	if inner.getTemplateByNameCalls != 1 {
		t.Fatalf("inner GetTemplateByName calls = %d, want 1", inner.getTemplateByNameCalls)
	}

	// A save (here: a rename of another template) must clear the name
	// index so the next by-name read goes back to the driver.
	if err := d.SaveTemplate(testTemplate("t2", "other"), nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	inner.templates["t1"] = testTemplate("t1", "renamed")
	if _, err := d.GetTemplateByName("web"); err == nil {
		t.Fatal("by-name read served from stale name index after save")
	}
	if _, err := d.GetTemplateByName("renamed"); err != nil {
		t.Fatalf("read by new name: %v", err)
	}
}

func TestCachingTemplateTTLClearsEntry(t *testing.T) {
	d, inner, now := newTestCachingDriver(t)
	inner.templates["t1"] = testTemplate("t1", "web")

	if _, err := d.GetTemplate("t1"); err != nil {
		t.Fatalf("first read: %v", err)
	}

	*now = now.Add(cacheTTL + time.Second)
	inner.templates["t1"] = testTemplate("t1", "web-v2")

	got, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("read after ttl: %v", err)
	}
	if got.Name != "web-v2" {
		t.Fatalf("expired entry served: %q", got.Name)
	}
	if inner.getTemplateCalls != 2 {
		t.Fatalf("inner GetTemplate called %d times, want 2", inner.getTemplateCalls)
	}
}

func TestCachingPoolMembers(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	member := &model.Space{Id: "s1", Name: "m1", PoolId: "p1"}
	inner.spaces["s1"] = member

	if _, err := d.GetSpacesByPoolId("p1"); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := d.GetSpacesByPoolId("p1"); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if inner.getPoolSpacesCalls != 1 {
		t.Fatalf("inner GetSpacesByPoolId called %d times, want 1", inner.getPoolSpacesCalls)
	}

	// Saving a member with its PoolId set must invalidate the pool list.
	member.LeaseId = "lease-1"
	if err := d.SaveSpace(member, []string{"LeaseId"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	if _, err := d.GetSpacesByPoolId("p1"); err != nil {
		t.Fatalf("read after save: %v", err)
	}
	if inner.getPoolSpacesCalls != 2 {
		t.Fatalf("pool list not invalidated by SaveSpace: %d inner calls", inner.getPoolSpacesCalls)
	}

	// A partial save that doesn't carry PoolId must still invalidate via
	// the reverse index.
	partial := &model.Space{Id: "s1"} // no PoolId
	if err := d.SaveSpace(partial, []string{"LeaseId"}); err != nil {
		t.Fatalf("partial SaveSpace: %v", err)
	}
	if _, err := d.GetSpacesByPoolId("p1"); err != nil {
		t.Fatalf("read after partial save: %v", err)
	}
	if inner.getPoolSpacesCalls != 3 {
		t.Fatalf("pool list not invalidated by partial SaveSpace: %d inner calls", inner.getPoolSpacesCalls)
	}
}

func TestCachingPoolMembersCopyOnRead(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.spaces["s1"] = &model.Space{Id: "s1", Name: "m1", PoolId: "p1", LeaseId: ""}

	got, err := d.GetSpacesByPoolId("p1")
	if err != nil || len(got) != 1 {
		t.Fatalf("read: %v %v", got, err)
	}
	got[0].Name = "mutated"

	again, err := d.GetSpacesByPoolId("p1")
	if err != nil || len(again) != 1 {
		t.Fatalf("second read: %v %v", again, err)
	}
	if again[0].Name != "m1" {
		t.Fatalf("pool cache corrupted by caller mutation: %q", again[0].Name)
	}
}

func TestCachingPoolDefinitionByName(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	pool := &model.PoolDefinition{Id: "p1", Name: "pg", CreatedUserId: "u1"}
	if err := inner.SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("seed pool: %v", err)
	}

	if _, err := d.GetPoolDefinitionByName("u1", "pg"); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := d.GetPoolDefinitionByName("u1", "pg"); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if inner.getPoolByNameCalls != 1 {
		t.Fatalf("inner GetPoolDefinitionByName called %d times, want 1", inner.getPoolByNameCalls)
	}

	pool.DesiredCount = 3
	if err := d.SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	got, err := d.GetPoolDefinitionByName("u1", "pg")
	if err != nil {
		t.Fatalf("read after save: %v", err)
	}
	if got.DesiredCount != 3 {
		t.Fatalf("stale pool definition served: %d", got.DesiredCount)
	}
}

func testUser(id, username string) *model.User {
	return &model.User{Id: id, Username: username, Email: username + "@test.local"}
}

func TestCachingUserReadThroughAndCopySafety(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	inner.getUser = func(id string) (*model.User, error) { return testUser("u1", "alice"), nil }
	inner.getUserByUsernameFn = func(username string) (*model.User, error) { return testUser("u1", "alice"), nil }

	// First read by name (miss → driver, fills both indexes), then by id:
	// a hit that must not touch the driver again.
	first, err := d.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	second, err := d.GetUser("u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	third, err := d.GetUser("u1")
	if err != nil {
		t.Fatalf("second GetUser: %v", err)
	}
	if inner.getUserCalls != 0 {
		t.Fatalf("inner GetUser called %d times, want 0 (name read fills the id index)", inner.getUserCalls)
	}
	if inner.getUserByUsernameCalls != 1 {
		t.Fatalf("inner GetUserByUsername called %d times, want 1", inner.getUserByUsernameCalls)
	}
	if first.Id != "u1" || second.Id != "u1" || third.Id != "u1" {
		t.Fatal("cache returned wrong user")
	}

	// Caller mutation must not corrupt the cache.
	first.Username = "mutated"
	got, _ := d.GetUser("u1")
	if got.Username != "alice" {
		t.Fatalf("cache corrupted by caller mutation: %q", got.Username)
	}
}

func TestCachingUserSlidingIdleWindow(t *testing.T) {
	d, inner, now := newTestCachingDriver(t)
	calls := 0
	inner.getUser = func(id string) (*model.User, error) {
		calls++
		return testUser(id, "alice"), nil
	}

	if _, err := d.GetUser("u1"); err != nil {
		t.Fatalf("first read: %v", err)
	}

	// Keep the user busy for far longer than one idle window: reads at
	// half-window intervals must keep the entry alive without re-fetching.
	for i := 0; i < 6; i++ {
		*now = now.Add(userIdleTTL / 2)
		if _, err := d.GetUser("u1"); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if calls != 1 {
		t.Fatalf("active user re-fetched: %d inner calls, want 1", calls)
	}

	// Now let it go quiet past one full idle window: the entry drops and
	// the next read goes back to the driver.
	*now = now.Add(userIdleTTL + time.Second)
	if _, err := d.GetUser("u1"); err != nil {
		t.Fatalf("read after idle: %v", err)
	}
	if calls != 2 {
		t.Fatalf("idle user not evicted: %d inner calls, want 2", calls)
	}
}

func TestCachingUserAbsoluteCap(t *testing.T) {
	d, inner, now := newTestCachingDriver(t)
	calls := 0
	inner.getUser = func(id string) (*model.User, error) {
		calls++
		u := testUser(id, "alice")
		if calls > 1 {
			u.Username = "refreshed"
		}
		return u, nil
	}

	if _, err := d.GetUser("u1"); err != nil {
		t.Fatalf("first read: %v", err)
	}

	// Read constantly so the idle window never lapses, but cross the
	// absolute lifetime cap: the entry must refresh regardless.
	for i := 0; i < 20; i++ {
		*now = now.Add(userMaxLifetime / 10)
		if _, err := d.GetUser("u1"); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if calls < 2 {
		t.Fatal("permanently-busy user never re-fetched past the absolute cap")
	}
	got, _ := d.GetUser("u1")
	if got.Username != "refreshed" {
		t.Fatalf("stale user served past the absolute cap: %q", got.Username)
	}
}

func TestCachingUserEvictOnSave(t *testing.T) {
	d, inner, _ := newTestCachingDriver(t)
	current := testUser("u1", "alice")
	inner.getUser = func(id string) (*model.User, error) { return current, nil }
	inner.getUserByUsernameFn = func(username string) (*model.User, error) {
		if username == current.Username {
			return current, nil
		}
		return nil, fmt.Errorf("user not found")
	}

	if _, err := d.GetUserByUsername("alice"); err != nil {
		t.Fatalf("read by name: %v", err)
	}

	// A save (here: a rename) must drop the id entry and the whole name
	// index so the next reads go back to the driver.
	current = testUser("u1", "alicia")
	if err := d.SaveUser(current, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	got, err := d.GetUser("u1")
	if err != nil || got.Username != "alicia" {
		t.Fatalf("stale user after save: %v %+v", err, got)
	}
	if got, err := d.GetUserByUsername("alicia"); err != nil || got.Username != "alicia" {
		t.Fatalf("new name not served after save: %v %+v", err, got)
	}
}
