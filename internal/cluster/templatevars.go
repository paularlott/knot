package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleTemplateVarFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received template vars full sync request")

	templateVars := []*model.TemplateVar{}
	if err := packet.Unmarshal(&templateVars); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal template vars full sync request")
		return nil, err
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	existingTemplateVars, err := db.GetTemplateVars()
	if err != nil {
		return nil, err
	}

	// Merge the template vars in the background
	go c.mergeTemplateVars(templateVars)

	// Return the full dataset directly as response
	return existingTemplateVars, nil
}

func (c *Cluster) handleTemplateVarGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received template var gossip request")

	templateVars := []*model.TemplateVar{}
	if err := packet.Unmarshal(&templateVars); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal template var gossip request")
		return err
	}

	// Merge the template vars with the local template vars
	if err := c.mergeTemplateVars(templateVars); err != nil {
		c.logger.WithError(err).Error("Failed to merge template vars")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, &templateVars)
	}

	return nil
}

func (c *Cluster) GossipTemplateVar(templateVar *model.TemplateVar) {
	varToGossip := templateVar

	// Only create a copy if we need to modify it
	if templateVar.Local {
		copied := *templateVar
		copied.IsDeleted = true
		copied.Value = ""
		copied.Name = copied.Id
		copied.Zones = []string{}
		varToGossip = &copied
	}

	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping template var")
		templateVars := []*model.TemplateVar{varToGossip}
		c.gossipCluster.Send(TemplateVarGossipMsg, &templateVars)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating template var on leaf nodes")

		// Only allow vars that have empty zones or explicitly mention leaf node zone
		allowVar := len(templateVar.Zones) == 0
		for _, zone := range templateVar.Zones {
			if zone == model.LeafNodeZone {
				allowVar = true
				break
			}
		}

		leafVarToGossip := varToGossip
		if !varToGossip.IsDeleted && (varToGossip.Restricted || templateVar.Local || !allowVar) {
			// Always create a copy for leaf nodes if we need to modify it to avoid race conditions
			copied := *templateVar
			copied.IsDeleted = true
			copied.Value = ""
			copied.Name = copied.Id
			copied.Zones = []string{}
			leafVarToGossip = &copied
		}
		c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, []*model.TemplateVar{leafVarToGossip})
	}
}

func (c *Cluster) DoTemplateVarFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of templates in the system
		db := database.GetInstance()
		templateVars, err := db.GetTemplateVars()
		if err != nil {
			return err
		}

		// Tag local variables for deletion
		for _, templateVar := range templateVars {
			if templateVar.Local {
				templateVar.IsDeleted = true
				templateVar.Value = ""
				templateVar.Name = templateVar.Id
				templateVar.Zones = []string{}
			}
		}

		// Exchange the template vars list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TemplateVarFullSyncMsg, &templateVars, &templateVars); err != nil {
			return err
		}

		// Merge the template vars with the local template vars
		if err := c.mergeTemplateVars(templateVars); err != nil {
			c.logger.WithError(err).Error("Failed to merge template vars")
			return err
		}
	}

	return nil
}

// Merges the template vars from a cluster member with the local template vars
func (c *Cluster) mergeTemplateVars(templateVars []*model.TemplateVar) error {
	c.logger.Debug("Merging template vars", "number_template_vars", len(templateVars))

	// Get the list of templates in the system
	db := database.GetInstance()
	localTemplateVars, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	// Convert the list of local template vars to a map
	localTemplateVarsMap := make(map[string]*model.TemplateVar)
	for _, templateVar := range localTemplateVars {
		localTemplateVarsMap[templateVar.Id] = templateVar
	}

	// Merge the template vars
	for _, templateVar := range templateVars {
		if localTemplateVar, ok := localTemplateVarsMap[templateVar.Id]; ok {
			if templateVar.UpdatedAt.After(localTemplateVar.UpdatedAt) {
				if err := db.SaveTemplateVar(templateVar); err != nil {
					c.logger.Error("Failed to update template var", "error", err, "name", templateVar.Name)
				}
			}
		} else if !templateVar.IsDeleted {
			// If the template doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveTemplateVar(templateVar); err != nil {
				return err
			}
		}
	}

	return nil
}

// Gossips a subset of the template vars to the cluster
func (c *Cluster) gossipTemplateVars() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	templateVars, err := db.GetTemplateVars()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get template variables")
		return
	}

	// Shuffle the template vars
	rand.Shuffle(len(templateVars), func(i, j int) {
		templateVars[i], templateVars[j] = templateVars[j], templateVars[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(templateVars))
		if batchSize > 0 {
			c.logger.Debug("Gossipping template vars", "batch_size", batchSize, "total", len(templateVars))

			// Get the 1st number of template vars up to the batch size & broadcast
			clusterVars := templateVars[:batchSize]
			for _, templateVar := range clusterVars {
				if templateVar.Local {
					templateVar.IsDeleted = true
					templateVar.Value = ""
					templateVar.Name = templateVar.Id
					templateVar.Zones = []string{}
				}
			}
			c.gossipCluster.Send(TemplateVarGossipMsg, &clusterVars)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(templateVars))
		if batchSize > 0 {
			c.logger.Debug("Template vars to leaf nodes", "batch_size", batchSize, "total", len(templateVars))
			leafVars := templateVars[:batchSize]
			for _, templateVar := range leafVars {
				// Only allow vars that have empty zones or explicitly mention leaf node zone
				allowVar := len(templateVar.Zones) == 0
				for _, zone := range templateVar.Zones {
					if zone == model.LeafNodeZone {
						allowVar = true
						break
					}
				}

				if templateVar.Restricted || templateVar.Local || !allowVar {
					templateVar.IsDeleted = true
					templateVar.Value = ""
					templateVar.Name = templateVar.Id
					templateVar.Zones = []string{}
				}
			}

			c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, &leafVars)
		}
	}
}
