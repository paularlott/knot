package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveAgentState(state *model.AgentState) error {
	// Calculate the expiration time
	state.ExpiresAfter = time.Now().UTC().Add(model.AGENT_STATE_TIMEOUT)

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sAgentState:%s", db.prefix, state.Id), data, model.AGENT_STATE_TIMEOUT).Err()
}

func (db *RedisDbDriver) DeleteAgentState(state *model.AgentState) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("%sAgentState:%s", db.prefix, state.Id)).Err()
}

func (db *RedisDbDriver) GetAgentState(id string) (*model.AgentState, error) {
	var state = &model.AgentState{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sAgentState:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &state)
	if err != nil {
		return nil, err
	}

	return state, nil
}
