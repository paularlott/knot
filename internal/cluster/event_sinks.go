package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleEventBroadcast(sender *gossip.Node, packet *gossip.Packet) error {
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		return nil
	}

	var envelope service.EventEnvelope
	if err := packet.Unmarshal(&envelope); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal event broadcast")
		return err
	}

	service.GetEventDispatcher().Dispatch(&envelope)
	return nil
}

func (c *Cluster) BroadcastEvent(envelope *service.EventEnvelope) {
	if c.gossipCluster != nil {
		cfg := config.GetServerConfig()
		for _, node := range c.gossipCluster.AliveNodes() {
			if node.Metadata.GetString("zone") == cfg.Zone {
				c.gossipCluster.SendTo(node, EventBroadcastMsg, envelope)
			}
		}
	}
}

func (c *Cluster) handleEventSinkFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received event sink full sync request")

	sinks := []*model.EventSink{}
	if err := packet.Unmarshal(&sinks); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal event sink full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingSinks, err := db.GetEventSinks()
	if err != nil {
		return nil, err
	}

	go c.mergeEventSinks(sinks)

	return existingSinks, nil
}

func (c *Cluster) handleEventSinkGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received event sink gossip request")

	sinks := []*model.EventSink{}
	if err := packet.Unmarshal(&sinks); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal event sink gossip request")
		return err
	}

	if err := c.mergeEventSinks(sinks); err != nil {
		c.logger.WithError(err).Error("Failed to merge event sinks")
		return err
	}

	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipEventSink, &sinks)
	}

	return nil
}

func (c *Cluster) GossipEventSink(sink *model.EventSink) {
	if c.gossipCluster != nil {
		c.logger.Trace("Gossipping event sink")

		sinks := []*model.EventSink{sink}
		c.gossipCluster.Send(EventSinkGossipMsg, &sinks)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Trace("Updating event sink on leaf nodes")

		sinks := []*model.EventSink{sink}
		c.sendToLeafNodes(leafmsg.MessageGossipEventSink, &sinks)
	}
}

func (c *Cluster) DoEventSinkFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		sinks, err := db.GetEventSinks()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, EventSinkFullSyncMsg, &sinks, &sinks); err != nil {
			return err
		}

		if err := c.mergeEventSinks(sinks); err != nil {
			c.logger.WithError(err).Error("Failed to merge event sinks")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeEventSinks(sinks []*model.EventSink) error {
	c.logger.Trace("Merging event sinks", "number_sinks", len(sinks))

	db := database.GetInstance()
	localSinks, err := db.GetEventSinks()
	if err != nil {
		return err
	}

	localMap := make(map[string]*model.EventSink)
	for _, sink := range localSinks {
		localMap[sink.Id] = sink
	}

	for _, sink := range sinks {
		if local, ok := localMap[sink.Id]; ok {
			if sink.UpdatedAt.After(local.UpdatedAt) {
				if err := db.SaveEventSink(sink, nil); err != nil {
					c.logger.Error("Failed to update event sink", "error", err, "name", sink.Name)
				}

				if sink.IsDeleted {
					sse.PublishEventSinksDeleted(sink.Id)
				} else {
					sse.PublishEventSinksChanged(sink.Id)
				}
			}
		} else {
			if err := db.SaveEventSink(sink, nil); err != nil {
				c.logger.Error("Failed to save event sink", "error", err, "name", sink.Name, "is_deleted", sink.IsDeleted)
			}

			if !sink.IsDeleted {
				sse.PublishEventSinksChanged(sink.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipEventSinks() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	db := database.GetInstance()
	sinks, err := db.GetEventSinks()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get event sinks")
		return
	}

	activeSinks := []*model.EventSink{}
	for _, sink := range sinks {
		if sink.Active {
			activeSinks = append(activeSinks, sink)
		}
	}

	rand.Shuffle(len(sinks), func(i, j int) {
		sinks[i], sinks[j] = sinks[j], sinks[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(sinks))
		if batchSize > 0 {
			c.logger.Trace("Gossipping event sinks", "batch_size", batchSize, "total", len(sinks))
			batch := sinks[:batchSize]
			c.gossipCluster.Send(EventSinkGossipMsg, &batch)
		}
	}

	if len(c.leafSessions) > 0 && len(activeSinks) > 0 {
		rand.Shuffle(len(activeSinks), func(i, j int) {
			activeSinks[i], activeSinks[j] = activeSinks[j], activeSinks[i]
		})
		batchSize := c.CalcLeafPayloadSize(len(activeSinks))
		if batchSize > 0 {
			c.logger.Trace("Event sinks to leaf nodes", "batch_size", batchSize, "total", len(activeSinks))
			batch := activeSinks[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipEventSink, &batch)
		}
	}
}
