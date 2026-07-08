package service

import (
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/log"
)

// Conversation retention. Conversations that haven't been updated within the
// retention age are tombstoned (soft-deleted + fresh HLC) and gossiped; the
// tombstone reaper then hard-removes them once they've had time to propagate.
//
// Two ages, two phases:
//   - conversationRetentionAge: how long a live conversation is kept. Past
//     this, the sweep tombstones + gossips it (same path as a user delete or
//     a user deleting a chat). Tombstoning — never a direct hard-delete — is
//     required for gossip correctness: a hard-deleted conversation is merely
//     absent locally, so anti-entropy from a peer that still has it would
//     re-save (resurrect) it and thrash the chat UI.
//   - conversationReapAge: how long a tombstone is kept before the reaper
//     hard-removes it. Generous enough for the tombstone (newest HLC) to
//     reach every server and win over any late-arriving live copy.
const (
	conversationRetentionAge   = 365 * 24 * time.Hour
	conversationReapAge        = 7 * 24 * time.Hour
	conversationRetentionSweep = 24 * time.Hour
)

// StartConversationRetentionSweep runs a background goroutine that tombstones
// stale conversations and reaps old tombstones. Runs once at start, then on a
// daily interval. Should be started only on full cluster members — leaf nodes
// keep chat history in the browser.
func StartConversationRetentionSweep() {
	go func() {
		sweepConversations()
		ticker := time.NewTicker(conversationRetentionSweep)
		defer ticker.Stop()
		for range ticker.C {
			sweepConversations()
		}
	}()
}

func sweepConversations() {
	db := database.GetInstance()
	now := time.Now().UTC()

	// 1. Reap tombstones that have propagated. This is the only place
	//    conversations are physically removed. BadgerDB also has a faster
	//    hourly reap, so this mainly serves the MySQL/Redis drivers.
	if err := db.DeleteTombstonedConversationsBefore(now.Add(-conversationReapAge)); err != nil {
		log.WithError(err).Error("failed to reap tombstoned chat conversations")
	}

	// 2. Tombstone + gossip conversations past the retention age. Gossiping
	//    the tombstone lets every server converge on the deletion; the reaper
	//    above then removes it locally after it has propagated.
	stale, err := db.GetStaleConversations(now.Add(-conversationRetentionAge))
	if err != nil {
		log.WithError(err).Error("failed to load stale chat conversations")
		return
	}
	transport := GetTransport()
	for _, conv := range stale {
		conv.IsDeleted = true
		conv.UpdatedAt = hlc.Now()
		if err := db.SaveConversation(conv); err != nil {
			log.WithError(err).Error("failed to tombstone stale conversation", "conversation_id", conv.Id)
			continue
		}
		if transport != nil {
			transport.GossipConversation(conv)
		}
	}
}
