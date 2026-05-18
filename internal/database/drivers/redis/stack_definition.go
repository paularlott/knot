package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveStackDefinition(def *model.StackDefinition, updateFields []string) error {
	existingDef, _ := db.GetStackDefinition(def.Id)

	if existingDef != nil {
		if (existingDef.Name != def.Name || existingDef.UserId != def.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
			db.connection.Del(context.Background(), fmt.Sprintf("%sStackDefsByName:%s:%s", db.prefix, existingDef.UserId, existingDef.Name))
		}

		if len(updateFields) > 0 {
			util.CopyFields(def, existingDef, updateFields)
			def = existingDef
		}
	}

	data, err := json.Marshal(def)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sStackDefs:%s", db.prefix, def.Id), data, 0).Err()
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sStackDefsByName:%s:%s", db.prefix, def.UserId, def.Name), def.Id, 0).Err()
}

func (db *RedisDbDriver) DeleteStackDefinition(def *model.StackDefinition) error {
	db.connection.Del(context.Background(), fmt.Sprintf("%sStackDefsByName:%s:%s", db.prefix, def.UserId, def.Name))
	return db.connection.Del(context.Background(), fmt.Sprintf("%sStackDefs:%s", db.prefix, def.Id)).Err()
}

func (db *RedisDbDriver) GetStackDefinition(id string) (*model.StackDefinition, error) {
	var def = &model.StackDefinition{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sStackDefs:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), def)
	if err != nil {
		return nil, err
	}

	return def, nil
}

func (db *RedisDbDriver) GetStackDefinitions() ([]*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sStackDefs:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		def, err := db.GetStackDefinition(iter.Val()[len(fmt.Sprintf("%sStackDefs:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		if !def.IsDeleted {
			defs = append(defs, def)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})

	return defs, nil
}

func (db *RedisDbDriver) GetStackDefinitionsByUserId(userId string) ([]*model.StackDefinition, error) {
	defs, err := db.GetStackDefinitions()
	if err != nil {
		return nil, err
	}

	var result []*model.StackDefinition
	for _, def := range defs {
		if def.UserId == userId {
			result = append(result, def)
		}
	}

	return result, nil
}

func (db *RedisDbDriver) GetStackDefinitionByName(name string, userId string) (*model.StackDefinition, error) {
	defs, err := db.GetStackDefinitions()
	if err != nil {
		return nil, err
	}

	for _, def := range defs {
		if def.Name == name && def.UserId == userId {
			return def, nil
		}
	}

	return nil, fmt.Errorf("stack definition not found")
}
