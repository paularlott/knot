package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveMCPServer(server *model.MCPServer, updateFields []string) error {
	existing, _ := db.GetMCPServer(server.Id)

	if existing != nil {
		if (existing.Namespace != server.Namespace || existing.UserId != server.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Namespace") || util.InArray(updateFields, "UserId")) {
			db.connection.Del(context.Background(), fmt.Sprintf("%sMCPServersByUser:%s:%s", db.prefix, existing.UserId, existing.Namespace))
		}

		if len(updateFields) > 0 {
			util.CopyFields(server, existing, updateFields)
			server = existing
		}
	}

	data, err := json.Marshal(server)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sMCPServers:%s", db.prefix, server.Id), data, 0).Err()
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sMCPServersByUser:%s:%s", db.prefix, server.UserId, server.Namespace), server.Id, 0).Err()
}

func (db *RedisDbDriver) DeleteMCPServer(server *model.MCPServer) error {
	db.connection.Del(context.Background(), fmt.Sprintf("%sMCPServersByUser:%s:%s", db.prefix, server.UserId, server.Namespace))
	return db.connection.Del(context.Background(), fmt.Sprintf("%sMCPServers:%s", db.prefix, server.Id)).Err()
}

func (db *RedisDbDriver) GetMCPServer(id string) (*model.MCPServer, error) {
	var server = &model.MCPServer{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sMCPServers:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &server)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func (db *RedisDbDriver) GetMCPServers() ([]*model.MCPServer, error) {
	var servers []*model.MCPServer

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sMCPServers:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		server, err := db.GetMCPServer(iter.Val()[len(fmt.Sprintf("%sMCPServers:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		servers = append(servers, server)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Namespace < servers[j].Namespace
	})

	return servers, nil
}

func (db *RedisDbDriver) GetMCPServersByUser(userId string) ([]*model.MCPServer, error) {
	servers, err := db.GetMCPServers()
	if err != nil {
		return nil, err
	}

	var result []*model.MCPServer
	for _, s := range servers {
		if s.UserId == userId {
			result = append(result, s)
		}
	}

	return result, nil
}
