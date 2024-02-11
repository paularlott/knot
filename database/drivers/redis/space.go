package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/paularlott/knot/database/model"

	"github.com/redis/go-redis/v9"
)

func (db *RedisDbDriver) SaveSpace(space *model.Space) error {
  // Load the existing space
  existingSpace, _ := db.GetSpace(space.Id)
  if existingSpace == nil {
    space.CreatedAt = time.Now().UTC()
  } else {
    space.UserId = existingSpace.UserId
  }

  // If new space or name changed check if the new name is unique
  if existingSpace == nil || space.Name != existingSpace.Name {
    exists, err := db.keyExists(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, space.Name))
    if err != nil {
      return err
    } else if exists {
      return fmt.Errorf("duplicate space name")
    }
  }

  space.UpdatedAt = time.Now().UTC()
  data, err := json.Marshal(space)
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("Spaces:%s", space.Id), data, 0).Err()
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("SpacesByUserId:%s:%s", space.UserId, space.Id), space.Id, 0).Err()
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(space.Name)), space.Id, 0).Err()
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id), space.Id, 0).Err()
  if err != nil {
    return err
  }

  return nil
}

func (db *RedisDbDriver) DeleteSpace(space *model.Space) error {
  err := db.connection.Del(context.Background(), fmt.Sprintf("Spaces:%s", space.Id)).Err()
  if err != nil {
    return err
  }

  err = db.connection.Del(context.Background(), fmt.Sprintf("SpacesByUserId:%s:%s", space.UserId, space.Id)).Err()
  if err != nil {
    return err
  }

  err = db.connection.Del(context.Background(), fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(space.Name))).Err()
  if err != nil {
    return err
  }

  err = db.connection.Del(context.Background(), fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id)).Err()
  if err != nil {
    return err
  }

  return nil
}

func (db *RedisDbDriver) GetSpace(id string) (*model.Space, error) {
  var space = &model.Space{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("Spaces:%s", id)).Result()
  if err != nil {
    if err == redis.Nil {
      return nil, fmt.Errorf("space not found")
    }
    return nil, convertRedisError(err)
  }

  err = json.Unmarshal([]byte(v), &space)
  if err != nil {
    return nil, err
  }

  return space, nil
}

func (db *RedisDbDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
  var spaces []*model.Space

  iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("SpacesByUserId:%s:*", userId), 0).Iterator()
  for iter.Next(context.Background()) {
    space, err := db.GetSpace(iter.Val()[52:])
    if err != nil {
      return nil, err
    }

    spaces = append(spaces, space)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the agents by name
  sort.Slice(spaces, func(i, j int) bool {
    return spaces[i].Name < spaces[j].Name
  })

  return spaces, nil
}

func (db *RedisDbDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
  v, err := db.connection.Get(context.Background(), fmt.Sprintf("SpacesByUserIdByName:%s:%s", userId, strings.ToLower(spaceName))).Result()
  if err != nil {
    return nil, err
  }

  return db.GetSpace(v)
}

func (db *RedisDbDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
  var spaces []*model.Space

  iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("SpacesByTemplateId:%s:*", templateId), 0).Iterator()
  for iter.Next(context.Background()) {
    space, err := db.GetSpace(iter.Val()[56:])
    if err != nil {
      return nil, err
    }

    spaces = append(spaces, space)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the agents by name
  sort.Slice(spaces, func(i, j int) bool {
    return spaces[i].Name < spaces[j].Name
  })

  return spaces, nil
}

func (db *RedisDbDriver) GetSpaces() ([]*model.Space, error) {
  var spaces []*model.Space

  iter := db.connection.Scan(context.Background(), 0, "Spaces:*", 0).Iterator()
  for iter.Next(context.Background()) {
    space, err := db.GetSpace(iter.Val()[7:])
    if err != nil {
      return nil, err
    }

    spaces = append(spaces, space)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the agents by name
  sort.Slice(spaces, func(i, j int) bool {
    return spaces[i].Name < spaces[j].Name
  })

  return spaces, nil
}
