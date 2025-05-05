package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleRoleFullSync(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	log.Debug().Msg("cluster: Received role full sync request")

	roles := []*model.Role{}
	if err := packet.Unmarshal(&roles); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal role full sync request")
		return gossip.NilMsg, nil, err
	}

	// Get the list of roles in the system
	db := database.GetInstance()
	existingRoles, err := db.GetRoles()
	if err != nil {
		return gossip.NilMsg, nil, err
	}

	// Merge the roles in the background
	go c.mergeRoles(roles)

	// Return the full dataset directly as response
	return RoleFullSyncMsg, existingRoles, nil
}

func (c *Cluster) handleRoleGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug().Msg("cluster: Received role gossip request")

	roles := []*model.Role{}
	if err := packet.Unmarshal(&roles); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal role gossip request")
		return err
	}

	// Merge the roles with the local roles
	if err := c.mergeRoles(roles); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge roles")
		return err
	}

	return nil
}

func (c *Cluster) GossipRole(role *model.Role) {
	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping role")

		roles := []*model.Role{role}
		c.gossipCluster.Send(RoleGossipMsg, &roles)
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
		if err := c.gossipCluster.SendToWithResponse(node, RoleFullSyncMsg, &roles, RoleFullSyncMsg, &roles); err != nil {
			return err
		}

		// Merge the roles with the local roles
		if err := c.mergeRoles(roles); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to merge roles")
			return err
		}
	}

	return nil
}

// Merges the roles from a cluster member with the local roles
func (c *Cluster) mergeRoles(roles []*model.Role) error {
	log.Debug().Int("number_roles", len(roles)).Msg("cluster: Merging roles")

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
					log.Error().Err(err).Str("name", role.Name).Msg("cluster: Failed to update role")
				}

				if role.IsDeleted {
					model.DeleteRoleFromCache(role.Id)
				} else {
					model.SaveRoleToCache(role)
				}
			}
		} else if !role.IsDeleted {
			// If the role doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveRole(role); err != nil {
				return err
			}
			model.SaveRoleToCache(role)
		}
	}

	return nil
}

// Gossips a subset of the roles to the cluster
func (c *Cluster) gossipRoles() {
	// Get the list of roles in the system
	db := database.GetInstance()
	roles, err := db.GetRoles()
	if err != nil {
		log.Error().Err(err).Msg("cluster: Failed to get roles")
		return
	}

	batchSize := c.gossipCluster.GetBatchSize(len(roles))
	if batchSize == 0 {
		return // No keys to send in this batch
	}

	log.Debug().Int("batch_size", batchSize).Int("total", len(roles)).Msg("cluster: Gossipping roles")

	// Shuffle the roles
	rand.Shuffle(len(roles), func(i, j int) {
		roles[i], roles[j] = roles[j], roles[i]
	})

	// Get the 1st number of roles up to the batch size & broadcast
	roles = roles[:batchSize]
	c.gossipCluster.Send(RoleGossipMsg, &roles)
}
