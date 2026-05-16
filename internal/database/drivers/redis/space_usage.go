package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveSpaceUsageSample(sample *model.SpaceUsageSample) error {
	data, err := json.Marshal(sample)
	if err != nil {
		return err
	}

	retention := model.SpaceUsageRetentionForKind(sample.BucketKind)

	if err := db.connection.Set(context.Background(), fmt.Sprintf("%sSpaceUsage:%s", db.prefix, sample.Id), data, retention).Err(); err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sSpaceUsageBySpace:%s:%s", db.prefix, sample.SpaceId, sample.Id), sample.Id, retention).Err()
}

func (db *RedisDbDriver) GetSpaceUsageSample(id string) (*model.SpaceUsageSample, error) {
	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSpaceUsage:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, err
	}

	var sample model.SpaceUsageSample
	if err := json.Unmarshal([]byte(v), &sample); err != nil {
		return nil, err
	}

	return &sample, nil
}

func (db *RedisDbDriver) GetSpaceUsageSamples(spaceId string, bucketKind string, from time.Time, to time.Time) ([]*model.SpaceUsageSample, error) {
	var samples []*model.SpaceUsageSample
	bucketKind = model.NormalizeSpaceUsageBucketKind(bucketKind)
	pattern := fmt.Sprintf("%sSpaceUsage:*", db.prefix)
	if spaceId != "" {
		pattern = fmt.Sprintf("%sSpaceUsageBySpace:%s:*", db.prefix, spaceId)
	}
	iter := db.connection.Scan(context.Background(), 0, pattern, 0).Iterator()
	for iter.Next(context.Background()) {
		id := iter.Val()[len(fmt.Sprintf("%sSpaceUsage:", db.prefix)):]
		if spaceId != "" {
			id = iter.Val()[len(fmt.Sprintf("%sSpaceUsageBySpace:%s:", db.prefix, spaceId)):]
		}
		v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSpaceUsage:%s", db.prefix, id)).Result()
		if err != nil {
			continue
		}

		var sample model.SpaceUsageSample
		if err := json.Unmarshal([]byte(v), &sample); err != nil {
			return nil, err
		}
		if sample.BucketKind != bucketKind || (spaceId != "" && sample.SpaceId != spaceId) || sample.BucketStart.Before(from.UTC()) || sample.BucketStart.After(to.UTC()) {
			continue
		}
		samples = append(samples, &sample)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].BucketStart.Before(samples[j].BucketStart)
	})

	return samples, nil
}
