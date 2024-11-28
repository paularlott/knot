package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveTemplateVar(templateVar *model.TemplateVar) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteTemplateVar(templateVar *model.TemplateVar) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
	return nil, errors.New("memorydb: not implemented")
}
