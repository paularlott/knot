package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/knot/internal/log"
)

func (c *Cluster) handleTokenFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	log.Debug("cluster: Received token full sync request")

	tokens := []*model.Token{}
	if err := packet.Unmarshal(&tokens); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal token full sync request")
		return nil, err
	}

	// Get the list of tokens in the system
	db := database.GetInstance()
	existingTokens, err := db.GetTokens()
	if err != nil {
		return nil, err
	}

	// Merge the tokens in the background
	go c.mergeTokens(tokens)

	// Return the full dataset directly as response
	return existingTokens, nil
}

func (c *Cluster) handleTokenGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug("cluster: Received token gossip request")

	tokens := []*model.Token{}
	if err := packet.Unmarshal(&tokens); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal token gossip request")
		return err
	}

	// Merge the tokens with the local tokens
	if err := c.mergeTokens(tokens); err != nil {
		log.WithError(err).Error("cluster: Failed to merge tokens")
		return err
	}

	return nil
}

func (c *Cluster) GossipToken(token *model.Token) {
	if c.gossipCluster != nil {
		tokens := []*model.Token{token}
		c.gossipCluster.Send(TokenGossipMsg, &tokens)
	}
}

func (c *Cluster) DoTokenFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {

		// Get the list of tokens in the system
		db := database.GetInstance()
		tokens, err := db.GetTokens()
		if err != nil {
			return err
		}

		// Exchange the token list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, TokenFullSyncMsg, &tokens, &tokens); err != nil {
			return err
		}

		// Merge the tokens with the local tokens
		if err := c.mergeTokens(tokens); err != nil {
			log.WithError(err).Error("cluster: Failed to merge tokens")
			return err
		}
	}

	return nil
}

// Merges the tokens from a cluster member with the local tokens
func (c *Cluster) mergeTokens(tokens []*model.Token) error {
	log.Debug("cluster: Merging tokens", "number_tokens", len(tokens))

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
					log.Error("cluster: Failed to update token", "error", err, "name", token.Name)
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
		log.WithError(err).Error("cluster: Failed to get tokens")
		return
	}

	// Shuffle the tokens
	rand.Shuffle(len(tokens), func(i, j int) {
		tokens[i], tokens[j] = tokens[j], tokens[i]
	})

	batchSize := c.gossipCluster.CalcPayloadSize(len(tokens))
	if batchSize > 0 {
		tokens = tokens[:batchSize]
		c.gossipCluster.Send(TokenGossipMsg, &tokens)
	}
}
