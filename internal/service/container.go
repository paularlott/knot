package service

import "github.com/paularlott/knot/database/model"

type Container interface {
	// Volumes
	CreateVolume(volume *model.Volume) error
	DeleteVolume(volume *model.Volume) error
}

var containerService Container

func SetContainerService(service Container) {
	containerService = service
}

func GetContainerService() Container {
	return containerService
}
