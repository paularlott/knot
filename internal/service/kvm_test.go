package service

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// TestValidateKvmSpaceAddressZoneScoped checks that IP uniqueness is enforced
// within the local zone only: another zone's bridged network may reuse the
// same address without blocking space creation here.
func TestValidateKvmSpaceAddressZoneScoped(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		Zone:     "zone-a",
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	template := &model.Template{
		Platform:        model.PlatformKvm,
		KvmNetworkMode:  model.KvmNetworkModeBridged,
		KvmNetworkCidr:  "192.0.2.0/24",
		KvmIPRangeStart: "192.0.2.10",
		KvmIPRangeEnd:   "192.0.2.20",
	}

	// A space in another zone holds the address — not a conflict here.
	other := model.NewSpace("other-zone", "", "user-1", "tmpl", "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	other.Zone = "zone-b"
	other.IPAddress = "192.0.2.10"
	if err := db.SaveSpace(other, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateKvmSpaceAddress(template, "192.0.2.10", ""); err != nil {
		t.Fatalf("expected an address taken in another zone to be usable: %v", err)
	}

	// A space in the local zone holds it — conflict.
	local := model.NewSpace("local-zone", "", "user-1", "tmpl", "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	local.Zone = "zone-a"
	local.IPAddress = "192.0.2.10"
	if err := db.SaveSpace(local, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateKvmSpaceAddress(template, "192.0.2.10", ""); err == nil {
		t.Fatal("expected an in-use error for a same-zone duplicate address")
	}
	if err := ValidateKvmSpaceAddress(template, "192.0.2.11", ""); err != nil {
		t.Fatalf("expected a free same-zone address to validate: %v", err)
	}
}
