package cluster

import (
	"math/rand"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
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
	c.logger.Debug("Received space usage gossip request")

	samples := []*model.SpaceUsageSample{}
	if err := packet.Unmarshal(&samples); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal space usage gossip request")
		return err
	}

	go func() {
		if err := c.mergeSpaceUsageSamples(samples); err != nil {
			c.logger.WithError(err).Error("Failed to merge space usage samples")
		}

		if len(c.leafSessions) > 0 {
			c.sendToLeafNodes(leafmsg.MessageGossipSpaceUsage, &samples)
		}
	}()

	return nil
}

func (c *Cluster) GossipSpaceUsageSample(sample *model.SpaceUsageSample) {
	if c.gossipCluster != nil {
		samples := []*model.SpaceUsageSample{sample}
		c.gossipCluster.Send(SpaceUsageGossipMsg, &samples)
	}

	if len(c.leafSessions) > 0 {
		samples := []*model.SpaceUsageSample{sample}
		c.sendToLeafNodes(leafmsg.MessageGossipSpaceUsage, &samples)
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
		if err == nil && existing != nil && !sample.UpdatedAt.After(existing.UpdatedAt) {
			continue
		}

		if err := db.SaveSpaceUsageSample(sample); err != nil {
			c.logger.WithError(err).Error("Failed to save space usage sample", "space_usage_id", sample.Id)
		}
	}
	return nil
}

func (c *Cluster) gossipSpaceUsage() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
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
			clusterSamples := samples[:batchSize]
			c.gossipCluster.Send(SpaceUsageGossipMsg, &clusterSamples)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := 0
		if c.gossipCluster != nil {
			batchSize = c.gossipCluster.CalcPayloadSize(len(samples))
		}
		if batchSize == 0 && len(samples) > 0 {
			batchSize = 1
		}
		if batchSize > 0 {
			leafSamples := samples[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipSpaceUsage, &leafSamples)
		}
	}
}
