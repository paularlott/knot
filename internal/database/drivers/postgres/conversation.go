package driver_postgres

import (
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database/model"
)

// SaveConversation upserts a conversation by its primary key.
//
// This is a single-statement INSERT ... ON CONFLICT DO UPDATE rather than
// the read-then-write used by SaveCommand: the chat UI fires persist() many
// times per turn, so overlapping saves of the same conversation are frequent
// and must not race each other into a duplicate key error. Concurrent saves
// serialize on the row lock instead.
//
// created_at and user_id are set on insert and never changed thereafter
// (a conversation's creation time and owner are immutable).
func (db *PostgresDriver) SaveConversation(conv *model.Conversation) error {
	_, err := db.connection.Exec(`
		INSERT INTO conversations (conversation_id, user_id, title, data, created_at, updated_at, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id) DO UPDATE SET
			title = EXCLUDED.title,
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at,
			is_deleted = EXCLUDED.is_deleted`,
		conv.Id, conv.UserId, conv.Title, conv.Data, conv.CreatedAt, conv.UpdatedAt, conv.IsDeleted)
	return err
}

func (db *PostgresDriver) DeleteConversation(conv *model.Conversation) error {
	_, err := db.connection.Exec("DELETE FROM conversations WHERE conversation_id = ?", conv.Id)
	return err
}

func (db *PostgresDriver) GetConversation(userId string, id string) (*model.Conversation, error) {
	var conversations []*model.Conversation
	err := db.read("conversations", &conversations, nil, "user_id = ? AND conversation_id = ?", userId, id)
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, fmt.Errorf("conversation not found")
	}
	return conversations[0], nil
}

func (db *PostgresDriver) GetConversationsByUser(userId string) ([]*model.Conversation, error) {
	var conversations []*model.Conversation
	err := db.read("conversations", &conversations, nil, "user_id = ? AND is_deleted = ?", userId, false)
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

func (db *PostgresDriver) GetConversations() ([]*model.Conversation, error) {
	var conversations []*model.Conversation
	err := db.read("conversations", &conversations, nil, "TRUE")
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

func (db *PostgresDriver) GetStaleConversations(before time.Time) ([]*model.Conversation, error) {
	var conversations []*model.Conversation
	threshold := hlc.FromTime(before).Uint64()
	err := db.read("conversations", &conversations, nil, "is_deleted = ? AND updated_at < ?", false, threshold)
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

// DeleteTombstonedConversationsBefore is the reaper: it hard-deletes only
// tombstoned conversations whose UpdatedAt predates the cutoff. updated_at
// stores the HLC as a BIGINT; hlc.FromTime yields the comparable threshold.
func (db *PostgresDriver) DeleteTombstonedConversationsBefore(before time.Time) error {
	threshold := hlc.FromTime(before).Uint64()
	_, err := db.connection.Exec("DELETE FROM conversations WHERE is_deleted = ? AND updated_at < ?", true, threshold)
	return err
}
