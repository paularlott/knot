package log

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPipelineSurvivesWarnLevel(t *testing.T) {
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

	ConfigureWithHTTP("warn", srv.URL, "ndjson", "knot", "", "", "")
	t.Cleanup(func() { Configure("info", "console", nil) })

	// Diagnostics below the configured level are filtered; pipeline records
	// are data promised to the service and must arrive regardless.
	Info("diagnostic noise below level")
	Pipeline("audit event body", "stream", "audit", "type", "audit")

	Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("expected exactly one delivered batch, got %d: %v", len(bodies), bodies)
	}
	if !strings.Contains(bodies[0], "audit event body") {
		t.Errorf("pipeline record missing from delivery: %s", bodies[0])
	}
	if strings.Contains(bodies[0], "diagnostic noise") {
		t.Errorf("filtered diagnostic must not be delivered: %s", bodies[0])
	}
	var rec map[string]any
	for _, line := range strings.Split(strings.TrimSpace(bodies[0]), "\n") {
		if strings.Contains(line, "audit event body") {
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatal(err)
			}
		}
	}
	if rec["stream"] != "audit" {
		t.Errorf("per-record stream must survive: %v", rec)
	}
}
