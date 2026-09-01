package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SavePoolDefinition(pool *model.PoolDefinition, updateFields []string) error {
	existingPool, _ := db.GetPoolDefinition(pool.Id)
	if existingPool != nil {
		if (existingPool.Name != pool.Name || existingPool.CreatedUserId != pool.CreatedUserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "CreatedUserId")) {
			db.del(context.Background(), fmt.Sprintf("%sPoolsByName:%s:%s", db.prefix, existingPool.CreatedUserId, existingPool.Name))
		}
		if len(updateFields) > 0 {
			util.CopyFields(pool, existingPool, updateFields)
			pool = existingPool
		}
	}

	data, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	if err := db.set(context.Background(), fmt.Sprintf("%sPools:%s", db.prefix, pool.Id), data, 0); err != nil {
		return err
	}
	return db.set(context.Background(), fmt.Sprintf("%sPoolsByName:%s:%s", db.prefix, pool.CreatedUserId, pool.Name), pool.Id, 0)
}

func (db *RedisDbDriver) DeletePoolDefinition(pool *model.PoolDefinition) error {
	db.del(context.Background(), fmt.Sprintf("%sPoolsByName:%s:%s", db.prefix, pool.CreatedUserId, pool.Name))
	return db.del(context.Background(), fmt.Sprintf("%sPools:%s", db.prefix, pool.Id))
}

func (db *RedisDbDriver) GetPoolDefinition(id string) (*model.PoolDefinition, error) {
	pool := &model.PoolDefinition{}
	v, err := db.get(context.Background(), fmt.Sprintf("%sPools:%s", db.prefix, id))
	if err != nil {
		return nil, convertRedisError(err)
	}
	if err := json.Unmarshal([]byte(v), pool); err != nil {
		return nil, err
	}
	return pool, nil
}

func (db *RedisDbDriver) GetPoolDefinitionByName(userId, name string) (*model.PoolDefinition, error) {
	id, err := db.get(context.Background(), fmt.Sprintf("%sPoolsByName:%s:%s", db.prefix, userId, name))
	if err != nil {
		return nil, convertRedisError(err)
	}
	pool, err := db.GetPoolDefinition(id)
	if err != nil || pool.IsDeleted {
		return nil, fmt.Errorf("pool not found")
	}
	return pool, nil
}

func (db *RedisDbDriver) GetPoolDefinitions() ([]*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	iter := db.scan(context.Background(), fmt.Sprintf("%sPools:*", db.prefix))
	for iter.Next(context.Background()) {
		pool, err := db.GetPoolDefinition(iter.Val()[len(fmt.Sprintf("%sPools:", db.prefix)):])
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, nil
}

// GetPoolDefinitionsByUser returns the live pools owned by a user by scanning
// only that user's PoolsByName keyspace rather than every pool.
func (db *RedisDbDriver) GetPoolDefinitionsByUser(userId string) ([]*model.PoolDefinition, error) {
	prefix := fmt.Sprintf("%sPoolsByName:%s:", db.prefix, userId)
	var ids []string
	iter := db.scan(context.Background(), prefix+"*")
	for iter.Next(context.Background()) {
		id, err := db.get(context.Background(), iter.Val())
		if err != nil {
			return nil, convertRedisError(err)
		}
		ids = append(ids, id)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	var pools []*model.PoolDefinition
	for _, id := range ids {
		pool, err := db.GetPoolDefinition(id)
		if err != nil {
			return nil, err
		}
		if pool.IsDeleted {
			continue
		}
		pools = append(pools, pool)
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, nil
}
