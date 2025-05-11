package helper

import (
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
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
	volume.Location = config.Location
	volume.Active = true

	var containerClient container.ContainerManager
	if volume.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
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
	volume.Location = ""
	volume.Active = false

	var containerClient container.ContainerManager
	if volume.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
	}

	// Delete the volume
	err = containerClient.DeleteVolume(volume, &vars)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		return err
	}

	return nil
}
