package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveRemoteServer(server *model.RemoteServer) error {
	// Calculate the expiration time
	server.ExpiresAfter = time.Now().UTC().Add(model.REMOTE_SERVER_TIMEOUT)

	data, err := json.Marshal(server)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("RemoteServer:%s", server.Id), data, model.REMOTE_SERVER_TIMEOUT).Err()
}

func (db *RedisDbDriver) DeleteRemoteServer(server *model.RemoteServer) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("RemoteServer:%s", server.Id)).Err()
}

func (db *RedisDbDriver) GetRemoteServer(id string) (*model.RemoteServer, error) {
	var server = &model.RemoteServer{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("RemoteServer:%s", id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &server)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func (db *RedisDbDriver) GetRemoteServers() ([]*model.RemoteServer, error) {
	var servers []*model.RemoteServer

	keys, err := db.connection.Keys(context.Background(), "RemoteServer:*").Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	for _, key := range keys {
		v, err := db.connection.Get(context.Background(), key).Result()
		if err != nil {
			return nil, convertRedisError(err)
		}

		var server = &model.RemoteServer{}
		err = json.Unmarshal([]byte(v), &server)
		if err != nil {
			return nil, err
		}

		servers = append(servers, server)
	}

	return servers, nil
}
