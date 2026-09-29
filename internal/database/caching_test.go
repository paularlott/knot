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

	getTemplateCalls       int
	getTemplateByNameCalls int
	saveTemplateCalls      int
	deleteTemplateCalls    int
	getPoolSpacesCalls     int
	saveSpaceCalls         int
	deleteSpaceCalls       int
	getPoolByNameCalls     int
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
			{Name: "db", Port: 5432, Protocol: "tcp", Public: true},
		},
	}

	got, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	// Mutate everything the caller could reach.
	got.Name = "mutated"
	got.Ports[0].Public = false
	inner.templates["t1"].Name = "changed-behind-cache"

	again, err := d.GetTemplate("t1")
	if err != nil {
		t.Fatalf("second GetTemplate: %v", err)
	}
	if again.Name != "web" {
		t.Fatalf("cache corrupted by caller mutation: name = %q", again.Name)
	}
	if !again.Ports[0].Public {
		t.Fatal("cache corrupted by caller mutation: port public flag lost")
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
