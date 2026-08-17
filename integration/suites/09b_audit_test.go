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

	// Generate a distinctive audited action.
	name := uniqueName("it-audit")
	ctx, cancel := testCtx(30)
	defer cancel()
	userId, code, err := admin.Client.CreateUser(ctx, &apiclient.CreateUserRequest{
		Username: name, Password: "Passw0rd!audit", Email: name + "@knot.test",
		Active: true, MaxSpaces: 1, ComputeUnits: 1, StorageUnits: 1, MaxTunnels: 1,
		PreferredShell: "bash", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("create audit user: %v (status %d)", err, code)
	}
	defer func() {
		ctx, cancel := testCtx(15)
		_ = admin.Client.DeleteUser(ctx, userId)
		cancel()
	}()

	// The audit log should contain the creation event.
	deadline := time.Now().Add(15 * time.Second)
	var found bool
	for time.Now().Before(deadline) && !found {
		ctx, cancel := testCtx(15)
		logs, code, err := admin.Client.GetAuditLogs(ctx, 0, 200, &apiclient.AuditLogFilter{
			Event: "User Create",
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
		t.Fatal("user creation not found in audit log (event user.created, actor admin)")
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
