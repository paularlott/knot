package configwizard

import (
	"os"
	"path/filepath"
	"testing"
)

// Audit settings prefer the [server.audit] section; the older flat
// server.audit_* keys still apply when the section is absent.
func TestPrefillAuditSectionOverridesFlatKeys(t *testing.T) {
	write := func(content string) string {
		path := filepath.Join(t.TempDir(), "knot.toml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Nested section wins over both the flat keys and the defaults.
	form := FormFromConfig(DefaultForm(), write(`
[server]
audit_routing = "internal"
audit_retention = 30

[server.audit]
routing = "external"
retention = 7
file_operations = true
space_sessions = true
`))
	if form.AuditRouting != "external" || form.AuditRetention != 7 {
		t.Errorf("[server.audit] should win, got routing=%q retention=%d", form.AuditRouting, form.AuditRetention)
	}
	if !form.AuditFileOps || !form.AuditSessions {
		t.Errorf("nested gates should prefill, got file_ops=%v sessions=%v", form.AuditFileOps, form.AuditSessions)
	}

	// Flat keys still work on their own.
	form = FormFromConfig(DefaultForm(), write(`
[server]
audit_routing = "both"
audit_file_operations = true
`))
	if form.AuditRouting != "both" {
		t.Errorf("flat audit_routing should still prefill, got %q", form.AuditRouting)
	}
	if !form.AuditFileOps {
		t.Error("flat audit_file_operations should still prefill")
	}
}
