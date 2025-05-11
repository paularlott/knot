package driver_redis

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) GetCfgValue(name string) (*model.CfgValue, error) {
	var v = &model.CfgValue{
		Name:  name,
		Value: "",
	}

	err := db.connection.Get(context.Background(), fmt.Sprintf("%Configs:%s", db.prefix, name)).Scan(&v.Value)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (db *RedisDbDriver) SaveCfgValue(cfgValue *model.CfgValue) error {
	err := db.connection.Set(context.Background(), fmt.Sprintf("%Configs:%s", db.prefix, cfgValue.Name), cfgValue.Value, 0).Err()
	if err != nil {
		return err
	}

	return nil
}
