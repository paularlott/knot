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

	if existingScript != nil {
		// If name or user_id changed, delete old index
		if (existingScript.Name != script.Name || existingScript.UserId != script.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
			db.connection.Del(context.Background(), fmt.Sprintf("%sScriptsByName:%s:%s", db.prefix, existingScript.UserId, existingScript.Name))
		}

		if len(updateFields) > 0 {
			util.CopyFields(script, existingScript, updateFields)
			script = existingScript
		}
	}

	data, err := json.Marshal(script)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sScripts:%s", db.prefix, script.Id), data, 0).Err()
	if err != nil {
		return err
	}

	// Create composite index with user_id:name
	return db.connection.Set(context.Background(), fmt.Sprintf("%sScriptsByName:%s:%s", db.prefix, script.UserId, script.Name), script.Id, 0).Err()
}

func (db *RedisDbDriver) DeleteScript(script *model.Script) error {
	db.connection.Del(context.Background(), fmt.Sprintf("%sScriptsByName:%s:%s", db.prefix, script.UserId, script.Name))
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
	scripts, err := db.GetScriptsByName(name)
	if err != nil {
		return nil, err
	}
	return scripts[0], nil
}

// GetScriptsByName returns all scripts matching a name (for zone-specific overrides)
func (db *RedisDbDriver) GetScriptsByName(name string) ([]*model.Script, error) {
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	var result []*model.Script
	for _, script := range scripts {
		// Filter by name and global (empty user_id)
		if script.Name == name && script.UserId == "" {
			result = append(result, script)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	// Sort by zone specificity (more zones = higher priority)
	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

// GetScriptByNameAndUser gets a script by name for a specific user
// If userId is empty, it searches for global scripts
// This supports the user script override functionality
func (db *RedisDbDriver) GetScriptByNameAndUser(name string, userId string) (*model.Script, error) {
	scripts, err := db.GetScriptsByNameAndUser(name, userId)
	if err != nil {
		return nil, err
	}
	return scripts[0], nil
}

// GetScriptsByNameAndUser returns all scripts matching a name and user_id (for zone-specific overrides)
func (db *RedisDbDriver) GetScriptsByNameAndUser(name string, userId string) ([]*model.Script, error) {
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	var result []*model.Script
	for _, script := range scripts {
		// Filter by name and user_id
		if script.Name == name && script.UserId == userId {
			result = append(result, script)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	// Sort by zone specificity (more zones = higher priority)
	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}
