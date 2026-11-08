package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
)

// With file storage turned off its permissions mean nothing, so a role editor must not offer them.
func TestGetPermissionsHidesFileStorageWhenDisabled(t *testing.T) {
	if filestore.Get() != nil {
		t.Skip("file storage is enabled in this test process")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/permissions", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	HandleGetPermissions(rec, req)

	var list apiclient.PermissionInfoList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("bad response %q: %v", rec.Body.String(), err)
	}

	if list.Count != len(list.Permissions) {
		t.Errorf("count %d does not match the %d permissions listed", list.Count, len(list.Permissions))
	}
	fileStorage := 0
	for _, p := range model.PermissionNames {
		if p.Group == model.PermissionGroupFileStorage {
			fileStorage++
		}
	}
	if fileStorage == 0 {
		t.Fatal("expected some file storage permissions to exist")
	}
	if want := len(model.PermissionNames) - fileStorage; len(list.Permissions) != want {
		t.Errorf("got %d permissions, want %d", len(list.Permissions), want)
	}
	for _, p := range list.Permissions {
		if p.Group == model.PermissionGroupFileStorage {
			t.Errorf("file storage permission %q offered while file storage is off", p.Name)
		}
	}
}
