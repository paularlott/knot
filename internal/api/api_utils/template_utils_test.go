package api_utils

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// TestTemplateDetailsRoundTripsVNC guards the template edit round-trip: the
// details response must carry with_vnc, or the form re-reads the flag as
// false and the next save silently turns the VM display off.
func TestTemplateDetailsRoundTripsVNC(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	user := model.NewUser("tmpl-details", "details@example.com", "password", nil, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	template := model.NewTemplate("kvm-vnc-roundtrip", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	template.WithVNC = true
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}

	data, err := GetTemplateDetails(template.Id, user)
	if err != nil {
		t.Fatal(err)
	}
	if !data.WithVNC {
		t.Fatal("template details lost with_vnc — the edit form would show the VM display disabled")
	}
}
