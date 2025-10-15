package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/knot/internal/log"
)

func (c *Cluster) handleUserFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	log.Debug("cluster: Received user full sync request")

	users := []*model.User{}
	if err := packet.Unmarshal(&users); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal user full sync request")
		return nil, err
	}

	// Get the list of users in the system
	db := database.GetInstance()
	existingUsers, err := db.GetUsers()
	if err != nil {
		return nil, err
	}

	// Merge the users in the background
	go c.mergeUsers(users)

	// Return the full dataset directly as response
	return existingUsers, nil
}

func (c *Cluster) handleUserGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug("cluster: Received user gossip request")

	users := []*model.User{}
	if err := packet.Unmarshal(&users); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal user gossip request")
		return err
	}

	// Merge the users with the local users
	if err := c.mergeUsers(users); err != nil {
		log.WithError(err).Error("cluster: Failed to merge users")
		return err
	}

	// Forward to any leaf nodes
	if len(c.leafSessions) > 0 {
		c.leafSessionMux.RLock()
		defer c.leafSessionMux.RUnlock()
		for _, session := range c.leafSessions {
			for _, user := range users {
				if session.user.Id == user.Id {
					session.SendMessage(leafmsg.MessageGossipUser, []*model.User{user})
				}
			}
		}
	}

	return nil
}

func (c *Cluster) GossipUser(user *model.User) {
	if c.gossipCluster != nil {
		log.Debug("cluster: Gossipping user")

		users := []*model.User{user}
		c.gossipCluster.Send(UserGossipMsg, &users)
	}

	if len(c.leafSessions) > 0 {
		// Look for sessions related to the user only
		c.leafSessionMux.RLock()
		defer c.leafSessionMux.RUnlock()
		for _, session := range c.leafSessions {
			if session.user.Id == user.Id {
				session.SendMessage(leafmsg.MessageGossipUser, []*model.User{user})
			}
		}
	}
}

func (c *Cluster) DoUserFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of users in the system
		db := database.GetInstance()
		users, err := db.GetUsers()
		if err != nil {
			return err
		}

		// Exchange the user list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, UserFullSyncMsg, &users, &users); err != nil {
			return err
		}

		// Merge the users with the local users
		if err := c.mergeUsers(users); err != nil {
			log.WithError(err).Error("cluster: Failed to merge users")
			return err
		}
	}

	return nil
}

// Merges the users from a cluster member with the local users
func (c *Cluster) mergeUsers(users []*model.User) error {
	log.Debug("cluster: Merging users", "number_users", len(users))

	// Get the list of users in the system
	db := database.GetInstance()
	localUsers, err := db.GetUsers()
	if err != nil {
		return err
	}

	// Convert the list of local users to a map
	localUsersMap := make(map[string]*model.User)
	for _, user := range localUsers {
		localUsersMap[user.Id] = user
	}

	// Merge the users
	for _, user := range users {
		if localUser, ok := localUsersMap[user.Id]; ok {
			// If the remote user is newer than the local user then use its data
			if user.UpdatedAt.After(localUser.UpdatedAt) {
				if err := db.SaveUser(user, nil); err != nil {
					log.Error("cluster: Failed to update user", "error", err, "name", user.Username)
				}

				// If deleting the user, then stop the spaces and delete them
				if user.IsDeleted && !localUser.IsDeleted {
					service.GetUserService().DeleteUser(user)
				} else {
					service.GetUserService().UpdateUserSpaces(user)
				}
			}
		} else if !user.IsDeleted {
			// If the user doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveUser(user, []string{}); err != nil {
				log.Error("cluster: Failed to create user", "error", err, "name", user.Username)
				return err
			}

			// Make sure we move to has users mode
			middleware.HasUsers = true
		}
	}

	return nil
}

// Gossips a subset of the users to the cluster
func (c *Cluster) gossipUsers() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of users in the system
	db := database.GetInstance()
	users, err := db.GetUsers()
	if err != nil {
		log.WithError(err).Error("cluster: Failed to get users")
		return
	}

	// Shuffle the users
	rand.Shuffle(len(users), func(i, j int) {
		users[i], users[j] = users[j], users[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(users))
		if batchSize == 0 {
			return // No keys to send in this batch
		}

		log.Debug("cluster: Gossipping users", "batch_size", batchSize, "total", len(users))

		// Get the 1st number of users up to the batch size & broadcast
		clusterUsers := users[:batchSize]
		c.gossipCluster.Send(UserGossipMsg, &clusterUsers)
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(users))
		if batchSize > 0 {
			log.Debug("cluster: Users to leaf nodes", "batch_size", batchSize, "total", len(users))

			c.leafSessionMux.RLock()
			defer c.leafSessionMux.RUnlock()

			for _, user := range users[:batchSize] {
				for _, session := range c.leafSessions {
					if session.user.Id == user.Id {
						session.SendMessage(leafmsg.MessageGossipUser, []*model.User{user})
					}
				}
			}
		}
	}
}
