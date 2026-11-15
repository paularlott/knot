package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// The Admin role is built in and never stored in the database, so a lookup
// by id must be served from the role cache rather than 404.
func TestHandleGetRoleBuiltinAdmin(t *testing.T) {
	model.SetRoleCache(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/roles/"+model.RoleAdminUUID, nil)
	req.SetPathValue("role_id", model.RoleAdminUUID)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	HandleGetRole(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var got apiclient.RoleDetails
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Id != model.RoleAdminUUID || got.Name != "Admin" || len(got.Permissions) == 0 {
		t.Errorf("unexpected role: %+v", got)
	}
}

func TestHandleUpdateDeleteBuiltinAdminRefused(t *testing.T) {
	model.SetRoleCache(nil)

	del := httptest.NewRequest(http.MethodDelete, "/api/roles/"+model.RoleAdminUUID, nil)
	del.SetPathValue("role_id", model.RoleAdminUUID)
	w := httptest.NewRecorder()
	HandleDeleteRole(w, del)
	if w.Code != http.StatusForbidden {
		t.Errorf("delete status = %d, want 403", w.Code)
	}
}
