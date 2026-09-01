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

	err := db.dbScanString(context.Background(), fmt.Sprintf("%sConfigs:%s", db.prefix, name), &v.Value)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (db *RedisDbDriver) SaveCfgValue(cfgValue *model.CfgValue) error {
	err := db.set(context.Background(), fmt.Sprintf("%sConfigs:%s", db.prefix, cfgValue.Name), cfgValue.Value, 0)
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetCfgValues() ([]*model.CfgValue, error) {
	var cfgValues []*model.CfgValue

	keys, err := db.keys(context.Background(), fmt.Sprintf("%sConfigs:*", db.prefix))
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		var v = &model.CfgValue{
			Name:  key[len(fmt.Sprintf("%sConfigs:", db.prefix)):],
			Value: "",
		}

		err = db.dbScanString(context.Background(), key, &v.Value)
		if err != nil {
			return nil, err
		}

		cfgValues = append(cfgValues, v)
	}

	return cfgValues, nil
}
