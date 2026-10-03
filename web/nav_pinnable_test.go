package web

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
)

// Every core sidebar entry must be pinnable: the preferences endpoint drops
// URLs missing from apiclient.ValidNavURLs, so a missing entry makes its star
// silently do nothing.
func TestEveryCoreNavItemIsPinnable(t *testing.T) {
	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	u := adminUser(t)
	u.Active = true
	cfg := &config.ServerConfig{ListenTunnel: ":3000"}
	cfg.Cluster.AdvertiseAddr = "127.0.0.1:9000"

	top, more := buildNav(u, cfg, true)
	all := append(urls(top), urls(more)...)
	found := false
	for _, url := range all {
		if url == "/plugins" {
			continue // only present with plugins loaded; pinned via plugin menus
		}
		if !apiclient.ValidNavURLs[url] {
			t.Errorf("nav item %s cannot be pinned: add it to apiclient.ValidNavURLs", url)
		}
		if url == "/files" {
			found = true
		}
	}
	if !found {
		t.Error("Files missing from the sidebar with file storage enabled")
	}
}

// Files is shown to users who can own buckets or have one shared with them,
// and to everyone on a leaf node, whose buckets are local to it.
func TestFilesNavVisibility(t *testing.T) {
	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	prev := config.GetServerConfig()
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
		config.SetServerConfig(prev)
	})
	config.SetServerConfig(&config.ServerConfig{})

	admin := adminUser(t)
	admin.Active = true
	plain := &model.User{Id: "u2", Username: "plain", Active: true}
	hasFiles := func(u *model.User, cfg *config.ServerConfig) bool {
		top, _ := buildNav(u, cfg, false)
		for _, url := range urls(top) {
			if url == "/files" {
				return true
			}
		}
		return false
	}

	cfg := &config.ServerConfig{}
	if !hasFiles(admin, cfg) {
		t.Error("admin without Files")
	}
	if hasFiles(plain, cfg) {
		t.Error("user with no permission and no shares sees Files")
	}
	// Manage File Storage alone is enough.
	manageRole := model.NewRole("manage-files-only", []uint16{model.PermissionManageFiles}, "")
	model.SaveRoleToCache(manageRole)
	manager := &model.User{Id: "u4", Username: "manager", Active: true, Roles: []string{manageRole.Id}}
	if !hasFiles(manager, cfg) {
		t.Error("user with Manage File Storage lacks Files")
	}

	p, _ := filestore.PrincipalFor(admin)
	if _, err := store.CreateBucket(p, "admin--shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGrant(p, "admin--shared", filestore.Grant{Type: filestore.GrantUser, Id: "u2", Access: filestore.GrantRead}); err != nil {
		t.Fatal(err)
	}
	if !hasFiles(plain, cfg) {
		t.Error("user with a shared bucket lacks Files")
	}

	other := &model.User{Id: "u3", Username: "other", Active: true}
	leaf := &config.ServerConfig{LeafNode: true}
	config.SetServerConfig(leaf)
	if !hasFiles(other, leaf) {
		t.Error("leaf user lacks Files")
	}
}
