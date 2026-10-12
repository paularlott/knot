package service

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// withCustomFields gives the fixture's template two fields, one with a
// default, and makes it active so members can be created.
func (f *reconcileFixture) withCustomFields(t *testing.T) {
	t.Helper()
	f.tmpl.Active = true
	f.tmpl.CustomFields = []model.TemplateCustomField{
		{Name: "region", Description: "Region"},
		{Name: "size", Description: "Size", Default: "small"},
	}
	if err := database.GetInstance().SaveTemplate(f.tmpl, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
}

func TestPoolMembersCreatedWithPoolCustomFields(t *testing.T) {
	f := newReconcileFixture(t, "cf-create")
	f.withCustomFields(t)
	pool := f.pool("p", true, 1)
	pool.CustomFields = []model.SpaceCustomField{{Name: "region", Value: "eu"}}

	f.svc.reconcile(pool, nil)

	members := poolMemberSpaces(t, pool)
	if len(members) != 1 {
		t.Fatalf("created %d members, want 1", len(members))
	}
	want := []model.SpaceCustomField{{Name: "region", Value: "eu"}, {Name: "size", Value: "small"}}
	if !slices.Equal(members[0].CustomFields, want) {
		t.Fatalf("member custom fields = %v, want %v", members[0].CustomFields, want)
	}
}

func TestPoolMemberStartPicksUpChangedCustomFields(t *testing.T) {
	f := newReconcileFixture(t, "cf-start")
	f.withCustomFields(t)
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, false, false, false) // stopped, no fields
	pool.CustomFields = []model.SpaceCustomField{{Name: "size", Value: "large"}}

	f.svc.reconcile(pool, []*model.Space{m})

	if f.fc.startedCount(m.Id) != 1 {
		t.Fatalf("reconcile should start the stopped member, got %d", f.fc.startedCount(m.Id))
	}
	saved, err := database.GetInstance().GetSpace(m.Id)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	want := []model.SpaceCustomField{{Name: "size", Value: "large"}}
	if !slices.Equal(saved.CustomFields, want) {
		t.Fatalf("member custom fields = %v, want %v", saved.CustomFields, want)
	}
}

func TestPoolCustomFieldsLeaveRunningMembersAlone(t *testing.T) {
	f := newReconcileFixture(t, "cf-running")
	f.withCustomFields(t)
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, true, false, false) // running, no fields
	pool.CustomFields = []model.SpaceCustomField{{Name: "region", Value: "us"}}

	f.svc.reconcile(pool, []*model.Space{m})

	saved, err := database.GetInstance().GetSpace(m.Id)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if len(saved.CustomFields) != 0 {
		t.Fatalf("running member custom fields changed to %v before a restart", saved.CustomFields)
	}
}

// blockingContainer holds every StartSpace until release is closed, like a
// slow container deploy.
type blockingContainer struct {
	*fakeContainer
	release chan struct{}
}

func (c *blockingContainer) StartSpace(space *model.Space, template *model.Template, user *model.User) error {
	<-c.release
	return c.fakeContainer.StartSpace(space, template, user)
}

// Creating an active pool returns once the members are saved; their
// containers deploy in the background, and the sweep leaves them alone
// meanwhile.
func TestPoolCreateStartsMembersInBackground(t *testing.T) {
	f := newReconcileFixture(t, "bg-start")
	f.tmpl.Active = true
	if err := database.GetInstance().SaveTemplate(f.tmpl, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	bc := &blockingContainer{fakeContainer: f.fc, release: make(chan struct{})}
	SetContainerService(bc)
	pool := f.pool("bg-"+uuid.NewString()[:8], true, 2)

	done := make(chan error, 1)
	go func() { done <- f.svc.Create(pool, f.user) }()
	select {
	case err := <-done:
		if err != nil {
			close(bc.release)
			t.Fatalf("Create: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(bc.release)
		t.Fatal("Create waited for the members to start")
	}

	members := poolMemberSpaces(t, pool)
	if len(members) != 2 {
		close(bc.release)
		t.Fatalf("created %d members, want 2", len(members))
	}
	f.svc.reconcile(pool, members)
	close(bc.release)

	deadline := time.Now().Add(5 * time.Second)
	for f.fc.startedTotal() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	for _, m := range members {
		if n := f.fc.startedCount(m.Id); n != 1 {
			t.Fatalf("member %s started %d times, want 1", m.Name, n)
		}
	}
}
