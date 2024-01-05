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
  var volById = make(map[string]*model.Volume)
  for _, volume := range volumes.Volumes {
    volById[volume.Id] = &volume

    // Check if the volume is already created for the space
    if data, ok := space.VolumeData[volume.Name]; !ok || data.Namespace != volume.Namespace {
      // Existing volume then destroy it as in wrong namespace
      if ok {
        log.Debug().Msgf("nomad: deleting volume %s due to wrong namespace", volume.Id)
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

  // Find the volumes deployed in the space but no longer in the template definition and remove them
  for _, volume := range space.VolumeData {
    // Check if the volume is defined in the template
    if _, ok := volById[volume.Id]; !ok {
      // Delete the volume
      err := client.DeleteCSIVolume(&volume)
      if err != nil {
        db.SaveSpace(space) // Save the space to capture the volumes
        return err
      }

      delete(space.VolumeData, volume.Id)
    }
  }

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

  log.Debug().Msgf("nomad: creating space job %s", space.Id)

  // Load the user
  db := database.GetInstance()
  user, err := db.GetUser(space.UserId)
  if err != nil {
    return err
  }

  // Pre-parse the job to fill out the knot variables
  jobHCL, err := model.ResolveVariables(template.Job, space, user)
  if err != nil {
    return err
  }

  // Convert job to JSON
  jobJSON, err := client.ParseJobHCL(jobHCL)
  if err != nil {
    log.Debug().Msgf("nomad: creating space job %s, parse error: %s", space.Id, err)
    return err
  }

  // Save the namespace and job ID to the space
  namespace, ok := jobJSON["Namespace"].(string)
  if !ok {
    namespace = "default"
  }
  space.NomadNamespace = namespace
  space.NomadJobId = jobJSON["ID"].(string)

  // Launch the job
  _, err = client.CreateJob(&jobJSON)
  if err != nil {
    log.Debug().Msgf("nomad: creating space job %s, error: %s", space.Id, err)
    return err
  }

  // Record deployed
  space.IsDeployed = true
  err = db.SaveSpace(space)
  if err != nil {
    log.Debug().Msgf("nomad: creating space job %s error %s", space.Id, err)
    return err
  }

  log.Debug().Msgf("nomad: created space job %s as %s", space.Id, space.NomadJobId)

  return nil
}

func (client *NomadClient) DeleteSpaceJob(space *model.Space) error {
  log.Debug().Msgf("nomad: deleting space job %s, %s", space.Id, space.NomadJobId)

  _, err := client.DeleteJob(space.NomadJobId, space.NomadNamespace)
  if err != nil {
    log.Debug().Msgf("nomad: deleting space job %s, error: %s", space.Id, err)
    return err
  }

  space.IsDeployed = false

  db := database.GetInstance()
  err = db.SaveSpace(space)
  if err != nil {
    log.Debug().Msgf("nomad: deleting space job %s error %s", space.Id, err)
    return err
  }

  log.Debug().Msgf("nomad: deleted space job %s, %s", space.Id, space.NomadJobId)

  return nil
}
