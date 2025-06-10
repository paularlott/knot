package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleTemplateVarFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	log.Debug().Msg("cluster: Received template vars full sync request")

	templateVars := []*model.TemplateVar{}
	if err := packet.Unmarshal(&templateVars); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal template vars full sync request")
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
	log.Debug().Msg("cluster: Received template var gossip request")

	templateVars := []*model.TemplateVar{}
	if err := packet.Unmarshal(&templateVars); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal template var gossip request")
		return err
	}

	// Merge the template vars with the local template vars
	if err := c.mergeTemplateVars(templateVars); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge template vars")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, &templateVars)
	}

	return nil
}

func (c *Cluster) GossipTemplateVar(templateVar *model.TemplateVar) {
	if templateVar.Local {
		templateVar.IsDeleted = true
		templateVar.Value = ""
		templateVar.Name = templateVar.Id
		templateVar.Zone = ""
	}

	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping template var")

		templateVars := []*model.TemplateVar{templateVar}
		c.gossipCluster.Send(TemplateVarGossipMsg, &templateVars)
	}

	if len(c.leafSessions) > 0 {
		log.Debug().Msg("cluster: Updating template var on leaf nodes")

		if templateVar.Restricted || templateVar.Local || templateVar.Zone != "" {
			templateVar.IsDeleted = true
			templateVar.Value = ""
			templateVar.Name = templateVar.Id
			templateVar.Zone = ""
		}
		c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, []*model.TemplateVar{templateVar})
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
				templateVar.Zone = ""
			}
		}

		// Exchange the template vars list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TemplateVarFullSyncMsg, &templateVars, &templateVars); err != nil {
			return err
		}

		// Merge the template vars with the local template vars
		if err := c.mergeTemplateVars(templateVars); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to merge template vars")
			return err
		}
	}

	return nil
}

// Merges the template vars from a cluster member with the local template vars
func (c *Cluster) mergeTemplateVars(templateVars []*model.TemplateVar) error {
	log.Debug().Int("number_template_vars", len(templateVars)).Msg("cluster: Merging template vars")

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
					log.Error().Err(err).Str("name", templateVar.Name).Msg("cluster: Failed to update template var")
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
		log.Error().Err(err).Msg("cluster: Failed to get templates")
		return
	}

	// Shuffle the template vars
	rand.Shuffle(len(templateVars), func(i, j int) {
		templateVars[i], templateVars[j] = templateVars[j], templateVars[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.GetBatchSize(len(templateVars))
		if batchSize > 0 {
			log.Debug().Int("batch_size", batchSize).Int("total", len(templateVars)).Msg("cluster: Gossipping template vars")

			// Get the 1st number of template vars up to the batch size & broadcast
			templateVars = templateVars[:batchSize]
			for _, templateVar := range templateVars {
				if templateVar.Local {
					templateVar.IsDeleted = true
					templateVar.Value = ""
					templateVar.Name = templateVar.Id
					templateVar.Zone = ""
				}
			}
			c.gossipCluster.Send(TemplateVarGossipMsg, &templateVars)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.getBatchSize(len(templateVars))
		if batchSize > 0 {
			log.Debug().Int("batch_size", batchSize).Int("total", len(templateVars)).Msg("cluster: Template vars to leaf nodes")
			templateVars = templateVars[:batchSize]
			for _, templateVar := range templateVars {
				if templateVar.Restricted || templateVar.Local || templateVar.Zone != "" {
					templateVar.IsDeleted = true
					templateVar.Value = ""
					templateVar.Name = templateVar.Id
					templateVar.Zone = ""
				}
			}

			c.sendToLeafNodes(leafmsg.MessageGossipTemplateVar, &templateVars)
		}
	}
}
