package helper

import (
	"fmt"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/apple"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/container/podman"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"

	"github.com/paularlott/knot/internal/log"
)

type Helper struct {
}

func NewContainerHelper() *Helper {
	return &Helper{}
}

func (h *Helper) createClient(platform string) (container.ContainerManager, error) {
	// Map "container" to detected runtime
	if platform == model.PlatformContainer {
		cfg := config.GetServerConfig()
		platform = cfg.LocalContainerRuntime
		if platform == "" {
			return nil, fmt.Errorf("no local container runtime detected")
		}
	}

	switch platform {
	case model.PlatformDocker:
		return docker.NewClient(), nil
	case model.PlatformPodman:
		return podman.NewClient(), nil
	case model.PlatformNomad:
		return nomad.NewClient()
	case model.PlatformApple:
		return apple.NewClient(), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

func (h *Helper) CreateVolume(volume *model.Volume) error {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	variables, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	vars := model.FilterVars(variables)

	// Mark volume as started
	volume.Zone = cfg.Zone
	volume.Active = true

	containerClient, err := h.createClient(volume.Platform)
	if err != nil {
		log.WithError(err).Error("CreateVolume: failed to create container client")
		return err
	}

	// Create volumes
	err = containerClient.CreateVolume(volume, vars)
	if err != nil {
		return err
	}

	return nil
}

func (h *Helper) DeleteVolume(volume *model.Volume) error {
	db := database.GetInstance()

	variables, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	vars := model.FilterVars(variables)

	// Record the volume as not deployed
	volume.Zone = ""
	volume.Active = false

	containerClient, err := h.createClient(volume.Platform)
	if err != nil {
		log.WithError(err).Error("DeleteVolume: failed to create container client")
		return err
	}

	// Delete the volume
	err = containerClient.DeleteVolume(volume, vars)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		return err
	}

	return nil
}

func (h *Helper) StartSpace(space *model.Space, template *model.Template, user *model.User) error {
	db := database.GetInstance()

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// Revert the pending status if the deploy fails
	var deployFailed = true
	defer func() {
		if deployFailed {
			// If the deploy failed then revert the space to not pending
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
			service.GetTransport().GossipSpace(space)
			sse.PublishSpaceChanged(space.Id, space.UserId)
		}
	}()

	// Get the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	vars := model.FilterVars(variables)

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("StartSpace: failed to create container client")
		return err
	}

	// Create volumes
	err = containerClient.CreateSpaceVolumes(user, template, space, vars)
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	// Start the job
	err = containerClient.CreateSpaceJob(user, template, space, vars)
	if err != nil {
		log.WithError(err).Error("StartSpace")
		return err
	}

	// Don't revert the space on success
	deployFailed = false

	return nil
}

func (h *Helper) StopSpace(space *model.Space) error {
	db := database.GetInstance()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.WithError(err).Error("StopSpace: failed to get template")
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("StopSpace: failed to save space")
		return err
	}
	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("StopSpace: failed to create container client")
		return err
	}

	// Stop the job
	err = containerClient.DeleteSpaceJob(space, nil)
	if err != nil {
		space.IsPending = false
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceChanged(space.Id, space.UserId)

		log.WithError(err).Error("StopSpace: failed to delete space")
		return err
	}

	return nil
}

func (h *Helper) RestartSpace(space *model.Space) error {
	db := database.GetInstance()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to get template")
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("RestartSpace: failed to save space")
		return err
	}
	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// Get the user from the space
	user, err := db.GetUser(space.UserId)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to get user")
		return err
	}

	containerClient, err := h.createClient(template.Platform)
	if err != nil {
		log.WithError(err).Error("RestartSpace: failed to create container client")
		return err
	}

	// Stop the job
	err = containerClient.DeleteSpaceJob(space, func() {
		// Start the container again
		h.StartSpace(space, template, user)
	})
	if err != nil {
		space.IsPending = false
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceChanged(space.Id, space.UserId)

		log.WithError(err).Error("RestartSpace: failed to delete space")
		return err
	}

	return nil
}

func (h *Helper) DeleteSpace(space *model.Space) {
	go func() {
		logger := log.WithGroup("server")
		logger.Info("deleting space", "space_id", space.Id)

		db := database.GetInstance()

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			logger.WithError(err).Error("load template")

			space.IsDeleting = false
			space.UpdatedAt = hlc.Now()
			db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
			service.GetTransport().GossipSpace(space)
			sse.PublishSpaceChanged(space.Id, space.UserId)
			return
		}

		// If not a manual space then we have to do additional checks and clean up
		if !template.IsManual() {
			containerClient, err := h.createClient(template.Platform)
			if err != nil {
				logger.WithError(err).Error("failed to create container client")

				space.IsDeleting = false
				space.UpdatedAt = hlc.Now()
				db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
				service.GetTransport().GossipSpace(space)
				sse.PublishSpaceChanged(space.Id, space.UserId)
				return
			}

			// If the space is deployed, stop the job
			if space.IsDeployed {
				err = containerClient.DeleteSpaceJob(space, nil)
				if err != nil {
					logger.WithError(err).Error("delete space job")
					space.IsDeleting = false
					space.UpdatedAt = hlc.Now()
					db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
					service.GetTransport().GossipSpace(space)
					sse.PublishSpaceChanged(space.Id, space.UserId)
					return
				}
			}

			// Delete volumes on failure we log the error and revert the space to not deleting
			err = containerClient.DeleteSpaceVolumes(space)
			if err != nil {
				logger.WithError(err).Error("delete space volumes")

				space.IsDeleting = false
				space.UpdatedAt = hlc.Now()
				db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
				service.GetTransport().GossipSpace(space)
				sse.PublishSpaceChanged(space.Id, space.UserId)
				return
			}
		}

		// Delete the space
		space.IsDeleted = true
		space.Name = space.Id
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"IsDeleted", "UpdatedAt", "Name"})
		if err != nil {
			logger.WithError(err).Error("delete space")
			return
		}

		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceDeleted(space.Id, space.UserId)

		// Delete the agent state if present
		agent_server.RemoveSession(space.Id)

		logger.Info("deleted space", "space_id", space.Id)
	}()
}

// Clean up spaces in broken states during boot
func (h *Helper) CleanupOnBoot() {
	logger := log.WithGroup("server")
	logger.Info("cleaning spaces...")

	db := database.GetInstance()
	cfg := config.GetServerConfig()
	spaces, err := db.GetSpaces()
	if err != nil {
		logger.WithError(err).Fatal("failed to get spaces")
	} else {
		for _, space := range spaces {
			// If space is deleted or not in this zone then ignore it
			if space.IsDeleted || space.Zone != cfg.Zone {
				continue
			}

			// If the space is deleting then ask it to delete again
			if space.IsDeleting {
				logger.Info("found space  pending delete, restarting delete...", "space_name", space.Name)
				h.DeleteSpace(space)
			} else if space.IsPending {
				// If starting then ask for start
				if !space.IsDeployed {
					logger.Info("found space  pending start, starting...", "space_name", space.Name)

					user, err := db.GetUser(space.UserId)
					if err != nil {
						logger.WithError(err).Error("failed to get user from space, stopping the space...")
						space.IsDeployed = true
						h.StopSpace(space)
						continue
					}

					template, err := db.GetTemplate(space.TemplateId)
					if err != nil {
						logger.WithError(err).Error("failed to get template from space, stopping the space...")
						space.IsDeployed = true
						h.StopSpace(space)
						continue
					}

					h.StartSpace(space, template, user)
				} else {
					logger.Info("found space  pending stop, stopping...", "space_name", space.Name)
					h.StopSpace(space)
				}
			}
		}
	}

	logger.Info("finished cleaning spaces...")
}
