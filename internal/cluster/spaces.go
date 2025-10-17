package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

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
	c.logger.Debug("Received space gossip request")

	spaces := []*model.Space{}
	if err := packet.Unmarshal(&spaces); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space gossip request")
		return err
	}

	// Merge the spaces with the local spaces
	if err := c.mergeSpaces(spaces); err != nil {
		c.logger.WithError(err).Error("Failed to merge spaces")
		return err
	}

	return nil
}

func (c *Cluster) GossipSpace(space *model.Space) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping space")

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

// Merges the spaces from a cluster member with the local spaces
func (c *Cluster) mergeSpaces(spaces []*model.Space) error {
	c.logger.Debug("Merging spaces", "number_spaces", len(spaces))

	// Get the list of spaces in the system
	db := database.GetInstance()
	localSpaces, err := db.GetSpaces()
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
				if err := db.SaveSpace(space, []string{}); err != nil {
					c.logger.Error("Failed to update space", "error", err, "name", space.Name)
				}

				//  If share user update the SSH keys
				if space.SharedWithUserId != localSpace.SharedWithUserId {
					user, err := db.GetUser(space.SharedWithUserId)
					if err != nil {
						c.logger.Error("Failed to get user", "error", err, "name", space.Name)
						continue
					}
					service.GetUserService().UpdateSpaceSSHKeys(space, user)
				}
			}
		} else if !space.IsDeleted {
			// If the space doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveSpace(space, []string{}); err != nil {
				return err
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

	c.logger.Debug("Gossipping spaces", "batch_size", batchSize, "total", len(spaces))

	// Shuffle the spaces
	rand.Shuffle(len(spaces), func(i, j int) {
		spaces[i], spaces[j] = spaces[j], spaces[i]
	})

	// Get the 1st number of spaces up to the batch size & broadcast
	spaces = spaces[:batchSize]
	c.gossipCluster.Send(SpaceGossipMsg, &spaces)
}
