package cluster

import (
	"testing"

	"github.com/google/uuid"
	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

// These tests drive the real cluster message handlers (handleEventBroadcast,
// handleEventDone, handleInFlightState) with constructed sender nodes and
// packets, exercising the actual per-zone gate (sender.zone == cfg.Zone) rather
// than a re-implementation. They rely on gossip.NewTestNode and the exported
// Packet build methods (added to the owned gossip library).

// nonLeaderTransport makes the shared EventDispatcher take its non-leader path
// (record the in-flight entry for failover) so a delivered event is observable
// via GetEntriesForGossip without configuring sinks.
type nonLeaderTransport struct{}

func (nonLeaderTransport) IsLeader() bool         { return false }
func (nonLeaderTransport) NotifyEventDone(string) {}

func (nonLeaderTransport) GossipGroup(*model.Group)                       {}
func (nonLeaderTransport) GossipRole(*model.Role)                         {}
func (nonLeaderTransport) GossipSpace(*model.Space)                       {}
func (nonLeaderTransport) GossipTemplate(*model.Template)                 {}
func (nonLeaderTransport) GossipTemplateVar(*model.TemplateVar)           {}
func (nonLeaderTransport) GossipUser(*model.User)                         {}
func (nonLeaderTransport) GossipToken(*model.Token)                       {}
func (nonLeaderTransport) GossipVolume(*model.Volume)                     {}
func (nonLeaderTransport) GossipSpaceUsageSample(*model.SpaceUsageSample) {}
func (nonLeaderTransport) GossipAuditLog(*model.AuditLogEntry)            {}
func (nonLeaderTransport) GossipSession(*model.Session)                   {}
func (nonLeaderTransport) GossipScript(*model.Script)                     {}
func (nonLeaderTransport) GossipSkill(*model.Skill)                       {}
func (nonLeaderTransport) GossipCommand(*model.Command)                   {}
func (nonLeaderTransport) GossipEventSink(*model.EventSink)               {}
func (nonLeaderTransport) GossipStackDefinition(*model.StackDefinition)   {}
func (nonLeaderTransport) GossipResponse(*model.Response)                 {}
func (nonLeaderTransport) GossipConversation(*model.Conversation)         {}
func (nonLeaderTransport) GossipMCPServer(*model.MCPServer)              {}
func (nonLeaderTransport) GossipPoolDefinition(*model.PoolDefinition)     {}
func (nonLeaderTransport) GossipPoolDrain(string)                        {}
func (nonLeaderTransport) GossipPoolUndrain(string)                      {}
func (nonLeaderTransport) BroadcastEvent(*service.EventEnvelope)          {}
func (nonLeaderTransport) GetAgentEndpoints() []string                   { return nil }
func (nonLeaderTransport) GetTunnelServers() []string                    { return nil }
func (nonLeaderTransport) LockResource(string) string                    { return "" }
func (nonLeaderTransport) UnlockResource(string, string)                 {}
func (nonLeaderTransport) Nodes() []*gossip.Node                         { return nil }
func (nonLeaderTransport) GetNodeByIDString(string) *gossip.Node         { return nil }
func (nonLeaderTransport) EnqueueSpaceCleanup(*model.Space)              {}

func testCluster() *Cluster {
	return &Cluster{logger: log.WithGroup("cluster-test")}
}

func zoneSender(zone string) *gossip.Node {
	return gossip.NewTestNode(gossip.NodeID(uuid.New()), "mem://peer", map[string]string{"zone": zone})
}

// packetFor builds a gossip packet carrying v, decodable by the handler.
func packetFor(t *testing.T, msgType gossip.MessageType, v interface{}) *gossip.Packet {
	t.Helper()
	ser := codec.NewVmihailencoMsgpackCodec()
	data, err := ser.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	p := gossip.NewPacket()
	p.MessageType = msgType
	p.SetCodec(ser)
	p.SetPayload(data)
	return p
}

func gossipEntry(eventId string) *service.InFlightEntry {
	for _, e := range service.GetEventDispatcher().GetEntriesForGossip() {
		if e.EventId == eventId {
			return e
		}
	}
	return nil
}

func setZone(t *testing.T, zone string) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{Zone: zone})
	t.Cleanup(func() { config.SetServerConfig(prev) })
}

func useNonLeaderTransport(t *testing.T) {
	t.Helper()
	prev := service.GetTransport()
	service.SetTransport(nonLeaderTransport{})
	t.Cleanup(func() { service.SetTransport(prev) })
}

// A same-zone event broadcast is dispatched (recorded); a wrong-zone broadcast
// is dropped by the gate and never recorded — the real "events never cross
// zones" guarantee.
func TestHandleEventBroadcastEnforcesZone(t *testing.T) {
	setZone(t, "z1")
	useNonLeaderTransport(t)
	c := testCluster()

	sameZoneId := "evt-same-" + uuid.NewString()
	if err := c.handleEventBroadcast(zoneSender("z1"), packetFor(t, EventBroadcastMsg, &service.EventEnvelope{
		EventId: sameZoneId, EventType: "space.started",
	})); err != nil {
		t.Fatalf("handleEventBroadcast same-zone: %v", err)
	}
	if gossipEntry(sameZoneId) == nil {
		t.Fatal("same-zone event was not recorded by the dispatcher")
	}

	wrongZoneId := "evt-wrong-" + uuid.NewString()
	if err := c.handleEventBroadcast(zoneSender("z2"), packetFor(t, EventBroadcastMsg, &service.EventEnvelope{
		EventId: wrongZoneId, EventType: "space.started",
	})); err != nil {
		t.Fatalf("handleEventBroadcast wrong-zone: %v", err)
	}
	if gossipEntry(wrongZoneId) != nil {
		t.Fatal("a wrong-zone event was recorded; it must be dropped (forwarded by gossip, never stored)")
	}
}

// handleEventDone from a same-zone leader tombstones the local entry; a done
// from a different zone is ignored.
func TestHandleEventDoneEnforcesZone(t *testing.T) {
	setZone(t, "z1")
	useNonLeaderTransport(t)
	c := testCluster()

	// Seed two recorded events in this zone.
	evtCross := "evt-done-cross-" + uuid.NewString()
	evtSame := "evt-done-same-" + uuid.NewString()
	for _, id := range []string{evtCross, evtSame} {
		if err := c.handleEventBroadcast(zoneSender("z1"), packetFor(t, EventBroadcastMsg, &service.EventEnvelope{
			EventId: id, EventType: "space.started",
		})); err != nil {
			t.Fatalf("seed broadcast: %v", err)
		}
	}

	// A done notification from a foreign zone must be ignored.
	if err := c.handleEventDone(zoneSender("z2"), packetFor(t, EventDoneMsg, evtCross)); err != nil {
		t.Fatalf("handleEventDone foreign zone: %v", err)
	}
	if e := gossipEntry(evtCross); e == nil || !e.TombstonedAt.IsZero() {
		t.Fatal("foreign-zone done notification wrongly tombstoned the entry")
	}

	// A done from the same zone tombstones it.
	if err := c.handleEventDone(zoneSender("z1"), packetFor(t, EventDoneMsg, evtSame)); err != nil {
		t.Fatalf("handleEventDone same zone: %v", err)
	}
	if e := gossipEntry(evtSame); e == nil || e.TombstonedAt.IsZero() {
		t.Fatal("same-zone done notification did not tombstone the entry")
	}
}

// Periodic in-flight gossip is merged only when it comes from the same zone.
func TestHandleInFlightStateEnforcesZone(t *testing.T) {
	setZone(t, "z1")
	useNonLeaderTransport(t)
	c := testCluster()

	foreignId := "evt-merge-foreign-" + uuid.NewString()
	sameId := "evt-merge-same-" + uuid.NewString()

	foreign := []*service.InFlightEntry{{EventId: foreignId, SinkId: "", Status: "pending", Version: hlc.Now()}}
	if err := c.handleInFlightState(zoneSender("z2"), packetFor(t, InFlightStateMsg, &foreign)); err != nil {
		t.Fatalf("handleInFlightState foreign: %v", err)
	}
	if gossipEntry(foreignId) != nil {
		t.Fatal("merged in-flight state from a foreign zone")
	}

	same := []*service.InFlightEntry{{EventId: sameId, SinkId: "", Status: "pending", Version: hlc.Now()}}
	if err := c.handleInFlightState(zoneSender("z1"), packetFor(t, InFlightStateMsg, &same)); err != nil {
		t.Fatalf("handleInFlightState same: %v", err)
	}
	if gossipEntry(sameId) == nil {
		t.Fatal("did not merge in-flight state from the same zone")
	}
}
