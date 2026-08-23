//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/database/model"
)

func TestTemplatesCRUD(t *testing.T) {
	harness.Feature(t, "templates")
	ctx, cancel := testCtx(30)
	defer cancel()

	name := uniqueName("it-tmpl")
	id, err := harness.CreateTemplate(server, admin.Client, name, harness.TemplateOptions{
		ExtraEnv: []string{"IT_VAR=hello"},
		Jobs: []model.SpaceJob{
			{Name: "backup", Command: "knot run-script backup", Schedule: "0 2 * * *", Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	details, code, err := admin.Client.GetTemplate(ctx, id)
	if err != nil {
		t.Fatalf("get template: %v (status %d)", err, code)
	}
	mustEqual(t, "template name", details.Name, name)
	mustContain(t, "template job", details.Job, "KNOT_SERVER=")
	mustContain(t, "template job env", details.Job, "IT_VAR=hello")
	mustEqual(t, "platform", details.Platform, "container")

	// List.
	list, code, err := admin.Client.GetTemplates(ctx)
	if err != nil {
		t.Fatalf("list templates: %v (status %d)", err, code)
	}
	found := false
	for _, tpl := range list.Templates {
		if tpl.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("created template missing from list")
	}

	// Update description. The update is a full replace, so carry the
	// fields the test cares about (jobs) through the request.
	details.Description = "updated description"
	upd := &apiclient.TemplateUpdateRequest{
		Name:            details.Name,
		Job:             details.Job,
		Description:     details.Description,
		Volumes:         details.Volumes,
		Platform:        details.Platform,
		Active:          true,
		WithTerminal:    true,
		WithSSH:         true,
		WithRunCommand:  true,
		ComputeUnits:    2,
		StorageUnits:    2,
		MaxUptime:       0,
		MaxUptimeUnit:   "disabled",
		HealthCheckType: "none",
		Jobs:            details.Jobs,
	}
	if code, err := admin.Client.UpdateTemplate(ctx, id, upd); err != nil {
		t.Fatalf("update template: %v (status %d)", err, code)
	}
	details, _, _ = admin.Client.GetTemplate(ctx, id)
	mustEqual(t, "description after update", details.Description, "updated description")
	if len(details.Jobs) != 1 || details.Jobs[0].Name != "backup" {
		t.Fatalf("jobs after update mismatch: %+v", details.Jobs)
	}

	// By name.
	byName, err := admin.Client.GetTemplateByName(ctx, name)
	if err != nil {
		t.Fatalf("get template by name: %v", err)
	}
	mustEqual(t, "by-name id", byName.TemplateId, id)

	// Export / import round trip.
	yamlText, code, err := admin.Client.ExportTemplate(ctx, id)
	if err != nil {
		t.Fatalf("export template: %v (status %d)", err, code)
	}
	mustContain(t, "export yaml", yamlText, name)
	mustContain(t, "export yaml jobs", yamlText, "knot run-script backup")
	// Import under a different name so a new template is created.
	importedId, code, err := admin.Client.ImportTemplate(ctx,
		strings.Replace(yamlText, "name: "+name, "name: "+name+"-imported", 1))
	if err != nil {
		t.Fatalf("import template: %v (status %d)", err, code)
	}
	if importedId == "" || importedId == id {
		t.Fatalf("import returned bad id %q", importedId)
	}
	imported, code, err := admin.Client.GetTemplate(ctx, importedId)
	if err != nil {
		t.Fatalf("get imported template: %v (status %d)", err, code)
	}
	if len(imported.Jobs) != 1 || imported.Jobs[0].Name != "backup" || imported.Jobs[0].Schedule != "0 2 * * *" || !imported.Jobs[0].Enabled {
		t.Fatalf("imported template jobs mismatch: %+v", imported.Jobs)
	}
	admin.Client.DeleteTemplate(ctx, importedId)

	if code, err := admin.Client.DeleteTemplate(ctx, id); err != nil {
		t.Fatalf("delete template: %v (status %d)", err, code)
	}
	if _, _, err := admin.Client.GetTemplate(ctx, id); err == nil {
		t.Fatal("deleted template still readable")
	}
	// unused import guard
	_ = strings.TrimSpace
}

// TestTemplateExportAuthorization proves the export endpoint enforces the
// same visibility as the read path: a template restricted to a group the
// caller is not in 404s on both, while templates the caller can see — and
// everything for template managers — still export.
func TestTemplateExportAuthorization(t *testing.T) {
	harness.Feature(t, "templates")
	ctx, cancel := testCtx(30)
	defer cancel()

	groupName := uniqueName("it-tpl-grp")
	groupId, code, err := admin.Client.CreateGroup(ctx, &apiclient.GroupRequest{Name: groupName})
	if err != nil {
		t.Fatalf("create group: %v (status %d)", err, code)
	}
	t.Cleanup(func() {
		cctx, ccancel := testCtx(15)
		admin.Client.DeleteGroup(cctx, groupId)
		ccancel()
	})

	hiddenName := uniqueName("it-tpl-hidden")
	hiddenId, code, err := admin.Client.CreateTemplate(ctx, &apiclient.TemplateCreateRequest{
		Name:          hiddenName,
		Job:           harness.TemplateJobYAML(cfg, harness.TemplateOptions{}),
		Description:   "group-restricted template",
		Platform:      "container",
		Active:        true,
		Groups:        []string{groupId},
		MaxUptime:     0,
		MaxUptimeUnit: "disabled",
	})
	if err != nil {
		t.Fatalf("create group-restricted template: %v (status %d)", err, code)
	}
	t.Cleanup(func() {
		cctx, ccancel := testCtx(15)
		admin.Client.DeleteTemplate(cctx, hiddenId)
		ccancel()
	})

	// Sanity: the read path hides it from user1 (not in the group, no
	// manage permission).
	if _, _, err := user1.Client.GetTemplate(ctx, hiddenId); err == nil {
		t.Fatal("user1 read a group-restricted template")
	}

	// The export must not bypass that visibility — job YAML can carry
	// registry credentials and secret env entries.
	if _, code, err := user1.Client.ExportTemplate(ctx, hiddenId); err == nil || code != 404 {
		t.Fatalf("user1 exported a group-restricted template (status %d, err %v)", code, err)
	}

	// Template managers can export any template.
	if _, code, err := admin.Client.ExportTemplate(ctx, hiddenId); err != nil || code != 200 {
		t.Fatalf("admin export of restricted template: status %d, err %v", code, err)
	}

	// Templates the user can see still export.
	if _, code, err := user1.Client.ExportTemplate(ctx, templateId); err != nil || code != 200 {
		t.Fatalf("user1 export of a visible template: status %d, err %v", code, err)
	}
}
