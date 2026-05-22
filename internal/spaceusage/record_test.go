package spaceusage

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func TestMergeLocalSpaceUsageSampleAccumulatesDayActivityCounts(t *testing.T) {
	target := &model.SpaceUsageSample{
		BucketKind:          model.SpaceUsageBucketDay,
		ActivityWriteCount:  15,
		ActivityCreateCount: 4,
	}
	incoming := &model.SpaceUsageSample{
		BucketKind:          model.SpaceUsageBucketDay,
		ActivityWriteCount:  3,
		ActivityCreateCount: 2,
	}

	mergeLocalSpaceUsageSample(target, incoming)

	if target.ActivityWriteCount != 18 {
		t.Fatalf("expected day write count 18, got %d", target.ActivityWriteCount)
	}
	if target.ActivityCreateCount != 6 {
		t.Fatalf("expected day create count 6, got %d", target.ActivityCreateCount)
	}
}

func TestMergeLocalSpaceUsageSampleReplacesInvalidFutureLastActivity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	targetFuture := now.Add(8 * time.Hour)
	incomingNow := now

	target := &model.SpaceUsageSample{LastActivityAt: &targetFuture}
	incoming := &model.SpaceUsageSample{LastActivityAt: &incomingNow}

	mergeLocalSpaceUsageSample(target, incoming)

	if target.LastActivityAt == nil || !target.LastActivityAt.Equal(incomingNow) {
		t.Fatalf("expected future last activity to be replaced with %v, got %v", incomingNow, target.LastActivityAt)
	}
}

func TestApplyActivityDeltaCountsNewMinuteOnce(t *testing.T) {
	target := &model.SpaceUsageSample{}
	current := &model.SpaceUsageSample{
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

	applyActivityDelta(target, nil, current)

	if target.ActivityWriteCount != 10 ||
		target.ActivityCreateCount != 3 ||
		target.ActivityDeleteCount != 2 ||
		target.ActivityRenameCount != 1 ||
		target.ActivityDistinctPaths != 4 ||
		target.ActivitySpaceStarts != 1 ||
		target.ActivitySpaceStops != 2 ||
		target.ActivitySpaceCreates != 3 ||
		target.ActivitySpaceDeletes != 4 {
		t.Fatalf("expected full current activity for new minute, got %+v", target)
	}
}

func TestApplyActivityDeltaOnlyCountsIncreaseForExistingMinute(t *testing.T) {
	previous := &model.SpaceUsageSample{
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
	current := &model.SpaceUsageSample{
		ActivityWriteCount:    13,
		ActivityCreateCount:   3,
		ActivityDeleteCount:   4,
		ActivityRenameCount:   1,
		ActivityDistinctPaths: 6,
		ActivitySpaceStarts:   2,
		ActivitySpaceStops:    2,
		ActivitySpaceCreates:  5,
		ActivitySpaceDeletes:  4,
	}
	target := &model.SpaceUsageSample{}

	applyActivityDelta(target, previous, current)

	if target.ActivityWriteCount != 3 ||
		target.ActivityCreateCount != 0 ||
		target.ActivityDeleteCount != 2 ||
		target.ActivityRenameCount != 0 ||
		target.ActivityDistinctPaths != 2 ||
		target.ActivitySpaceStarts != 1 ||
		target.ActivitySpaceStops != 0 ||
		target.ActivitySpaceCreates != 2 ||
		target.ActivitySpaceDeletes != 0 {
		t.Fatalf("expected only activity increases for existing minute, got %+v", target)
	}
}

func TestApplyActivityDeltaIgnoresEqualOrLowerReplay(t *testing.T) {
	previous := &model.SpaceUsageSample{
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
	current := &model.SpaceUsageSample{
		ActivityWriteCount:    9,
		ActivityCreateCount:   3,
		ActivityDeleteCount:   1,
		ActivityRenameCount:   1,
		ActivityDistinctPaths: 4,
		ActivitySpaceStarts:   1,
		ActivitySpaceStops:    1,
		ActivitySpaceCreates:  3,
		ActivitySpaceDeletes:  2,
	}
	target := &model.SpaceUsageSample{}

	applyActivityDelta(target, previous, current)

	if target.ActivityWriteCount != 0 ||
		target.ActivityCreateCount != 0 ||
		target.ActivityDeleteCount != 0 ||
		target.ActivityRenameCount != 0 ||
		target.ActivityDistinctPaths != 0 ||
		target.ActivitySpaceStarts != 0 ||
		target.ActivitySpaceStops != 0 ||
		target.ActivitySpaceCreates != 0 ||
		target.ActivitySpaceDeletes != 0 {
		t.Fatalf("expected no activity for equal or lower replay, got %+v", target)
	}
}
