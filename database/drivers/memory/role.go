package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveRole(role *model.Role) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteRole(role *model.Role) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetRole(id string) (*model.Role, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetRoles() ([]*model.Role, error) {
	return nil, errors.New("memorydb: not implemented")
}
