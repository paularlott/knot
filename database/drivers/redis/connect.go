package driver_redis

import (
	"context"

	"github.com/paularlott/knot/util"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type RedisDbDriver struct{
  connection *redis.Client
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
  if err == redis.Nil {
    exists = v != ""
  }

  return exists, convertRedisError(err)
}

func (db *RedisDbDriver) Connect() error {
  log.Debug().Msg("db: connecting to Redis")

  // If the host starts with srv+ then lookup the SRV record
  host := viper.GetString("server.redis.host")
  if host[:4] == "srv+" {
    hostSrv, portSrv, err := util.GetTargetFromSRV(host[4:], viper.GetString("server.nameserver"))
    if err != nil {
      log.Fatal().Err(err).Msg("db: failed to lookup SRV record for Redis Server")
    }

    host = hostSrv + ":" + portSrv
  }

  log.Debug().Msgf("db: connecting to redis server: %s, db: %d", host, viper.GetInt("server.redis.db"))

  db.connection = redis.NewClient(&redis.Options{
    Addr    : host,
    Password: viper.GetString("server.redis.password"),
    DB      : viper.GetInt("server.redis.db"),
  })

  return nil
}
