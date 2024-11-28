package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveSpace(space *model.Space) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteSpace(space *model.Space) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetSpace(id string) (*model.Space, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetSpaces() ([]*model.Space, error) {
	return nil, errors.New("memorydb: not implemented")
}
