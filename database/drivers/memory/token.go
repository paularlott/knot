package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveToken(token *model.Token) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteToken(token *model.Token) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetToken(id string) (*model.Token, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	return nil, errors.New("memorydb: not implemented")
}
