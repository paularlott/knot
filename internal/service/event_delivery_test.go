package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// shrinkRetryDelays makes the delivery retry back-off negligible for the
// duration of a test so the full retry/give-up path runs quickly.
func shrinkRetryDelays(t *testing.T) {
	t.Helper()
	d1, d2 := RetryDelay1, RetryDelay2
	RetryDelay1 = time.Millisecond
	RetryDelay2 = time.Millisecond
	t.Cleanup(func() { RetryDelay1, RetryDelay2 = d1, d2 })
}

// deliveryEnvelope has no SpaceId, so render data is built without touching the
// database — keeping the delivery-mechanics tests free of DB setup.
func deliveryEnvelope(id string) *EventEnvelope {
	return &EventEnvelope{EventId: id, EventType: "space.started", UserId: "user-1", Ts: hlc.Now()}
}

func webhookSink(id, url, template string) *model.EventSink {
	return &model.EventSink{
		Id:       id,
		Name:     "wh-" + id,
		SinkType: "webhook",
		Active:   true,
		Events:   []string{"*"},
		Webhook: &model.WebhookConfig{
			URL:          url,
			Secret:       "shh",
			BodyTemplate: template,
		},
	}
}

// ---------------------------------------------------------------------------
// deliverWebhook / deliverOnce
// ---------------------------------------------------------------------------

func TestDeliverWebhookSuccessSignsAndSetsHeaders(t *testing.T) {
	var gotBody []byte
	var gotSig, gotEventId string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-Knot-Signature")
		gotEventId = r.Header.Get("X-Knot-Event-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := newTestDispatcher()
	task := &deliveryTask{
		envelope: deliveryEnvelope("evt-wh-ok"),
		sink:     webhookSink("s1", srv.URL, `{"id":"{{.EventId}}"}`),
	}

	if err := d.deliverOnce(task); err != nil {
		t.Fatalf("deliverOnce: %v", err)
	}
	if gotEventId != "evt-wh-ok" {
		t.Fatalf("X-Knot-Event-Id = %q", gotEventId)
	}
	mac := hmac.New(sha256.New, []byte("shh"))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Fatalf("signature = %q, want %q", gotSig, want)
	}
}

func TestDeliverWebhookNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := newTestDispatcher()
	err := d.deliverWebhook(&deliveryTask{envelope: deliveryEnvelope("e"), sink: webhookSink("s", srv.URL, "{}")})
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestDeliverWebhookNilConfigIsError(t *testing.T) {
	d := newTestDispatcher()
	sink := &model.EventSink{Id: "s", SinkType: "webhook", Active: true}
	if err := d.deliverWebhook(&deliveryTask{envelope: deliveryEnvelope("e"), sink: sink}); err == nil {
		t.Fatal("expected error when webhook config is nil")
	}
}

func TestDeliverOnceUnknownSinkType(t *testing.T) {
	d := newTestDispatcher()
	sink := &model.EventSink{Id: "s", SinkType: "carrier-pigeon", Active: true}
	if err := d.deliverOnce(&deliveryTask{envelope: deliveryEnvelope("e"), sink: sink}); err == nil {
		t.Fatal("expected error for unknown sink type")
	}
}

// ---------------------------------------------------------------------------
// deliver: retry orchestration, tombstoning, pending accounting
// ---------------------------------------------------------------------------

// enqueueTask seeds a task in the sink queue and reserves the pending counter,
// mirroring what processEvent does before starting a delivery.
func (d *EventDispatcher) seedTask(sink *model.EventSink, env *EventEnvelope) {
	q := d.getQueue(sink.Id)
	q.mu.Lock()
	q.queue = append(q.queue, &deliveryTask{envelope: env, sink: sink})
	q.mu.Unlock()
	d.mu.Lock()
	d.pendingDeliveries[env.EventId]++
	d.mu.Unlock()
}

func TestDeliverRetriesThenSucceeds(t *testing.T) {
	shrinkRetryDelays(t)
	restoreTransport(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := newTestDispatcher()
	env := deliveryEnvelope("evt-retry-ok")
	sink := webhookSink("s-retry", srv.URL, "{}")
	d.seedTask(sink, env)

	d.deliver(sink.Id)

	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("server hits = %d, want 3 (two failures then success)", got)
	}
	e := d.entry("evt-retry-ok", sink.Id)
	if e == nil || e.Status != "done" || e.TombstonedAt.IsZero() {
		t.Fatalf("entry not marked done: %+v", e)
	}
	if c := ft.doneCount("evt-retry-ok"); c != 1 {
		t.Fatalf("NotifyEventDone count = %d, want 1", c)
	}
}

func TestDeliverGivesUpAfterAllAttempts(t *testing.T) {
	setupPoolTestDB(t) // config + DB for logAudit on give-up/drop
	shrinkRetryDelays(t)
	restoreTransport(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := newTestDispatcher()
	env := deliveryEnvelope("evt-giveup")
	sink := webhookSink("s-giveup", srv.URL, "{}")
	d.seedTask(sink, env)

	d.deliver(sink.Id)

	if got := atomic.LoadInt32(&hits); got != int32(RetryAttempts) {
		t.Fatalf("server hits = %d, want %d", got, RetryAttempts)
	}
	e := d.entry("evt-giveup", sink.Id)
	if e == nil || e.Status != "given_up" || e.TombstonedAt.IsZero() {
		t.Fatalf("entry not marked given_up: %+v", e)
	}
	// Terminal outcomes decrement the pending counter exactly once.
	if c := ft.doneCount("evt-giveup"); c != 1 {
		t.Fatalf("NotifyEventDone count = %d, want 1", c)
	}
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	setupPoolTestDB(t) // config + DB for logAudit on give-up/drop
	d := newTestDispatcher()
	sink := webhookSink("s-full", "http://127.0.0.1:0", "{}")

	// Pre-fill the queue to capacity without starting workers.
	q := d.getQueue(sink.Id)
	q.mu.Lock()
	for i := 0; i < SinkQueueSize; i++ {
		q.queue = append(q.queue, &deliveryTask{envelope: deliveryEnvelope("filler"), sink: sink})
	}
	q.mu.Unlock()

	if d.enqueue(sink, deliveryEnvelope("overflow")) {
		t.Fatal("enqueue should return false when the queue is full")
	}
}

// ---------------------------------------------------------------------------
// deliverScript error branches (no scriptling execution needed)
// ---------------------------------------------------------------------------

func TestDeliverScriptMissingScriptIsError(t *testing.T) {
	setupPoolTestDB(t) // initialises a badger-backed database
	d := newTestDispatcher()
	sink := &model.EventSink{Id: "s", SinkType: "script", Active: true, ScriptId: "does-not-exist"}
	if err := d.deliverScript(&deliveryTask{envelope: deliveryEnvelope("e"), sink: sink}); err == nil {
		t.Fatal("expected error when the sink script does not exist")
	}
}

func TestDeliverScriptInactiveIsError(t *testing.T) {
	setupPoolTestDB(t)
	script := &model.Script{Id: "inactive-" + deliveryEnvelope("x").EventId, Active: false}
	if err := database.GetInstance().SaveScript(script, nil); err != nil {
		t.Fatalf("SaveScript: %v", err)
	}
	d := newTestDispatcher()
	sink := &model.EventSink{Id: "s", SinkType: "script", Active: true, ScriptId: script.Id}
	if err := d.deliverScript(&deliveryTask{envelope: deliveryEnvelope("e"), sink: sink}); err == nil {
		t.Fatal("expected error when the sink script is inactive")
	}
}

// ---------------------------------------------------------------------------
// deliverToSubscriptions / deliverJSONRPCWithRetry
// ---------------------------------------------------------------------------

type recordingCaller struct {
	mu      sync.Mutex
	calls   []string // spaceId/localName
	failFor int      // return error for the first N calls
	n       int
}

func (rc *recordingCaller) call(spaceId, localMethod string, _ json.RawMessage) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.n++
	rc.calls = append(rc.calls, spaceId+"/"+localMethod)
	if rc.n <= rc.failFor {
		return errContext("boom")
	}
	return nil
}

func (rc *recordingCaller) count() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.calls)
}

type errContext string

func (e errContext) Error() string { return string(e) }

func restoreJSONRPCCaller(t *testing.T) {
	t.Helper()
	prev := jsonrpcCaller
	t.Cleanup(func() { jsonrpcCaller = prev })
}

func subEnvelope(eventId, userId string) *EventEnvelope {
	return &EventEnvelope{EventId: eventId, EventType: "space.started", UserId: userId, Ts: hlc.Now()}
}

func TestDeliverToSubscriptionsCallsMethodAndDedups(t *testing.T) {
	restoreJSONRPCCaller(t)
	restoreTransport(t)
	SetTransport(&fakeTransport{leader: true})
	rc := &recordingCaller{}
	SetJSONRPCCaller(rc.call)

	d := newTestDispatcher()
	d.subscriptions["space-1"] = []*JSONRPCSubscription{{
		SpaceId: "space-1", UserId: "user-1", MethodName: "onStart", LocalName: "on_start",
		Events: []string{"space.*"},
	}}

	env := subEnvelope("evt-sub-1", "user-1")
	d.deliverToSubscriptions(env)
	waitFor(t, func() bool { return rc.count() == 1 })
	if rc.calls[0] != "space-1/on_start" {
		t.Fatalf("call = %q, want space-1/on_start", rc.calls[0])
	}

	// Same event id again: deduped, no second call.
	d.deliverToSubscriptions(env)
	time.Sleep(20 * time.Millisecond)
	if rc.count() != 1 {
		t.Fatalf("call count = %d, want 1 (dedup by event id)", rc.count())
	}
}

func TestDeliverToSubscriptionsSkipsOtherUsers(t *testing.T) {
	restoreJSONRPCCaller(t)
	restoreTransport(t)
	SetTransport(&fakeTransport{leader: true})
	rc := &recordingCaller{}
	SetJSONRPCCaller(rc.call)

	d := newTestDispatcher()
	d.subscriptions["space-1"] = []*JSONRPCSubscription{{
		SpaceId: "space-1", UserId: "someone-else", LocalName: "on_start", Events: []string{"*"},
	}}

	d.deliverToSubscriptions(subEnvelope("evt-sub-2", "user-1"))
	time.Sleep(20 * time.Millisecond)
	if rc.count() != 0 {
		t.Fatalf("call count = %d, want 0 (subscription belongs to another user)", rc.count())
	}
}

func TestDeliverJSONRPCRetriesThenGivesUp(t *testing.T) {
	setupPoolTestDB(t) // config + DB for logAudit on give-up/drop
	shrinkRetryDelays(t)
	restoreJSONRPCCaller(t)
	restoreTransport(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)
	rc := &recordingCaller{failFor: 1000} // always fail
	SetJSONRPCCaller(rc.call)

	d := newTestDispatcher()
	d.subscriptions["space-1"] = []*JSONRPCSubscription{{
		SpaceId: "space-1", UserId: "user-1", LocalName: "on_start", Events: []string{"*"},
	}}

	env := subEnvelope("evt-sub-giveup", "user-1")
	d.deliverToSubscriptions(env)
	waitFor(t, func() bool { return rc.count() == RetryAttempts })

	// The subscription delivery decrements the pending counter exactly once.
	if c := ft.doneCount("evt-sub-giveup"); c != 1 {
		t.Fatalf("NotifyEventDone count = %d, want 1", c)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
