package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type JSONDbArray []string

func (m *JSONDbArray) Scan(src interface{}) error {
	var source []byte
	_m := make([]string, 0)

	switch src := src.(type) {
	case []uint8:
		source = []byte(src)
		err := json.Unmarshal(source, &_m)
		if err != nil {
			return err
		}
	case nil:

	default:
		return errors.New("incompatible type for JSONDbArray")
	}

	*m = JSONDbArray(_m)
	return nil
}

func (m JSONDbArray) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	j, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return driver.Value([]byte(j)), nil
}

type JSONDbScheduleDays []TemplateScheduleDays

func (m *JSONDbScheduleDays) Scan(src interface{}) error {
	var source []byte
	_m := make([]TemplateScheduleDays, 0)

	switch src := src.(type) {
	case []uint8:
		source = []byte(src)
		err := json.Unmarshal(source, &_m)
		if err != nil {
			return err
		}
	case nil:

	default:
		return errors.New("incompatible type for JSONDbScheduleDays")
	}

	*m = JSONDbScheduleDays(_m)
	return nil
}

func (m JSONDbScheduleDays) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	j, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return driver.Value([]byte(j)), nil
}
