package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleVolumeFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received volume full sync request")

	volumes := []*model.Volume{}
	if err := packet.Unmarshal(&volumes); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal volume full sync request")
		return nil, err
	}

	// Get the list of volumes in the system
	db := database.GetInstance()
	existingVolumes, err := db.GetVolumes()
	if err != nil {
		return nil, err
	}

	// Merge the volumes in the background
	go c.mergeVolumes(volumes)

	// Return the full dataset directly as response
	return existingVolumes, nil
}

func (c *Cluster) handleVolumeGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received volume gossip request")

	volumes := []*model.Volume{}
	if err := packet.Unmarshal(&volumes); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal volume gossip request")
		return err
	}

	// Merge the volumes with the local volumes
	if err := c.mergeVolumes(volumes); err != nil {
		c.logger.WithError(err).Error("Failed to merge volumes")
		return err
	}

	return nil
}

func (c *Cluster) GossipVolume(volume *model.Volume) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping volume")

		volumes := []*model.Volume{volume}
		c.gossipCluster.Send(VolumeGossipMsg, &volumes)
	}
}

func (c *Cluster) DoVolumeFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		// Get the list of volumes in the system
		db := database.GetInstance()
		volumes, err := db.GetVolumes()
		if err != nil {
			return err
		}

		// Exchange the volume list with the remote node
		if err := c.gossipCluster.SendToWithResponse(node, VolumeFullSyncMsg, &volumes, &volumes); err != nil {
			return err
		}

		// Merge the volumes with the local volumes
		if err := c.mergeVolumes(volumes); err != nil {
			c.logger.WithError(err).Error("Failed to merge volumes")
			return err
		}
	}

	return nil
}

// Merges the volumes from a cluster member with the local volumes
func (c *Cluster) mergeVolumes(volumes []*model.Volume) error {
	c.logger.Debug("Merging volumes", "number_volumes", len(volumes))

	// Get the list of volumes in the system
	db := database.GetInstance()
	localVolumes, err := db.GetVolumes()
	if err != nil {
		return err
	}

	// Convert the list of local volumes to a map
	localVolumesMap := make(map[string]*model.Volume)
	for _, volume := range localVolumes {
		localVolumesMap[volume.Id] = volume
	}

	// Merge the volumes
	for _, volume := range volumes {
		if localVolume, ok := localVolumesMap[volume.Id]; ok {
			// If the remote volume is newer than the local volume then use its data
			if volume.UpdatedAt.After(localVolume.UpdatedAt) {

				// If the volume is deleted on the remote but not local then we need to stop it
				if volume.IsDeleted && !localVolume.IsDeleted && localVolume.Active {
					c.logger.Debug("Stopping deleted volume", "name", volume.Name)

					if err := service.GetContainerService().DeleteVolume(localVolume); err != nil {
						c.logger.Error("Failed to delete volume", "error", err, "name", volume.Name)
					}
				}

				if err := db.SaveVolume(volume, nil); err != nil {
					c.logger.Error("Failed to update volume", "error", err, "name", volume.Name)
				}
			}
		} else {
			// If the volume doesn't exist locally, create it (even if deleted) to prevent resurrection
			if err := db.SaveVolume(volume, nil); err != nil {
				c.logger.Error("Failed to save volume", "error", err, "name", volume.Name, "is_deleted", volume.IsDeleted)
			}
		}
	}

	sse.PublishVolumesChanged()

	return nil
}

// Gossips a subset of the volumes to the cluster
func (c *Cluster) gossipVolumes() {
	// Get the list of volumes in the system
	db := database.GetInstance()
	volumes, err := db.GetVolumes()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get volumes")
		return
	}

	batchSize := c.gossipCluster.CalcPayloadSize(len(volumes))
	if batchSize == 0 {
		return // No keys to send in this batch
	}

	c.logger.Debug("Gossipping volumes", "batch_size", batchSize, "total", len(volumes))

	// Shuffle the volumes
	rand.Shuffle(len(volumes), func(i, j int) {
		volumes[i], volumes[j] = volumes[j], volumes[i]
	})

	// Get the 1st number of volumes up to the batch size & broadcast
	volumes = volumes[:batchSize]
	c.gossipCluster.Send(VolumeGossipMsg, &volumes)
}
