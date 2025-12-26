package driver_redis

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/logger"

	"github.com/paularlott/knot/internal/log"
	"github.com/redis/go-redis/v9"
)

const (
	healthCheckInterval = 10 * time.Second
	gcInterval          = 1 * time.Hour
	garbageMaxAge       = 3 * 24 * time.Hour
)

// redisLogger adapts our logger to Redis's logging interface
type redisLogger struct {
	logGroup logger.Logger
}

func newRedisLogger() *redisLogger {
	return &redisLogger{
		logGroup: log.WithGroup("redis"),
	}
}

func (l *redisLogger) Printf(ctx context.Context, format string, v ...interface{}) {
	l.logGroup.Debug(fmt.Sprintf(format, v...))
}

type RedisDbDriver struct {
	prefix      string
	connection  redis.UniversalClient
	logger      logger.Logger
	redisLogger *redisLogger
}

func convertRedisError(err error) error {
	if err == redis.Nil {
		return nil
	}
	return err
}

func (db *RedisDbDriver) keyExists(key string) (bool, error) {
	var exists = false

	v, err := db.connection.Get(context.Background(), key).Result()
	if err == nil {
		exists = v != ""
	}

	return exists, convertRedisError(err)
}

// Performs the real connection to the database, we use this to reconnect if the database moves to a new server etc.
func (db *RedisDbDriver) realConnect() {
	db.logger.Debug("connecting to Redis")

	// Look through the list of hosts and any that start with srv+ lookup the SRV record
	cfg := config.GetServerConfig()
	hosts := cfg.Redis.Hosts
	for idx, host := range hosts {
		if host[:4] == "srv+" {
			for i := 0; i < 10; i++ {
				hostPort, err := dns.LookupSRV(host[4:])
				if err != nil {
					if i == 9 {
						db.logger.Fatal("failed to lookup SRV record for Redis server aborting after 10 attempts", "error", err)
					} else {
						db.logger.WithError(err).Error("failed to lookup SRV record for Redis server")
					}
					time.Sleep(3 * time.Second)
					continue
				}

				hosts[idx] = hostPort[0].String()
			}
		}
	}

	db.logger.Debug("connecting to redis server: , db:", "db", cfg.Redis.DB)

	db.connection = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:      hosts,
		Password:   cfg.Redis.Password,
		DB:         cfg.Redis.DB,
		MasterName: cfg.Redis.MasterName,
	})
	redis.SetLogger(db.redisLogger)

	db.logger.Debug("connected to Redis")
}

func (db *RedisDbDriver) Connect() error {
	db.logger = log.WithGroup("db")
	db.redisLogger = newRedisLogger()

	// If prefix doesn't end with : append it
	cfg := config.GetServerConfig()
	db.prefix = cfg.Redis.KeyPrefix
	if db.prefix != "" && db.prefix[len(db.prefix)-1:] != ":" {
		db.prefix += ":"
	}

	db.realConnect()

	// Monitor the connection and reconnect if the connection is lost
	go func() {
		interval := time.NewTicker(healthCheckInterval)
		defer interval.Stop()

		for range interval.C {
			db.logger.Debug("testing Redis connection")

			_, err := db.connection.Ping(context.Background()).Result()
			if err != nil {
				db.logger.WithError(err).Error("redis connection lost")
				db.connection.Close()
				db.realConnect()
			}
		}
	}()

	// Start a go routine to clear deleted items from the database
	go func() {
		intervalTimer := time.NewTicker(gcInterval)
		defer intervalTimer.Stop()

		for range intervalTimer.C {
			db.logger.Debug("running garbage collector")

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
							db.logger.Error("failed to delete group", "error", err, "group_id", group.Id)
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
							db.logger.Error("failed to delete role", "error", err, "role_id", role.Id)
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
							db.logger.Error("failed to delete space", "error", err, "space_id", space.Id)
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
							db.logger.Error("failed to delete template", "error", err, "template_id", template.Id)
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
							db.logger.Error("failed to delete template var", "error", err, "template_var_id", templateVar.Id)
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
							db.logger.Error("failed to delete user", "error", err, "user_id", user.Id)
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
							db.logger.Error("failed to delete token", "error", err, "token_id", token.Id)
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
							db.logger.Error("failed to delete volume", "error", err, "volume_id", volume.Id)
						}
					}
				}
			}

			// Remove old scripts
			scripts, err := db.GetScripts()
			if err != nil {
				db.logger.WithError(err).Error("failed to get scripts")
			} else {
				for _, script := range scripts {
					if script.IsDeleted && script.UpdatedAt.Time().Before(before) {
						err := db.DeleteScript(script)
						if err != nil {
							db.logger.Error("failed to delete script", "error", err, "script_id", script.Id)
						}
					}
				}
			}

			// Remove old responses
			responses, err := db.GetResponses()
			if err != nil {
				db.logger.WithError(err).Error("failed to get responses")
			} else {
				for _, response := range responses {
					if response.IsDeleted && response.UpdatedAt.Time().Before(before) {
						err := db.DeleteResponse(response)
						if err != nil {
							db.logger.Error("failed to delete response", "error", err, "response_id", response.Id)
						}
					}
				}
			}
		}
	}()

	return nil
}
