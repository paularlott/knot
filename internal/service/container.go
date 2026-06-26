package service

import "github.com/paularlott/knot/internal/database/model"

type Container interface {
	// Volumes
	CreateVolume(volume *model.Volume) error
	DeleteVolume(volume *model.Volume) error

	// Spaces. StopSpace/RestartSpace run the shutdown script and container
	// teardown synchronously and return any teardown error. The shutdown script
	// is bounded by a timeout (helper.ShutdownScriptTimeout) so a hung agent
	// script cannot block the caller indefinitely.
	StartSpace(space *model.Space, template *model.Template, user *model.User) error
	StopSpace(space *model.Space) error
	RestartSpace(space *model.Space) error
	DeleteSpace(space *model.Space)

	// Helpers
	CleanupOnBoot()
}

var containerService Container

func SetContainerService(service Container) {
	containerService = service
}

func GetContainerService() Container {
	return containerService
}
