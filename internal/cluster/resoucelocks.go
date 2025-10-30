package cluster

import (
	"math/rand"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
)

const (
	ResourceLockTTL        = 1 * time.Minute
	ResourceLockGCInterval = 30 * time.Second
)

type ResourceLockRequestMsg struct {
	ResourceId string
}

type ResourceLockResponseMsg struct {
	UnlockToken string
}

type ResourceUnlockRequestMsg struct {
	ResourceId  string
	UnlockToken string
}

type ResourceLock struct {
	Id           string
	UnlockToken  string
	IsDeleted    bool
	ExpiresAfter time.Time
	UpdatedAt    hlc.Timestamp
}

func (c *Cluster) handleResourceLockFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received resource lock full sync request")

	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		c.logger.Debug("Ignoring resource lock full sync request from a different zone")
		return []*ResourceLock{}, nil
	}

	resourceLocks := []*ResourceLock{}
	if err := packet.Unmarshal(&resourceLocks); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal resource lock full sync request")
		return nil, err
	}

	// Get the list of locks in the system
	c.resourceLocksMux.RLock()
	existingLocks := []*ResourceLock{}
	for _, lock := range c.resourceLocks {
		existingLocks = append(existingLocks, lock)
	}
	c.resourceLocksMux.RUnlock()

	// Merge the locks in the background
	go c.mergeResourceLocks(resourceLocks)

	// Return the full dataset directly as response
	return existingLocks, nil
}

func (c *Cluster) handleResourceLockGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received resource lock gossip request")

	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		c.logger.Debug("Ignoring resource lock gossip request from a different zone")
		return nil
	}

	resourceLocks := []*ResourceLock{}
	if err := packet.Unmarshal(&resourceLocks); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal resource lock gossip request")
		return err
	}

	// Merge the resource locks with the local resource locks
	if err := c.mergeResourceLocks(resourceLocks); err != nil {
		c.logger.WithError(err).Error("Failed to merge resource locks")
		return err
	}

	return nil
}

func (c *Cluster) GossipResourceLock(resourceLock *ResourceLock) {
	if c.sessionGossip && c.gossipCluster != nil {
		resourceLocks := []*ResourceLock{resourceLock}
		c.election.GetNodeGroup().SendToPeers(ResourceLockGossipMsg, &resourceLocks)
	}
}

func (c *Cluster) DoResourceLockFullSync(node *gossip.Node) error {
	if c.sessionGossip && c.gossipCluster != nil {

		// If the node doesn't match our zone then ignore the request
		cfg := config.GetServerConfig()
		if node.Metadata.GetString("zone") != cfg.Zone {
			c.logger.Debug("Ignoring resource lock full sync with node from a different zone")
			return nil
		}

		// Get the list of resource locks in the system
		c.resourceLocksMux.RLock()
		resourceLocks := []*ResourceLock{}
		for _, lock := range c.resourceLocks {
			resourceLocks = append(resourceLocks, lock)
		}
		c.resourceLocksMux.RUnlock()

		// Exchange the resource lock list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, ResourceLockFullSyncMsg, &resourceLocks, &resourceLocks); err != nil {
			return err
		}

		// Merge the resource locks with the local resource locks
		if err := c.mergeResourceLocks(resourceLocks); err != nil {
			c.logger.WithError(err).Error("Failed to merge resource locks")
			return err
		}
	}

	return nil
}

// Merges the resource locks from a cluster member with the local resource locks
func (c *Cluster) mergeResourceLocks(resourceLocks []*ResourceLock) error {
	c.logger.Debug("Merging resource locks", "number_resource_locks", len(resourceLocks))

	c.resourceLocksMux.Lock()
	defer c.resourceLocksMux.Unlock()

	// Merge the locks
	for _, lock := range resourceLocks {
		if localLock, ok := c.resourceLocks[lock.Id]; ok {
			// If the remote session is newer than the local session then use it's data
			if lock.UpdatedAt.After(localLock.UpdatedAt) {
				c.resourceLocks[lock.Id].UnlockToken = lock.UnlockToken
				c.resourceLocks[lock.Id].IsDeleted = lock.IsDeleted
				c.resourceLocks[lock.Id].ExpiresAfter = lock.ExpiresAfter
				c.resourceLocks[lock.Id].UpdatedAt = lock.UpdatedAt
			}
		} else if lock.ExpiresAfter.After(time.Now().UTC()) {
			// If the lock doesn't exist locally and hasn't expired, create it (even if deleted) to prevent resurrection
			c.resourceLocks[lock.Id] = lock
		}
		// Note: We don't save expired locks since they're already obsolete
	}

	return nil
}

// Gossips a subset of the resource locks to the cluster
func (c *Cluster) gossipResourceLocks() {
	if !c.sessionGossip || c.gossipCluster == nil || len(c.resourceLocks) == 0 {
		return
	}

	// Get the list of locks in the system
	locks := []*ResourceLock{}
	c.resourceLocksMux.RLock()
	for _, lock := range c.resourceLocks {
		locks = append(locks, lock)
	}
	c.resourceLocksMux.RUnlock()

	// Shuffle the locks
	rand.Shuffle(len(locks), func(i, j int) {
		locks[i], locks[j] = locks[j], locks[i]
	})

	batchSize := c.gossipCluster.CalcPayloadSize(len(locks))
	if batchSize > 0 {
		locks = locks[:batchSize]
		c.gossipCluster.Send(ResourceLockGossipMsg, &locks)
	}
}
