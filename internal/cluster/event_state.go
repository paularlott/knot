package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/service"
)

// handleEventDone handles an EventDoneMsg from the zone leader. The leader
// sends this when all sink deliveries for an event are complete. Non-leaders
// tombstone their in-flight entries so they don't replay stale events on a
// leadership change.
func (c *Cluster) handleEventDone(sender *gossip.Node, packet *gossip.Packet) error {
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		return nil
	}

	var eventId string
	if err := packet.Unmarshal(&eventId); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal event done message")
		return err
	}

	service.GetEventDispatcher().MarkEventDone(eventId)
	return nil
}

// handleInFlightState handles periodic gossip of pending in-flight records
// from a zone peer. The records carry their HLC timestamp for merge conflict
// resolution. This ensures nodes that missed the original event broadcast
// (e.g. joining after an upgrade) receive pending events for leader-failover
// replay.
func (c *Cluster) handleInFlightState(sender *gossip.Node, packet *gossip.Packet) error {
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		return nil
	}

	var entries []*service.InFlightEntry
	if err := packet.Unmarshal(&entries); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal in-flight state message")
		return err
	}

	service.GetEventDispatcher().MergeInFlight(entries)
	return nil
}

// NotifyEventDone sends an immediate notification to all zone members that an
// event is fully delivered. Called by the leader via the Transport interface.
func (c *Cluster) NotifyEventDone(eventId string) {
	c.sendToZoneMembers(EventDoneMsg, eventId)
}

// gossipInFlight periodically gossips pending in-flight entries to zone peers.
func (c *Cluster) gossipInFlight() {
	if c.gossipCluster == nil {
		return
	}

	entries := service.GetEventDispatcher().GetEntriesForGossip()
	if len(entries) == 0 {
		return
	}

	c.gossipCluster.Send(InFlightStateMsg, &entries)
}
