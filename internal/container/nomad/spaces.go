package nomad

import (
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

func (client *NomadClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	db := database.GetInstance()

	// Get the volume definitions
	volumes, err := template.GetVolumes(space, user, variables)
	if err != nil {
		return err
	}

	if len(volumes.Volumes) == 0 && len(space.VolumeData) == 0 {
		client.logger.Debug("no volumes to create")
		return nil
	}

	defer func() {
		// Save the space with the volume data
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"}); err != nil {
			client.logger.Error("saving space  error", "space_id", space.Id)
		}
		service.GetTransport().GossipSpace(space)
	}()

	client.logger.Debug("checking for required volumes")

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
				client.logger.Debug("deleting volume  due to wrong namespace", "volume_id", volume.Id)
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

	client.logger.Debug("volumes checked")

	return nil
}

func (client *NomadClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	client.logger.Debug("deleting volumes")

	if len(space.VolumeData) == 0 {
		client.logger.Debug("no volumes to delete")
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

	client.logger.Debug("volumes deleted")

	return nil
}

func (client *NomadClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	client.logger.Debug("creating space job", "space_id", space.Id)

	// Pre-parse the job to fill out the knot variables
	jobHCL, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	// Convert job to JSON
	jobJSON, err := client.ParseJobHCL(jobHCL)
	if err != nil {
		client.logger.Error("creating space job , parse error:", "space_id", space.Id)
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
		client.logger.Error("creating space job , error:", "space_id", space.Id)
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
		client.logger.Error("creating space job  error", "space_id", space.Id)
		return err
	}

	service.GetTransport().GossipSpace(space)
	client.MonitorJobState(space, nil)

	return nil
}

func (client *NomadClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	client.logger.Debug("deleting space job ,", "space_id", space.Id, "space", space.ContainerId)

	_, err := client.DeleteJob(space.ContainerId, space.NomadNamespace)
	if err != nil {
		client.logger.Debug("deleting space job , error:", "space_id", space.Id)
		return err
	}

	// Record stopping
	space.IsPending = true
	space.UpdatedAt = hlc.Now()

	db := database.GetInstance()
	err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	if err != nil {
		client.logger.Debug("deleting space job  error", "space_id", space.Id)
		return err
	}

	service.GetTransport().GossipSpace(space)
	client.MonitorJobState(space, onStopped)

	return nil
}

func (client *NomadClient) MonitorJobState(space *model.Space, onDone func()) {
	go func() {
		client.logger.Info("watching job  status for change", "nomad", space.ContainerId)

		for {
			code, data, err := client.ReadJob(space.ContainerId, space.NomadNamespace)
			if err != nil && code != 404 {
				client.logger.WithError(err).Error("reading space job error", "space", space.ContainerId)
			} else {
				if code == 404 {
					client.logger.Debug("reading space job status", "space", space.ContainerId, "reading", "404")
				} else {
					client.logger.Debug("reading space job status", "space", space.ContainerId)
				}

				if code == 200 && data["Status"] == "running" {
					// If waiting for job to start then done
					if space.IsPending && !space.IsDeployed {
						client.logger.Info("space job  is running", "space", space.ContainerId)

						space.IsPending = false
						space.IsDeployed = true
						break
					}
				} else if code == 404 || data["Status"] == "dead" {
					client.logger.Info("space job is dead", "space", space.ContainerId)

					space.IsPending = false
					space.IsDeployed = false
					break
				}
			}

			// Sleep for a bit
			time.Sleep(500 * time.Millisecond)
		}

		client.logger.Info("update space job  status", "space", space.ContainerId)
		space.UpdatedAt = hlc.Now()
		err := database.GetInstance().SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			client.logger.Error("updating space job  error", "space", space.ContainerId)
		}
		service.GetTransport().GossipSpace(space)

		if onDone != nil {
			onDone()
		}
	}()
}
