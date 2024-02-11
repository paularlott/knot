package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveVolume(volume *model.Volume) error {
  // Load the existing volume
  existingVolume, _ := db.GetTemplate(volume.Id)
  if existingVolume == nil {
    volume.CreatedAt = time.Now().UTC()
  }

  volume.UpdatedAt = time.Now().UTC()
  data, err := json.Marshal(volume)
  if err != nil {
    return err
  }

  return db.connection.Set(context.Background(), fmt.Sprintf("Volumes:%s", volume.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteVolume(volume *model.Volume) error {
  return db.connection.Del(context.Background(), fmt.Sprintf("Volumes:%s", volume.Id)).Err()
}

func (db *RedisDbDriver) GetVolume(id string) (*model.Volume, error) {
  var volume = &model.Volume{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("Volumes:%s", id)).Result()
  if err != nil {
    return nil, convertRedisError(err)
  }

  err = json.Unmarshal([]byte(v), &volume)
  if err != nil {
    return nil, err
  }

  return volume, nil
}

func (db *RedisDbDriver) GetVolumes() ([]*model.Volume, error) {
  var volumes []*model.Volume

  iter := db.connection.Scan(context.Background(), 0, "Volumes:*", 0).Iterator()
  for iter.Next(context.Background()) {
    volume, err := db.GetVolume(iter.Val()[8:])
    if err != nil {
      return nil, err
    }

    volumes = append(volumes, volume)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the volumes by name
  sort.Slice(volumes, func(i, j int) bool {
    return volumes[i].Name < volumes[j].Name
  })

  return volumes, nil
}
