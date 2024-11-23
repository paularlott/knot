package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveTemplate(template *model.Template) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) DeleteTemplate(template *model.Template) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetTemplate(id string) (*model.Template, error) {
	return nil, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetTemplates() ([]*model.Template, error) {
	return nil, errors.New("memorydb: not implemented")
}
