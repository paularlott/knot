package audit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// Audit events routed externally must reach the service even when the server
// log level filters info records — the audit trail is data, not diagnostics.
func TestAuditReachesExternalAtWarnLevel(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		mu.Lock()
		bodies = append(bodies, string(buf))
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	log.ConfigureWithHTTP("warn", srv.URL, "ndjson", "knot", "", "", "")
	t.Cleanup(func() { log.Configure("info", "console", nil) })

	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		Audit: config.AuditConfig{Routing: "external", AuditStream: "audit"},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	if err := Log("alice", model.AuditActorTypeUser, model.AuditEventSpaceStart,
		"Started space web", &map[string]interface{}{"space_name": "web"}); err != nil {
		t.Fatal(err)
	}
	log.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 || !strings.Contains(bodies[0], "Started space web") {
		t.Fatalf("audit entry missing from external delivery: %v", bodies)
	}
	if !strings.Contains(bodies[0], `"service":"audit"`) {
		t.Errorf("audit stream label missing: %s", bodies[0])
	}
}
