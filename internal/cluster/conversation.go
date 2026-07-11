package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	knotlmchatkit "github.com/paularlott/knot/internal/lmchatkit"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// Chat conversation replication. Conversations are per-user history written
// by whichever server the user's browser hits; gossip fans each save/delete
// to the other servers in the zone so a user logged in on multiple servers
// (or load-balanced across them) sees the same history and gets live SSE
// updates. There is no leaf-node fanout — conversations live on origin
// (full-member) servers only.

func (c *Cluster) handleConversationFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received conversation full sync request")

	incoming := []*model.Conversation{}
	if err := packet.Unmarshal(&incoming); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal conversation full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existing, err := db.GetConversations()
	if err != nil {
		return nil, err
	}

	go c.mergeConversations(incoming)

	return existing, nil
}

func (c *Cluster) handleConversationGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received conversation gossip request")

	conversations := []*model.Conversation{}
	if err := packet.Unmarshal(&conversations); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal conversation gossip request")
		return err
	}

	if err := c.mergeConversations(conversations); err != nil {
		c.logger.WithError(err).Error("Failed to merge conversations")
		return err
	}

	return nil
}

func (c *Cluster) GossipConversation(conv *model.Conversation) {
	if c.gossipCluster != nil {
		c.logger.Trace("Gossipping conversation", "conversation_id", conv.Id, "user_id", conv.UserId)

		conversations := []*model.Conversation{conv}
		c.gossipCluster.Send(ConversationGossipMsg, &conversations)
	}
}

func (c *Cluster) DoConversationFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		conversations, err := db.GetConversations()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, ConversationFullSyncMsg, &conversations, &conversations); err != nil {
			return err
		}

		if err := c.mergeConversations(conversations); err != nil {
			c.logger.WithError(err).Error("Failed to merge conversations")
			return err
		}
	}

	return nil
}

// mergeConversations applies gossiped conversations using HLC UpdatedAt for
// conflict resolution (newest wins). On every applied change it pushes an
// event into the lmchatkit SSE broadcaster so chat windows connected to THIS
// server reload — that's the gossip→SSE bridge that makes cross-server
// history changes reach browsers on every server.
func (c *Cluster) mergeConversations(incoming []*model.Conversation) error {
	c.logger.Trace("Merging conversations", "number_conversations", len(incoming))

	db := database.GetInstance()
	local, err := db.GetConversations()
	if err != nil {
		return err
	}

	// Key by (UserId, Id) since conversations are per-user and ids are only
	// unique within a user's history.
	localMap := make(map[string]*model.Conversation)
	for _, conv := range local {
		localMap[conv.UserId+":"+conv.Id] = conv
	}

	for _, conv := range incoming {
		key := conv.UserId + ":" + conv.Id
		if localConv, ok := localMap[key]; ok {
			if conv.UpdatedAt.After(localConv.UpdatedAt) {
				if err := db.SaveConversation(conv); err != nil {
					c.logger.Error("Failed to update conversation", "error", err, "id", conv.Id)
				}
				c.notifyConversationChanged(conv)
			}
		} else {
			if err := db.SaveConversation(conv); err != nil {
				c.logger.Error("Failed to save conversation", "error", err, "id", conv.Id, "is_deleted", conv.IsDeleted)
			}
			if !conv.IsDeleted {
				c.notifyConversationChanged(conv)
			}
		}
	}

	return nil
}

// notifyConversationChanged pushes the right event type to the lmchatkit
// broadcaster: a tombstoned conversation is a delete, anything else a save.
func (c *Cluster) notifyConversationChanged(conv *model.Conversation) {
	if conv.IsDeleted {
		knotlmchatkit.BroadcastConversationEvent("conversation_deleted", conv.Id)
	} else {
		knotlmchatkit.BroadcastConversationEvent("conversation_saved", conv.Id)
	}
}

func (c *Cluster) gossipConversations() {
	if c.gossipCluster == nil {
		return
	}

	db := database.GetInstance()
	conversations, err := db.GetConversations()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get conversations")
		return
	}

	rand.Shuffle(len(conversations), func(i, j int) {
		conversations[i], conversations[j] = conversations[j], conversations[i]
	})

	batchSize := c.gossipCluster.CalcPayloadSize(len(conversations))
	if batchSize > 0 {
		c.logger.Trace("Gossipping conversations", "batch_size", batchSize, "total", len(conversations))
		batch := conversations[:batchSize]
		c.gossipCluster.Send(ConversationGossipMsg, &batch)
	}
}
