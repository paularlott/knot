package cluster

import (
	"fmt"
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/audit"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleTemplateFullSync(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	log.Debug().Msg("cluster: Received template full sync request")

	templates := []*model.Template{}
	if err := packet.Unmarshal(&templates); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal template full sync request")
		return gossip.NilMsg, nil, err
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	existingTemplates, err := db.GetTemplates()
	if err != nil {
		return gossip.NilMsg, nil, err
	}

	// Merge the templates in the background
	go c.mergeTemplates(templates)

	// Return the full dataset directly as response
	return TemplateFullSyncMsg, existingTemplates, nil
}

func (c *Cluster) handleTemplateGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug().Msg("cluster: Received template gossip request")

	templates := []*model.Template{}
	if err := packet.Unmarshal(&templates); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal template gossip request")
		return err
	}

	// Merge the templates with the local templates
	if err := c.mergeTemplates(templates); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge templates")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipTemplate, &templates)
	}

	return nil
}

func (c *Cluster) GossipTemplate(template *model.Template) {
	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping template")

		templates := []*model.Template{template}
		c.gossipCluster.Send(TemplateGossipMsg, &templates)
	}

	if len(c.leafSessions) > 0 {
		log.Debug().Msg("cluster: Updating template on leaf nodes")

		templates := []*model.Template{template}
		c.sendToLeafNodes(leafmsg.MessageGossipTemplate, templates)
	}
}

func (c *Cluster) DoTemplateFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of templates in the system
		db := database.GetInstance()
		templates, err := db.GetTemplates()
		if err != nil {
			return err
		}

		// Exchange the template list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TemplateFullSyncMsg, &templates, TemplateFullSyncMsg, &templates); err != nil {
			return err
		}

		// Merge the templates with the local templates
		if err := c.mergeTemplates(templates); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to merge templates")
			return err
		}
	}

	return nil
}

// Merges the templates from a cluster member with the local templates
func (c *Cluster) mergeTemplates(templates []*model.Template) error {
	log.Debug().Int("number_templates", len(templates)).Msg("cluster: Merging templates")

	// Get the list of templates in the system
	db := database.GetInstance()
	localTemplates, err := db.GetTemplates()
	if err != nil {
		return err
	}

	// Convert the list of local templates to a map
	localTemplatesMap := make(map[string]*model.Template)
	for _, template := range localTemplates {
		localTemplatesMap[template.Id] = template
	}

	// Merge the templates
	for _, template := range templates {
		if localTemplate, ok := localTemplatesMap[template.Id]; ok {
			if template.UpdatedAt.After(localTemplate.UpdatedAt) {
				refuteDelete := false

				// If the template is changing to deleted, then we need to stop and remove spaces
				if template.IsDeleted && !localTemplate.IsDeleted {
					// Get a list of the spaces using the template on this server
					spaces, err := db.GetSpacesByTemplateId(template.Id)
					if err != nil {
						log.Error().Err(err).Msg("cluster: Failed to get spaces by template")
						continue
					}

					// Count the spaces on this server
					activeSpaces := 0
					for _, space := range spaces {
						if space.Location == config.Location && !space.IsDeleted {
							activeSpaces++
						}
					}

					// If we have spaces using the template then we refute the template delete
					if activeSpaces > 0 {
						log.Error().Msg("cluster: Template is in use by spaces, cannot delete")
						template.IsDeleted = false
						template.Name = localTemplate.Name
						template.UpdatedAt = localTemplate.UpdatedAt

						refuteDelete = true

						audit.Log(
							"cluster",
							model.AuditActorSystem,
							model.AuditEventTemplateDelete,
							fmt.Sprintf("Refuted delete of template as in use on %s (%s)", config.Location, template.Name),
							&map[string]interface{}{},
						)
					}
				}

				if err := db.SaveTemplate(template, nil); err != nil {
					log.Error().Err(err).Str("name", template.Name).Msg("cluster: Failed to update template")
				}

				if refuteDelete {
					c.GossipTemplate(template)
					log.Debug().Str("name", template.Name).Msg("cluster: Refuted template delete")
				}
			}
		} else if !template.IsDeleted {
			// If the template doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveTemplate(template, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// Gossips a subset of the templates to the cluster
func (c *Cluster) gossipTemplates() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		log.Error().Err(err).Msg("cluster: Failed to get templates")
		return
	}

	// Shuffle the templates
	rand.Shuffle(len(templates), func(i, j int) {
		templates[i], templates[j] = templates[j], templates[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.GetBatchSize(len(templates))
		if batchSize > 0 {
			log.Debug().Int("batch_size", batchSize).Int("total", len(templates)).Msg("cluster: Gossipping templates")
			templates = templates[:batchSize]
			c.gossipCluster.Send(TemplateGossipMsg, &templates)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.getBatchSize(len(templates))
		if batchSize > 0 {
			log.Debug().Int("batch_size", batchSize).Int("total", len(templates)).Msg("cluster: Templates to leaf nodes")
			templates = templates[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipTemplate, &templates)
		}
	}
}
