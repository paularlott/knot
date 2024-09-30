package driver_redis

import (
	"context"
	"time"

	"github.com/paularlott/knot/util"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type RedisDbDriver struct {
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
	hosts := viper.GetStringSlice("server.redis.hosts")
	for idx, host := range hosts {
		if host[:4] == "srv+" {
			for i := 0; i < 10; i++ {
				hostPort, err := util.LookupSRV(host[4:])
				if err != nil {
					if i == 9 {
						log.Fatal().Err(err).Msg("db: failed to lookup SRV record for Redis server aborting after 10 attempts")
					} else {
						log.Error().Err(err).Msg("db: failed to lookup SRV record for Redis server")
					}
					time.Sleep(3 * time.Second)
					continue
				}

				hosts[idx] = (*hostPort)[0].Host + ":" + (*hostPort)[0].Port
			}
		}
	}

	log.Debug().Msgf("db: connecting to redis server: %s, db: %d", hosts, viper.GetInt("server.redis.db"))

	db.connection = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:      hosts,
		Password:   viper.GetString("server.redis.password"),
		DB:         viper.GetInt("server.redis.db"),
		MasterName: viper.GetString("server.redis.master_name"),
	})

	log.Debug().Msg("db: connected to Redis")
}

func (db *RedisDbDriver) Connect() error {
	db.realConnect()

	// Monitor the connection and reconnect if the connection is lost
	go func() {
		for {
			time.Sleep(10 * time.Second)

			log.Debug().Msg("db: testing Redis connection")

			_, err := db.connection.Ping(context.Background()).Result()
			if err != nil {
				log.Error().Err(err).Msg("db: redis connection lost")
				db.connection.Close()
				db.realConnect()
			}
		}
	}()

	return nil
}
