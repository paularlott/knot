package driver_badgerdb

import (
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"

	badger "github.com/dgraph-io/badger/v4"
)

const (
	badgerGCInterval = 5 * time.Minute
	gcInterval       = 1 * time.Hour
	garbageMaxAge    = 3 * 24 * time.Hour
)

type BadgerDbDriver struct {
	connection *badger.DB
	logger     logger.Logger
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
	db.logger = log.WithGroup("db")
	db.logger.Debug("connecting to BadgerDB")

	var err error
	cfg := config.GetServerConfig()
	options := badger.DefaultOptions(cfg.BadgerDB.Path)
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
				db.logger.Debug("running GC")
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
			db.logger.Debug("running garbage collector")

			// Clear old audit logs
			db.deleteAuditLogs()

			before := time.Now().UTC()
			before = before.Add(-garbageMaxAge)

			// Remove old groups
			groups, err := db.GetGroups()
			if err != nil {
				db.logger.WithError(err).Error("failed to get groups")
			} else {
				for _, group := range groups {
					if group.IsDeleted && group.UpdatedAt.Time().Before(before) {
						err := db.DeleteGroup(group)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete group", "group_id", group.Id)
						}
					}
				}
			}

			// Remove old roles
			roles, err := db.GetRoles()
			if err != nil {
				db.logger.WithError(err).Error("failed to get roles")
			} else {
				for _, role := range roles {
					if role.IsDeleted && role.UpdatedAt.Time().Before(before) {
						err := db.DeleteRole(role)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete role", "role_id", role.Id)
						}
					}
				}
			}

			// Remove old spaces
			spaces, err := db.GetSpaces()
			if err != nil {
				db.logger.WithError(err).Error("failed to get spaces")
			} else {
				for _, space := range spaces {
					if space.IsDeleted && space.UpdatedAt.Time().Before(before) {
						err := db.DeleteSpace(space)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete space", "space_id", space.Id)
						}
					}
				}
			}

			// Remove old templates
			templates, err := db.GetTemplates()
			if err != nil {
				db.logger.WithError(err).Error("failed to get templates")
			} else {
				for _, template := range templates {
					if template.IsDeleted && template.UpdatedAt.Time().Before(before) {
						err := db.DeleteTemplate(template)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete template", "template_id", template.Id)
						}
					}
				}
			}

			// Remove old template vars
			templateVars, err := db.GetTemplateVars()
			if err != nil {
				db.logger.WithError(err).Error("failed to get template vars")
			} else {
				for _, templateVar := range templateVars {
					if templateVar.IsDeleted && templateVar.UpdatedAt.Time().Before(before) {
						err := db.DeleteTemplateVar(templateVar)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete template var", "template_var_id", templateVar.Id)
						}
					}
				}
			}

			// Remove old users
			users, err := db.GetUsers()
			if err != nil {
				db.logger.WithError(err).Error("failed to get users")
			} else {
				for _, user := range users {
					if user.IsDeleted && user.UpdatedAt.Time().Before(before) {
						err := db.DeleteUser(user)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete user", "user_id", user.Id)
						}
					}
				}
			}

			// Remove old tokens
			tokens, err := db.GetTokens()
			if err != nil {
				db.logger.WithError(err).Error("failed to get tokens")
			} else {
				for _, token := range tokens {
					if token.IsDeleted && token.UpdatedAt.Time().Before(before) {
						err := db.DeleteToken(token)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete token", "token_id", token.Id)
						}
					}
				}
			}

			// Remove old volumes
			volumes, err := db.GetVolumes()
			if err != nil {
				db.logger.WithError(err).Error("failed to get volumes")
			} else {
				for _, volume := range volumes {
					if volume.IsDeleted && volume.UpdatedAt.Time().Before(before) {
						err := db.DeleteVolume(volume)
						if err != nil {
							db.logger.WithError(err).Error("failed to delete volume", "volume_id", volume.Id)
						}
					}
				}
			}
		}
	}()

	return err
}
