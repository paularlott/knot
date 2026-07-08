package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// Conversations are keyed by Conversations:<userId>:<id> (namespaced with
// db.prefix) so listing one user's history is a cheap SCAN rather than a
// scan-and-filter of every user's conversations.

func (db *RedisDbDriver) conversationKey(userId, id string) string {
	return fmt.Sprintf("%sConversations:%s:%s", db.prefix, userId, id)
}

func (db *RedisDbDriver) SaveConversation(conv *model.Conversation) error {
	data, err := json.Marshal(conv)
	if err != nil {
		return err
	}
	return db.connection.Set(context.Background(), db.conversationKey(conv.UserId, conv.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteConversation(conv *model.Conversation) error {
	return db.connection.Del(context.Background(), db.conversationKey(conv.UserId, conv.Id)).Err()
}

func (db *RedisDbDriver) GetConversation(userId string, id string) (*model.Conversation, error) {
	conv := &model.Conversation{}

	v, err := db.connection.Get(context.Background(), db.conversationKey(userId, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	if err := json.Unmarshal([]byte(v), conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (db *RedisDbDriver) GetConversationsByUser(userId string) ([]*model.Conversation, error) {
	return db.scanConversations(fmt.Sprintf("%sConversations:%s:*", db.prefix, userId), false)
}

func (db *RedisDbDriver) GetConversations() ([]*model.Conversation, error) {
	return db.scanConversations(fmt.Sprintf("%sConversations:*", db.prefix), true)
}

func (db *RedisDbDriver) scanConversations(pattern string, includeDeleted bool) ([]*model.Conversation, error) {
	var conversations []*model.Conversation

	iter := db.connection.Scan(context.Background(), 0, pattern, 0).Iterator()
	for iter.Next(context.Background()) {
		v, err := db.connection.Get(context.Background(), iter.Val()).Result()
		if err != nil {
			return nil, convertRedisError(err)
		}
		conv := &model.Conversation{}
		if err := json.Unmarshal([]byte(v), conv); err != nil {
			return nil, err
		}
		if !includeDeleted && conv.IsDeleted {
			continue
		}
		conversations = append(conversations, conv)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].UpdatedAt.After(conversations[j].UpdatedAt)
	})

	return conversations, nil
}

// DeleteTombstonedConversationsBefore is the reaper: it hard-deletes only
// tombstoned conversations whose UpdatedAt predates the cutoff. Redis can't
// filter server-side on the JSON blob, so we SCAN, decode, and DEL.
func (db *RedisDbDriver) DeleteTombstonedConversationsBefore(before time.Time) error {
	return db.deleteConversationsMatching(before, true)
}

// GetStaleConversations returns live (non-deleted) conversations whose
// UpdatedAt predates the cutoff — candidates for retention tombstoning.
func (db *RedisDbDriver) GetStaleConversations(before time.Time) ([]*model.Conversation, error) {
	ctx := context.Background()
	pattern := fmt.Sprintf("%sConversations:*", db.prefix)
	var out []*model.Conversation

	iter := db.connection.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		v, err := db.connection.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}
		conv := &model.Conversation{}
		if err := json.Unmarshal([]byte(v), conv); err != nil {
			continue
		}
		if !conv.IsDeleted && conv.UpdatedAt.Time().Before(before) {
			out = append(out, conv)
		}
	}
	return out, iter.Err()
}

// deleteConversationsMatching deletes conversations older than before,
// restricted to tombstones (deleted=true) or live (deleted=false) per flag.
func (db *RedisDbDriver) deleteConversationsMatching(before time.Time, tombstonesOnly bool) error {
	ctx := context.Background()
	pattern := fmt.Sprintf("%sConversations:*", db.prefix)

	var keys []string
	iter := db.connection.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		v, err := db.connection.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		conv := &model.Conversation{}
		if err := json.Unmarshal([]byte(v), conv); err != nil {
			continue
		}
		if conv.UpdatedAt.Time().Before(before) && (!tombstonesOnly || conv.IsDeleted) {
			keys = append(keys, key)
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return db.connection.Del(ctx, keys...).Err()
	}
	return nil
}
