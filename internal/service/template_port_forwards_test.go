package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// Creating a space from a template with port forward wiring seeds the
// space's own forward list — the restore-on-start replay then connects the
// entries when the space boots.
func TestCreateSpaceSeedsTemplatePortForwards(t *testing.T) {
	restorePoolDeps(t)
	setServerZone(t, "wire-"+uuid.NewString()[:8])

	user := newPoolTestUser("wire-user-" + uuid.NewString()[:8])
	template := newPoolTestTemplate(t, "wire-tmpl-"+uuid.NewString()[:8])
	template.PortForwards = []model.PortForwardEntry{
		{LocalPort: 5432, Space: "paul--shared-pg", RemotePort: 5432},
		{LocalPort: 6379, Space: "cache-pool", RemotePort: 6379},
	}
	if err := database.GetInstance().SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	space := model.NewSpace("wired", "", user.Id, template.Id, "bash", &[]model.AltNameEntry{}, "z1", "", nil)
	if err := GetSpaceService().CreateSpace(space, user); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	stored, err := database.GetInstance().GetSpace(space.Id)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if len(stored.PortForwards) != 2 {
		t.Fatalf("space has %d seeded forwards, want 2: %+v", len(stored.PortForwards), stored.PortForwards)
	}
	if stored.PortForwards[0].Space != "paul--shared-pg" || stored.PortForwards[0].LocalPort != 5432 || stored.PortForwards[0].RemotePort != 5432 {
		t.Fatalf("first seeded entry mismatch: %+v", stored.PortForwards[0])
	}

	// The space owns a copy: later template edits must not propagate.
	template.PortForwards = []model.PortForwardEntry{
		{LocalPort: 1, Space: "other", RemotePort: 1},
	}
	if err := database.GetInstance().SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate update: %v", err)
	}
	stored, _ = database.GetInstance().GetSpace(space.Id)
	if len(stored.PortForwards) != 2 {
		t.Fatalf("template edit propagated to existing space: %+v", stored.PortForwards)
	}
}

func TestValidateTemplatePortForwards(t *testing.T) {
	valid := []model.PortForwardEntry{
		{LocalPort: 5432, Space: "db", RemotePort: 5432},
		{LocalPort: 5433, Space: "paul--shared-pg", RemotePort: 5432},
		{LocalPort: 5434, Space: "01a0eaa0-1f64-723a-a052-337b211bb07b", RemotePort: 5432},
	}
	if err := validateTemplatePortForwards(valid); err != nil {
		t.Fatalf("valid wiring rejected: %v", err)
	}
	if err := validateTemplatePortForwards(nil); err != nil {
		t.Fatalf("empty wiring rejected: %v", err)
	}

	cases := []struct {
		name     string
		forwards []model.PortForwardEntry
	}{
		{"bad port range", []model.PortForwardEntry{{LocalPort: 0, Space: "db", RemotePort: 80}}},
		{"bad remote port", []model.PortForwardEntry{{LocalPort: 80, Space: "db", RemotePort: 0}}},
		{"bad ref", []model.PortForwardEntry{{LocalPort: 80, Space: "paul--svc--x", RemotePort: 80}}},
		{"duplicate local port", []model.PortForwardEntry{
			{LocalPort: 80, Space: "a", RemotePort: 80},
			{LocalPort: 80, Space: "b", RemotePort: 80},
		}},
		{"too many entries", func() []model.PortForwardEntry {
			entries := make([]model.PortForwardEntry, 17)
			for i := range entries {
				entries[i] = model.PortForwardEntry{LocalPort: uint16(1000 + i), Space: "db", RemotePort: 80}
			}
			return entries
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateTemplatePortForwards(tc.forwards); err == nil {
				t.Fatal("invalid wiring accepted")
			}
		})
	}
}
