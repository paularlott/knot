package driver_badgerdb

import (
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	badgerGCInterval = 5 * time.Minute
	gcInterval       = 1 * time.Minute
	garbageMaxAge    = 10 * time.Minute
)

type BadgerDbDriver struct {
	connection *badger.DB
}

func (db *BadgerDbDriver) keyExists(key string) (bool, error) {
	var exists = false

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}

		exists = item != nil
		return nil
	})

	return exists, err
}

func (db *BadgerDbDriver) Connect() error {
	log.Debug().Msg("db: connecting to BadgerDB")

	var err error
	options := badger.DefaultOptions(viper.GetString("server.badgerdb.path"))
	options.Logger = badgerdbLogger()
	options.IndexCacheSize = 100 << 20 // 100MB

	db.connection, err = badger.Open(options)
	if err == nil {

		// Start the garbage collector
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
			again:
				log.Debug().Msg("db: running GC")
				err := db.connection.RunValueLogGC(0.5)
				if err == nil {
					goto again
				}
			}
		}()
	}

	// Start a go routine to clear deleted items from the database
	go func() {
		intervalTimer := time.NewTicker(gcInterval)
		defer intervalTimer.Stop()

		for range intervalTimer.C {
			log.Debug().Msg("db: running garbage collector")

			before := time.Now().UTC()
			before = before.Add(-garbageMaxAge)

			// Remove old groups
			groups, err := db.GetGroups()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get groups")
				continue
			}

			for _, group := range groups {
				if group.IsDeleted && group.UpdatedAt.Before(before) {
					err := db.DeleteGroup(group)
					if err != nil {
						log.Error().Err(err).Str("group_id", group.Id).Msg("db: failed to delete group")
					}
				}
			}

			// Remove old roles
			roles, err := db.GetRoles()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get roles")
				continue
			}

			for _, role := range roles {
				if role.IsDeleted && role.UpdatedAt.Before(before) {
					err := db.DeleteRole(role)
					if err != nil {
						log.Error().Err(err).Str("role_id", role.Id).Msg("db: failed to delete role")
					}
				}
			}

			// Remove old spaces
			spaces, err := db.GetSpaces()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get spaces")
				continue
			}

			for _, space := range spaces {
				if space.IsDeleted && space.UpdatedAt.Before(before) {
					err := db.DeleteSpace(space)
					if err != nil {
						log.Error().Err(err).Str("space_id", space.Id).Msg("db: failed to delete space")
					}
				}
			}

			// Remove old users
			users, err := db.GetUsers()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get users")
				continue
			}

			for _, user := range users {
				if user.IsDeleted && user.UpdatedAt.Before(before) {
					err := db.DeleteUser(user)
					if err != nil {
						log.Error().Err(err).Str("user_id", user.Id).Msg("db: failed to delete user")
					}
				}
			}

			// TODO Add cleanup for other tables
		}
	}()

	return err
}
