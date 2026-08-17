//go:build integration

package suites

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestEventSinkWebhook(t *testing.T) {
	harness.Feature(t, "event-sinks")

	var mu sync.Mutex
	var received []map[string]interface{}
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("sink decode: %v", err)
		}
		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer sink.Close()

	ctx, cancel := testCtx(60)
	defer cancel()

	// RaiseCustomEvent prefixes emitted types with "custom."
	eventType := "custom.it.test.event"
	resp, err := user1.Client.CreateEventSink(ctx, apiclient.EventSinkCreateRequest{
		UserId:      user1.Id,
		Name:        uniqueVarName("it_sink"),
		Description: "integration sink",
		Events:      []string{eventType},
		SinkType:    "webhook",
		Webhook: &apiclient.WebhookConfig{
			URL:           sink.URL,
			SkipTLSVerify: true,
		},
		Active: true,
	})
	if err != nil {
		t.Fatalf("create event sink: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := testCtx(15)
		user1.Client.DeleteEventSink(ctx, resp.Id)
		cancel()
	})

	// Emit a custom event; the sink webhook should receive it.
	if code, err := user1.Client.Do(ctx, "POST", "/api/events/emit", apiclient.EmitEventRequest{
		Type:    "it.test.event",
		Payload: map[string]interface{}{"hello": "world"},
	}, nil); err != nil {
		t.Fatalf("emit event: %v (status %d)", err, code)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(received) > 0
		mu.Unlock()
		if got {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("event sink webhook never received the emitted event")
}

func TestEventsSSE(t *testing.T) {
	harness.Feature(t, "events")

	// Subscribe to the SSE stream with the bearer token.
	req, _ := http.NewRequest("GET", server.BaseURL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+user1.Token)
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Timeout: 0}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("subscribe SSE: %v", err)
	}
	defer resp.Body.Close()
	mustEqual(t, "sse status", resp.StatusCode, 200)

	// The SSE stream carries domain events; creating a group must produce
	// a groups:changed message.
	emitted := make(chan struct{})
	go func() {
		defer close(emitted)
		time.Sleep(1 * time.Second) // let the subscription settle
		ctx, cancel := testCtx(30)
		defer cancel()
		id, code, err := admin.Client.CreateGroup(ctx, &apiclient.GroupRequest{
			Name: uniqueName("it-sse-group"), MaxSpaces: 1, ComputeUnits: 1, StorageUnits: 1, MaxTunnels: 1,
		})
		if err != nil {
			t.Errorf("create group for SSE: %v (status %d)", err, code)
			return
		}
		dctx, dcancel := testCtx(15)
		admin.Client.DeleteGroup(dctx, id)
		dcancel()
	}()

	scanner := bufio.NewScanner(resp.Body)
	deadline := time.Now().Add(30 * time.Second)
	for scanner.Scan() && time.Now().Before(deadline) {
		line := scanner.Text()
		if len(line) > 6 && line[:6] == "data: " && contains(line, "groups:changed") {
			<-emitted
			return
		}
	}
	t.Fatalf("groups:changed not seen on SSE stream")
}

