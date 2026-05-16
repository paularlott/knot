package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/internal/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveSpaceUsageSample(sample *model.SpaceUsageSample) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(sample)
		if err != nil {
			return err
		}

		retention := model.SpaceUsageRetentionForKind(sample.BucketKind)

		entry := badger.NewEntry([]byte(fmt.Sprintf("SpaceUsage:%s", sample.Id)), data).WithTTL(retention)
		if err := txn.SetEntry(entry); err != nil {
			return err
		}

		idx := badger.NewEntry([]byte(fmt.Sprintf("SpaceUsageBySpace:%s:%s", sample.SpaceId, sample.Id)), []byte(sample.Id)).WithTTL(retention)
		return txn.SetEntry(idx)
	})
}

func (db *BadgerDbDriver) GetSpaceUsageSample(id string) (*model.SpaceUsageSample, error) {
	var sample *model.SpaceUsageSample

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("SpaceUsage:%s", id)))
		if err != nil {
			return err
		}

		obj := &model.SpaceUsageSample{}
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, obj)
		}); err != nil {
			return err
		}

		sample = obj
		return nil
	})
	if err != nil {
		return nil, err
	}

	return sample, nil
}

func (db *BadgerDbDriver) GetSpaceUsageSamples(spaceId string, bucketKind string, from time.Time, to time.Time) ([]*model.SpaceUsageSample, error) {
	var samples []*model.SpaceUsageSample
	bucketKind = model.NormalizeSpaceUsageBucketKind(bucketKind)
	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		if spaceId != "" {
			prefix := []byte(fmt.Sprintf("SpaceUsageBySpace:%s:", spaceId))
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				var sampleId string
				if err := item.Value(func(val []byte) error {
					sampleId = string(val)
					return nil
				}); err != nil {
					return err
				}

				sampleItem, err := txn.Get([]byte(fmt.Sprintf("SpaceUsage:%s", sampleId)))
				if err != nil {
					continue
				}

				sample := &model.SpaceUsageSample{}
				if err := sampleItem.Value(func(val []byte) error {
					return json.Unmarshal(val, sample)
				}); err != nil {
					return err
				}
				if sample.BucketKind != bucketKind || sample.BucketStart.Before(from.UTC()) || sample.BucketStart.After(to.UTC()) {
					continue
				}
				samples = append(samples, sample)
			}
			return nil
		}

		prefix := []byte("SpaceUsage:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			sample := &model.SpaceUsageSample{}
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, sample)
			}); err != nil {
				return err
			}
			if sample.BucketKind != bucketKind || sample.BucketStart.Before(from.UTC()) || sample.BucketStart.After(to.UTC()) {
				continue
			}
			samples = append(samples, sample)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].BucketStart.Before(samples[j].BucketStart)
	})
	return samples, nil
}
