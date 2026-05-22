package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *MySQLDriver) SaveSpaceUsageSample(sample *model.SpaceUsageSample) error {
	query := `INSERT INTO space_usage (
space_usage_id,
space_id,
user_id,
bucket_kind,
bucket_start,
cpu_percent,
memory_used_bytes,
memory_limit_bytes,
disk_used_bytes,
disk_limit_bytes,
activity_write_count,
activity_create_count,
activity_delete_count,
activity_rename_count,
activity_distinct_paths,
activity_space_starts,
activity_space_stops,
activity_space_creates,
activity_space_deletes,
last_activity_at,
created_at,
updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
space_id = VALUES(space_id),
user_id = VALUES(user_id),
bucket_kind = VALUES(bucket_kind),
bucket_start = VALUES(bucket_start),
cpu_percent = VALUES(cpu_percent),
memory_used_bytes = VALUES(memory_used_bytes),
memory_limit_bytes = VALUES(memory_limit_bytes),
disk_used_bytes = VALUES(disk_used_bytes),
disk_limit_bytes = VALUES(disk_limit_bytes),
activity_write_count = VALUES(activity_write_count),
activity_create_count = VALUES(activity_create_count),
activity_delete_count = VALUES(activity_delete_count),
activity_rename_count = VALUES(activity_rename_count),
activity_distinct_paths = VALUES(activity_distinct_paths),
activity_space_starts = VALUES(activity_space_starts),
activity_space_stops = VALUES(activity_space_stops),
activity_space_creates = VALUES(activity_space_creates),
activity_space_deletes = VALUES(activity_space_deletes),
last_activity_at = VALUES(last_activity_at),
created_at = VALUES(created_at),
updated_at = VALUES(updated_at)`

	_, err := db.connection.Exec(
		query,
		sample.Id,
		sample.SpaceId,
		sample.UserId,
		sample.BucketKind,
		sample.BucketStart.UTC(),
		sample.CPUPercent,
		sample.MemoryUsedBytes,
		sample.MemoryLimitBytes,
		sample.DiskUsedBytes,
		sample.DiskLimitBytes,
		sample.ActivityWriteCount,
		sample.ActivityCreateCount,
		sample.ActivityDeleteCount,
		sample.ActivityRenameCount,
		sample.ActivityDistinctPaths,
		sample.ActivitySpaceStarts,
		sample.ActivitySpaceStops,
		sample.ActivitySpaceCreates,
		sample.ActivitySpaceDeletes,
		nullableDatabaseTime(sample.LastActivityAt, "2006-01-02 15:04:05.000000"),
		sample.CreatedAt.UTC(),
		sample.UpdatedAt,
	)
	return err
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

func nullableDatabaseTime(value *time.Time, layout string) interface{} {
	if value == nil {
		return nil
	}
	return formatDatabaseTime(value, layout)
}
