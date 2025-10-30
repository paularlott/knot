package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleGroupFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received group full sync request")

	groups := []*model.Group{}
	if err := packet.Unmarshal(&groups); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal group full sync request")
		return nil, err
	}

	// Get the list of groups in the system
	db := database.GetInstance()
	existingGroups, err := db.GetGroups()
	if err != nil {
		return nil, err
	}

	// Merge the groups in the background
	go c.mergeGroups(groups)

	// Return the full dataset directly as response
	return existingGroups, nil
}

func (c *Cluster) handleGroupGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received group gossip request")

	groups := []*model.Group{}
	if err := packet.Unmarshal(&groups); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal group gossip request")
		return err
	}

	// Merge the groups with the local groups
	if err := c.mergeGroups(groups); err != nil {
		c.logger.WithError(err).Error("Failed to merge groups")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipGroup, &groups)
	}

	return nil
}

func (c *Cluster) GossipGroup(group *model.Group) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping group")

		groups := []*model.Group{group}
		c.gossipCluster.Send(GroupGossipMsg, &groups)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating group on leaf nodes")

		groups := []*model.Group{group}
		c.sendToLeafNodes(leafmsg.MessageGossipGroup, groups)
	}
}

func (c *Cluster) DoGroupFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of groups in the system
		db := database.GetInstance()
		groups, err := db.GetGroups()
		if err != nil {
			return err
		}

		// Exchange the group list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, GroupFullSyncMsg, &groups, &groups); err != nil {
			return err
		}

		// Merge the groups with the local groups
		if err := c.mergeGroups(groups); err != nil {
			c.logger.WithError(err).Error("Failed to merge groups")
			return err
		}
	}

	return nil
}

// Merges the groups from a cluster member with the local groups
func (c *Cluster) mergeGroups(groups []*model.Group) error {
	c.logger.Debug("Merging groups", "number_groups", len(groups))

	// Get the list of groups in the system
	db := database.GetInstance()
	localGroups, err := db.GetGroups()
	if err != nil {
		return err
	}

	// Convert the list of local groups to a map
	localGroupsMap := make(map[string]*model.Group)
	for _, group := range localGroups {
		localGroupsMap[group.Id] = group
	}

	// Merge the groups
	for _, group := range groups {
		if localGroup, ok := localGroupsMap[group.Id]; ok {
			// If the remote group is newer than the local group then use it's data
			if group.UpdatedAt.After(localGroup.UpdatedAt) {
				if err := db.SaveGroup(group); err != nil {
					c.logger.Error("Failed to update group", "error", err, "name", group.Name)
				}
			}
		} else {
			// If the group doesn't exist locally, create it (even if deleted) to prevent resurrection
			if err := db.SaveGroup(group); err != nil {
				c.logger.Error("Failed to save group", "error", err, "name", group.Name, "is_deleted", group.IsDeleted)
			}
		}
	}

	return nil
}

// Gossips a subset of the groups to the cluster
func (c *Cluster) gossipGroups() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of groups in the system
	db := database.GetInstance()
	groups, err := db.GetGroups()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get groups")
		return
	}

	// Shuffle the groups
	rand.Shuffle(len(groups), func(i, j int) {
		groups[i], groups[j] = groups[j], groups[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(groups))
		if batchSize > 0 {
			c.logger.Debug("Gossipping groups", "batch_size", batchSize, "total", len(groups))
			clusterGroups := groups[:batchSize]
			c.gossipCluster.Send(GroupGossipMsg, &clusterGroups)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.gossipCluster.CalcPayloadSize(len(groups))
		if batchSize > 0 {
			c.logger.Debug("Groups to leaf nodes", "batch_size", batchSize, "total", len(groups))
			leafGroups := groups[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipGroup, &leafGroups)
		}
	}
}
