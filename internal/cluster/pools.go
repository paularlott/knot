package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

type PoolDrainRequest struct {
	SpaceID string `json:"space_id" msgpack:"space_id"`
	Undrain bool   `json:"undrain" msgpack:"undrain"`
}

func (c *Cluster) handlePoolDefinitionFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	pools := []*model.PoolDefinition{}
	if err := packet.Unmarshal(&pools); err != nil {
		return nil, err
	}

	db := database.GetInstance()
	existingPools, err := db.GetPoolDefinitions()
	if err != nil {
		return nil, err
	}

	go c.mergePoolDefinitions(pools)
	return existingPools, nil
}

func (c *Cluster) handlePoolDefinitionGossip(sender *gossip.Node, packet *gossip.Packet) error {
	pools := []*model.PoolDefinition{}
	if err := packet.Unmarshal(&pools); err != nil {
		return err
	}
	return c.mergePoolDefinitions(pools)
}

// GossipPoolDefinition fans a pool definition change out to the cluster and
// notifies local SSE clients. It is the single choke point for local mutations
// (Create/SetSize/Start/Stop/Delete/UpdateStartupScript all route through it),
// so publishing the SSE event here ensures the editing server's other clients
// refresh immediately; mergePoolDefinitions does the same on receiving servers.
func (c *Cluster) GossipPoolDefinition(pool *model.PoolDefinition) {
	if c.gossipCluster == nil || pool == nil {
		return
	}
	pools := []*model.PoolDefinition{pool}
	c.gossipCluster.Send(PoolDefinitionGossipMsg, &pools)
	if pool.IsDeleted {
		sse.PublishPoolDeleted(pool.Id)
	} else {
		sse.PublishPoolChanged(pool.Id)
	}
}

func (c *Cluster) DoPoolDefinitionFullSync(node *gossip.Node) error {
	if c.gossipCluster == nil {
		return nil
	}

	db := database.GetInstance()
	pools, err := db.GetPoolDefinitions()
	if err != nil {
		return err
	}
	if err := c.gossipCluster.SendToWithResponse(node, PoolDefinitionFullSyncMsg, &pools, &pools); err != nil {
		return err
	}
	return c.mergePoolDefinitions(pools)
}

func (c *Cluster) mergePoolDefinitions(pools []*model.PoolDefinition) error {
	db := database.GetInstance()
	localPools, err := db.GetPoolDefinitions()
	if err != nil {
		return err
	}

	local := make(map[string]*model.PoolDefinition)
	for _, pool := range localPools {
		local[pool.Id] = pool
	}

	for _, pool := range pools {
		if localPool, ok := local[pool.Id]; ok {
			if pool.UpdatedAt.After(localPool.UpdatedAt) {
				if err := db.SavePoolDefinition(pool, nil); err != nil {
					c.logger.Error("failed to update pool definition", "error", err, "name", pool.Name)
					continue
				}
				if pool.IsDeleted {
					sse.PublishPoolDeleted(pool.Id)
				} else {
					sse.PublishPoolChanged(pool.Id)
				}
			}
		} else {
			if err := db.SavePoolDefinition(pool, nil); err != nil {
				c.logger.Error("failed to save pool definition", "error", err, "name", pool.Name)
				continue
			}
			if pool.IsDeleted {
				sse.PublishPoolDeleted(pool.Id)
			} else {
				sse.PublishPoolChanged(pool.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipPoolDefinitions() {
	if c.gossipCluster == nil {
		return
	}

	db := database.GetInstance()
	pools, err := db.GetPoolDefinitions()
	if err != nil {
		c.logger.WithError(err).Error("failed to get pool definitions")
		return
	}
	rand.Shuffle(len(pools), func(i, j int) {
		pools[i], pools[j] = pools[j], pools[i]
	})
	batchSize := c.gossipCluster.CalcPayloadSize(len(pools))
	if batchSize > 0 {
		pools = pools[:batchSize]
		c.gossipCluster.Send(PoolDefinitionGossipMsg, &pools)
	}
}

func (c *Cluster) GossipPoolDrain(spaceID string) {
	if spaceID == "" {
		return
	}
	methods.DefaultRegistry().Drain(spaceID)
	if c.gossipCluster == nil {
		return
	}
	c.gossipCluster.Send(PoolDrainMsg, &PoolDrainRequest{SpaceID: spaceID})
}

func (c *Cluster) GossipPoolUndrain(spaceID string) {
	if spaceID == "" {
		return
	}
	methods.DefaultRegistry().Undrain(spaceID)
	if c.gossipCluster == nil {
		return
	}
	c.gossipCluster.Send(PoolDrainMsg, &PoolDrainRequest{SpaceID: spaceID, Undrain: true})
}

func (c *Cluster) handlePoolDrain(sender *gossip.Node, packet *gossip.Packet) error {
	req := &PoolDrainRequest{}
	if err := packet.Unmarshal(req); err != nil {
		return err
	}
	poolSvc := service.GetPoolService()
	if req.Undrain {
		methods.DefaultRegistry().Undrain(req.SpaceID)
		poolSvc.MarkUndrained(req.SpaceID)
	} else {
		methods.DefaultRegistry().Drain(req.SpaceID)
		poolSvc.MarkDrained(req.SpaceID)
	}
	return nil
}

func (c *Cluster) IsLeader() bool {
	if c.election == nil || !c.electionRunning {
		return true
	}
	return c.election.IsLeader()
}
