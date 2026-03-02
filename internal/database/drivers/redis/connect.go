package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
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
			db.cleanupObjectType("Groups", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Group
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old roles
			db.cleanupObjectType("Roles", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Role
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old spaces
			db.cleanupObjectType("Spaces", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Space
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old templates
			db.cleanupObjectType("Templates", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Template
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old template vars
			db.cleanupObjectType("TemplateVars", before, func(data []byte) (bool, time.Time, error) {
				var obj model.TemplateVar
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old users
			db.cleanupObjectType("Users", before, func(data []byte) (bool, time.Time, error) {
				var obj model.User
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old tokens
			db.cleanupObjectType("Tokens", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Token
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old volumes
			db.cleanupObjectType("Volumes", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Volume
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old scripts
			db.cleanupObjectType("Scripts", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Script
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old responses
			db.cleanupObjectType("Responses", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Response
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})
		}
	}()

	return nil
}

// cleanupObjectType iterates through keys of a given object type and deletes old soft-deleted entries
func (db *RedisDbDriver) cleanupObjectType(objectType string, before time.Time, checkFunc func([]byte) (bool, time.Time, error)) {
	prefix := fmt.Sprintf("%s%s:", db.prefix, objectType)
	iter := db.connection.Scan(context.Background(), 0, prefix+"*", 0).Iterator()

	for iter.Next(context.Background()) {
		key := iter.Val()

		// Extract the ID from the key (prefix:ID)
		id := strings.TrimPrefix(key, prefix)
		if id == key {
			// Key didn't have expected prefix, skip it
			continue
		}

		// Get the object data
		data, err := db.connection.Get(context.Background(), key).Bytes()
		if err != nil {
			if err != redis.Nil {
				db.logger.WithError(err).Error("failed to get object for cleanup", "object_type", objectType, "key", key)
			}
			continue
		}

		// Check if it should be deleted
		isDeleted, updatedAt, err := checkFunc(data)
		if err != nil {
			db.logger.WithError(err).Error("failed to unmarshal object for cleanup", "object_type", objectType, "key", key)
			continue
		}

		if isDeleted && updatedAt.Before(before) {
			// Delete the key
			if err := db.connection.Del(context.Background(), key).Err(); err != nil {
				db.logger.Error("failed to delete expired object", "error", err, "object_type", objectType, "id", id)
			}
		}
	}

	if err := iter.Err(); err != nil {
		db.logger.WithError(err).Error("failed to iterate during cleanup", "object_type", objectType)
	}
}
