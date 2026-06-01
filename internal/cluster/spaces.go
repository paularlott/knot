package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/container/helper"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

type spaceCleanupTask struct {
	space *model.Space
}

func (c *Cluster) handleSpaceFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received space full sync request")

	spaces := []*model.Space{}
	if err := packet.Unmarshal(&spaces); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space full sync request")
		return nil, err
	}

	// Get the list of spaces in the system
	db := database.GetInstance()
	existingSpaces, err := db.GetSpaces()
	if err != nil {
		return nil, err
	}

	// Merge the spaces in the background
	go c.mergeSpaces(spaces)

	// Return the full dataset directly as response
	return existingSpaces, nil
}

func (c *Cluster) handleSpaceGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received space gossip request")

	spaces := []*model.Space{}
	if err := packet.Unmarshal(&spaces); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space gossip request")
		return err
	}

	// Merge the spaces in the background
	go c.mergeSpaces(spaces)

	return nil
}

func (c *Cluster) GossipSpace(space *model.Space) {
	if c.gossipCluster != nil {
		c.logger.Trace("Gossipping space")

		spaces := []*model.Space{space}
		c.gossipCluster.Send(SpaceGossipMsg, &spaces)
	}
}

func (c *Cluster) DoSpaceFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of spaces in the system
		db := database.GetInstance()
		spaces, err := db.GetSpaces()
		if err != nil {
			return err
		}

		// Exchange the space list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, SpaceFullSyncMsg, &spaces, &spaces); err != nil {
			return err
		}

		// Merge the spaces with the local spaces
		if err := c.mergeSpaces(spaces); err != nil {
			c.logger.WithError(err).Error("Failed to merge spaces")
			return err
		}
	}

	return nil
}

func (c *Cluster) EnqueueSpaceCleanup(space *model.Space) {
	if space == nil {
		return
	}

	c.spaceCleanupMux.Lock()
	if c.spaceCleanupBusy[space.Id] {
		c.spaceCleanupMux.Unlock()
		return
	}
	c.spaceCleanupBusy[space.Id] = true
	c.spaceCleanupMux.Unlock()

	c.spaceCleanupQ <- &spaceCleanupTask{space: space}
}

func (c *Cluster) runSpaceCleanupQueue() {
	for task := range c.spaceCleanupQ {
		if task == nil || task.space == nil {
			continue
		}

		db := database.GetInstance()
		template, err := db.GetTemplate(task.space.TemplateId)
		if err != nil {
			c.logger.Error("Failed to load template during queued space cleanup", "error", err, "name", task.space.Name)
		} else if template.IsLocalContainer() {
			if err := helper.NewContainerHelper().CleanupMigratedSpaceArtifacts(task.space, template); err != nil {
				c.logger.Error("Failed to clean migrated space artifacts", "error", err, "name", task.space.Name)
			}
		}

		c.spaceCleanupMux.Lock()
		delete(c.spaceCleanupBusy, task.space.Id)
		c.spaceCleanupMux.Unlock()
	}
}

// Merges the spaces from a cluster member with the local spaces
func (c *Cluster) mergeSpaces(spaces []*model.Space) error {
	c.logger.Trace("Merging spaces", "number_spaces", len(spaces))
	c.spaceMergeMux.Lock()
	defer c.spaceMergeMux.Unlock()

	// Get the list of spaces in the system
	db := database.GetInstance()
	localSpaces, err := db.GetSpaces()
	if err != nil {
		return err
	}

	localNodeId, err := c.GetLocalNodeId()
	if err != nil {
		return err
	}

	// Convert the list of local spaces to a map
	localSpacesMap := make(map[string]*model.Space)
	for _, space := range localSpaces {
		localSpacesMap[space.Id] = space
	}

	// Merge the spaces
	for _, space := range spaces {
		if localSpace, ok := localSpacesMap[space.Id]; ok {
			// If the remote space is newer than the local space then use its data
			if space.UpdatedAt.After(localSpace.UpdatedAt) {
				shouldQueueCleanup := localSpace.NodeId != "" && localSpace.NodeId == localNodeId && space.NodeId != localNodeId
				var cleanupSpace *model.Space
				if shouldQueueCleanup {
					cleanupSpace = &model.Space{
						Id:          localSpace.Id,
						UserId:      localSpace.UserId,
						TemplateId:  localSpace.TemplateId,
						Name:        localSpace.Name,
						NodeId:      localSpace.NodeId,
						ContainerId: localSpace.ContainerId,
						VolumeData:  make(model.VolumeDataMap),
					}
					for volumeName, volume := range localSpace.VolumeData {
						cleanupSpace.VolumeData[volumeName] = volume
					}
				}

				if err := db.SaveSpace(space, []string{}); err != nil {
					c.logger.Error("Failed to update space", "error", err, "name", space.Name)
					continue
				}

				if shouldQueueCleanup {
					c.EnqueueSpaceCleanup(cleanupSpace)
				}

				//  If share user update the SSH keys
				if space.SharedWithUserId != localSpace.SharedWithUserId {
					userId := space.SharedWithUserId
					if userId == "" && len(space.SharedUserIds()) > 0 {
						userId = space.SharedUserIds()[0]
					}
					if userId != "" {
						user, err := db.GetUser(userId)
						if err != nil {
							c.logger.Error("Failed to get user", "error", err, "name", space.Name)
							continue
						}
						service.GetUserService().UpdateSpaceSSHKeys(space, user)
					}
				}

				// Only publish SSE events when stateful fields that the UI cares about change
				// This prevents spamming events during startup when only UpdatedAt changes
				stateChanged := space.IsDeleted != localSpace.IsDeleted ||
					space.IsDeployed != localSpace.IsDeployed ||
					space.IsPending != localSpace.IsPending ||
					space.SharedWithUserId != localSpace.SharedWithUserId

				if space.IsDeleted {
					if stateChanged {
						sse.PublishSpaceDeleted(space.Id, space.UserId)
					}
				} else if stateChanged {
					sse.PublishSpaceChanged(space.Id, space.UserId)
				}
			}
		} else {
			// If the space doesn't exist locally, create it (even if deleted) to prevent resurrection
			if err := db.SaveSpace(space, []string{}); err != nil {
				c.logger.Error("Failed to save space", "error", err, "name", space.Name, "is_deleted", space.IsDeleted)
			}

			if space.IsDeleted {
				// Usage history ages out via retention and local reapers.
			} else {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}
	}

	return nil
}

// Gossips a subset of the space to the cluster
func (c *Cluster) gossipSpaces() {
	// Get the list of spaces in the system
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get groups")
		return
	}

	batchSize := c.gossipCluster.CalcPayloadSize(len(spaces))
	if batchSize == 0 {
		return // No keys to send in this batch
	}

	c.logger.Trace("Gossipping spaces", "batch_size", batchSize, "total", len(spaces))

	// Shuffle the spaces
	rand.Shuffle(len(spaces), func(i, j int) {
		spaces[i], spaces[j] = spaces[j], spaces[i]
	})

	// Get the 1st number of spaces up to the batch size & broadcast
	spaces = spaces[:batchSize]
	c.gossipCluster.Send(SpaceGossipMsg, &spaces)
}
