package model

import (
	"time"

	"github.com/paularlott/gossip/hlc"
)

// Conversation is a stored chat conversation (server-side chat history).
//
// It is owned by a single user (UserId) and holds the full lmchatkit
// StoredConversation (summary + messages) as an opaque JSON blob in Data —
// the HistoryStore implementation (internal/lmchatkit) owns the encoding,
// so this package stays free of any lmchatkit dependency.
//
// UpdatedAt is a hybrid-logical-clock timestamp used for gossip conflict
// resolution between cluster servers (the newest copy wins). IsDeleted is a
// soft-delete tombstone that is itself gossiped, so a late-arriving save can
// never resurrect a conversation the user deleted.
type Conversation struct {
	Id        string        `json:"conversation_id" db:"conversation_id,pk" msgpack:"conversation_id"`
	UserId    string        `json:"user_id" db:"user_id" msgpack:"user_id"`
	Title     string        `json:"title" db:"title" msgpack:"title"`
	Data      string        `json:"data" db:"data" msgpack:"data"`
	CreatedAt time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedAt hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
	IsDeleted bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
}
