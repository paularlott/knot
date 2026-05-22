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
	lastWriteMu           sync.Mutex
	lastSpaceMinuteBucket = map[string]int64{}
)

func RecordFromAgentState(spaceId, userId string, state *msg.AgentState) {
	if state == nil || spaceId == "" || userId == "" {
		return
	}

	now := time.Now().UTC()
	minuteBucketStart := model.BucketStartForKind(now, model.SpaceUsageBucketMinute).Unix()

	lastWriteMu.Lock()
	if lastSpaceMinuteBucket[spaceId] == minuteBucketStart {
		lastWriteMu.Unlock()
		return
	}
	lastSpaceMinuteBucket[spaceId] = minuteBucketStart
	lastWriteMu.Unlock()

	db := database.GetInstance()
	minuteSample := buildSampleFromState(spaceId, userId, model.SpaceUsageBucketMinute, now, state)
	var err error
	minuteSample, err = saveMergedSample(db, minuteSample)
	if err != nil {
		log.WithError(err).Error("failed to save minute space usage sample", "space_id", spaceId)
		return
	}

	daySample := buildSampleFromState(spaceId, userId, model.SpaceUsageBucketDay, now, state)
	daySample, err = saveMergedSample(db, daySample)
	if err != nil {
		log.WithError(err).Error("failed to save daily space usage sample", "space_id", spaceId)
		return
	}

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpaceUsageSample(minuteSample)
		transport.GossipSpaceUsageSample(daySample)
	}
}

func ForgetSpace(spaceId string) {
	lastWriteMu.Lock()
	delete(lastSpaceMinuteBucket, spaceId)
	lastWriteMu.Unlock()
}

func buildSampleFromState(spaceId, userId, bucketKind string, now time.Time, state *msg.AgentState) *model.SpaceUsageSample {
	sample := model.NewSpaceUsageSample(spaceId, userId, bucketKind, now)
	sample.CPUPercent = state.CPUPercent
	sample.MemoryUsedBytes = state.MemoryUsedBytes
	sample.MemoryLimitBytes = state.MemoryLimitBytes
	sample.DiskUsedBytes = state.DiskUsedBytes
	sample.DiskLimitBytes = state.DiskLimitBytes
	sample.ActivityWriteCount = state.ActivityWriteCount
	sample.ActivityCreateCount = state.ActivityCreateCount
	sample.ActivityDeleteCount = state.ActivityDeleteCount
	sample.ActivityRenameCount = state.ActivityRenameCount
	sample.ActivityDistinctPaths = state.ActivityDistinctPaths
	if state.LastActivityAtUnix > 0 {
		sample.LastActivityAt = sanitizeLastActivityAtForSave(time.Unix(state.LastActivityAtUnix, 0).UTC())
	}
	sample.UpdatedAt = hlc.Now()
	return sample
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
	target.LastActivityAt = sanitizeLastActivityAtForRead(target.LastActivityAt)
	incoming.LastActivityAt = sanitizeLastActivityAtForRead(incoming.LastActivityAt)

	target.CPUPercent = maxFloat(target.CPUPercent, incoming.CPUPercent)
	target.MemoryUsedBytes = maxUint64(target.MemoryUsedBytes, incoming.MemoryUsedBytes)
	target.MemoryLimitBytes = maxUint64(target.MemoryLimitBytes, incoming.MemoryLimitBytes)
	target.DiskUsedBytes = maxUint64(target.DiskUsedBytes, incoming.DiskUsedBytes)
	target.DiskLimitBytes = maxUint64(target.DiskLimitBytes, incoming.DiskLimitBytes)

	if target.BucketKind == model.SpaceUsageBucketDay {
		target.ActivityWriteCount += incoming.ActivityWriteCount
		target.ActivityCreateCount += incoming.ActivityCreateCount
		target.ActivityDeleteCount += incoming.ActivityDeleteCount
		target.ActivityRenameCount += incoming.ActivityRenameCount
		target.ActivitySpaceStarts += incoming.ActivitySpaceStarts
		target.ActivitySpaceStops += incoming.ActivitySpaceStops
		target.ActivitySpaceCreates += incoming.ActivitySpaceCreates
		target.ActivitySpaceDeletes += incoming.ActivitySpaceDeletes
		target.ActivityDistinctPaths += incoming.ActivityDistinctPaths
	} else {
		target.ActivityWriteCount = maxUint32(target.ActivityWriteCount, incoming.ActivityWriteCount)
		target.ActivityCreateCount = maxUint32(target.ActivityCreateCount, incoming.ActivityCreateCount)
		target.ActivityDeleteCount = maxUint32(target.ActivityDeleteCount, incoming.ActivityDeleteCount)
		target.ActivityRenameCount = maxUint32(target.ActivityRenameCount, incoming.ActivityRenameCount)
		target.ActivitySpaceStarts = maxUint32(target.ActivitySpaceStarts, incoming.ActivitySpaceStarts)
		target.ActivitySpaceStops = maxUint32(target.ActivitySpaceStops, incoming.ActivitySpaceStops)
		target.ActivitySpaceCreates = maxUint32(target.ActivitySpaceCreates, incoming.ActivitySpaceCreates)
		target.ActivitySpaceDeletes = maxUint32(target.ActivitySpaceDeletes, incoming.ActivitySpaceDeletes)
		target.ActivityDistinctPaths = maxUint32(target.ActivityDistinctPaths, incoming.ActivityDistinctPaths)
	}

	if incoming.LastActivityAt != nil && (target.LastActivityAt == nil || incoming.LastActivityAt.After(*target.LastActivityAt)) {
		lastActivityAt := incoming.LastActivityAt.UTC()
		target.LastActivityAt = &lastActivityAt
	}
}

func sanitizeLastActivityAtForSave(lastActivityAt time.Time) *time.Time {
	sanitized := lastActivityAt.UTC()
	if sanitized.After(time.Now().UTC().Add(5 * time.Minute)) {
		return nil
	}
	return &sanitized
}

func sanitizeLastActivityAtForRead(lastActivityAt *time.Time) *time.Time {
	if lastActivityAt == nil {
		return nil
	}
	return sanitizeLastActivityAtForSave(*lastActivityAt)
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

func maxUint32(left, right uint32) uint32 {
	if right > left {
		return right
	}
	return left
}
