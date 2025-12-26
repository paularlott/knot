package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

const (
	// ResponseTTL is the default time-to-live for responses (30 days)
	ResponseTTL = 30 * 24 * time.Hour
)

func (db *RedisDbDriver) SaveResponse(response *model.Response) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	// Use TTL for the response (30 days default)
	ttl := ResponseTTL
	if response.ExpiresAt != nil {
		ttl = time.Until(*response.ExpiresAt)
		if ttl < 0 {
			ttl = time.Minute // Minimal TTL if already expired
		}
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sResponses:%s", db.prefix, response.Id), data, ttl).Err()
}

func (db *RedisDbDriver) DeleteResponse(response *model.Response) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("%sResponses:%s", db.prefix, response.Id)).Err()
}

func (db *RedisDbDriver) GetResponse(id string) (*model.Response, error) {
	var response = &model.Response{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sResponses:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (db *RedisDbDriver) GetResponses() ([]*model.Response, error) {
	var responses []*model.Response

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sResponses:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		response, err := db.GetResponse(iter.Val()[len(fmt.Sprintf("%sResponses:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		responses = append(responses, response)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort by created_at (newest first)
	sort.Slice(responses, func(i, j int) bool {
		return responses[i].CreatedAt.After(responses[j].CreatedAt)
	})

	return responses, nil
}

func (db *RedisDbDriver) GetResponsesByUser(userId string) ([]*model.Response, error) {
	responses, err := db.GetResponses()
	if err != nil {
		return nil, err
	}

	// Filter by user_id
	var filtered []*model.Response
	for _, response := range responses {
		if response.UserId == userId {
			filtered = append(filtered, response)
		}
	}

	return filtered, nil
}

func (db *RedisDbDriver) GetResponsesByStatus(status model.ResponseStatus) ([]*model.Response, error) {
	responses, err := db.GetResponses()
	if err != nil {
		return nil, err
	}

	// Filter by status
	var filtered []*model.Response
	for _, response := range responses {
		if response.Status == status {
			filtered = append(filtered, response)
		}
	}

	// For pending/in_progress, sort by created_at (oldest first for processing)
	if status == model.StatusPending || status == model.StatusInProgress {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		})
	}

	return filtered, nil
}
