package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleScriptFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received script full sync request")

	scripts := []*model.Script{}
	if err := packet.Unmarshal(&scripts); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal script full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingScripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	go c.mergeScripts(scripts)

	return existingScripts, nil
}

func (c *Cluster) handleScriptGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received script gossip request")

	scripts := []*model.Script{}
	if err := packet.Unmarshal(&scripts); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal script gossip request")
		return err
	}

	if err := c.mergeScripts(scripts); err != nil {
		c.logger.WithError(err).Error("Failed to merge scripts")
		return err
	}

	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipScript, &scripts)
	}

	return nil
}

func (c *Cluster) GossipScript(script *model.Script) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping script")

		scripts := []*model.Script{script}
		c.gossipCluster.Send(ScriptGossipMsg, &scripts)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating script on leaf nodes")

		scripts := []*model.Script{script}
		c.sendToLeafNodes(leafmsg.MessageGossipScript, scripts)
	}
}

func (c *Cluster) DoScriptFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		scripts, err := db.GetScripts()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, ScriptFullSyncMsg, &scripts, &scripts); err != nil {
			return err
		}

		if err := c.mergeScripts(scripts); err != nil {
			c.logger.WithError(err).Error("Failed to merge scripts")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeScripts(scripts []*model.Script) error {
	c.logger.Debug("Merging scripts", "number_scripts", len(scripts))

	db := database.GetInstance()
	localScripts, err := db.GetScripts()
	if err != nil {
		return err
	}

	localScriptsMap := make(map[string]*model.Script)
	for _, script := range localScripts {
		localScriptsMap[script.Id] = script
	}

	for _, script := range scripts {
		if localScript, ok := localScriptsMap[script.Id]; ok {
			if script.UpdatedAt.After(localScript.UpdatedAt) {
				if err := db.SaveScript(script, nil); err != nil {
					c.logger.Error("Failed to update script", "error", err, "name", script.Name)
				}

				if script.IsDeleted {
					sse.PublishScriptsDeleted(script.Id)
				} else {
					sse.PublishScriptsChanged(script.Id)
				}
			}
		} else {
			if err := db.SaveScript(script, nil); err != nil {
				c.logger.Error("Failed to save script", "error", err, "name", script.Name, "is_deleted", script.IsDeleted)
			}

			if !script.IsDeleted {
				sse.PublishScriptsChanged(script.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipScripts() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	db := database.GetInstance()
	scripts, err := db.GetScripts()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get scripts")
		return
	}

	rand.Shuffle(len(scripts), func(i, j int) {
		scripts[i], scripts[j] = scripts[j], scripts[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(scripts))
		if batchSize > 0 {
			c.logger.Debug("Gossipping scripts", "batch_size", batchSize, "total", len(scripts))
			clusterScripts := scripts[:batchSize]
			c.gossipCluster.Send(ScriptGossipMsg, &clusterScripts)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(scripts))
		if batchSize > 0 {
			c.logger.Debug("Scripts to leaf nodes", "batch_size", batchSize, "total", len(scripts))
			leafScripts := scripts[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipScript, &leafScripts)
		}
	}
}
