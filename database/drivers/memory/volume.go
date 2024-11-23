package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveVolume(volume *model.Volume) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteVolume(volume *model.Volume) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetVolume(id string) (*model.Volume, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetVolumes() ([]*model.Volume, error) {
	return nil, errors.New("memorydb: not implemented")
}
