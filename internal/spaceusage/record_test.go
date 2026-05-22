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
