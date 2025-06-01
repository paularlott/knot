package driver_redis

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) GetCfgValue(name string) (*model.CfgValue, error) {
	var v = &model.CfgValue{
		Name:  name,
		Value: "",
	}

	err := db.connection.Get(context.Background(), fmt.Sprintf("%sConfigs:%s", db.prefix, name)).Scan(&v.Value)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (db *RedisDbDriver) SaveCfgValue(cfgValue *model.CfgValue) error {
	err := db.connection.Set(context.Background(), fmt.Sprintf("%sConfigs:%s", db.prefix, cfgValue.Name), cfgValue.Value, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetCfgValues() ([]*model.CfgValue, error) {
	var cfgValues []*model.CfgValue

	keys, err := db.connection.Keys(context.Background(), fmt.Sprintf("%sConfigs:*", db.prefix)).Result()
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		var v = &model.CfgValue{
			Name:  key[len(fmt.Sprintf("%sConfigs:", db.prefix)):],
			Value: "",
		}

		err = db.connection.Get(context.Background(), key).Scan(&v.Value)
		if err != nil {
			return nil, err
		}

		cfgValues = append(cfgValues, v)
	}

	return cfgValues, nil
}
