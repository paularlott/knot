package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/devicegrant"
)

// handleDeviceGrantGossip applies device sign-in changes received from
// cluster peers, so a sign-in approved on one server can be collected by a
// poll on another.
func (c *Cluster) handleDeviceGrantGossip(sender *gossip.Node, packet *gossip.Packet) error {
	grants := []*devicegrant.Grant{}
	if err := packet.Unmarshal(&grants); err != nil {
		c.logger.WithError(err).Error("failed to unmarshal device grant gossip request")
		return err
	}

	for _, g := range grants {
		devicegrant.Apply(g)
	}
	return nil
}

func (c *Cluster) GossipDeviceGrant(grant *devicegrant.Grant) {
	if c.gossipCluster != nil {
		grants := []*devicegrant.Grant{grant}
		c.gossipCluster.Send(DeviceGrantGossipMsg, &grants)
	}
}
