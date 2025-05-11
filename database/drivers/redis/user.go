package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"

	"github.com/redis/go-redis/v9"
)

func (db *RedisDbDriver) SaveUser(user *model.User, updateFields []string) error {
	var err error
	var newUser bool = true

	// Load the existing user
	existingUser, _ := db.GetUser(user.Id)
	if existingUser != nil {
		newUser = false

		// Don't allow username to be changed unless deleting the user
		if !user.IsDeleted || (len(updateFields) > 0 && !util.InArray(updateFields, "IsDeleted")) {
			user.Username = existingUser.Username
		}
	}

	// If email address changed, check if the new email address is unique
	if newUser || (user.Email != existingUser.Email && (len(updateFields) == 0 || util.InArray(updateFields, "Email"))) {
		exists, err := db.keyExists(fmt.Sprintf("%sUsersByEmail:%s", db.prefix, user.Email))
		if err != nil {
			return err
		} else if exists {
			return fmt.Errorf("duplicate email address")
		}

		if !newUser {
			// Delete the old email address
			err = db.connection.Del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, existingUser.Email)).Err()
			if err != nil {
				return err
			}
		}
	}

	// Check if the new username is unique
	if newUser {
		exists, err := db.keyExists(fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(user.Username)))
		if err != nil {
			return err
		} else if exists {
			return fmt.Errorf("duplicate username")
		}
	}

	if existingUser != nil {
		if existingUser.Email != user.Email && (len(updateFields) == 0 || util.InArray(updateFields, "Email")) {
			// Delete the old email address
			err = db.connection.Del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, existingUser.Email)).Err()
			if err != nil {
				return err
			}
		}

		if existingUser.Username != user.Username && (len(updateFields) == 0 || util.InArray(updateFields, "Username")) {
			// Delete the old username
			err = db.connection.Del(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(existingUser.Username))).Err()
			if err != nil {
				return err
			}
		}
	}

	// Apply changes from new to existing existing if doing partial update
	if existingUser != nil && len(updateFields) > 0 {
		util.CopyFields(user, existingUser, updateFields)
		user = existingUser
	}

	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	// Save the new user
	err = db.connection.Set(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, user.Id), data, 0).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, user.Email), user.Id, 0).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(user.Username)), user.Id, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) DeleteUser(user *model.User) error {
	err := db.connection.Del(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, user.Id)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, user.Email)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(user.Username))).Err()
	if err != nil {
		return err
	}

	return err
}

func (db *RedisDbDriver) GetUser(id string) (*model.User, error) {
	var user = &model.User{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, id)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	err = json.Unmarshal([]byte(v), &user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (db *RedisDbDriver) GetUserByEmail(email string) (*model.User, error) {
	var user *model.User = nil

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, email)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user, err = db.GetUser(v)
	return user, err
}

func (db *RedisDbDriver) GetUserByUsername(name string) (*model.User, error) {
	var user *model.User = nil

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, name)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user, err = db.GetUser(v)
	return user, err
}

func (db *RedisDbDriver) GetUsers() ([]*model.User, error) {
	var users []*model.User

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sUsers:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		user, err := db.GetUser(iter.Val()[len(fmt.Sprintf("%sUsers:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		users = append(users, user)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort the users by username
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	return users, nil
}

func (db *RedisDbDriver) HasUsers() (bool, error) {
	var count int = 0

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sUsers:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		count++
	}
	if err := iter.Err(); err != nil {
		return false, err
	}

	return count > 0, nil
}
