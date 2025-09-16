package nomad

import (
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/rs/zerolog/log"
)

func (client *NomadClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	db := database.GetInstance()

	// Get the volume definitions
	volumes, err := template.GetVolumes(space, user, variables)
	if err != nil {
		return err
	}

	if len(volumes.Volumes) == 0 && len(space.VolumeData) == 0 {
		log.Debug().Msg("nomad: no volumes to create")
		return nil
	}

	defer func() {
		// Save the space with the volume data
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"}); err != nil {
			log.Error().Msgf("nomad: saving space %s error %s", space.Id, err)
		}
		service.GetTransport().GossipSpace(space)
	}()

	log.Debug().Msg("nomad: checking for required volumes")

	// Find the volumes that are defined but not yet created in the space and create them
	var volById = make(map[string]*model.CSIVolume)
	for _, volume := range volumes.Volumes {
		if volume.Id == "" {
			volume.Id = volume.Name
		}

		volById[volume.Id] = &volume

		// Check if the volume is already created for the space
		if data, ok := space.VolumeData[volume.Id]; !ok || data.Namespace != volume.Namespace {
			// Existing volume then destroy it as in wrong namespace
			if ok {
				log.Debug().Msgf("nomad: deleting volume %s due to wrong namespace", volume.Id)
				if volume.Type == "host" {
					var id string
					id, err = client.GetIdHostVolume(data.Id, data.Namespace)
					if err == nil {
						client.DeleteHostVolume(id, data.Namespace)
					}
				} else {
					client.DeleteCSIVolume(data.Id, data.Namespace)
				}
				delete(space.VolumeData, volume.Id)
			}

			// Create the volume
			switch volume.Type {
			case "csi":
				err = client.CreateCSIVolume(&volume)
			case "host":
				_, err = client.CreateHostVolume(&volume)
			default:
				err = fmt.Errorf("unsupported volume type: %s", volume.Type)
			}
			if err != nil {
				return err
			}

			// Remember the volume
			space.VolumeData[volume.Id] = model.SpaceVolume{
				Id:        volume.Id,
				Namespace: volume.Namespace,
				Type:      volume.Type,
			}
		}
	}

	// Find the volumes deployed in the space but no longer in the template definition and remove them
	for _, volume := range space.VolumeData {
		// Check if the volume is defined in the template
		if _, ok := volById[volume.Id]; !ok {
			// Delete the volume
			var err error
			if volume.Type == "host" {
				var id string
				id, err = client.GetIdHostVolume(volume.Id, volume.Namespace)
				if err == nil {
					client.DeleteHostVolume(id, volume.Namespace)
				}
			} else {
				err = client.DeleteCSIVolume(volume.Id, volume.Namespace)
			}
			if err != nil {
				return err
			}

			delete(space.VolumeData, volume.Id)
		}
	}

	log.Debug().Msg("nomad: volumes checked")

	return nil
}

func (client *NomadClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	log.Debug().Msg("nomad: deleting volumes")

	if len(space.VolumeData) == 0 {
		log.Debug().Msg("nomad: no volumes to delete")
		return nil
	}

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
	}()

	// For all volumes in the space delete them
	for _, volume := range space.VolumeData {
		var err error
		if volume.Type == "host" {
			var id string
			id, err = client.GetIdHostVolume(volume.Id, volume.Namespace)
			if err == nil {
				client.DeleteHostVolume(id, volume.Namespace)
			}
		} else {
			err = client.DeleteCSIVolume(volume.Id, volume.Namespace)
		}
		if err != nil {
			return err
		}

		delete(space.VolumeData, volume.Id)
	}

	log.Debug().Msg("nomad: volumes deleted")

	return nil
}

func (client *NomadClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

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
	space.Zone = cfg.Zone
	space.StartedAt = time.Now().UTC()
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"NomadNamespace", "ContainerId", "IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Zone", "UpdatedAt", "StartedAt"})
	if err != nil {
		log.Error().Msgf("nomad: creating space job %s error %s", space.Id, err)
		return err
	}

	service.GetTransport().GossipSpace(space)
	client.MonitorJobState(space, nil)

	return nil
}

func (client *NomadClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	log.Debug().Msgf("nomad: deleting space job %s, %s", space.Id, space.ContainerId)

	_, err := client.DeleteJob(space.ContainerId, space.NomadNamespace)
	if err != nil {
		log.Debug().Msgf("nomad: deleting space job %s, error: %s", space.Id, err)
		return err
	}

	// Record stopping
	space.IsPending = true
	space.UpdatedAt = hlc.Now()

	db := database.GetInstance()
	err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	if err != nil {
		log.Debug().Msgf("nomad: deleting space job %s error %s", space.Id, err)
		return err
	}

	service.GetTransport().GossipSpace(space)
	client.MonitorJobState(space, onStopped)

	return nil
}

func (client *NomadClient) MonitorJobState(space *model.Space, onDone func()) {
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
						break
					}
				} else if code == 404 || data["Status"] == "dead" {
					log.Info().Msgf("nomad: space job %s is dead", space.ContainerId)

					space.IsPending = false
					space.IsDeployed = false
					break
				}
			}

			// Sleep for a bit
			time.Sleep(500 * time.Millisecond)
		}

		log.Info().Msgf("nomad: update space job %s status", space.ContainerId)
		space.UpdatedAt = hlc.Now()
		err := database.GetInstance().SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			log.Error().Msgf("nomad: updating space job %s error %s", space.ContainerId, err)
		}
		service.GetTransport().GossipSpace(space)

		if onDone != nil {
			onDone()
		}
	}()
}
