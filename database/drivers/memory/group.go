package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveGroup(group *model.Group) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteGroup(group *model.Group) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetGroup(id string) (*model.Group, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetGroups() ([]*model.Group, error) {
	return nil, errors.New("memorydb: not implemented")
}
