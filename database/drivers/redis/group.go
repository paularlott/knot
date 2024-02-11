package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveGroup(group *model.Group) error {
  // Load the existing space
  existingSpace, _ := db.GetGroup(group.Id)
  if existingSpace == nil {
    group.CreatedAt = time.Now().UTC()
  }

  group.UpdatedUserId = group.CreatedUserId
  group.UpdatedAt = time.Now().UTC()
  data, err := json.Marshal(group)
  if err != nil {
    return err
  }

  return db.connection.Set(context.Background(), fmt.Sprintf("Groups:%s", group.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteGroup(group *model.Group) error {
  return db.connection.Del(context.Background(), fmt.Sprintf("Groups:%s", group.Id)).Err()
}

func (db *RedisDbDriver) GetGroup(id string) (*model.Group, error) {
  var group = &model.Group{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("Groups:%s", id)).Result()
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

  iter := db.connection.Scan(context.Background(), 0, "Groups:*", 0).Iterator()
  for iter.Next(context.Background()) {
    group, err := db.GetGroup(iter.Val()[7:])
    if err != nil {
      return nil, err
    }

    groups = append(groups, group)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the templates by name
  sort.Slice(groups, func(i, j int) bool {
    return groups[i].Name < groups[j].Name
  })

  return groups, nil
}
