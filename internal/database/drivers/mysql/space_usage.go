package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *MySQLDriver) SaveSpaceUsageSample(sample *model.SpaceUsageSample) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM space_usage WHERE space_usage_id=?)", sample.Id).Scan(&exists); err != nil {
		tx.Rollback()
		return err
	}

	if exists {
		err = db.update("space_usage", sample, nil)
	} else {
		err = db.create("space_usage", sample)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (db *MySQLDriver) GetSpaceUsageSamples(spaceId string, bucketKind string, from time.Time, to time.Time) ([]*model.SpaceUsageSample, error) {
	var samples []*model.SpaceUsageSample
	where := "bucket_kind = ? AND bucket_start >= ? AND bucket_start <= ? ORDER BY bucket_start ASC"
	args := []interface{}{model.NormalizeSpaceUsageBucketKind(bucketKind), from.UTC(), to.UTC()}
	if spaceId != "" {
		where = "space_id = ? AND " + where
		args = append([]interface{}{spaceId}, args...)
	}
	err := db.read("space_usage", &samples, nil, where, args...)
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func (db *MySQLDriver) GetSpaceUsageSample(id string) (*model.SpaceUsageSample, error) {
	var samples []*model.SpaceUsageSample
	err := db.read("space_usage", &samples, nil, "space_usage_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("space usage sample not found")
	}
	return samples[0], nil
}

func (db *MySQLDriver) cleanupExpiredSpaceUsageSamples() error {
	now := time.Now().UTC()
	_, err := db.connection.Exec(
		"DELETE FROM space_usage WHERE (bucket_kind = ? AND bucket_start < ?) OR (bucket_kind = ? AND bucket_start < ?)",
		model.SpaceUsageBucketMinute,
		now.Add(-model.SpaceUsageMinuteRetention),
		model.SpaceUsageBucketDay,
		now.Add(-model.SpaceUsageDayRetention),
	)
	return err
}
