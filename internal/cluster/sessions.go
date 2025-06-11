package cluster

import (
	"math/rand"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleSessionFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	log.Debug().Msg("cluster: Received session full sync request")

	// If the sender doesn't match our zone then ignore the request
	if sender.Metadata.GetString("zone") != config.Zone {
		log.Debug().Msg("cluster: Ignoring session full sync request from a different zone")
		return []*model.Session{}, nil
	}

	sessions := []*model.Session{}
	if err := packet.Unmarshal(&sessions); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal token full sync request")
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
	log.Debug().Msg("cluster: Received session gossip request")

	// If the sender doesn't match our zone then ignore the request
	if sender.Metadata.GetString("zone") != config.Zone {
		log.Debug().Msg("cluster: Ignoring session gossip request from a different zone")
		return nil
	}

	sessions := []*model.Session{}
	if err := packet.Unmarshal(&sessions); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal session gossip request")
		return err
	}

	// Merge the sessions with the local sessions
	if err := c.mergeSessions(sessions); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge sessions")
		return err
	}

	return nil
}

func (c *Cluster) GossipSession(session *model.Session) {
	if c.sessionGossip && c.gossipCluster != nil {
		sessions := []*model.Session{session}
		c.gossipInZone(SessionGossipMsg, &sessions)
	}
}

func (c *Cluster) DoSessionFullSync(node *gossip.Node) error {
	if c.sessionGossip && c.gossipCluster != nil {

		// If the node doesn't match our zone then ignore the request
		if node.Metadata.GetString("zone") != config.Zone {
			log.Debug().Msg("cluster: Ignoring session full sync with node from a different zone")
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
			log.Error().Err(err).Msg("cluster: Failed to merge sessions")
			return err
		}
	}

	return nil
}

// Merges the sessions from a cluster member with the local sessions
func (c *Cluster) mergeSessions(sessions []*model.Session) error {
	log.Debug().Int("number_sessions", len(sessions)).Msg("cluster: Merging sessions")

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
					log.Error().Err(err).Str("id", session.Id).Msg("cluster: Failed to update session")
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
		log.Error().Err(err).Msg("cluster: Failed to get sessions")
		return
	}

	// Shuffle the sessions
	rand.Shuffle(len(sessions), func(i, j int) {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	})

	batchSize := c.gossipCluster.GetBatchSize(len(sessions))
	if batchSize > 0 {
		sessions = sessions[:batchSize]
		c.gossipInZone(SessionGossipMsg, &sessions)
	}
}
