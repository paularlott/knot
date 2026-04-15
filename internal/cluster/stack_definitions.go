package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleStackDefinitionFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received stack definition full sync request")

	defs := []*model.StackDefinition{}
	if err := packet.Unmarshal(&defs); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal stack definition full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingDefs, err := db.GetStackDefinitions()
	if err != nil {
		return nil, err
	}

	go c.mergeStackDefinitions(defs)

	return existingDefs, nil
}

func (c *Cluster) handleStackDefinitionGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received stack definition gossip request")

	defs := []*model.StackDefinition{}
	if err := packet.Unmarshal(&defs); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal stack definition gossip request")
		return err
	}

	if err := c.mergeStackDefinitions(defs); err != nil {
		c.logger.WithError(err).Error("Failed to merge stack definitions")
		return err
	}

	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipStackDefinition, &defs)
	}

	return nil
}

func (c *Cluster) GossipStackDefinition(stackDef *model.StackDefinition) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping stack definition")

		defs := []*model.StackDefinition{stackDef}
		c.gossipCluster.Send(StackDefinitionGossipMsg, &defs)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating stack definition on leaf nodes")

		defs := []*model.StackDefinition{stackDef}
		c.sendToLeafNodes(leafmsg.MessageGossipStackDefinition, &defs)
	}
}

func (c *Cluster) DoStackDefinitionFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		defs, err := db.GetStackDefinitions()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, StackDefinitionFullSyncMsg, &defs, &defs); err != nil {
			return err
		}

		if err := c.mergeStackDefinitions(defs); err != nil {
			c.logger.WithError(err).Error("Failed to merge stack definitions")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeStackDefinitions(defs []*model.StackDefinition) error {
	c.logger.Debug("Merging stack definitions", "number_definitions", len(defs))

	db := database.GetInstance()
	localDefs, err := db.GetStackDefinitions()
	if err != nil {
		return err
	}

	localDefsMap := make(map[string]*model.StackDefinition)
	for _, def := range localDefs {
		localDefsMap[def.Id] = def
	}

	for _, def := range defs {
		if localDef, ok := localDefsMap[def.Id]; ok {
			if def.UpdatedAt.After(localDef.UpdatedAt) {
				if err := db.SaveStackDefinition(def, nil); err != nil {
					c.logger.Error("Failed to update stack definition", "error", err, "name", def.Name)
				}

				if def.IsDeleted {
					sse.PublishStackDefinitionsDeleted(def.Id)
				} else {
					sse.PublishStackDefinitionsChanged(def.Id)
				}
			}
		} else {
			if err := db.SaveStackDefinition(def, nil); err != nil {
				c.logger.Error("Failed to save stack definition", "error", err, "name", def.Name, "is_deleted", def.IsDeleted)
			}

			if !def.IsDeleted {
				sse.PublishStackDefinitionsChanged(def.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipStackDefinitions() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	db := database.GetInstance()
	defs, err := db.GetStackDefinitions()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get stack definitions")
		return
	}

	// Filter to only active, global definitions for leaf nodes
	activeDefs := []*model.StackDefinition{}
	for _, def := range defs {
		if def.Active && def.UserId == "" {
			activeDefs = append(activeDefs, def)
		}
	}

	rand.Shuffle(len(defs), func(i, j int) {
		defs[i], defs[j] = defs[j], defs[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(defs))
		if batchSize > 0 {
			c.logger.Debug("Gossipping stack definitions", "batch_size", batchSize, "total", len(defs))
			clusterDefs := defs[:batchSize]
			c.gossipCluster.Send(StackDefinitionGossipMsg, &clusterDefs)
		}
	}

	if len(c.leafSessions) > 0 && len(activeDefs) > 0 {
		rand.Shuffle(len(activeDefs), func(i, j int) {
			activeDefs[i], activeDefs[j] = activeDefs[j], activeDefs[i]
		})
		batchSize := c.CalcLeafPayloadSize(len(activeDefs))
		if batchSize > 0 {
			c.logger.Debug("Stack definitions to leaf nodes", "batch_size", batchSize, "total", len(activeDefs))
			leafDefs := activeDefs[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipStackDefinition, &leafDefs)
		}
	}
}
