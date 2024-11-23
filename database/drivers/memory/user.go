package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveUser(user *model.User) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteUser(user *model.User) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetUser(id string) (*model.User, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetUserByEmail(email string) (*model.User, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetUserByUsername(name string) (*model.User, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetUsers() ([]*model.User, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) HasUsers() (bool, error) {
	return false, errors.New("memorydb: not implemented")
}
