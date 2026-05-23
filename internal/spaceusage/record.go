package spaceusage

import (
	"sync"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

var (
	lastWriteMu            sync.Mutex
	lastSpaceMinuteBucket  = map[string]int64{}
	lastSpaceFlushAt       = map[string]time.Time{}
	lastRolledMinuteBucket = map[string]int64{}
)

const spaceUsageFlushInterval = 15 * time.Second

func RecordFromAgentState(spaceId, userId string, state *msg.AgentState) {
	if state == nil || spaceId == "" || userId == "" {
		return
	}

	now := time.Now().UTC()
	minuteBucketStart := model.BucketStartForKind(now, model.SpaceUsageBucketMinute).Unix()

	var rollupBucket int64
	lastWriteMu.Lock()
	if previousBucket := lastSpaceMinuteBucket[spaceId]; previousBucket > 0 && previousBucket != minuteBucketStart && lastRolledMinuteBucket[spaceId] != previousBucket {
		rollupBucket = previousBucket
		lastRolledMinuteBucket[spaceId] = previousBucket
	}
	lastFlush := lastSpaceFlushAt[spaceId]
	shouldFlush := lastSpaceMinuteBucket[spaceId] != minuteBucketStart || lastFlush.IsZero() || now.Sub(lastFlush) >= spaceUsageFlushInterval
	lastSpaceMinuteBucket[spaceId] = minuteBucketStart
	if shouldFlush {
		lastSpaceFlushAt[spaceId] = now
	}
	lastWriteMu.Unlock()

	db := database.GetInstance()

	var minuteSample *model.SpaceUsageSample
	if shouldFlush {
		var err error
		minuteSample = buildSampleFromState(spaceId, userId, model.SpaceUsageBucketMinute, now, state)
		minuteSample, err = saveMergedSample(db, minuteSample)
		if err != nil {
			log.WithError(err).Error("failed to save minute space usage sample", "space_id", spaceId)
			return
		}
	}

	var daySample *model.SpaceUsageSample
	if rollupBucket > 0 {
		var err error
		daySample, err = rollupMinuteIntoDay(db, spaceId, userId, rollupBucket)
		if err != nil {
			log.WithError(err).Error("failed to roll up daily space usage sample", "space_id", spaceId)
			return
		}
	}

	if transport := service.GetTransport(); transport != nil {
		if minuteSample != nil {
			transport.GossipSpaceUsageSample(minuteSample)
		}
		if daySample != nil {
			transport.GossipSpaceUsageSample(daySample)
		}
	}
}

func ForgetSpace(spaceId string) {
	lastWriteMu.Lock()
	delete(lastSpaceMinuteBucket, spaceId)
	delete(lastSpaceFlushAt, spaceId)
	delete(lastRolledMinuteBucket, spaceId)
	lastWriteMu.Unlock()
}

func buildSampleFromState(spaceId, userId, bucketKind string, now time.Time, state *msg.AgentState) *model.SpaceUsageSample {
	sample := model.NewSpaceUsageSample(spaceId, userId, bucketKind, now)
	sample.CPUPercent = state.CPUPercent
	sample.MemoryUsedBytes = state.MemoryUsedBytes
	sample.MemoryLimitBytes = state.MemoryLimitBytes
	sample.DiskUsedBytes = state.DiskUsedBytes
	sample.DiskLimitBytes = state.DiskLimitBytes
	sample.UpdatedAt = hlc.Now()
	return sample
}

func rollupMinuteIntoDay(db database.DbDriver, spaceId, userId string, minuteBucketStartUnix int64) (*model.SpaceUsageSample, error) {
	minuteBucketStart := time.Unix(minuteBucketStartUnix, 0).UTC()
	minuteSampleId := model.SpaceUsageSampleIdForKind(spaceId, model.SpaceUsageBucketMinute, minuteBucketStart)
	minuteSample, err := db.GetSpaceUsageSample(minuteSampleId)
	if err != nil || minuteSample == nil {
		return nil, nil
	}

	daySample := model.NewSpaceUsageSample(spaceId, userId, model.SpaceUsageBucketDay, minuteBucketStart)
	daySample.CPUPercent = minuteSample.CPUPercent
	daySample.MemoryUsedBytes = minuteSample.MemoryUsedBytes
	daySample.MemoryLimitBytes = minuteSample.MemoryLimitBytes
	daySample.DiskUsedBytes = minuteSample.DiskUsedBytes
	daySample.DiskLimitBytes = minuteSample.DiskLimitBytes
	daySample.UpdatedAt = hlc.Now()

	return saveMergedSample(db, daySample)
}

func saveMergedSample(db database.DbDriver, sample *model.SpaceUsageSample) (*model.SpaceUsageSample, error) {
	existing, err := db.GetSpaceUsageSample(sample.Id)
	if err == nil && existing != nil {
		mergeLocalSpaceUsageSample(existing, sample)
		sample = existing
	}

	if err := db.SaveSpaceUsageSample(sample); err != nil {
		return nil, err
	}

	return sample, nil
}

func mergeLocalSpaceUsageSample(target, incoming *model.SpaceUsageSample) {
	target.UserId = incoming.UserId
	target.UpdatedAt = incoming.UpdatedAt

	target.CPUPercent = maxFloat(target.CPUPercent, incoming.CPUPercent)
	target.MemoryUsedBytes = maxUint64(target.MemoryUsedBytes, incoming.MemoryUsedBytes)
	target.MemoryLimitBytes = maxUint64(target.MemoryLimitBytes, incoming.MemoryLimitBytes)
	target.DiskUsedBytes = maxUint64(target.DiskUsedBytes, incoming.DiskUsedBytes)
	target.DiskLimitBytes = maxUint64(target.DiskLimitBytes, incoming.DiskLimitBytes)
}

func maxFloat(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func maxUint64(left, right uint64) uint64 {
	if right > left {
		return right
	}
	return left
}
