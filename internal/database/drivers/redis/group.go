package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveGroup(group *model.Group) error {
	data, err := json.Marshal(group)
	if err != nil {
		return err
	}

	return db.set(context.Background(), fmt.Sprintf("%sGroups:%s", db.prefix, group.Id), data, 0)
}

func (db *RedisDbDriver) DeleteGroup(group *model.Group) error {
	return db.del(context.Background(), fmt.Sprintf("%sGroups:%s", db.prefix, group.Id))
}

func (db *RedisDbDriver) GetGroup(id string) (*model.Group, error) {
	var group = &model.Group{}

	v, err := db.get(context.Background(), fmt.Sprintf("%sGroups:%s", db.prefix, id))
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (db *RedisDbDriver) GetGroups() ([]*model.Group, error) {
	var groups []*model.Group

	iter := db.scan(context.Background(), fmt.Sprintf("%sGroups:*", db.prefix))
	for iter.Next(context.Background()) {
		group, err := db.GetGroup(iter.Val()[len(fmt.Sprintf("%sGroups:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort the groups by name
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups, nil
}
