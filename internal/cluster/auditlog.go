package cluster

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleAuditLogGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug().Msg("cluster: Received audit log gossip request")

	logs := []*model.AuditLogEntry{}
	if err := packet.Unmarshal(&logs); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal audit log gossip request")
		return err
	}

	// Merge the logs with the local logs
	db := database.GetInstance()
	for _, logEntry := range logs {
		if err := db.SaveAuditLog(logEntry); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to save audit log entry")
		}
	}

	return nil
}

func (c *Cluster) GossipAuditLog(entry *model.AuditLogEntry) {
	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping audit log")

		entries := []*model.AuditLogEntry{entry}
		c.gossipCluster.Send(AuditLogGossipMsg, &entries)
	}
}
