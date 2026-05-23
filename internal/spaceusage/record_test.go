package spaceusage

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestMergeLocalSpaceUsageSampleKeepsResourceMaximums(t *testing.T) {
	target := &model.SpaceUsageSample{
		CPUPercent:       12.5,
		MemoryUsedBytes:  100,
		MemoryLimitBytes: 200,
		DiskUsedBytes:    300,
		DiskLimitBytes:   400,
	}
	incoming := &model.SpaceUsageSample{
		CPUPercent:       20.5,
		MemoryUsedBytes:  90,
		MemoryLimitBytes: 250,
		DiskUsedBytes:    500,
		DiskLimitBytes:   350,
	}

	mergeLocalSpaceUsageSample(target, incoming)

	if target.CPUPercent != 20.5 ||
		target.MemoryUsedBytes != 100 ||
		target.MemoryLimitBytes != 250 ||
		target.DiskUsedBytes != 500 ||
		target.DiskLimitBytes != 400 {
		t.Fatalf("expected resource maximums, got %+v", target)
	}
}

func TestMergeLocalSpaceUsageSampleDoesNotRecordActivity(t *testing.T) {
	target := &model.SpaceUsageSample{}
	incoming := &model.SpaceUsageSample{
		ActivityWriteCount:    10,
		ActivityCreateCount:   3,
		ActivityDeleteCount:   2,
		ActivityRenameCount:   1,
		ActivityDistinctPaths: 4,
		ActivitySpaceStarts:   1,
		ActivitySpaceStops:    2,
		ActivitySpaceCreates:  3,
		ActivitySpaceDeletes:  4,
	}

	mergeLocalSpaceUsageSample(target, incoming)

	if target.ActivityWriteCount != 0 ||
		target.ActivityCreateCount != 0 ||
		target.ActivityDeleteCount != 0 ||
		target.ActivityRenameCount != 0 ||
		target.ActivityDistinctPaths != 0 ||
		target.ActivitySpaceStarts != 0 ||
		target.ActivitySpaceStops != 0 ||
		target.ActivitySpaceCreates != 0 ||
		target.ActivitySpaceDeletes != 0 {
		t.Fatalf("expected activity fields to stay zero in OSS, got %+v", target)
	}
}
