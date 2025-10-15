package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/knot/internal/log"
)

func (c *Cluster) handleAuditLogGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug("cluster: Received audit log gossip request")

	logs := []*model.AuditLogEntry{}
	if err := packet.Unmarshal(&logs); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal audit log gossip request")
		return err
	}

	// Merge the logs with the local logs
	db := database.GetInstance()
	for _, logEntry := range logs {
		if err := db.SaveAuditLog(logEntry); err != nil {
			log.WithError(err).Error("cluster: Failed to save audit log entry")
		}
	}

	return nil
}

func (c *Cluster) GossipAuditLog(entry *model.AuditLogEntry) {
	if c.gossipCluster != nil {
		log.Debug("cluster: Gossipping audit log")

		entries := []*model.AuditLogEntry{entry}
		c.gossipCluster.Send(AuditLogGossipMsg, &entries)
	}
}
