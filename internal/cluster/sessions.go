package cluster

import (
	"math/rand"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleSessionFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received session full sync request")

	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		c.logger.Debug("Ignoring session full sync request from a different zone")
		return []*model.Session{}, nil
	}

	sessions := []*model.Session{}
	if err := packet.Unmarshal(&sessions); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal token full sync request")
		return nil, err
	}

	// Get the list of sessions in the system
	db := database.GetSessionStorage()
	existingSessions, err := db.GetSessions()
	if err != nil {
		return nil, err
	}

	// Merge the sessions in the background
	go c.mergeSessions(sessions)

	// Return the full dataset directly as response
	return existingSessions, nil
}

func (c *Cluster) handleSessionGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received session gossip request")

	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		c.logger.Debug("Ignoring session gossip request from a different zone")
		return nil
	}

	sessions := []*model.Session{}
	if err := packet.Unmarshal(&sessions); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal session gossip request")
		return err
	}

	// Merge the sessions with the local sessions
	if err := c.mergeSessions(sessions); err != nil {
		c.logger.WithError(err).Error("Failed to merge sessions")
		return err
	}

	return nil
}

func (c *Cluster) GossipSession(session *model.Session) {
	if c.sessionGossip && c.gossipCluster != nil {
		sessions := []*model.Session{session}
		c.gossipCluster.Send(SessionGossipMsg, &sessions)
	}
}

func (c *Cluster) DoSessionFullSync(node *gossip.Node) error {
	if c.sessionGossip && c.gossipCluster != nil {

		// If the node doesn't match our zone then ignore the request
		cfg := config.GetServerConfig()
		if node.Metadata.GetString("zone") != cfg.Zone {
			c.logger.Debug("Ignoring session full sync with node from a different zone")
			return nil
		}

		// Get the list of sessions in the system
		db := database.GetSessionStorage()
		sessions, err := db.GetSessions()
		if err != nil {
			return err
		}

		// Exchange the session list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, SessionFullSyncMsg, &sessions, &sessions); err != nil {
			return err
		}

		// Merge the sessions with the local sessions
		if err := c.mergeSessions(sessions); err != nil {
			c.logger.WithError(err).Error("Failed to merge sessions")
			return err
		}
	}

	return nil
}

// Merges the sessions from a cluster member with the local sessions
func (c *Cluster) mergeSessions(sessions []*model.Session) error {
	c.logger.Debug("Merging sessions", "number_sessions", len(sessions))

	// Get the list of sessions in the system
	db := database.GetSessionStorage()
	localSessions, err := db.GetSessions()
	if err != nil {
		return err
	}

	// Convert the list of local sessions to a map
	localSessionsMap := make(map[string]*model.Session)
	for _, session := range localSessions {
		localSessionsMap[session.Id] = session
	}

	// Merge the sessions
	for _, session := range sessions {
		if localSession, ok := localSessionsMap[session.Id]; ok {
			// If the remote session is newer than the local session then use it's data
			if session.UpdatedAt.After(localSession.UpdatedAt) {
				if err := db.SaveSession(session); err != nil {
					c.logger.Error("Failed to update session", "error", err, "id", session.Id)
				}
			}
		} else if session.ExpiresAfter.After(time.Now().UTC()) && !session.IsDeleted {
			// If the session doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveSession(session); err != nil {
				return err
			}
		}
	}

	return nil
}

// Gossips a subset of the sessions to the cluster
func (c *Cluster) gossipSessions() {
	if !c.sessionGossip || c.gossipCluster == nil {
		return
	}

	// Get the list of sessions in the system
	db := database.GetSessionStorage()
	sessions, err := db.GetSessions()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get sessions")
		return
	}

	// Shuffle the sessions
	rand.Shuffle(len(sessions), func(i, j int) {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	})

	batchSize := c.gossipCluster.CalcPayloadSize(len(sessions))
	if batchSize > 0 {
		sessions = sessions[:batchSize]
		c.gossipCluster.Send(SessionGossipMsg, &sessions)
	}
}
