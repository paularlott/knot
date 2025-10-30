package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleRoleFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received role full sync request")

	roles := []*model.Role{}
	if err := packet.Unmarshal(&roles); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal role full sync request")
		return nil, err
	}

	// Get the list of roles in the system
	db := database.GetInstance()
	existingRoles, err := db.GetRoles()
	if err != nil {
		return nil, err
	}

	// Merge the roles in the background
	go c.mergeRoles(roles)

	// Return the full dataset directly as response
	return existingRoles, nil
}

func (c *Cluster) handleRoleGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received role gossip request")

	roles := []*model.Role{}
	if err := packet.Unmarshal(&roles); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal role gossip request")
		return err
	}

	// Merge the roles with the local roles
	if err := c.mergeRoles(roles); err != nil {
		c.logger.WithError(err).Error("Failed to merge roles")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipRole, &roles)
	}

	return nil
}

func (c *Cluster) GossipRole(role *model.Role) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping role")

		roles := []*model.Role{role}
		c.gossipCluster.Send(RoleGossipMsg, &roles)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating role on leaf nodes")

		roles := []*model.Role{role}
		c.sendToLeafNodes(leafmsg.MessageGossipRole, roles)
	}
}

func (c *Cluster) DoRoleFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of roles in the system
		db := database.GetInstance()
		roles, err := db.GetRoles()
		if err != nil {
			return err
		}

		// Exchange the role list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, RoleFullSyncMsg, &roles, &roles); err != nil {
			return err
		}

		// Merge the roles with the local roles
		if err := c.mergeRoles(roles); err != nil {
			c.logger.WithError(err).Error("Failed to merge roles")
			return err
		}
	}

	return nil
}

// Merges the roles from a cluster member with the local roles
func (c *Cluster) mergeRoles(roles []*model.Role) error {
	c.logger.Debug("Merging roles", "number_roles", len(roles))

	// Get the list of roles in the system
	db := database.GetInstance()
	localRoles, err := db.GetRoles()
	if err != nil {
		return err
	}

	// Convert the list of local roles to a map
	localRolesMap := make(map[string]*model.Role)
	for _, role := range localRoles {
		localRolesMap[role.Id] = role
	}

	// Merge the roles with the local roles
	for _, role := range roles {
		if localRole, ok := localRolesMap[role.Id]; ok {
			// If the remote role is newer than the local role then use it's data
			if role.UpdatedAt.After(localRole.UpdatedAt) {
				if err := db.SaveRole(role); err != nil {
					c.logger.Error("Failed to update role", "error", err, "name", role.Name)
				}

				if role.IsDeleted {
					model.DeleteRoleFromCache(role.Id)
				} else {
					model.SaveRoleToCache(role)
				}
			}
		} else {
			// If the role doesn't exist locally, create it (even if deleted) to prevent resurrection
			if err := db.SaveRole(role); err != nil {
				c.logger.Error("Failed to save role", "error", err, "name", role.Name, "is_deleted", role.IsDeleted)
			} else if !role.IsDeleted {
				model.SaveRoleToCache(role)
			}
		}
	}

	return nil
}

// Gossips a subset of the roles to the cluster
func (c *Cluster) gossipRoles() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of roles in the system
	db := database.GetInstance()
	roles, err := db.GetRoles()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get roles")
		return
	}

	// Shuffle the roles
	rand.Shuffle(len(roles), func(i, j int) {
		roles[i], roles[j] = roles[j], roles[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(roles))
		if batchSize > 0 {
			c.logger.Debug("Gossipping roles", "batch_size", batchSize, "total", len(roles))
			clusterRoles := roles[:batchSize]
			c.gossipCluster.Send(RoleGossipMsg, &clusterRoles)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(roles))
		if batchSize > 0 {
			c.logger.Debug("Roles to leaf nodes", "batch_size", batchSize, "total", len(roles))
			leafRoles := roles[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipRole, &leafRoles)
		}
	}
}
