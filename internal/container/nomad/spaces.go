package nomad

import (
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error {
	db := database.GetInstance()

	// Get the volume definitions
	volumes, err := template.GetVolumes(space, user, variables)
	if err != nil {
		return err
	}

	log.Debug().Msg("nomad: checking for required volumes")

	// Find the volumes that are defined but not yet created in the space and create them
	var volById = make(map[string]*model.CSIVolume)
	for _, volume := range volumes.Volumes {
		volById[volume.Id] = &volume

		// Check if the volume is already created for the space
		if data, ok := space.VolumeData[volume.Id]; !ok || data.Namespace != volume.Namespace {
			// Existing volume then destroy it as in wrong namespace
			if ok {
				log.Debug().Msgf("nomad: deleting volume %s due to wrong namespace", volume.Id)
				client.DeleteCSIVolume(data.Id, data.Namespace)
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
				Id:        volume.Id,
				Namespace: volume.Namespace,
			}
		}
	}

	// Find the volumes deployed in the space but no longer in the template definition and remove them
	for _, volume := range space.VolumeData {
		// Check if the volume is defined in the template
		if _, ok := volById[volume.Id]; !ok {
			// Delete the volume
			err := client.DeleteCSIVolume(volume.Id, volume.Namespace)
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
		err := client.DeleteCSIVolume(volume.Id, volume.Namespace)
		if err != nil {
			db.SaveSpace(space) // Save the space to capture the volumes
			return err
		}

		delete(space.VolumeData, volume.Id)
		db.SaveSpace(space)
	}

	log.Debug().Msg("nomad: volumes deleted")

	return nil
}

func (client *NomadClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error {
	db := database.GetInstance()

	log.Debug().Msgf("nomad: creating space job %s", space.Id)

	// Pre-parse the job to fill out the knot variables
	jobHCL, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	// Convert job to JSON
	jobJSON, err := client.ParseJobHCL(jobHCL)
	if err != nil {
		log.Error().Msgf("nomad: creating space job %s, parse error: %s", space.Id, err)
		return err
	}

	// Save the namespace and job ID to the space
	namespace, ok := jobJSON["Namespace"].(string)
	if !ok {
		namespace = "default"
	}
	space.NomadNamespace = namespace
	space.ContainerId = jobJSON["ID"].(string)

	// Launch the job
	_, err = client.CreateJob(&jobJSON)
	if err != nil {
		log.Error().Msgf("nomad: creating space job %s, error: %s", space.Id, err)
		return err
	}

	// Record deploying
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.Location = server_info.LeafLocation
	err = db.SaveSpace(space)
	if err != nil {
		log.Error().Msgf("nomad: creating space job %s error %s", space.Id, err)
		return err
	}

	client.MonitorJobState(space)

	return nil
}

func (client *NomadClient) DeleteSpaceJob(space *model.Space) error {
	log.Debug().Msgf("nomad: deleting space job %s, %s", space.Id, space.ContainerId)

	_, err := client.DeleteJob(space.ContainerId, space.NomadNamespace)
	if err != nil {
		log.Debug().Msgf("nomad: deleting space job %s, error: %s", space.Id, err)
		return err
	}

	// Record stopping
	space.IsPending = true

	db := database.GetInstance()
	err = db.SaveSpace(space)
	if err != nil {
		log.Debug().Msgf("nomad: deleting space job %s error %s", space.Id, err)
		return err
	}

	client.MonitorJobState(space)

	return nil
}

func (client *NomadClient) MonitorJobState(space *model.Space) {
	go func() {
		log.Info().Msgf("nomad: watching job %s status for change", space.ContainerId)

		for {
			code, data, err := client.ReadJob(space.ContainerId, space.NomadNamespace)
			if err != nil && code != 404 {
				log.Error().Msgf("nomad: reading space job %s, error: %s", space.ContainerId, err)
			} else {
				if code == 404 {
					log.Debug().Msgf("nomad: reading space job %s, status: %s", space.ContainerId, "404")
				} else {
					log.Debug().Msgf("nomad: reading space job %s, status: %s", space.ContainerId, data["Status"])
				}

				if code == 200 && data["Status"] == "running" {
					// If waiting for job to start then done
					if space.IsPending && !space.IsDeployed {
						log.Info().Msgf("nomad: space job %s is running", space.ContainerId)

						space.IsPending = false
						space.IsDeployed = true
						err = database.GetInstance().SaveSpace(space)
						if err != nil {
							log.Error().Msgf("nomad: updating space job %s error %s", space.ContainerId, err)
						}

						origin.UpdateSpace(space)
						break
					}
				} else if code == 404 || data["Status"] == "dead" {
					log.Info().Msgf("nomad: space job %s is dead", space.ContainerId)

					space.IsPending = false
					space.IsDeployed = false
					err = database.GetInstance().SaveSpace(space)
					if err != nil {
						log.Error().Msgf("nomad: updating space job %s error %s", space.ContainerId, err)
					}

					origin.UpdateSpace(space)

					break
				}
			}

			// Sleep for a bit
			time.Sleep(2 * time.Second)
		}
	}()
}
