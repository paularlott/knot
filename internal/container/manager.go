package container

import "github.com/paularlott/knot/database/model"

type ContainerManager interface {
	// space management
	CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error
	DeleteSpaceJob(space *model.Space) error
	CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error
	DeleteSpaceVolumes(space *model.Space) error

	// volume management
	CreateVolume(vol *model.Volume, variables *map[string]interface{}) error
	DeleteVolume(vol *model.Volume, variables *map[string]interface{}) error
}
