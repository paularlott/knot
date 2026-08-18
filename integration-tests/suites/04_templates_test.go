//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

func TestTemplatesCRUD(t *testing.T) {
	harness.Feature(t, "templates")
	ctx, cancel := testCtx(30)
	defer cancel()

	name := uniqueName("it-tmpl")
	id, err := harness.CreateTemplate(server, admin.Client, name, harness.TemplateOptions{
		ExtraEnv: []string{"IT_VAR=hello"},
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

	// Update description.
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
	}
	if code, err := admin.Client.UpdateTemplate(ctx, id, upd); err != nil {
		t.Fatalf("update template: %v (status %d)", err, code)
	}
	details, _, _ = admin.Client.GetTemplate(ctx, id)
	mustEqual(t, "description after update", details.Description, "updated description")

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
	// Import under a different name so a new template is created.
	importedId, code, err := admin.Client.ImportTemplate(ctx,
		strings.Replace(yamlText, "name: "+name, "name: "+name+"-imported", 1))
	if err != nil {
		t.Fatalf("import template: %v (status %d)", err, code)
	}
	if importedId == "" || importedId == id {
		t.Fatalf("import returned bad id %q", importedId)
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
