package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/database/model"

	"github.com/redis/go-redis/v9"
)

func (db *RedisDbDriver) SaveDbService(service *model.DbService) error {
  var err error

  // Load the existing service
  existingUser, _ := db.GetDbService(service.Id)

  // Check if the new name is unique
  if existingUser != nil {
    exists, err := db.keyExists(fmt.Sprintf("DbServiceByName:%s", strings.ToLower(service.Name)))
    if err != nil {
      return err
    } else if exists {
      return fmt.Errorf("duplicate database service name")
    }
  } else {
    service.Name = existingUser.Name
  }

  data, err := json.Marshal(service)
  if err != nil {
    return err
  }

  // Save the new user
  err = db.connection.Set(context.Background(), fmt.Sprintf("DbService:%s", service.Id), data, 0).Err()
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("DbServiceByName:%s", service.Name), service.Id, 0).Err()
  if err != nil {
    return err
  }

  return nil
}

func (db *RedisDbDriver) DeleteDbService(service *model.DbService) error {
  err := db.connection.Del(context.Background(), fmt.Sprintf("DbService:%s", service.Id)).Err()
  if err != nil {
    return err
  }

  err = db.connection.Del(context.Background(), fmt.Sprintf("DbServiceByName:%s", service.Name)).Err()
  if err != nil {
    return err
  }

  return err
}

func (db *RedisDbDriver) GetDbService(id string) (*model.DbService, error) {
  var service = &model.DbService{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("DbService:%s", id)).Result()
  if err != nil {
    if err == redis.Nil {
      return nil, fmt.Errorf("database service not found")
    }
    return nil, err
  }

  err = json.Unmarshal([]byte(v), &service)
  if err != nil {
    return nil,err
  }

  return service, nil
}

func (db *RedisDbDriver) GetDbServiceByName(name string) (*model.DbService, error) {
  var service = &model.DbService{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("DbServiceByName:%s", name)).Result()
  if err != nil {
    if err == redis.Nil {
      return nil, fmt.Errorf("database service not found")
    }
    return nil, err
  }

  err = json.Unmarshal([]byte(v), &service)
  if err != nil {
    return nil,err
  }

  return service, nil
}

func (db *RedisDbDriver) GetDbServices() ([]*model.DbService, error) {
  var services []*model.DbService

  iter := db.connection.Scan(context.Background(), 0, "DbService:*", 0).Iterator()
  for iter.Next(context.Background()) {
    service, err := db.GetDbService(iter.Val()[6:])
    if err != nil {
      return nil, err
    }

    services = append(services, service)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort by name
  sort.Slice(services, func(i, j int) bool {
    return services[i].Name < services[j].Name
  })

  return services, nil
}
