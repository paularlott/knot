package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/authratelimit"
)

// handleAuthFailureGossip applies rate-limit state changes received from
// cluster peers so failed-login tracking and blocking are cluster-wide
// across every zone. The gossip mesh contains only full members — leaf
// nodes connect to servers directly and never see these messages.
func (c *Cluster) handleAuthFailureGossip(sender *gossip.Node, packet *gossip.Packet) error {
	events := []*authratelimit.Event{}
	if err := packet.Unmarshal(&events); err != nil {
		c.logger.WithError(err).Error("failed to unmarshal auth failure gossip request")
		return err
	}

	for _, evt := range events {
		authratelimit.ApplyEvent(evt)
	}
	return nil
}

func (c *Cluster) GossipAuthFailure(evt *authratelimit.Event) {
	if c.gossipCluster != nil {
		events := []*authratelimit.Event{evt}
		c.gossipCluster.Send(AuthFailureGossipMsg, &events)
	}
}
