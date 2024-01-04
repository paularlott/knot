package nomad

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateSpaceVolumes(template *model.Template, space *model.Space) error {
  db := database.GetInstance()

  // Load the user
  user, err := db.GetUser(space.UserId)
  if err != nil {
    return err
  }

  // Get the volume definitions
  volumes, err := template.GetVolumes(space, user)
  if err != nil {
    return err
  }

  log.Debug().Msg("nomad: checking for required volumes")

  // Find the volumes that are defined but not yet created in the space and create them
  for _, volume := range volumes.Volumes {

    // Check if the volume is already created for the space
    if data, ok := space.VolumeData[volume.Name]; !ok || data.Namespace != volume.Namespace {
      // Existing volume then destroy it as in wrong namespace
      if ok {
        log.Debug().Msgf("nomad: deleting volume %s from wrong namespace", volume.Id)
        client.DeleteCSIVolume(&data)
        delete(space.VolumeData, volume.Id)
      }

      // Create the volume
      err := client.CreateCSIVolume(&volume)
      if err != nil {
        db.SaveSpace(space) // Save the space to capture the volumes
        return err
      }

      // Remember the volume
      space.VolumeData[volume.Id] = model.SpaceVolume{
        Id: volume.Id,
        Namespace: volume.Namespace,
      }
    }
  }

  // FIXME this is a hack to get the space to show as deployed
  space.IsDeployed = true

  // Save the space with the volume data
  err = db.SaveSpace(space)
  if err != nil {
    return err
  }

  log.Debug().Msg("nomad: volumes checked")

  return nil
}

func (client *NomadClient) DeleteSpaceVolumes(space *model.Space) error {
  db := database.GetInstance()

  log.Debug().Msg("nomad: deleting volumes")

  // For all volumes in the space delete them
  for _, volume := range space.VolumeData {
    err := client.DeleteCSIVolume(&volume)
    if err != nil {
      db.SaveSpace(space) // Save the space to capture the volumes
      return err
    }

    delete(space.VolumeData, volume.Id)
  }

  // Save the space with the volume data
  err := db.SaveSpace(space)
  if err != nil {
    return err
  }

  log.Debug().Msg("nomad: volumes deleted")

  return nil
}

func (client *NomadClient) CreateSpaceJob(template *model.Template, space *model.Space) error {
  // TODO Implement
  return nil
}

func (client *NomadClient) DeleteSpaceJob(space *model.Space) error {
  // TODO Implement
  return nil
}
