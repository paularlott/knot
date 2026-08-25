package agent_server

import (
	"testing"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// SSH auth attempts are audited when server.audit.space_sessions is on,
// carrying the outcome, key fingerprint, source IP and the space's template.
func TestAuditSSHAuthGated(t *testing.T) {
	var entries []*model.AuditLogEntry
	model.AuditHook = func(e *model.AuditLogEntry) { entries = append(entries, e) }
	t.Cleanup(func() { model.AuditHook = nil })

	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		Audit:    config.AuditConfig{Routing: "external"},
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()
	template := &model.Template{Id: "tpl-1", Name: "python-3.12"}
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSpace(&model.Space{Id: "space-a", Name: "web", TemplateId: "tpl-1"}, nil); err != nil {
		t.Fatal(err)
	}

	session := &Session{Id: "space-a", SpaceName: "web", Username: "alice"}
	failure := msg.SSHAuthResultMessage{Success: false, KeyFingerprint: "SHA256:bad", RemoteAddr: "203.0.113.9:51422"}

	// Gate off: nothing.
	session.auditSSHAuth(failure)
	if len(entries) != 0 {
		t.Fatalf("session auditing is off by default, got %d entries", len(entries))
	}

	config.GetServerConfig().Audit.SpaceSessions = true

	// Failed attempt.
	session.auditSSHAuth(failure)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Event != model.AuditEventSpaceSessionOpen || e.Actor != "alice" {
		t.Errorf("unexpected entry: %+v", e)
	}
	if e.Properties["auth"] != "failed" || e.Properties["key"] != "SHA256:bad" || e.Properties["source_ip"] != "203.0.113.9" {
		t.Errorf("entry should carry outcome, key and source ip: %v", e.Properties)
	}
	if e.Properties["template"] != "python-3.12" {
		t.Errorf("entry should carry the space's template: %v", e.Properties)
	}

	// Successful attempt.
	session.auditSSHAuth(msg.SSHAuthResultMessage{Success: true, KeyFingerprint: "SHA256:good", RemoteAddr: "203.0.113.9:51423"})
	if entries[1].Properties["auth"] != "success" {
		t.Errorf("success should be recorded: %v", entries[1].Properties)
	}
}
