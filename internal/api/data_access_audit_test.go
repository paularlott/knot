package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

func setupDataAccessAudit(t *testing.T) *[]*model.AuditLogEntry {
	t.Helper()

	var entries []*model.AuditLogEntry
	model.AuditHook = func(e *model.AuditLogEntry) { entries = append(entries, e) }
	t.Cleanup(func() { model.AuditHook = nil })

	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		Audit:    config.AuditConfig{Routing: "external"},
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	// Handlers gossip tokens; a no-op transport keeps that off the network.
	prevT := service.GetTransport()
	service.SetTransport(nopTransport{})
	t.Cleanup(func() { service.SetTransport(prevT) })

	return &entries
}

type nopTransport struct{ service.Transport }

func (nopTransport) GossipToken(*model.Token)            {}
func (nopTransport) GossipAuditLog(*model.AuditLogEntry) {}

func fileOpRequest(user *model.User, spaceId string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/spaces/"+spaceId+"/files/read", nil)
	r.SetPathValue("space_id", spaceId)
	return r.WithContext(context.WithValue(r.Context(), "user", user))
}

// File operation auditing is gated: nothing when off, path and byte count
// (never content) when on.
func TestSpaceFileOpAuditGated(t *testing.T) {
	entries := setupDataAccessAudit(t)

	db := database.GetInstance()
	space := &model.Space{Id: "space-a", Name: "web", TemplateId: "tpl-1"}
	if err := db.SaveSpace(space, nil); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "alice"}

	auditSpaceFileOp(fileOpRequest(user, "space-a"), "read", "/data/customer-dump.sql", true, 4096)
	if len(*entries) != 0 {
		t.Fatalf("file op auditing is off by default, got %d entries", len(*entries))
	}

	cfg := config.GetServerConfig()
	cfg.Audit.FileOperations = true

	auditSpaceFileOp(fileOpRequest(user, "space-a"), "read", "/data/customer-dump.sql", true, 4096)
	if len(*entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(*entries))
	}
	e := (*entries)[0]
	if e.Event != model.AuditEventSpaceFileOp || e.Actor != "alice" {
		t.Errorf("unexpected entry: %+v", e)
	}
	props := e.Properties
	if props["op"] != "read" || props["path"] != "/data/customer-dump.sql" || props["size"] != int64(4096) {
		t.Errorf("entry should carry op, path and size: %v", props)
	}
	if props["space_name"] != "web" {
		t.Errorf("entry should carry the space name: %v", props)
	}
	if _, hasContent := props["content"]; hasContent {
		t.Error("file contents must never enter the audit trail")
	}
}

// Token lifecycle auditing is always on — these are credentials.
func TestTokenCreateAudited(t *testing.T) {
	entries := setupDataAccessAudit(t)

	user := &model.User{Id: "user-1", Username: "alice"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/tokens", strings.NewReader(`{"name":"ci-token"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "user", user))
	rec := httptest.NewRecorder()

	HandleCreateToken(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("token create returned %d: %s", rec.Code, rec.Body.String())
	}
	if len(*entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(*entries))
	}
	e := (*entries)[0]
	if e.Event != model.AuditEventTokenCreate || e.Actor != "alice" {
		t.Errorf("unexpected entry: %+v", e)
	}
	if e.Properties == nil || e.Properties["token_name"] != "ci-token" {
		t.Errorf("entry should carry the token name: %v", e.Properties)
	}
}
