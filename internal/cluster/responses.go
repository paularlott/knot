package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleResponseFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received response full sync request")

	responses := []*model.Response{}
	if err := packet.Unmarshal(&responses); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal response full sync request")
		return nil, err
	}

	// Get the list of responses in the system
	db := database.GetInstance()
	existingResponses, err := db.GetResponses()
	if err != nil {
		return nil, err
	}

	// Merge the responses in the background
	go c.mergeResponses(responses)

	// Return the full dataset directly as response
	return existingResponses, nil
}

func (c *Cluster) handleResponseGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received response gossip request")

	responses := []*model.Response{}
	if err := packet.Unmarshal(&responses); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal response gossip request")
		return err
	}

	// Merge the responses in the background
	go func() {
		if err := c.mergeResponses(responses); err != nil {
			c.logger.WithError(err).Error("Failed to merge responses")
		}

		// Forward to any leaf nodes
		if len(c.leafSessions) > 0 {
			c.sendToLeafNodes(leafmsg.MessageGossipResponse, &responses)
		}
	}()

	return nil
}

func (c *Cluster) GossipResponse(response *model.Response) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping response")

		responses := []*model.Response{response}
		c.gossipCluster.Send(ResponseGossipMsg, &responses)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating response on leaf nodes")

		responses := []*model.Response{response}
		c.sendToLeafNodes(leafmsg.MessageGossipResponse, responses)
	}
}

func (c *Cluster) DoResponseFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of responses in the system
		db := database.GetInstance()
		responses, err := db.GetResponses()
		if err != nil {
			return err
		}

		// Exchange the response list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, ResponseFullSyncMsg, &responses, &responses); err != nil {
			return err
		}

		// Merge the responses with the local responses
		if err := c.mergeResponses(responses); err != nil {
			c.logger.WithError(err).Error("Failed to merge responses")
			return err
		}
	}

	return nil
}

// Merges the responses from a cluster member with the local responses
func (c *Cluster) mergeResponses(responses []*model.Response) error {
	c.logger.Debug("Merging responses", "number_responses", len(responses))

	// Get the list of responses in the system
	db := database.GetInstance()
	localResponses, err := db.GetResponses()
	if err != nil {
		return err
	}

	// Convert the list of local responses to a map
	localResponsesMap := make(map[string]*model.Response)
	for _, response := range localResponses {
		localResponsesMap[response.Id] = response
	}

	// Merge the responses
	for _, response := range responses {
		if localResponse, ok := localResponsesMap[response.Id]; ok {
			// If the remote response is newer than the local response then use it's data
			if response.UpdatedAt.After(localResponse.UpdatedAt) {
				if err := db.SaveResponse(response); err != nil {
					c.logger.Error("Failed to update response", "error", err, "id", response.Id)
				}

				if response.IsDeleted {
					// No SSE event for deleted responses
				} else {
					// Could publish SSE event here if needed
				}
			}
		} else {
			// If the response doesn't exist locally, create it (even if deleted) to prevent resurrection
			if err := db.SaveResponse(response); err != nil {
				c.logger.Error("Failed to save response", "error", err, "id", response.Id, "is_deleted", response.IsDeleted)
			}

			if !response.IsDeleted {
				// Could publish SSE event here if needed
			}
		}
	}

	return nil
}

// Gossips a subset of the responses to the cluster
func (c *Cluster) gossipResponses() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	// Get the list of responses in the system
	db := database.GetInstance()
	responses, err := db.GetResponses()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get responses")
		return
	}

	// Shuffle the responses
	rand.Shuffle(len(responses), func(i, j int) {
		responses[i], responses[j] = responses[j], responses[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(responses))
		if batchSize > 0 {
			c.logger.Debug("Gossipping responses", "batch_size", batchSize, "total", len(responses))
			clusterResponses := responses[:batchSize]
			c.gossipCluster.Send(ResponseGossipMsg, &clusterResponses)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.gossipCluster.CalcPayloadSize(len(responses))
		if batchSize > 0 {
			c.logger.Debug("Responses to leaf nodes", "batch_size", batchSize, "total", len(responses))
			leafResponses := responses[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipResponse, &leafResponses)
		}
	}
}
