package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleTokenFullSync(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	log.Debug().Msg("cluster: Received token full sync request")

	// If the sender doesn't match our location then ignore the request
	if sender.Metadata.GetString("location") != config.Location {
		log.Debug().Msg("cluster: Ignoring token full sync request from a different location")
		return TokenFullSyncMsg, []*model.Token{}, nil
	}

	tokens := []*model.Token{}
	if err := packet.Unmarshal(&tokens); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal token full sync request")
		return gossip.NilMsg, nil, err
	}

	// Get the list of tokens in the system
	db := database.GetInstance()
	existingTokens, err := db.GetTokens()
	if err != nil {
		return gossip.NilMsg, nil, err
	}

	// Merge the tokens in the background
	go c.mergeTokens(tokens)

	// Return the full dataset directly as response
	return TokenFullSyncMsg, existingTokens, nil
}

func (c *Cluster) handleTokenGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug().Msg("cluster: Received token gossip request")

	// If the sender doesn't match our location then ignore the request
	if sender.Metadata.GetString("location") != config.Location {
		log.Debug().Msg("cluster: Ignoring token gossip request from a different location")
		return nil
	}

	tokens := []*model.Token{}
	if err := packet.Unmarshal(&tokens); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal token gossip request")
		return err
	}

	// Merge the tokens with the local tokens
	if err := c.mergeTokens(tokens); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge tokens")
		return err
	}

	return nil
}

func (c *Cluster) GossipToken(token *model.Token) {
	if c.gossipCluster != nil {
		tokens := []*model.Token{token}
		c.gossipInLocation(TokenGossipMsg, &tokens)
	}
}

func (c *Cluster) DoTokenFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {

		// If the node doesn't match our location then ignore the request
		if node.Metadata.GetString("location") != config.Location {
			log.Debug().Msg("cluster: Ignoring token full sync with node from a different location")
			return nil
		}

		// Get the list of tokens in the system
		db := database.GetInstance()
		tokens, err := db.GetTokens()
		if err != nil {
			return err
		}

		// Exchange the token list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TokenFullSyncMsg, &tokens, TokenFullSyncMsg, &tokens); err != nil {
			return err
		}

		// Merge the tokens with the local tokens
		if err := c.mergeTokens(tokens); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to merge tokens")
			return err
		}
	}

	return nil
}

// Merges the tokens from a cluster member with the local tokens
func (c *Cluster) mergeTokens(tokens []*model.Token) error {
	log.Debug().Int("number_tokens", len(tokens)).Msg("cluster: Merging tokens")

	// Get the list of tokens in the system
	db := database.GetInstance()
	localTokens, err := db.GetTokens()
	if err != nil {
		return err
	}

	// Convert the list of local tokens to a map
	localTokensMap := make(map[string]*model.Token)
	for _, token := range localTokens {
		localTokensMap[token.Id] = token
	}

	// Merge the tokens
	for _, token := range tokens {
		if localToken, ok := localTokensMap[token.Id]; ok {
			// If the remote token is newer than the local token then use it's data
			if token.UpdatedAt.After(localToken.UpdatedAt) {
				if err := db.SaveToken(token); err != nil {
					log.Error().Err(err).Str("name", token.Name).Msg("cluster: Failed to update token")
				}
			}
		} else if !token.IsDeleted {
			// If the token doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveToken(token); err != nil {
				return err
			}
		}
	}

	return nil
}

// Gossips a subset of the tokens to the cluster
func (c *Cluster) gossipTokens() {
	if c.gossipCluster == nil {
		return
	}

	// Get the list of tokens in the system
	db := database.GetInstance()
	tokens, err := db.GetTokens()
	if err != nil {
		log.Error().Err(err).Msg("cluster: Failed to get tokens")
		return
	}

	// Shuffle the tokens
	rand.Shuffle(len(tokens), func(i, j int) {
		tokens[i], tokens[j] = tokens[j], tokens[i]
	})

	batchSize := c.gossipCluster.GetBatchSize(len(tokens))
	if batchSize > 0 {
		tokens = tokens[:batchSize]
		c.gossipInLocation(TokenGossipMsg, &tokens)
	}
}

func (c *Cluster) gossipInLocation(msgType gossip.MessageType, data interface{}) {
	nodes := c.gossipCluster.AliveNodes()
	sameLocationNodes := []*gossip.Node{}
	localNode := c.gossipCluster.LocalNode()
	for _, node := range nodes {
		if node.ID != localNode.ID && node.Metadata.GetString("location") == config.Location {
			sameLocationNodes = append(sameLocationNodes, node)
		}
	}

	if len(sameLocationNodes) > 0 {
		batchSize := c.gossipCluster.GetBatchSize(len(sameLocationNodes))
		if batchSize < len(sameLocationNodes) {
			rand.Shuffle(len(sameLocationNodes), func(i, j int) {
				sameLocationNodes[i], sameLocationNodes[j] = sameLocationNodes[j], sameLocationNodes[i]
			})
			sameLocationNodes = sameLocationNodes[:batchSize]
		}

		for _, node := range sameLocationNodes {
			if err := c.gossipCluster.SendTo(node, msgType, data); err != nil {
				log.Error().Err(err).Str("node_id", node.ID.String()).Msg("cluster: Failed to gossip to nodes in location")
			}
		}
	}
}
