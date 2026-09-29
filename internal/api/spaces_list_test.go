package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// Non-admin space listing: an omitted user_id means the requester — the CLI
// and scriptling libraries call /api/spaces with no filter — while another
// user's id still returns nothing. Admins keep the unfiltered view.
func TestHandleGetSpacesImplicitSelfForNonAdmin(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-api-list",
	})

	limited := &model.Role{Id: "role-limited", Name: "limited"}
	manager := &model.Role{Id: "role-manager", Name: "manager", Permissions: []uint16{model.PermissionManageSpaces}}
	model.SetRoleCache([]*model.Role{limited, manager})
	t.Cleanup(func() { model.SetRoleCache(nil) })

	db := database.GetInstance()
	owner := &model.User{Id: "u-owner", Username: "owner", Email: "owner@test.local", Roles: []string{limited.Id}}
	managerUser := &model.User{Id: "u-manager", Username: "manager", Email: "manager@test.local", Roles: []string{manager.Id}}
	for _, u := range []*model.User{owner, managerUser} {
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatalf("SaveUser %s: %v", u.Username, err)
		}
	}

	space := model.NewSpace("web", "", owner.Id, "tmpl", "bash", &[]model.AltNameEntry{}, "z-api-list", "", nil)
	if err := db.SaveSpace(space, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	list := func(t *testing.T, user *model.User, query string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/spaces"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), "user", user))
		rec := httptest.NewRecorder()

		HandleGetSpaces(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %q as %s: status %d: %s", query, user.Username, rec.Code, rec.Body.String())
		}
		var out apiclient.SpaceInfoList
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return out.Count
	}

	// The regression: non-admin with no filter used to get an empty list.
	if n := list(t, owner, ""); n != 1 {
		t.Errorf("non-admin, no user_id: %d spaces, want 1 (own spaces)", n)
	}
	// Non-admin asking for someone else still gets nothing.
	if n := list(t, owner, "?user_id="+managerUser.Id); n != 0 {
		t.Errorf("non-admin, other user's id: %d spaces, want 0", n)
	}
	// Non-admin asking for themselves explicitly is unchanged.
	if n := list(t, owner, "?user_id="+owner.Id); n != 1 {
		t.Errorf("non-admin, own id: %d spaces, want 1", n)
	}
	// Admins keep listing any user's spaces with an explicit filter. (The
	// unfiltered admin view isn't asserted here: other tests in this
	// package share the database and leave spaces whose users don't
	// resolve, which the response builder treats as an error.)
	if n := list(t, managerUser, "?user_id="+owner.Id); n != 1 {
		t.Errorf("admin, explicit user_id: %d spaces, want 1", n)
	}
}
