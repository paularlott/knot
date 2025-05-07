package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleTemplateVarFullSync(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	log.Debug().Msg("cluster: Received template vars full sync request")

	templateVars := []*model.TemplateVar{}
	if err := packet.Unmarshal(&templateVars); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal template vars full sync request")
		return gossip.NilMsg, nil, err
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	existingTemplateVars, err := db.GetTemplateVars()
	if err != nil {
		return gossip.NilMsg, nil, err
	}

	// Merge the template vars in the background
	go c.mergeTemplateVars(existingTemplateVars)

	// Return the full dataset directly as response
	return TemplateVarFullSyncMsg, existingTemplateVars, nil
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

	return nil
}

func (c *Cluster) GossipTemplateVar(templateVar *model.TemplateVar) {
	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping template var")

		templateVars := []*model.TemplateVar{templateVar}
		c.gossipCluster.Send(TemplateVarGossipMsg, &templateVars)
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

		// Exchange the template vars list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TemplateVarFullSyncMsg, &templateVars, TemplateVarFullSyncMsg, &templateVars); err != nil {
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
	// Get the list of templates in the system
	db := database.GetInstance()
	templateVars, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Err(err).Msg("cluster: Failed to get templates")
		return
	}

	batchSize := c.gossipCluster.GetBatchSize(len(templateVars))
	if batchSize == 0 {
		return // No keys to send in this batch
	}

	log.Debug().Int("batch_size", batchSize).Int("total", len(templateVars)).Msg("cluster: Gossipping template vars")

	// Shuffle the template vars
	rand.Shuffle(len(templateVars), func(i, j int) {
		templateVars[i], templateVars[j] = templateVars[j], templateVars[i]
	})

	// Get the 1st number of template vars up to the batch size & broadcast
	templateVars = templateVars[:batchSize]
	c.gossipCluster.Send(TemplateVarGossipMsg, &templateVars)
}
