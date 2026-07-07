package service

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database/model"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeTransport is a configurable stand-in for the cluster Transport. It records
// the calls the event/pool logic makes (NotifyEventDone, BroadcastEvent, pool
// drain/undrain) and lets a test pretend to be the zone leader or a follower.
// All other Transport methods are no-ops.
type fakeTransport struct {
	mu          sync.Mutex
	leader      bool
	doneEvents  []string
	broadcasts  []*EventEnvelope
	drained     []string
	undrained   []string
}

func (f *fakeTransport) IsLeader() bool { return f.leader }

func (f *fakeTransport) NotifyEventDone(eventId string) {
	f.mu.Lock()
	f.doneEvents = append(f.doneEvents, eventId)
	f.mu.Unlock()
}

func (f *fakeTransport) BroadcastEvent(envelope *EventEnvelope) {
	f.mu.Lock()
	f.broadcasts = append(f.broadcasts, envelope)
	f.mu.Unlock()
}

func (f *fakeTransport) GossipPoolDrain(spaceID string) {
	f.mu.Lock()
	f.drained = append(f.drained, spaceID)
	f.mu.Unlock()
}

func (f *fakeTransport) GossipPoolUndrain(spaceID string) {
	f.mu.Lock()
	f.undrained = append(f.undrained, spaceID)
	f.mu.Unlock()
}

func (f *fakeTransport) doneCount(eventId string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, id := range f.doneEvents {
		if id == eventId {
			n++
		}
	}
	return n
}

// Remaining Transport methods — unused no-ops.
func (f *fakeTransport) GossipGroup(*model.Group)                      {}
func (f *fakeTransport) GossipRole(*model.Role)                        {}
func (f *fakeTransport) GossipSpace(*model.Space)                      {}
func (f *fakeTransport) GossipTemplate(*model.Template)                {}
func (f *fakeTransport) GossipTemplateVar(*model.TemplateVar)          {}
func (f *fakeTransport) GossipUser(*model.User)                        {}
func (f *fakeTransport) GossipToken(*model.Token)                      {}
func (f *fakeTransport) GossipVolume(*model.Volume)                    {}
func (f *fakeTransport) GossipSpaceUsageSample(*model.SpaceUsageSample) {}
func (f *fakeTransport) GossipAuditLog(*model.AuditLogEntry)           {}
func (f *fakeTransport) GossipSession(*model.Session)                  {}
func (f *fakeTransport) GossipScript(*model.Script)                    {}
func (f *fakeTransport) GossipSkill(*model.Skill)                      {}
func (f *fakeTransport) GossipCommand(*model.Command)                  {}
func (f *fakeTransport) GossipEventSink(*model.EventSink)              {}
func (f *fakeTransport) GossipStackDefinition(*model.StackDefinition)  {}
func (f *fakeTransport) GossipResponse(*model.Response)                {}
func (f *fakeTransport) GossipPoolDefinition(*model.PoolDefinition)    {}
func (f *fakeTransport) GetAgentEndpoints() []string                  { return nil }
func (f *fakeTransport) GetTunnelServers() []string                   { return nil }
func (f *fakeTransport) LockResource(string) string                   { return "" }
func (f *fakeTransport) UnlockResource(string, string)                {}
func (f *fakeTransport) Nodes() []*gossip.Node                        { return nil }
func (f *fakeTransport) GetNodeByIDString(string) *gossip.Node        { return nil }
func (f *fakeTransport) EnqueueSpaceCleanup(*model.Space)             {}

// newTestDispatcher builds an isolated EventDispatcher with no background GC
// goroutine and no singleton state, so a test can stand up several of them to
// model independent servers.
func newTestDispatcher() *EventDispatcher {
	return &EventDispatcher{
		inFlight:          make(map[string]*InFlightEntry),
		processed:         make(map[string]time.Time),
		jsonrpcDelivered:  make(map[string]time.Time),
		queues:            make(map[string]*sinkQueue),
		subscriptions:     make(map[string][]*JSONRPCSubscription),
		pendingDeliveries: make(map[string]int),
		httpClient:        &http.Client{},
		insecureClient:    &http.Client{},
	}
}

func testEnvelope(eventId string) *EventEnvelope {
	return &EventEnvelope{
		EventId:   eventId,
		EventType: "space.started",
		SpaceId:   "space-1",
		UserId:    "user-1",
		Payload:   map[string]interface{}{"k": "v"},
		Ts:        hlc.Now(),
	}
}

func (d *EventDispatcher) entry(eventId, sinkId string) *InFlightEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.inFlight[inFlightKey(eventId, sinkId)]
}

func restoreTransport(t *testing.T) {
	prev := GetTransport()
	t.Cleanup(func() { SetTransport(prev) })
}

// ---------------------------------------------------------------------------
// Single-node dispatcher behaviour
// ---------------------------------------------------------------------------

// A non-leader must still record an in-flight entry (with an empty sink id) so
// that, on a leadership change, the new leader can replay the event. This is the
// "all servers record, only the leader processes" contract.
func TestNonLeaderRecordsInFlightForFailover(t *testing.T) {
	restoreTransport(t)
	SetTransport(&fakeTransport{leader: false})

	d := newTestDispatcher()
	d.Dispatch(testEnvelope("e1"))

	e := d.entry("e1", "")
	if e == nil {
		t.Fatal("expected non-leader to record an in-flight entry for the event")
	}
	if e.Status != "pending" {
		t.Fatalf("status = %q, want pending", e.Status)
	}
	if !e.TombstonedAt.IsZero() {
		t.Fatal("a freshly recorded entry must not be tombstoned")
	}
}

// MarkEventDone must tombstone every entry for the event, including the
// empty-sink-id key that non-leaders record. Regression test: the prefix match
// once used len(key) > len(prefix), which skipped the exact "eventId:" key.
func TestMarkEventDoneTombstonesEmptySinkKey(t *testing.T) {
	d := newTestDispatcher()
	env := testEnvelope("e1")
	d.recordInFlight(env, "")        // non-leader failover entry
	d.recordInFlight(env, "sink-a")  // a real sink delivery entry

	d.MarkEventDone("e1")

	for _, sinkId := range []string{"", "sink-a"} {
		e := d.entry("e1", sinkId)
		if e == nil {
			t.Fatalf("entry for sink %q missing", sinkId)
		}
		if e.TombstonedAt.IsZero() || e.Status != "done" {
			t.Fatalf("entry for sink %q not tombstoned: status=%q tombstoned=%v", sinkId, e.Status, e.TombstonedAt)
		}
	}
}

func TestMergeInFlightNewerVersionWins(t *testing.T) {
	d := newTestDispatcher()
	env := testEnvelope("e1")
	d.recordInFlight(env, "")
	localVersion := d.entry("e1", "").Version

	// A newer, tombstoned copy from a peer must overwrite the local pending one.
	incoming := &InFlightEntry{
		EventId:      "e1",
		SinkId:       "",
		Status:       "done",
		TombstonedAt: time.Now().UTC(),
		Version:      hlc.Now(),
	}
	if !incoming.Version.After(localVersion) {
		t.Fatal("precondition: incoming version must be newer")
	}
	d.MergeInFlight([]*InFlightEntry{incoming})

	if got := d.entry("e1", ""); got.Status != "done" || got.TombstonedAt.IsZero() {
		t.Fatalf("merge did not apply newer tombstone: %+v", got)
	}
}

func TestMergeInFlightOlderVersionIgnored(t *testing.T) {
	d := newTestDispatcher()
	env := testEnvelope("e1")
	// Make the local copy "done" with a fresh version.
	d.recordInFlight(env, "")
	d.MarkEventDone("e1")
	doneVersion := d.entry("e1", "").Version

	// A stale pending copy (older version) must NOT resurrect the entry.
	stale := &InFlightEntry{EventId: "e1", SinkId: "", Status: "pending", Version: env.Ts}
	if stale.Version.After(doneVersion) {
		t.Fatal("precondition: stale version must be older")
	}
	d.MergeInFlight([]*InFlightEntry{stale})

	if got := d.entry("e1", ""); got.Status != "done" {
		t.Fatalf("stale merge resurrected entry: status=%q", got.Status)
	}
}

func TestMergeInFlightAddsUnknownTombstone(t *testing.T) {
	d := newTestDispatcher()
	// A node that never saw the event still accepts a gossiped tombstone so it
	// converges and eventually GCs the record (rather than holding it until
	// MaxPendingAge).
	incoming := &InFlightEntry{
		EventId:      "e9",
		SinkId:       "",
		Status:       "done",
		TombstonedAt: time.Now().UTC(),
		Version:      hlc.Now(),
	}
	d.MergeInFlight([]*InFlightEntry{incoming})

	if got := d.entry("e9", ""); got == nil || got.TombstonedAt.IsZero() {
		t.Fatalf("unknown tombstone not added: %+v", got)
	}
}

func TestGetEntriesForGossipIncludesTombstones(t *testing.T) {
	d := newTestDispatcher()
	d.recordInFlight(testEnvelope("pending-evt"), "")
	d.recordInFlight(testEnvelope("done-evt"), "")
	d.MarkEventDone("done-evt")

	entries := d.GetEntriesForGossip()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (tombstones must be gossiped)", len(entries))
	}
	var sawTombstone bool
	for _, e := range entries {
		if e.EventId == "done-evt" && !e.TombstonedAt.IsZero() {
			sawTombstone = true
		}
	}
	if !sawTombstone {
		t.Fatal("tombstoned entry missing from gossip set")
	}
}

// The leader must fire exactly one "done" notification once every reserved sink
// delivery has completed — not before. Regression test: the pending counter was
// once incremented after spawning deliveries, letting a fast delivery fire a
// premature done.
func TestPendingCounterFiresDoneExactlyOnce(t *testing.T) {
	restoreTransport(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)

	d := newTestDispatcher()

	// Reserve two deliveries up front (as processEvent does), then complete them.
	d.mu.Lock()
	d.pendingDeliveries["e1"] = 2
	d.mu.Unlock()

	d.decrementPending("e1")
	if c := ft.doneCount("e1"); c != 0 {
		t.Fatalf("done fired after 1/2 deliveries: count=%d", c)
	}
	d.decrementPending("e1")
	if c := ft.doneCount("e1"); c != 1 {
		t.Fatalf("done count = %d, want exactly 1", c)
	}
}

func TestCleanupTombstonedRemovesAfterRetention(t *testing.T) {
	d := newTestDispatcher()
	env := testEnvelope("old")
	d.recordInFlight(env, "")
	d.recordInFlight(testEnvelope("recent"), "")

	// Tombstone both, but backdate one beyond the retention window.
	d.MarkEventDone("old")
	d.MarkEventDone("recent")
	d.mu.Lock()
	d.inFlight[inFlightKey("old", "")].TombstonedAt = time.Now().UTC().Add(-TombstoneRetention - time.Minute)
	d.cleanupTombstoned()
	d.mu.Unlock()

	if d.entry("old", "") != nil {
		t.Fatal("expired tombstone was not garbage collected")
	}
	if d.entry("recent", "") == nil {
		t.Fatal("recent tombstone was collected too early")
	}
}

// ReplayPending re-dispatches still-pending entries when a node becomes leader.
// With no matching sinks the event is immediately complete, so the new leader
// notifies the zone that the event is done.
func TestReplayPendingReprocessesOnLeadershipChange(t *testing.T) {
	restoreTransport(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)

	d := newTestDispatcher()
	// Entry recorded earlier while this node was a follower.
	d.recordInFlight(testEnvelope("e1"), "")

	d.ReplayPending()

	// checkEventComplete notifies asynchronously; wait briefly for it.
	deadline := time.Now().Add(2 * time.Second)
	for ft.doneCount("e1") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ft.doneCount("e1") == 0 {
		t.Fatal("new leader did not complete the replayed event")
	}
}

// ---------------------------------------------------------------------------
// Multi-server, single zone: in-flight state converges across peers
// ---------------------------------------------------------------------------

// Three servers in the same zone all record an event. The leader finishes it and
// directly notifies one peer; a third peer that missed the direct notification
// still converges via periodic gossip of the (tombstoned) in-flight set.
func TestInFlightConvergesAcrossZonePeers(t *testing.T) {
	leader := newTestDispatcher()
	peerDirect := newTestDispatcher()
	peerGossip := newTestDispatcher()
	nodes := []*EventDispatcher{leader, peerDirect, peerGossip}

	env := testEnvelope("e1")
	// Every server in the zone records the in-flight entry (agents queue to all
	// connected servers; followers keep it for failover replay).
	for _, n := range nodes {
		n.recordInFlight(env, "")
	}

	// Leader completes delivery and directly notifies peers in its zone. One
	// peer receives it; simulate the third being momentarily unreachable.
	leader.MarkEventDone("e1")
	peerDirect.MarkEventDone("e1")

	// Before gossip, the unreachable peer would wrongly replay the event.
	if e := peerGossip.entry("e1", ""); e == nil || !e.TombstonedAt.IsZero() {
		t.Fatal("precondition: third peer should still hold a pending entry")
	}

	// Periodic gossip carries the tombstone to the lagging peer.
	peerGossip.MergeInFlight(leader.GetEntriesForGossip())

	for i, n := range nodes {
		e := n.entry("e1", "")
		if e == nil || e.TombstonedAt.IsZero() || e.Status != "done" {
			t.Fatalf("node %d did not converge to done: %+v", i, e)
		}
	}
}

// ---------------------------------------------------------------------------
// Multiple zones: events and done-notifications never cross a zone boundary
// ---------------------------------------------------------------------------

// zoneBus models the cluster's zone-scoped delivery. It mirrors the gates in
// internal/cluster: handleEventBroadcast / BroadcastEvent only deliver to
// same-zone nodes, and sendToZoneMembers (NotifyEventDone) only reaches the
// sender's zone. Wrong-zone events are forwarded by real gossip but never
// recorded, which is what these tests assert.
type zoneBus struct {
	nodes map[*EventDispatcher]string // dispatcher -> zone
}

func newZoneBus() *zoneBus { return &zoneBus{nodes: map[*EventDispatcher]string{}} }

func (b *zoneBus) addNode(zone string) *EventDispatcher {
	d := newTestDispatcher()
	b.nodes[d] = zone
	return d
}

// broadcastEvent delivers an event raised in fromZone. Only same-zone nodes
// record it; other zones drop it (forwarded by gossip, never stored).
func (b *zoneBus) broadcastEvent(fromZone string, env *EventEnvelope) {
	for d, zone := range b.nodes {
		if zone == fromZone {
			d.recordInFlight(env, "")
		}
	}
}

// notifyDone tombstones the event only on same-zone nodes.
func (b *zoneBus) notifyDone(fromZone, eventId string) {
	for d, zone := range b.nodes {
		if zone == fromZone {
			d.MarkEventDone(eventId)
		}
	}
}

func TestEventsNeverCrossZones(t *testing.T) {
	bus := newZoneBus()
	z1a := bus.addNode("z1")
	z1b := bus.addNode("z1")
	z2 := bus.addNode("z2")

	env := testEnvelope("e1")
	bus.broadcastEvent("z1", env)

	if z1a.entry("e1", "") == nil || z1b.entry("e1", "") == nil {
		t.Fatal("same-zone nodes should have recorded the event")
	}
	if z2.entry("e1", "") != nil {
		t.Fatal("a different zone must not record the event")
	}

	// Completing the event in z1 must not touch z2 (which has nothing anyway).
	bus.notifyDone("z1", "e1")
	if e := z1a.entry("e1", ""); e == nil || e.TombstonedAt.IsZero() {
		t.Fatal("z1 node should be tombstoned after done")
	}
	if z2.entry("e1", "") != nil {
		t.Fatal("z2 must remain untouched by z1's done notification")
	}
}

func TestConcurrentZonesAreIndependent(t *testing.T) {
	bus := newZoneBus()
	z1 := bus.addNode("z1")
	z2 := bus.addNode("z2")

	bus.broadcastEvent("z1", testEnvelope("e-z1"))
	bus.broadcastEvent("z2", testEnvelope("e-z2"))

	if z1.entry("e-z1", "") == nil || z1.entry("e-z2", "") != nil {
		t.Fatal("z1 should hold only its own zone's event")
	}
	if z2.entry("e-z2", "") == nil || z2.entry("e-z1", "") != nil {
		t.Fatal("z2 should hold only its own zone's event")
	}
}
