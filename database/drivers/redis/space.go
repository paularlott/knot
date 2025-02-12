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

// Function to take a mutex lock on the redis database so we can do save operations without overwrites
func (db *RedisDbDriver) mutexLock() error {

	for i := 0; i < 10; i++ {
		set, err := db.connection.SetNX(context.Background(), fmt.Sprintf("%sSpacesWriteLock", db.prefix), "1", 10*time.Second).Result()
		if err == nil && set {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("failed to lock")
}

// Function to release the mutex lock on the redis database
func (db *RedisDbDriver) mutexUnlock() error {
	_, err := db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesWriteLock", db.prefix)).Result()
	return err
}

func (db *RedisDbDriver) SaveSpace(space *model.Space) error {

	// Grab a mutex lock on the redis database, automatically release on function exit
	err := db.mutexLock()
	if err != nil {
		return err
	}
	defer db.mutexUnlock()

	// Load the existing space
	existingSpace, _ := db.GetSpace(space.Id)
	if existingSpace == nil {
		space.CreatedAt = time.Now().UTC()
	} else {
		// If user changed then delete the space and add back in with new user
		if existingSpace.UserId != space.UserId {
			db.DeleteSpace(existingSpace)
			existingSpace = nil
		}
	}

	// If new space or name changed check if the new name is unique
	if existingSpace == nil || space.Name != existingSpace.Name {
		exists, err := db.keyExists(fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(space.Name)))
		if err != nil {
			return err
		} else if exists {
			return fmt.Errorf("space name already used")
		}
	}

	// If existing space then check if the key exists for each new alt name
	for _, name := range space.AltNames {
		found := false
		if existingSpace != nil {
			for _, altName := range existingSpace.AltNames {
				if altName == name {
					found = true
					break
				}
			}
		}

		if !found {
			exists, err := db.keyExists(fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(name)))
			if err != nil {
				return err
			} else if exists {
				return fmt.Errorf("space name already used")
			}
		}
	}

	space.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(space)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSpaces:%s", db.prefix, space.Id), data, 0).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSpacesByUserId:%s:%s", db.prefix, space.UserId, space.Id), space.Id, 0).Err()
	if err != nil {
		return err
	}

	if existingSpace != nil && existingSpace.Name != space.Name {
		err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(existingSpace.Name))).Err()
		if err != nil {
			return err
		}
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(space.Name)), space.Id, 0).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSpacesByTemplateId:%s:%s", db.prefix, space.TemplateId, space.Id), space.Id, 0).Err()
	if err != nil {
		return err
	}

	// If existing space
	if existingSpace != nil {

		// Delete alternate names that are no longer in the list
		for _, altName := range existingSpace.AltNames {
			found := false
			for _, name := range space.AltNames {
				if altName == name {
					found = true
					break
				}
			}
			if !found {
				err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(altName))).Err()
				if err != nil {
					return err
				}
			}
		}
	}

	// Add alt names
	for _, name := range space.AltNames {
		found := false
		if existingSpace != nil {
			for _, altName := range existingSpace.AltNames {
				if altName == name {
					found = true
					break
				}
			}
		}

		if !found {
			err = db.connection.Set(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(name)), space.Id, 0).Err()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (db *RedisDbDriver) DeleteSpace(space *model.Space) error {
	err := db.connection.Del(context.Background(), fmt.Sprintf("%sSpaces:%s", db.prefix, space.Id)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByUserId:%s:%s", db.prefix, space.UserId, space.Id)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(space.Name))).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByTemplateId:%s:%s", db.prefix, space.TemplateId, space.Id)).Err()
	if err != nil {
		return err
	}

	// Delete alternate names
	for _, altName := range space.AltNames {
		err = db.connection.Del(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, space.UserId, strings.ToLower(altName))).Err()
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *RedisDbDriver) GetSpace(id string) (*model.Space, error) {
	var space = &model.Space{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSpaces:%s", db.prefix, id)).Result()
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

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSpacesByUserId:%s:*", db.prefix, userId), 0).Iterator()
	for iter.Next(context.Background()) {
		space, err := db.GetSpace(iter.Val()[len(fmt.Sprintf("%sSpacesByUserId:00000000-0000-0000-0000-000000000000:", db.prefix)):])
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
	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSpacesByUserIdByName:%s:%s", db.prefix, userId, strings.ToLower(spaceName))).Result()
	if err != nil {
		return nil, err
	}

	return db.GetSpace(v)
}

func (db *RedisDbDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
	var spaces []*model.Space

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSpacesByTemplateId:%s:*", db.prefix, templateId), 0).Iterator()
	for iter.Next(context.Background()) {
		space, err := db.GetSpace(iter.Val()[len(fmt.Sprintf("%sSpacesByTemplateId:00000000-0000-0000-0000-000000000000:", db.prefix)):])
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

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSpaces:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		space, err := db.GetSpace(iter.Val()[len(fmt.Sprintf("%sSpaces:", db.prefix)):])
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
