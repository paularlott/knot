package nomad

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

const jobMonitorTimeout = 30 * time.Minute

// volumeDeleteJobWaitTimeout is the maximum time to wait for a Nomad job to
// reach a terminal state before attempting volume deletion. Nomad will not
// release volume claims until all allocations have terminated.
const volumeDeleteJobWaitTimeout = 5 * time.Minute

func (client *NomadClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	db := database.GetInstance()

	volumes, err := template.GetVolumes(space, user, variables)
	if err != nil {
		return err
	}
	storage, err := model.LoadManagedPathsFromYaml(template.Volumes, template, space, user, variables)
	if err != nil {
		return err
	}

	if len(volumes.Volumes) == 0 && len(storage.Paths) == 0 && len(space.VolumeData) == 0 {
		client.logger.Debug("no volumes to create")
		return nil
	}

	// Store initial volume data to detect changes
	initialVolumeData := make(map[string]model.SpaceVolume)
	for k, v := range space.VolumeData {
		initialVolumeData[k] = v
	}

	defer func() {
		// Only save and publish if volumes actually changed
		volumesChanged := false
		if len(initialVolumeData) != len(space.VolumeData) {
			volumesChanged = true
		} else {
			for k, v := range space.VolumeData {
				if initialV, ok := initialVolumeData[k]; !ok || v != initialV {
					volumesChanged = true
					break
				}
			}
		}

		if volumesChanged {
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"}); err != nil {
				client.logger.Error("saving space  error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			sse.PublishSpaceChanged(space.Id, space.UserId)
		}
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
					id, lookupErr := client.GetIdHostVolume(data.Id, data.Namespace)
					if lookupErr != nil {
						client.logger.WithError(lookupErr).Warn("failed to find volume for namespace cleanup", "volume_id", volume.Id)
					} else if delErr := client.DeleteHostVolume(id, data.Namespace); delErr != nil {
						client.logger.WithError(delErr).Warn("failed to delete volume for namespace cleanup", "volume_id", volume.Id)
					}
				} else {
					if delErr := client.DeleteCSIVolume(data.Id, data.Namespace); delErr != nil {
						client.logger.WithError(delErr).Warn("failed to delete volume for namespace cleanup", "volume_id", volume.Id)
					}
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

	requiredPaths := make(map[string]bool)
	for _, path := range storage.Paths {
		client.logger.Debug("checking path", "path", path)
		requiredPaths[path] = true
		data, ok := space.VolumeData[path]
		if !ok || data.Type != container.ManagedPathType {
			client.logger.Debug("creating path", "path", path)
			resolved, err := container.CreateManagedPath(path)
			if err != nil {
				return err
			}
			space.VolumeData[path] = model.SpaceVolume{
				Id:        resolved,
				Namespace: "_path",
				Type:      container.ManagedPathType,
			}
		} else {
			resolved, err := container.ResolveManagedPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); os.IsNotExist(err) {
				client.logger.Debug("recreating missing path", "path", path)
				if err := os.MkdirAll(resolved, 0755); err != nil {
					return err
				}
			}
		}
	}

	// Find the volumes deployed in the space but no longer in the template definition and remove them
	var cleanupErr error
	for key, volume := range space.VolumeData {
		if volume.Type == container.ManagedPathType {
			if requiredPaths[key] {
				continue
			}
			client.logger.Debug("deleting path", "path", key)
			if err := container.DeleteManagedPath(volume.Id); err != nil {
				client.logger.WithError(err).Error("deleting managed path", "path", key)
				if cleanupErr == nil {
					cleanupErr = err
				}
				continue
			}
			delete(space.VolumeData, key)
			continue
		}

		// Check if the volume is defined in the template
		if _, ok := volById[volume.Id]; !ok {
			var delErr error
			if volume.Type == "host" {
				id, lookupErr := client.GetIdHostVolume(volume.Id, volume.Namespace)
				if lookupErr != nil {
					if strings.Contains(lookupErr.Error(), "not found") {
						client.logger.Debug("host volume already gone", "volume_id", volume.Id)
						delete(space.VolumeData, key)
						continue
					}
					delErr = lookupErr
				} else {
					delErr = client.DeleteHostVolume(id, volume.Namespace)
				}
			} else {
				delErr = client.DeleteCSIVolume(volume.Id, volume.Namespace)
			}

			if delErr != nil {
				client.logger.WithError(delErr).Error("deleting volume", "volume_id", volume.Id)
				if cleanupErr == nil {
					cleanupErr = delErr
				}
				continue
			}

			delete(space.VolumeData, key)
		}
	}

	client.logger.Debug("volumes checked")

	return cleanupErr
}

// waitForJobStopped polls the Nomad job until it reaches a terminal state
// (dead or not found) or the timeout expires. Nomad does not release volume
// claims until all allocations have terminated, so callers must wait before
// attempting volume deletion.
func (client *NomadClient) waitForJobStopped(jobId, namespace string) {
	client.logger.Debug("waiting for job to stop before volume deletion", "job_id", jobId)

	ctx, cancel := context.WithTimeout(context.Background(), volumeDeleteJobWaitTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			client.logger.Warn("timed out waiting for job to stop before volume deletion", "job_id", jobId)
			return
		default:
		}

		code, data, err := client.ReadJob(ctx, jobId, namespace)
		if code == 404 || (err == nil && data["Status"] == "dead") {
			client.logger.Debug("job stopped, proceeding with volume deletion", "job_id", jobId)
			return
		}
		if err != nil {
			client.logger.WithError(err).Warn("error checking job status before volume deletion", "job_id", jobId)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func (client *NomadClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	client.logger.Debug("deleting volumes")

	if len(space.VolumeData) == 0 {
		client.logger.Debug("no volumes to delete")
		return nil
	}

	// Wait for the Nomad job to reach a terminal state before deleting volumes.
	// Nomad holds volume claims open until allocations terminate, so deleting
	// while the job is still shutting down will fail.
	if space.ContainerId != "" {
		client.waitForJobStopped(space.ContainerId, space.NomadNamespace)
	}

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}()

	// Delete all volumes. Continue past errors so one failure doesn't prevent
	// the remaining volumes from being cleaned up. Entries are only removed
	// from VolumeData when the volume is actually gone (or already gone), so
	// that failed deletions can be retried later.
	var firstErr error
	for key, volume := range space.VolumeData {
		if volume.Type == container.ManagedPathType {
			client.logger.Debug("deleting path", "path", key)
			if err := container.DeleteManagedPath(volume.Id); err != nil {
				client.logger.WithError(err).Error("deleting managed path", "path", key)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			delete(space.VolumeData, key)
			continue
		}

		var err error
		if volume.Type == "host" {
			var id string
			id, err = client.GetIdHostVolume(volume.Id, volume.Namespace)
			if err != nil {
				// Volume not found in Nomad — already deleted, remove from tracking.
				if strings.Contains(err.Error(), "not found") {
					client.logger.Debug("host volume already gone", "volume_id", volume.Id)
					delete(space.VolumeData, key)
					continue
				}
			} else {
				err = client.DeleteHostVolume(id, volume.Namespace)
			}
		} else {
			err = client.DeleteCSIVolume(volume.Id, volume.Namespace)
		}

		if err != nil {
			client.logger.WithError(err).Error("deleting volume", "volume_id", volume.Id)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		delete(space.VolumeData, key)
	}

	client.logger.Debug("volumes deleted")

	return firstErr
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

	// Inject port env vars from template into all task groups
	portEnvs := container.BuildPortEnvVars(template)
	if len(portEnvs) > 0 {
		injectNomadEnvVars(jobJSON, portEnvs)
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

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)
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

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)
	client.MonitorJobState(space, onStopped)

	return nil
}

func (client *NomadClient) CleanupSpaceArtifacts(space *model.Space) error {
	for key, volume := range space.VolumeData {
		if volume.Type != container.ManagedPathType {
			continue
		}
		client.logger.Debug("cleaning migrated space path", "space_id", space.Id, "path", key)
		if err := container.DeleteManagedPath(volume.Id); err != nil {
			return err
		}
	}

	return nil
}

func (client *NomadClient) StopSpaceRuntime(space *model.Space) error {
	if space.ContainerId == "" {
		return nil
	}

	_, err := client.DeleteJob(space.ContainerId, space.NomadNamespace)
	return err
}

func (client *NomadClient) ListRunningSpaceRuntimeRefs(namespaces []string) (map[string]bool, error) {
	refs := make(map[string]bool)
	seenNamespaces := make(map[string]bool)

	for _, namespace := range namespaces {
		if namespace == "" {
			namespace = "default"
		}
		if seenNamespaces[namespace] {
			continue
		}
		seenNamespaces[namespace] = true

		jobs, err := client.ListJobs(context.Background(), namespace)
		if err != nil {
			return nil, err
		}

		for _, job := range jobs {
			id, _ := job["ID"].(string)
			status, _ := job["Status"].(string)
			if id == "" || status != "running" {
				continue
			}
			refs[namespace+"\x00"+id] = true
		}
	}

	return refs, nil
}

func (client *NomadClient) MonitorJobState(space *model.Space, onDone func()) {
	go func() {
		client.logger.Info("watching job  status for change", "nomad", space.ContainerId)

		// Job startup can include large image pulls on the client, so keep watching longer.
		ctx, cancel := context.WithTimeout(context.Background(), jobMonitorTimeout)
		defer cancel()
		oldSpace := *space

		for {
			// Check if context has been cancelled
			select {
			case <-ctx.Done():
				client.logger.Warn("job monitoring cancelled due to timeout", "space_id", space.Id, "nomad_job", space.ContainerId, "timeout", jobMonitorTimeout)
				return
			default:
			}

			code, data, err := client.ReadJob(ctx, space.ContainerId, space.NomadNamespace)
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
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceChanged(space.Id, space.UserId)
		service.CheckSpaceLifecycleEvents(&oldSpace, space)

		if onDone != nil {
			onDone()
		}
	}()
}

func injectNomadEnvVars(jobJSON map[string]interface{}, envVars []string) {
	taskGroups, ok := jobJSON["TaskGroups"].([]interface{})
	if !ok {
		return
	}
	for _, tg := range taskGroups {
		tgMap, ok := tg.(map[string]interface{})
		if !ok {
			continue
		}
		tasks, ok := tgMap["Tasks"].([]interface{})
		if !ok {
			continue
		}
		for _, t := range tasks {
			taskMap, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			env, ok := taskMap["Env"].(map[string]interface{})
			if !ok {
				env = make(map[string]interface{})
			}
			for _, ev := range envVars {
				parts := strings.SplitN(ev, "=", 2)
				if len(parts) == 2 {
					env[parts[0]] = parts[1]
				}
			}
			taskMap["Env"] = env
		}
	}
}
