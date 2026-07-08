package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database/model"
)

// SaveConversation upserts a conversation by its primary key.
//
// This is an atomic single-statement INSERT ... ON DUPLICATE KEY UPDATE
// rather than the read-then-write (EXISTS check inside a transaction +
// INSERT/UPDATE on a pooled connection) used by SaveCommand. That older
// pattern self-deadlocks here: the transaction's SELECT takes an InnoDB
// gap lock on the (non-existent) row, then the INSERT runs on a DIFFERENT
// pooled connection and blocks waiting for that gap lock — which is only
// released when the transaction commits, which can't happen until the INSERT
// returns. The chat UI fires persist() many times per turn, so overlapping
// saves of the same conversation are frequent and the deadlock triggers
// constantly. The upsert has no separate locking SELECT, so it can't
// deadlock; concurrent saves serialize on the row lock instead.
//
// created_at and user_id are set on insert and never changed thereafter
// (a conversation's creation time and owner are immutable).
func (db *MySQLDriver) SaveConversation(conv *model.Conversation) error {
	_, err := db.connection.Exec(`
		INSERT INTO conversations (conversation_id, user_id, title, data, created_at, updated_at, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			title = VALUES(title),
			data = VALUES(data),
			updated_at = VALUES(updated_at),
			is_deleted = VALUES(is_deleted)`,
		conv.Id, conv.UserId, conv.Title, conv.Data, conv.CreatedAt, conv.UpdatedAt, conv.IsDeleted)
	return err
}

func (db *MySQLDriver) DeleteConversation(conv *model.Conversation) error {
	_, err := db.connection.Exec("DELETE FROM conversations WHERE conversation_id = ?", conv.Id)
	return err
}

func (db *MySQLDriver) GetConversation(userId string, id string) (*model.Conversation, error) {
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

func (db *MySQLDriver) GetConversationsByUser(userId string) ([]*model.Conversation, error) {
	var conversations []*model.Conversation
	err := db.read("conversations", &conversations, nil, "user_id = ? AND is_deleted = ?", userId, false)
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

func (db *MySQLDriver) GetConversations() ([]*model.Conversation, error) {
	var conversations []*model.Conversation
	err := db.read("conversations", &conversations, nil, "1")
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

func (db *MySQLDriver) GetStaleConversations(before time.Time) ([]*model.Conversation, error) {
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
func (db *MySQLDriver) DeleteTombstonedConversationsBefore(before time.Time) error {
	threshold := hlc.FromTime(before).Uint64()
	_, err := db.connection.Exec("DELETE FROM conversations WHERE is_deleted = ? AND updated_at < ?", true, threshold)
	return err
}
