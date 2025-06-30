package driver_redis

import (
	"context"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const (
	healthCheckInterval = 10 * time.Second
	gcInterval          = 1 * time.Hour
	garbageMaxAge       = 3 * 24 * time.Hour
)

type RedisDbDriver struct {
	prefix     string
	connection redis.UniversalClient
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
	log.Debug().Msg("db: connecting to Redis")

	// Look through the list of hosts and any that start with srv+ lookup the SRV record
	cfg := config.GetServerConfig()
	hosts := cfg.Redis.Hosts
	for idx, host := range hosts {
		if host[:4] == "srv+" {
			for i := 0; i < 10; i++ {
				hostPort, err := dns.LookupSRV(host[4:])
				if err != nil {
					if i == 9 {
						log.Fatal().Err(err).Msg("db: failed to lookup SRV record for Redis server aborting after 10 attempts")
					} else {
						log.Error().Err(err).Msg("db: failed to lookup SRV record for Redis server")
					}
					time.Sleep(3 * time.Second)
					continue
				}

				hosts[idx] = hostPort[0].Host + ":" + hostPort[0].Port
			}
		}
	}

	log.Debug().Msgf("db: connecting to redis server: %s, db: %d", hosts, cfg.Redis.DB)

	db.connection = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:      hosts,
		Password:   cfg.Redis.Password,
		DB:         cfg.Redis.DB,
		MasterName: cfg.Redis.MasterName,
	})

	log.Debug().Msg("db: connected to Redis")
}

func (db *RedisDbDriver) Connect() error {

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
			log.Debug().Msg("db: testing Redis connection")

			_, err := db.connection.Ping(context.Background()).Result()
			if err != nil {
				log.Error().Err(err).Msg("db: redis connection lost")
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
			log.Debug().Msg("db: running garbage collector")

			before := time.Now().UTC()
			before = before.Add(-garbageMaxAge)

			// Remove old groups
			groups, err := db.GetGroups()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get groups")
			} else {
				for _, group := range groups {
					if group.IsDeleted && group.UpdatedAt.Before(before) {
						err := db.DeleteGroup(group)
						if err != nil {
							log.Error().Err(err).Str("group_id", group.Id).Msg("db: failed to delete group")
						}
					}
				}
			}

			// Remove old roles
			roles, err := db.GetRoles()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get roles")
			} else {
				for _, role := range roles {
					if role.IsDeleted && role.UpdatedAt.Before(before) {
						err := db.DeleteRole(role)
						if err != nil {
							log.Error().Err(err).Str("role_id", role.Id).Msg("db: failed to delete role")
						}
					}
				}
			}

			// Remove old spaces
			spaces, err := db.GetSpaces()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get spaces")
			} else {
				for _, space := range spaces {
					if space.IsDeleted && space.UpdatedAt.Before(before) {
						err := db.DeleteSpace(space)
						if err != nil {
							log.Error().Err(err).Str("space_id", space.Id).Msg("db: failed to delete space")
						}
					}
				}
			}

			// Remove old templates
			templates, err := db.GetTemplates()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get templates")
			} else {
				for _, template := range templates {
					if template.IsDeleted && template.UpdatedAt.Before(before) {
						err := db.DeleteTemplate(template)
						if err != nil {
							log.Error().Err(err).Str("template_id", template.Id).Msg("db: failed to delete template")
						}
					}
				}
			}

			// Remove old template vars
			templateVars, err := db.GetTemplateVars()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get template vars")
			} else {
				for _, templateVar := range templateVars {
					if templateVar.IsDeleted && templateVar.UpdatedAt.Before(before) {
						err := db.DeleteTemplateVar(templateVar)
						if err != nil {
							log.Error().Err(err).Str("template_var_id", templateVar.Id).Msg("db: failed to delete template var")
						}
					}
				}
			}

			// Remove old users
			users, err := db.GetUsers()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get users")
			} else {
				for _, user := range users {
					if user.IsDeleted && user.UpdatedAt.Before(before) {
						err := db.DeleteUser(user)
						if err != nil {
							log.Error().Err(err).Str("user_id", user.Id).Msg("db: failed to delete user")
						}
					}
				}
			}

			// Remove old tokens
			tokens, err := db.GetTokens()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get tokens")
			} else {
				for _, token := range tokens {
					if token.IsDeleted && token.UpdatedAt.Before(before) {
						err := db.DeleteToken(token)
						if err != nil {
							log.Error().Err(err).Str("token_id", token.Id).Msg("db: failed to delete token")
						}
					}
				}
			}

			// Remove old volumes
			volumes, err := db.GetVolumes()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to get volumes")
			} else {
				for _, volume := range volumes {
					if volume.IsDeleted && volume.UpdatedAt.Before(before) {
						err := db.DeleteVolume(volume)
						if err != nil {
							log.Error().Err(err).Str("volume_id", volume.Id).Msg("db: failed to delete volume")
						}
					}
				}
			}
		}
	}()

	return nil
}
