package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/mcpstatekey"
)

// The key that signs MCP question state (see mcpstatekey) is shared across
// the cluster, so a tool's question can be answered through any node. Each
// node keeps the oldest key it has seen; exchanging keys on join and in every
// gossip round brings the cluster to one. It is never logged.

func (c *Cluster) handleMCPStateKeyFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	var incoming mcpstatekey.Key
	if err := packet.Unmarshal(&incoming); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal MCP state key full sync request")
		return nil, err
	}
	c.mergeMCPStateKey(incoming)
	mine, err := mcpstatekey.Current()
	if err != nil {
		return nil, err
	}
	return &mine, nil
}

func (c *Cluster) handleMCPStateKeyGossip(sender *gossip.Node, packet *gossip.Packet) error {
	var incoming mcpstatekey.Key
	if err := packet.Unmarshal(&incoming); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal MCP state key gossip")
		return err
	}
	c.mergeMCPStateKey(incoming)
	return nil
}

// DoMCPStateKeyFullSync exchanges keys with node, keeping the winner.
func (c *Cluster) DoMCPStateKeyFullSync(node *gossip.Node) error {
	if c.gossipCluster == nil {
		return nil
	}
	mine, err := mcpstatekey.Current()
	if err != nil {
		return err
	}
	var theirs mcpstatekey.Key
	if err := c.gossipCluster.SendToWithResponse(node, MCPStateKeyFullSyncMsg, &mine, &theirs); err != nil {
		return err
	}
	c.mergeMCPStateKey(theirs)
	return nil
}

func (c *Cluster) mergeMCPStateKey(incoming mcpstatekey.Key) {
	changed, err := mcpstatekey.Merge(incoming)
	if err != nil {
		c.logger.Debug("Ignoring MCP state key from another node", "error", err.Error())
		return
	}
	if changed {
		c.logger.Info("Adopted the cluster's MCP state key")
	}
}

// gossipMCPStateKey sends this node's key on each gossip round: one small
// value, so nodes that joined apart still converge.
func (c *Cluster) gossipMCPStateKey() {
	if c.gossipCluster == nil {
		return
	}
	mine, err := mcpstatekey.Current()
	if err != nil {
		c.logger.WithError(err).Error("Failed to load the MCP state key")
		return
	}
	c.gossipCluster.Send(MCPStateKeyGossipMsg, &mine)
}
