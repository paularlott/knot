package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveScript(script *model.Script, updateFields []string) error {
	existingScript, _ := db.GetScript(script.Id)

	if existingScript != nil && len(updateFields) > 0 {
		util.CopyFields(script, existingScript, updateFields)
		script = existingScript
	}

	data, err := json.Marshal(script)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sScripts:%s", db.prefix, script.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteScript(script *model.Script) error {
	return db.connection.Del(context.Background(), fmt.Sprintf("%sScripts:%s", db.prefix, script.Id)).Err()
}

func (db *RedisDbDriver) GetScript(id string) (*model.Script, error) {
	var script = &model.Script{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sScripts:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &script)
	if err != nil {
		return nil, err
	}

	return script, nil
}

func (db *RedisDbDriver) GetScripts() ([]*model.Script, error) {
	var scripts []*model.Script

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sScripts:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		script, err := db.GetScript(iter.Val()[len(fmt.Sprintf("%sScripts:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		scripts = append(scripts, script)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Name < scripts[j].Name
	})

	return scripts, nil
}

func (db *RedisDbDriver) GetScriptByName(name string) (*model.Script, error) {
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	for _, script := range scripts {
		if script.Name == name {
			return script, nil
		}
	}

	return nil, fmt.Errorf("script not found")
}
