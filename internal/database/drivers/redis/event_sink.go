package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveEventSink(sink *model.EventSink, updateFields []string) error {
	existing, _ := db.GetEventSink(sink.Id)

	if existing != nil {
		if len(updateFields) > 0 {
			util.CopyFields(sink, existing, updateFields)
			sink = existing
		}
	}

	data, err := json.Marshal(sink)
	if err != nil {
		return err
	}

	return db.set(context.Background(), fmt.Sprintf("%sEventSinks:%s", db.prefix, sink.Id), data, 0)
}

func (db *RedisDbDriver) DeleteEventSink(sink *model.EventSink) error {
	return db.del(context.Background(), fmt.Sprintf("%sEventSinks:%s", db.prefix, sink.Id))
}

func (db *RedisDbDriver) GetEventSink(id string) (*model.EventSink, error) {
	sink := &model.EventSink{}

	v, err := db.get(context.Background(), fmt.Sprintf("%sEventSinks:%s", db.prefix, id))
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), sink)
	if err != nil {
		return nil, err
	}

	return sink, nil
}

func (db *RedisDbDriver) GetEventSinks() ([]*model.EventSink, error) {
	var sinks []*model.EventSink

	iter := db.scan(context.Background(), fmt.Sprintf("%sEventSinks:*", db.prefix))
	for iter.Next(context.Background()) {
		sink, err := db.GetEventSink(iter.Val()[len(fmt.Sprintf("%sEventSinks:", db.prefix)):])
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, sink)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(sinks, func(i, j int) bool {
		return sinks[i].Name < sinks[j].Name
	})

	return sinks, nil
}
