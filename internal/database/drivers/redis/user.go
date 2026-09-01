package driver_redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"

	"github.com/valkey-io/valkey-go"
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

	var oldProviders map[string]model.ExternalProvider
	if existingUser != nil {
		oldProviders = existingUser.ExternalAuthProviders
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
			err = db.del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, existingUser.Email))
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
			err = db.del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, existingUser.Email))
			if err != nil {
				return err
			}
		}

		if existingUser.Username != user.Username && (len(updateFields) == 0 || util.InArray(updateFields, "Username")) {
			// Delete the old username
			err = db.del(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(existingUser.Username)))
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
	err = db.set(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, user.Id), data, 0)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, user.Email), user.Id, 0)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(user.Username)), user.Id, 0)
	if err != nil {
		return err
	}

	// Maintain provider index
	if len(updateFields) == 0 || util.InArray(updateFields, "ExternalAuthProviders") {
		for providerID, ep := range oldProviders {
			if newEp, ok := user.ExternalAuthProviders[providerID]; !ok || newEp.ProviderUID != ep.ProviderUID {
				db.del(context.Background(), fmt.Sprintf("%sUsersByProvider:%s:%s", db.prefix, providerID, ep.ProviderUID))
			}
		}
		for providerID, ep := range user.ExternalAuthProviders {
			if ep.ProviderUID == "" {
				continue
			}
			err = db.set(context.Background(), fmt.Sprintf("%sUsersByProvider:%s:%s", db.prefix, providerID, ep.ProviderUID), user.Id, 0)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (db *RedisDbDriver) DeleteUser(user *model.User) error {
	scripts, err := db.GetScripts()
	if err == nil {
		for _, script := range scripts {
			if script.UserId == user.Id {
				db.del(context.Background(), fmt.Sprintf("%sScripts:%s", db.prefix, script.Id))
				db.del(context.Background(), fmt.Sprintf("%sScriptsByName:%s:%s", db.prefix, script.UserId, script.Name))
			}
		}
	}

	for providerID, ep := range user.ExternalAuthProviders {
		db.del(context.Background(), fmt.Sprintf("%sUsersByProvider:%s:%s", db.prefix, providerID, ep.ProviderUID))
	}

	if err = db.del(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, user.Id)); err != nil {
		return err
	}
	if err = db.del(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, user.Email)); err != nil {
		return err
	}
	return db.del(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, strings.ToLower(user.Username)))
}

func (db *RedisDbDriver) GetUser(id string) (*model.User, error) {
	var user = &model.User{}

	v, err := db.get(context.Background(), fmt.Sprintf("%sUsers:%s", db.prefix, id))
	if err != nil {
		if errors.Is(err, valkey.Nil) {
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

	v, err := db.get(context.Background(), fmt.Sprintf("%sUsersByEmail:%s", db.prefix, email))
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user, err = db.GetUser(v)
	return user, err
}

func (db *RedisDbDriver) GetUserByUsername(name string) (*model.User, error) {
	var user *model.User = nil

	v, err := db.get(context.Background(), fmt.Sprintf("%sUsersByUsername:%s", db.prefix, name))
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user, err = db.GetUser(v)
	return user, err
}

func (db *RedisDbDriver) GetUserByProviderUID(providerID, providerUID string) (*model.User, error) {
	v, err := db.get(context.Background(), fmt.Sprintf("%sUsersByProvider:%s:%s", db.prefix, providerID, providerUID))
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return db.GetUser(v)
}

func (db *RedisDbDriver) GetUsers() ([]*model.User, error) {
	var users []*model.User

	iter := db.scan(context.Background(), fmt.Sprintf("%sUsers:*", db.prefix))
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

	iter := db.scan(context.Background(), fmt.Sprintf("%sUsers:*", db.prefix))
	for iter.Next(context.Background()) {
		count++
	}
	if err := iter.Err(); err != nil {
		return false, err
	}

	return count > 0, nil
}
