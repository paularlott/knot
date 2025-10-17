package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleAuditLogGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("received audit log gossip request")

	logs := []*model.AuditLogEntry{}
	if err := packet.Unmarshal(&logs); err != nil {
		c.logger.WithError(err).Error("failed to unmarshal audit log gossip request")
		return err
	}

	// Merge the logs with the local logs
	db := database.GetInstance()
	for _, logEntry := range logs {
		if err := db.SaveAuditLog(logEntry); err != nil {
			c.logger.WithError(err).Error("failed to save audit log entry")
		}
	}

	return nil
}

func (c *Cluster) GossipAuditLog(entry *model.AuditLogEntry) {
	if c.gossipCluster != nil {
		c.logger.Debug("gossipping audit log")

		entries := []*model.AuditLogEntry{entry}
		c.gossipCluster.Send(AuditLogGossipMsg, &entries)
	}
}
