package cluster

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func TestMergeIncomingSpaceUsageSnapshotUsesMaximums(t *testing.T) {
	lastActivityAt := time.Now().UTC().Add(-2 * time.Minute)
	incomingLastActivityAt := time.Now().UTC().Add(-1 * time.Minute)
	target := &model.SpaceUsageSample{
		UserId:                "old-user",
		CPUPercent:            12.5,
		MemoryUsedBytes:       100,
		MemoryLimitBytes:      300,
		DiskUsedBytes:         400,
		DiskLimitBytes:        800,
		ActivityWriteCount:    10,
		ActivityCreateCount:   1,
		ActivityDeleteCount:   8,
		ActivityRenameCount:   2,
		ActivityDistinctPaths: 7,
		ActivitySpaceStarts:   1,
		ActivitySpaceStops:    3,
		ActivitySpaceCreates:  2,
		ActivitySpaceDeletes:  0,
		LastActivityAt:        &lastActivityAt,
	}
	incoming := &model.SpaceUsageSample{
		UserId:                "new-user",
		CPUPercent:            10.5,
		MemoryUsedBytes:       200,
		MemoryLimitBytes:      250,
		DiskUsedBytes:         300,
		DiskLimitBytes:        900,
		ActivityWriteCount:    6,
		ActivityCreateCount:   4,
		ActivityDeleteCount:   5,
		ActivityRenameCount:   9,
		ActivityDistinctPaths: 3,
		ActivitySpaceStarts:   2,
		ActivitySpaceStops:    1,
		ActivitySpaceCreates:  5,
		ActivitySpaceDeletes:  1,
		LastActivityAt:        &incomingLastActivityAt,
	}

	mergeIncomingSpaceUsageSnapshot(target, incoming)

	if target.UserId != "new-user" ||
		target.CPUPercent != 12.5 ||
		target.MemoryUsedBytes != 200 ||
		target.MemoryLimitBytes != 300 ||
		target.DiskUsedBytes != 400 ||
		target.DiskLimitBytes != 900 ||
		target.ActivityWriteCount != 10 ||
		target.ActivityCreateCount != 4 ||
		target.ActivityDeleteCount != 8 ||
		target.ActivityRenameCount != 9 ||
		target.ActivityDistinctPaths != 7 ||
		target.ActivitySpaceStarts != 2 ||
		target.ActivitySpaceStops != 3 ||
		target.ActivitySpaceCreates != 5 ||
		target.ActivitySpaceDeletes != 1 {
		t.Fatalf("expected maximum snapshot fields, got %+v", target)
	}
	if target.LastActivityAt == nil || !target.LastActivityAt.Equal(incomingLastActivityAt) {
		t.Fatalf("expected latest activity timestamp %v, got %v", incomingLastActivityAt, target.LastActivityAt)
	}
}

func TestMergeIncomingSpaceUsageSnapshotDoesNotDoubleCountRepeatedGossip(t *testing.T) {
	target := &model.SpaceUsageSample{
		ActivityWriteCount:   10,
		ActivityCreateCount:  3,
		ActivitySpaceStarts:  1,
		ActivitySpaceDeletes: 2,
	}
	incoming := &model.SpaceUsageSample{
		ActivityWriteCount:   10,
		ActivityCreateCount:  3,
		ActivitySpaceStarts:  1,
		ActivitySpaceDeletes: 2,
	}

	mergeIncomingSpaceUsageSnapshot(target, incoming)
	mergeIncomingSpaceUsageSnapshot(target, incoming)

	if target.ActivityWriteCount != 10 ||
		target.ActivityCreateCount != 3 ||
		target.ActivitySpaceStarts != 1 ||
		target.ActivitySpaceDeletes != 2 {
		t.Fatalf("expected repeated gossip to remain idempotent, got %+v", target)
	}
}

func TestMergeIncomingSpaceUsageSnapshotIgnoresFutureLastActivity(t *testing.T) {
	validLastActivityAt := time.Now().UTC().Add(-time.Minute)
	futureLastActivityAt := time.Now().UTC().Add(10 * time.Minute)
	target := &model.SpaceUsageSample{LastActivityAt: &validLastActivityAt}
	incoming := &model.SpaceUsageSample{LastActivityAt: &futureLastActivityAt}

	mergeIncomingSpaceUsageSnapshot(target, incoming)

	if target.LastActivityAt == nil || !target.LastActivityAt.Equal(validLastActivityAt) {
		t.Fatalf("expected future last activity to be ignored, got %v", target.LastActivityAt)
	}
}
