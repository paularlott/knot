package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveTemplateVar(templateVar *model.TemplateVar) error {
	templateVar.Value = templateVar.GetValueEncrypted()
	data, err := json.Marshal(templateVar)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sTemplateVars:%s", db.prefix, templateVar.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteTemplateVar(templateVar *model.TemplateVar) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("%sTemplateVars:%s", db.prefix, templateVar.Id)).Err()
}

func (db *RedisDbDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
	var templateVar = &model.TemplateVar{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sTemplateVars:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &templateVar)
	if err != nil {
		return nil, err
	}

	templateVar.DecryptSetValue(templateVar.Value)

	return templateVar, nil
}

func (db *RedisDbDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
	var templateVars []*model.TemplateVar

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sTemplateVars:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		templateVar, err := db.GetTemplateVar(iter.Val()[len(fmt.Sprintf("%sTemplateVars:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		templateVars = append(templateVars, templateVar)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort the template vars by name
	sort.Slice(templateVars, func(i, j int) bool {
		return templateVars[i].Name < templateVars[j].Name
	})

	return templateVars, nil
}
