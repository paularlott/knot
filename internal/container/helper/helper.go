package helper

import (
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/rs/zerolog/log"
)

type Helper struct {
}

func NewContainerHelper() *Helper {
	return &Helper{}
}

func (h *Helper) CreateVolume(volume *model.Volume) error {
	db := database.GetInstance()

	variables, err := db.GetTemplateVars()
	if err != nil {
		return err
	}

	vars := model.FilterVars(variables)

	// Mark volume as started
	volume.Zone = config.Zone
	volume.Active = true

	// TODO Change this it look at the platform to use
	var containerClient container.ContainerManager
	if volume.Platform == model.PlatformDocker {
		containerClient = docker.NewClient()
	} else {
		var err error
		containerClient, err = nomad.NewClient()
		if err != nil {
			log.Error().Err(err).Msg("CreateVolume: failed to create nomad client")
			return err
		}
	}

	// Create volumes
	err = containerClient.CreateVolume(volume, &vars)
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

	// TODO Change this it look at the platform to use
	var containerClient container.ContainerManager
	if volume.Platform == model.PlatformDocker {
		containerClient = docker.NewClient()
	} else {
		var err error
		containerClient, err = nomad.NewClient()
		if err != nil {
			log.Error().Err(err).Msg("DeleteVolume: failed to create nomad client")
			return err
		}
	}

	// Delete the volume
	err = containerClient.DeleteVolume(volume, &vars)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		return err
	}

	return nil
}

func (h *Helper) StartSpace(space *model.Space, template *model.Template, user *model.User) error {
	db := database.GetInstance()

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = time.Now().UTC()
	if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.Error().Err(err).Msg("StartSpace")
		return err
	}

	service.GetTransport().GossipSpace(space)

	// Revert the pending status if the deploy fails
	var deployFailed = true
	defer func() {
		if deployFailed {
			// If the deploy failed then revert the space to not pending
			space.IsPending = false
			space.UpdatedAt = time.Now().UTC()
			db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
			service.GetTransport().GossipSpace(space)
		}
	}()

	// Get the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Err(err).Msg("StartSpace")
		return err
	}

	vars := model.FilterVars(variables)

	// TODO Change this it look at the platform to use
	var containerClient container.ContainerManager
	if template.IsLocalContainer() {
		containerClient = docker.NewClient()
	} else {
		var err error
		containerClient, err = nomad.NewClient()
		if err != nil {
			log.Error().Err(err).Msg("StartSpace: failed to create nomad client")
			return err
		}
	}

	// Create volumes
	err = containerClient.CreateSpaceVolumes(user, template, space, &vars)
	if err != nil {
		log.Error().Err(err).Msg("StartSpace")
		return err
	}

	// Start the job
	err = containerClient.CreateSpaceJob(user, template, space, &vars)
	if err != nil {
		log.Error().Err(err).Msg("StartSpace")
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
		log.Error().Msgf("StopSpace: failed to get template %s", err.Error())
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	space.UpdatedAt = time.Now().UTC()
	if err = db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		log.Error().Msgf("StopSpace: failed to save space %s", err.Error())
		return err
	}
	service.GetTransport().GossipSpace(space)

	// TODO Change this it look at the platform to use
	var containerClient container.ContainerManager
	if template.IsLocalContainer() {
		containerClient = docker.NewClient()
	} else {
		var err error
		containerClient, err = nomad.NewClient()
		if err != nil {
			log.Error().Msgf("StopSpace: failed to create nomad client %s", err.Error())
			return err
		}
	}

	// Stop the job
	err = containerClient.DeleteSpaceJob(space)
	if err != nil {
		space.IsPending = false
		space.UpdatedAt = time.Now().UTC()
		db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)

		log.Error().Msgf("StopSpace: failed to delete space %s", err.Error())
		return err
	}

	return nil
}

func (h *Helper) DeleteSpace(space *model.Space) {
	go func() {
		log.Info().Msgf("DeleteSpace: deleting %s", space.Id)

		db := database.GetInstance()

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			log.Error().Err(err).Msg("DeleteSpace: load template")

			space.IsDeleting = false
			space.UpdatedAt = time.Now().UTC()
			db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
			service.GetTransport().GossipSpace(space)
			return
		}

		// TODO Change this it look at the platform to use
		var containerClient container.ContainerManager
		if template.IsLocalContainer() {
			containerClient = docker.NewClient()
		} else {
			var err error
			containerClient, err = nomad.NewClient()
			if err != nil {
				log.Error().Err(err).Msg("DeleteSpace: failed to create nomad client")

				space.IsDeleting = false
				space.UpdatedAt = time.Now().UTC()
				db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
				service.GetTransport().GossipSpace(space)
				return
			}
		}

		// If the space is deployed, stop the job
		if space.IsDeployed {
			err = containerClient.DeleteSpaceJob(space)
			if err != nil {
				log.Error().Err(err).Msg("DeleteSpace: delete space job")
				space.IsDeleting = false
				space.UpdatedAt = time.Now().UTC()
				db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
				service.GetTransport().GossipSpace(space)
				return
			}
		}

		// Delete volumes on failure we log the error and revert the space to not deleting
		err = containerClient.DeleteSpaceVolumes(space)
		if err != nil {
			log.Error().Err(err).Msgf("DeleteSpace")

			space.IsDeleting = false
			space.UpdatedAt = time.Now().UTC()
			db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
			service.GetTransport().GossipSpace(space)
			return
		}

		// Delete the space
		space.IsDeleted = true
		space.Name = space.Id
		space.UpdatedAt = time.Now().UTC()
		err = db.SaveSpace(space, []string{"IsDeleted", "UpdatedAt", "Name"})
		if err != nil {
			log.Error().Err(err).Msg("DeleteSpace")
			return
		}

		service.GetTransport().GossipSpace(space)

		// Delete the agent state if present
		agent_server.RemoveSession(space.Id)

		log.Info().Msgf("DeleteSpace: deleted %s", space.Id)
	}()
}
