//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestAuditLogRecordsActivity(t *testing.T) {
	harness.Feature(t, "audit-log")

	// Generate a distinctive audited action: create a space record without
	// starting it (no container needed; also stays within the pro build's
	// two-user tier).
	name := uniqueName("it-audit-space")
	ctx, cancel := testCtx(30)
	defer cancel()
	spaceId, code, err := admin.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name: name, TemplateId: templateId, UserId: admin.Id, Shell: "bash",
	})
	if err != nil {
		t.Fatalf("create audit space: %v (status %d)", err, code)
	}
	defer func() {
		dctx, dcancel := testCtx(15)
		admin.Client.DeleteSpace(dctx, spaceId)
		dcancel()
	}()

	// The audit log should contain the creation event.
	deadline := time.Now().Add(15 * time.Second)
	var found bool
	for time.Now().Before(deadline) && !found {
		ctx, cancel := testCtx(15)
		logs, code, err := admin.Client.GetAuditLogs(ctx, 0, 200, &apiclient.AuditLogFilter{
			Event: "Space Create",
		})
		cancel()
		if err != nil {
			t.Fatalf("get audit logs: %v (status %d)", err, code)
		}
		for _, entry := range logs.Items {
			if entry.Actor == "admin" && contains(entry.Details, name) {
				found = true
				break
			}
		}
		if !found {
			time.Sleep(1 * time.Second)
		}
	}
	if !found {
		t.Fatal("space creation not found in audit log (event Space Create, actor admin)")
	}

	// Filter by actor returns only that actor's entries.
	ctx, cancel = testCtx(15)
	defer cancel()
	logs, _, err := admin.Client.GetAuditLogs(ctx, 0, 50, &apiclient.AuditLogFilter{Actor: "admin"})
	if err != nil {
		t.Fatalf("get audit logs by actor: %v", err)
	}
	if len(logs.Items) == 0 {
		t.Fatal("actor filter returned no entries")
	}
	for _, entry := range logs.Items {
		if entry.Actor != "admin" {
			t.Fatalf("actor filter leaked entry with actor %q", entry.Actor)
		}
	}
}
