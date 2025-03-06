package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type JSONDbUIntArray []uint16

func (m *JSONDbUIntArray) Scan(src interface{}) error {
	var source []byte
	_m := make([]uint16, 0)

	switch src := src.(type) {
	case []uint8:
		source = []byte(src)
		err := json.Unmarshal(source, &_m)
		if err != nil {
			return err
		}
	case nil:

	default:
		return errors.New("incompatible type for JSONDbUIntArray")
	}

	*m = JSONDbUIntArray(_m)
	return nil
}

func (m JSONDbUIntArray) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	j, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return driver.Value([]byte(j)), nil
}
