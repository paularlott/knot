package cluster

import (
	"math/rand"
	"reflect"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func (c *Cluster) handleSpaceUsageFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received space usage full sync request")

	samples := []*model.SpaceUsageSample{}
	if err := packet.Unmarshal(&samples); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space usage full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingMinute, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketMinute, time.Now().UTC().Add(-model.SpaceUsageMinuteRetention), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	existingDay, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketDay, time.Now().UTC().Add(-model.SpaceUsageDayRetention), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	existing := append(existingMinute, existingDay...)

	go func() {
		if err := c.mergeSpaceUsageSamples(samples); err != nil {
			c.logger.WithError(err).Error("Failed to merge space usage samples")
		}
	}()

	return existing, nil
}

func (c *Cluster) handleSpaceUsageGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received space usage gossip request")

	samples := []*model.SpaceUsageSample{}
	if err := packet.Unmarshal(&samples); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space usage gossip request")
		return err
	}

	go func() {
		if err := c.mergeSpaceUsageSamples(samples); err != nil {
			c.logger.WithError(err).Error("Failed to merge space usage samples")
		}
	}()

	return nil
}

func (c *Cluster) GossipSpaceUsageSample(sample *model.SpaceUsageSample) {
	if c.gossipCluster != nil {
		samples := []*model.SpaceUsageSample{sample}
		c.gossipCluster.Send(SpaceUsageGossipMsg, &samples)
	}
}

func (c *Cluster) DoSpaceUsageFullSync(node *gossip.Node) error {
	if c.gossipCluster == nil {
		return nil
	}

	db := database.GetInstance()
	minuteSamples, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketMinute, time.Now().UTC().Add(-model.SpaceUsageMinuteRetention), time.Now().UTC())
	if err != nil {
		return err
	}
	daySamples, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketDay, time.Now().UTC().Add(-model.SpaceUsageDayRetention), time.Now().UTC())
	if err != nil {
		return err
	}
	samples := append(minuteSamples, daySamples...)

	if err := c.gossipCluster.SendToWithResponse(node, SpaceUsageFullSyncMsg, &samples, &samples); err != nil {
		return err
	}

	return c.mergeSpaceUsageSamples(samples)
}

func (c *Cluster) mergeSpaceUsageSamples(samples []*model.SpaceUsageSample) error {
	db := database.GetInstance()
	for _, sample := range samples {
		if sample == nil || sample.SpaceId == "" {
			continue
		}
		existing, err := db.GetSpaceUsageSample(sample.Id)
		if err == nil && existing != nil {
			if existing.UpdatedAt.After(sample.UpdatedAt) {
				continue
			}
			if !sample.UpdatedAt.After(existing.UpdatedAt) {
				merged := *existing
				mergeIncomingSpaceUsageSnapshot(&merged, sample)
				if reflect.DeepEqual(existing, &merged) {
					continue
				}
				sample = &merged
			}
		}

		if err := db.SaveSpaceUsageSample(sample); err != nil {
			c.logger.WithError(err).Error("Failed to save space usage sample", "space_usage_id", sample.Id)
		}
	}
	return nil
}

func mergeIncomingSpaceUsageSnapshot(target, incoming *model.SpaceUsageSample) {
	target.UserId = incoming.UserId
	target.UpdatedAt = incoming.UpdatedAt
	target.LastActivityAt = sanitizeClusterLastActivityAt(target.LastActivityAt)
	incoming.LastActivityAt = sanitizeClusterLastActivityAt(incoming.LastActivityAt)

	target.CPUPercent = maxFloat(target.CPUPercent, incoming.CPUPercent)
	target.MemoryUsedBytes = maxUint64(target.MemoryUsedBytes, incoming.MemoryUsedBytes)
	target.MemoryLimitBytes = maxUint64(target.MemoryLimitBytes, incoming.MemoryLimitBytes)
	target.DiskUsedBytes = maxUint64(target.DiskUsedBytes, incoming.DiskUsedBytes)
	target.DiskLimitBytes = maxUint64(target.DiskLimitBytes, incoming.DiskLimitBytes)
	target.ActivityWriteCount = maxUint32(target.ActivityWriteCount, incoming.ActivityWriteCount)
	target.ActivityCreateCount = maxUint32(target.ActivityCreateCount, incoming.ActivityCreateCount)
	target.ActivityDeleteCount = maxUint32(target.ActivityDeleteCount, incoming.ActivityDeleteCount)
	target.ActivityRenameCount = maxUint32(target.ActivityRenameCount, incoming.ActivityRenameCount)
	target.ActivityDistinctPaths = maxUint32(target.ActivityDistinctPaths, incoming.ActivityDistinctPaths)
	target.ActivitySpaceStarts = maxUint32(target.ActivitySpaceStarts, incoming.ActivitySpaceStarts)
	target.ActivitySpaceStops = maxUint32(target.ActivitySpaceStops, incoming.ActivitySpaceStops)
	target.ActivitySpaceCreates = maxUint32(target.ActivitySpaceCreates, incoming.ActivitySpaceCreates)
	target.ActivitySpaceDeletes = maxUint32(target.ActivitySpaceDeletes, incoming.ActivitySpaceDeletes)

	if incoming.LastActivityAt != nil && (target.LastActivityAt == nil || incoming.LastActivityAt.After(*target.LastActivityAt)) {
		lastActivityAt := incoming.LastActivityAt.UTC()
		target.LastActivityAt = &lastActivityAt
	}
}

func sanitizeClusterLastActivityAt(lastActivityAt *time.Time) *time.Time {
	if lastActivityAt == nil {
		return nil
	}

	sanitized := lastActivityAt.UTC()
	if sanitized.After(time.Now().UTC().Add(5 * time.Minute)) {
		return nil
	}

	return &sanitized
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

func (c *Cluster) gossipSpaceUsage() {
	if c.gossipCluster == nil {
		return
	}

	db := database.GetInstance()
	minuteSamples, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketMinute, time.Now().UTC().Add(-model.SpaceUsageMinuteRetention), time.Now().UTC())
	if err != nil {
		c.logger.WithError(err).Error("Failed to get space usage samples")
		return
	}
	daySamples, err := db.GetSpaceUsageSamples("", model.SpaceUsageBucketDay, time.Now().UTC().Add(-model.SpaceUsageDayRetention), time.Now().UTC())
	if err != nil {
		c.logger.WithError(err).Error("Failed to get daily space usage samples")
		return
	}
	samples := append(minuteSamples, daySamples...)

	rand.Shuffle(len(samples), func(i, j int) {
		samples[i], samples[j] = samples[j], samples[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(samples))
		if batchSize > 0 {
			c.logger.Debug("Gossipping space usage", "batch_size", batchSize, "total", len(samples))
			clusterSamples := samples[:batchSize]
			c.gossipCluster.Send(SpaceUsageGossipMsg, &clusterSamples)
		}
	}
}
