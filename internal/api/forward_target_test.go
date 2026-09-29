package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// The forward-target check backs the agent-side pre-flight for in-space
// port forward requests: same answer as the proxy's per-connection rule.
func TestHandleForwardTargetCheck(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-fwd-check",
	})

	model.SetRoleCache(nil)

	db := database.GetInstance()
	owner := &model.User{Id: "u-owner", Username: "owner", Email: "owner@test.local"}
	other := &model.User{Id: "u-other", Username: "other", Email: "other@test.local"}
	for _, u := range []*model.User{owner, other} {
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatalf("SaveUser %s: %v", u.Username, err)
		}
	}

	template := &model.Template{
		Id:       uuid.NewString(),
		Name:     "fwd-check-tmpl-" + uuid.NewString()[:8],
		Active:   true,
		Platform: model.PlatformManual,
		Ports: []model.TemplatePort{
			{Name: "svc", Port: 5432, Protocol: "shared"},
			{Name: "priv", Port: 8080, Protocol: "tcp"},
		},
	}
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	space := model.NewSpace("svc", "", owner.Id, template.Id, "bash", &[]model.AltNameEntry{}, "z-fwd-check", "", nil)
	if err := db.SaveSpace(space, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	check := func(t *testing.T, user *model.User, query string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/forward-target"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), "user", user))
		rec := httptest.NewRecorder()

		HandleForwardTargetCheck(rec, req)

		var body ErrorResponse
		message := ""
		if json.Unmarshal(rec.Body.Bytes(), &body) == nil {
			message = body.Error
		}
		return rec.Code, message
	}

	// Owner: any port on their own space.
	if code, msg := check(t, owner, "?target=svc&port=8080"); code != http.StatusOK {
		t.Fatalf("owner any port: %d %s", code, msg)
	}
	// Another user: shared port allowed.
	if code, msg := check(t, other, "?target=owner--svc&port=5432"); code != http.StatusOK {
		t.Fatalf("shared port: %d %s", code, msg)
	}
	// Another user: non-shared port denied with the readable message.
	if code, msg := check(t, other, "?target=owner--svc&port=8080"); code != http.StatusForbidden || msg != "port 8080 is not a shared port on space svc" {
		t.Fatalf("non-shared port: %d %q", code, msg)
	}
	// Unknown target.
	if code, _ := check(t, other, "?target=owner--nope&port=5432"); code != http.StatusNotFound {
		t.Fatalf("unknown target: %d", code)
	}
	// Malformed input.
	if code, _ := check(t, other, "?target=paul--svc--x&port=5432"); code != http.StatusBadRequest {
		t.Fatalf("malformed target: %d", code)
	}
	if code, _ := check(t, other, "?target=svc&port=99999"); code != http.StatusBadRequest {
		t.Fatalf("bad port: %d", code)
	}
}
