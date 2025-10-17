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
)

func (c *Cluster) handleTemplateFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received template full sync request")

	templates := []*model.Template{}
	if err := packet.Unmarshal(&templates); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal template full sync request")
		return nil, err
	}

	// Get the list of templates in the system
	db := database.GetInstance()
	existingTemplates, err := db.GetTemplates()
	if err != nil {
		return nil, err
	}

	// Merge the templates in the background
	go c.mergeTemplates(templates)

	// Return the full dataset directly as response
	return existingTemplates, nil
}

func (c *Cluster) handleTemplateGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received template gossip request")

	templates := []*model.Template{}
	if err := packet.Unmarshal(&templates); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal template gossip request")
		return err
	}

	// Merge the templates with the local templates
	if err := c.mergeTemplates(templates); err != nil {
		c.logger.WithError(err).Error("Failed to merge templates")
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
		c.logger.Debug("Gossipping template")

		templates := []*model.Template{template}
		c.gossipCluster.Send(TemplateGossipMsg, &templates)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating template on leaf nodes")

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
		if err := c.gossipCluster.SendToWithResponse(node, TemplateFullSyncMsg, &templates, &templates); err != nil {
			return err
		}

		// Merge the templates with the local templates
		if err := c.mergeTemplates(templates); err != nil {
			c.logger.WithError(err).Error("Failed to merge templates")
			return err
		}
	}

	return nil
}

// Merges the templates from a cluster member with the local templates
func (c *Cluster) mergeTemplates(templates []*model.Template) error {
	c.logger.Debug("Merging templates", "number_templates", len(templates))

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
	cfg := config.GetServerConfig()
	for _, template := range templates {
		if localTemplate, ok := localTemplatesMap[template.Id]; ok {
			if template.UpdatedAt.After(localTemplate.UpdatedAt) {
				refuteDelete := false

				// If the template is changing to deleted, then we need to stop and remove spaces
				if template.IsDeleted && !localTemplate.IsDeleted {
					// Get a list of the spaces using the template on this server
					spaces, err := db.GetSpacesByTemplateId(template.Id)
					if err != nil {
						c.logger.WithError(err).Error("Failed to get spaces by template")
						continue
					}

					// Count the spaces on this server
					activeSpaces := 0
					for _, space := range spaces {
						if space.Zone == cfg.Zone && !space.IsDeleted {
							activeSpaces++
						}
					}

					// If we have spaces using the template then we refute the template delete
					if activeSpaces > 0 {
						c.logger.Error("Template is in use by spaces, cannot delete")
						template.IsDeleted = false
						template.Name = localTemplate.Name
						template.UpdatedAt = localTemplate.UpdatedAt

						refuteDelete = true

						audit.Log(
							"cluster",
							model.AuditActorSystem,
							model.AuditEventTemplateDelete,
							fmt.Sprintf("Refuted delete of template as in use on %s (%s)", cfg.Zone, template.Name),
							&map[string]interface{}{},
						)
					}
				}

				if err := db.SaveTemplate(template, nil); err != nil {
					c.logger.Error("Failed to update template", "error", err, "name", template.Name)
				}

				if refuteDelete {
					c.GossipTemplate(template)
					c.logger.Debug("Refuted template delete", "name", template.Name)
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
		c.logger.WithError(err).Error("Failed to get templates")
		return
	}

	// Shuffle the templates
	rand.Shuffle(len(templates), func(i, j int) {
		templates[i], templates[j] = templates[j], templates[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(templates))
		if batchSize > 0 {
			c.logger.Debug("Gossipping templates", "batch_size", batchSize, "total", len(templates))
			clusterTemplates := templates[:batchSize]
			c.gossipCluster.Send(TemplateGossipMsg, &clusterTemplates)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(templates))
		if batchSize > 0 {
			c.logger.Debug("Templates to leaf nodes", "batch_size", batchSize, "total", len(templates))
			leafTemplates := templates[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipTemplate, &leafTemplates)
		}
	}
}
