package lmchatkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/lmchatkit"
)

// conversationHistoryStore implements lmchatkit.HistoryStore against knot's
// per-user database. The owning user is taken from the request context (set
// by ApiAuth, the same key userFromCtx reads), so every conversation is
// scoped to the authenticated user — one user can never read or write
// another's history.
//
// Every Save/Delete also gossips the change so the other servers in the zone
// converge. The originating server's own chat tabs are notified by lmchatkit's
// own broadcast (the PUT/DELETE handlers fire it automatically); remote
// servers' tabs are notified by the gossip→SSE bridge in internal/cluster.
type conversationHistoryStore struct{}

// NewHistoryStore returns the lmchatkit.HistoryStore backed by knot's
// database + gossip. nil user in the context yields an empty store, so
// unauthenticated requests simply see no history.
func NewHistoryStore() lmchatkit.HistoryStore {
	return &conversationHistoryStore{}
}

func (h *conversationHistoryStore) List(ctx context.Context) ([]lmchatkit.ConversationSummary, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return []lmchatkit.ConversationSummary{}, nil
	}

	rows, err := database.GetInstance().GetConversationsByUser(user.Id)
	if err != nil {
		return nil, err
	}

	out := make([]lmchatkit.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		conv, err := decodeConversation(row)
		if err != nil {
			continue
		}
		out = append(out, conv.ConversationSummary)
	}

	// Newest first — matches the sidebar ordering the lmchatkit UI expects.
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func (h *conversationHistoryStore) Get(ctx context.Context, id string) (*lmchatkit.StoredConversation, error) {
	user := userFromCtx(ctx)
	if user == nil {
		return nil, fmt.Errorf("not found")
	}

	row, err := database.GetInstance().GetConversation(user.Id, id)
	if err != nil || row == nil || row.IsDeleted {
		return nil, fmt.Errorf("not found")
	}
	return decodeConversation(row)
}

func (h *conversationHistoryStore) Save(ctx context.Context, conv *lmchatkit.StoredConversation) error {
	user := userFromCtx(ctx)
	if user == nil {
		log.Warn("chat history save: no authenticated user in context")
		return fmt.Errorf("no authenticated user")
	}

	data, err := json.Marshal(conv)
	if err != nil {
		log.WithError(err).Error("chat history save: marshal failed", "conversation_id", conv.ID)
		return err
	}

	row := &model.Conversation{
		Id:        conv.ID,
		UserId:    user.Id,
		Title:     conv.Title,
		Data:      string(data),
		UpdatedAt: hlc.Now(),
	}
	if conv.CreatedAt > 0 {
		row.CreatedAt = time.UnixMilli(conv.CreatedAt).UTC()
	} else {
		row.CreatedAt = time.Now().UTC()
	}

	if err := database.GetInstance().SaveConversation(row); err != nil {
		// The browser only sees a 500 with err.Error() in the body; make sure
		// the full error (driver-specific) is also in the server logs.
		log.WithError(err).Error("chat history save: database write failed",
			"conversation_id", conv.ID, "user_id", user.Id, "bytes", len(data))
		return err
	}

	// Propagate to the rest of the zone so a user on another server (or
	// another tab hitting another server) sees the update live.
	if transport := service.GetTransport(); transport != nil {
		transport.GossipConversation(row)
	}
	return nil
}

// Delete soft-deletes (tombstoned via IsDeleted + a fresh HLC UpdatedAt) and
// gossips the tombstone. Soft-delete is required for gossip correctness: a
// late-arriving save must not resurrect a conversation the user removed, so
// the tombstone — with the newest timestamp — always wins.
func (h *conversationHistoryStore) Delete(ctx context.Context, id string) error {
	user := userFromCtx(ctx)
	if user == nil {
		return nil
	}

	db := database.GetInstance()
	row, _ := db.GetConversation(user.Id, id)
	if row == nil {
		// Already absent — nothing to tombstone. Treat as success so the
		// browser's DELETE is idempotent.
		return nil
	}

	row.IsDeleted = true
	row.UpdatedAt = hlc.Now()
	if err := db.SaveConversation(row); err != nil {
		return err
	}

	if transport := service.GetTransport(); transport != nil {
		transport.GossipConversation(row)
	}
	return nil
}

// BroadcastConversationEvent pushes a chat-history SSE event to every
// lmchatkit client connected to THIS server. It is the receive-side half of
// the gossip→SSE bridge: when a remote server gossips a conversation change,
// internal/cluster calls this so local chat windows reload. (The originating
// server's own clients are notified automatically by lmchatkit's handlers.)
func BroadcastConversationEvent(eventType, id string) {
	if eventBroadcaster != nil {
		eventBroadcaster.Broadcast(lmchatkit.ServerEvent{Type: eventType, ID: id})
	}
}

// decodeConversation unmarshals the stored JSON blob back into the
// lmchatkit type. A corrupt blob is skipped by callers (List) rather than
// failing the whole request.
func decodeConversation(row *model.Conversation) (*lmchatkit.StoredConversation, error) {
	var conv lmchatkit.StoredConversation
	if err := json.Unmarshal([]byte(row.Data), &conv); err != nil {
		return nil, err
	}
	// Defensive: never let a tombstoned row surface through the decoded
	// summary, even if a query didn't filter it.
	conv.ID = row.Id
	return &conv, nil
}
