package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) handleVolumeFullSync(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	log.Debug().Msg("cluster: Received volume full sync request")

	volumes := []*model.Volume{}
	if err := packet.Unmarshal(&volumes); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal volume full sync request")
		return gossip.NilMsg, nil, err
	}

	// Get the list of volumes in the system
	db := database.GetInstance()
	existingVolumes, err := db.GetVolumes()
	if err != nil {
		return gossip.NilMsg, nil, err
	}

	// Merge the volumes in the background
	go c.mergeVolumes(volumes)

	// Return the full dataset directly as response
	return VolumeFullSyncMsg, existingVolumes, nil
}

func (c *Cluster) handleVolumeGossip(sender *gossip.Node, packet *gossip.Packet) error {
	log.Debug().Msg("cluster: Received volume gossip request")

	volumes := []*model.Volume{}
	if err := packet.Unmarshal(&volumes); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal volume gossip request")
		return err
	}

	// Merge the volumes with the local volumes
	if err := c.mergeVolumes(volumes); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to merge volumes")
		return err
	}

	return nil
}

func (c *Cluster) GossipVolume(volume *model.Volume) {
	if c.gossipCluster != nil {
		log.Debug().Msg("cluster: Gossipping volume")

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
		if err := c.gossipCluster.SendToWithResponse(node, VolumeFullSyncMsg, &volumes, VolumeFullSyncMsg, &volumes); err != nil {
			return err
		}

		// Merge the volumes with the local volumes
		if err := c.mergeVolumes(volumes); err != nil {
			log.Error().Err(err).Msg("cluster: Failed to merge volumes")
			return err
		}
	}

	return nil
}

// Merges the volumes from a cluster member with the local volumes
func (c *Cluster) mergeVolumes(volumes []*model.Volume) error {
	log.Debug().Int("number_volumes", len(volumes)).Msg("cluster: Merging volumes")

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
				if err := db.SaveVolume(volume, nil); err != nil {
					log.Error().Err(err).Str("name", volume.Name).Msg("cluster: Failed to update volume")
				}
			}
		} else if !volume.IsDeleted { // Changed from group.IsDeleted to volume.IsDeleted
			// If the volume doesn't exist, create it unless it's deleted on the remote node
			if err := db.SaveVolume(volume, nil); err != nil { // Changed from db.SaveGroup to db.SaveVolume
				return err
			}
		}
	}

	return nil
}

// Gossips a subset of the volumes to the cluster
func (c *Cluster) gossipVolumes() {
	// Get the list of volumes in the system
	db := database.GetInstance()
	volumes, err := db.GetVolumes()
	if err != nil {
		log.Error().Err(err).Msg("cluster: Failed to get volumes")
		return
	}

	batchSize := c.gossipCluster.GetBatchSize(len(volumes))
	if batchSize == 0 {
		return // No keys to send in this batch
	}

	log.Debug().Int("batch_size", batchSize).Int("total", len(volumes)).Msg("cluster: Gossipping volumes")

	// Shuffle the volumes
	rand.Shuffle(len(volumes), func(i, j int) {
		volumes[i], volumes[j] = volumes[j], volumes[i]
	})

	// Get the 1st number of volumes up to the batch size & broadcast
	volumes = volumes[:batchSize]
	c.gossipCluster.Send(VolumeGossipMsg, &volumes)
}
