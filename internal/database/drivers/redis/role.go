package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveRole(role *model.Role) error {
	data, err := json.Marshal(role)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sRoles:%s", db.prefix, role.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteRole(role *model.Role) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("%sRoles:%s", db.prefix, role.Id)).Err()
}

func (db *RedisDbDriver) GetRole(id string) (*model.Role, error) {
	var role = &model.Role{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sRoles:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &role)
	if err != nil {
		return nil, err
	}

	return role, nil
}

func (db *RedisDbDriver) GetRoles() ([]*model.Role, error) {
	var roles []*model.Role

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sRoles:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		role, err := db.GetRole(iter.Val()[len(fmt.Sprintf("%sRoles:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		roles = append(roles, role)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort the groups by name
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})

	return roles, nil
}
