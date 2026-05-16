package model

import (
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
)

const (
	SpaceUsageBucketMinute = "minute"
	SpaceUsageBucketDay    = "day"

	SpaceUsageMinuteRetention = 1 * time.Hour
	SpaceUsageDayRetention    = 7 * 24 * time.Hour
)

type SpaceUsageSample struct {
	Id                    string        `json:"space_usage_id" db:"space_usage_id,pk" msgpack:"space_usage_id"`
	SpaceId               string        `json:"space_id" db:"space_id" msgpack:"space_id"`
	UserId                string        `json:"user_id" db:"user_id" msgpack:"user_id"`
	BucketKind            string        `json:"bucket_kind" db:"bucket_kind" msgpack:"bucket_kind"`
	BucketStart           time.Time     `json:"bucket_start" db:"bucket_start" msgpack:"bucket_start"`
	CPUPercent            float64       `json:"cpu_percent" db:"cpu_percent" msgpack:"cpu_percent"`
	MemoryUsedBytes       uint64        `json:"memory_used_bytes" db:"memory_used_bytes" msgpack:"memory_used_bytes"`
	MemoryLimitBytes      uint64        `json:"memory_limit_bytes" db:"memory_limit_bytes" msgpack:"memory_limit_bytes"`
	DiskUsedBytes         uint64        `json:"disk_used_bytes" db:"disk_used_bytes" msgpack:"disk_used_bytes"`
	DiskLimitBytes        uint64        `json:"disk_limit_bytes" db:"disk_limit_bytes" msgpack:"disk_limit_bytes"`
	ActivityWriteCount    uint32        `json:"activity_write_count" db:"activity_write_count" msgpack:"activity_write_count"`
	ActivityCreateCount   uint32        `json:"activity_create_count" db:"activity_create_count" msgpack:"activity_create_count"`
	ActivityDeleteCount   uint32        `json:"activity_delete_count" db:"activity_delete_count" msgpack:"activity_delete_count"`
	ActivityRenameCount   uint32        `json:"activity_rename_count" db:"activity_rename_count" msgpack:"activity_rename_count"`
	ActivityDistinctPaths uint32        `json:"activity_distinct_paths" db:"activity_distinct_paths" msgpack:"activity_distinct_paths"`
	ActivityDistinctDirs  uint32        `json:"activity_distinct_dirs" db:"activity_distinct_dirs" msgpack:"activity_distinct_dirs"`
	ActivitySpaceStarts   uint32        `json:"activity_space_starts" db:"activity_space_starts" msgpack:"activity_space_starts"`
	ActivitySpaceStops    uint32        `json:"activity_space_stops" db:"activity_space_stops" msgpack:"activity_space_stops"`
	ActivitySpaceCreates  uint32        `json:"activity_space_creates" db:"activity_space_creates" msgpack:"activity_space_creates"`
	ActivitySpaceDeletes  uint32        `json:"activity_space_deletes" db:"activity_space_deletes" msgpack:"activity_space_deletes"`
	LastActivityAt        *time.Time    `json:"last_activity_at,omitempty" db:"last_activity_at" msgpack:"last_activity_at,omitempty"`
	CreatedAt             time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedAt             hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewSpaceUsageSample(spaceId, userId, bucketKind string, bucketStart time.Time) *SpaceUsageSample {
	bucketKind = NormalizeSpaceUsageBucketKind(bucketKind)
	bucketStart = BucketStartForKind(bucketStart, bucketKind)
	return &SpaceUsageSample{
		Id:          SpaceUsageSampleIdForKind(spaceId, bucketKind, bucketStart),
		SpaceId:     spaceId,
		UserId:      userId,
		BucketKind:  bucketKind,
		BucketStart: bucketStart,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   hlc.Now(),
	}
}

func SpaceUsageSampleIdForKind(spaceId, bucketKind string, bucketStart time.Time) string {
	bucketKind = NormalizeSpaceUsageBucketKind(bucketKind)
	return fmt.Sprintf("%s:%s:%s", spaceId, bucketKind, BucketStartForKind(bucketStart, bucketKind).UTC().Format("200601021504"))
}

func BucketStartForKind(bucketStart time.Time, bucketKind string) time.Time {
	bucketStart = bucketStart.UTC()
	switch NormalizeSpaceUsageBucketKind(bucketKind) {
	case SpaceUsageBucketDay:
		return bucketStart.Truncate(24 * time.Hour)
	default:
		return bucketStart.Truncate(time.Minute)
	}
}

func NormalizeSpaceUsageBucketKind(bucketKind string) string {
	switch bucketKind {
	case SpaceUsageBucketDay:
		return SpaceUsageBucketDay
	default:
		return SpaceUsageBucketMinute
	}
}

func SpaceUsageRetentionForKind(bucketKind string) time.Duration {
	switch NormalizeSpaceUsageBucketKind(bucketKind) {
	case SpaceUsageBucketDay:
		return SpaceUsageDayRetention
	default:
		return SpaceUsageMinuteRetention
	}
}
