package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
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
		return errors.New("incompatible type for JSONDbArray")
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

type NullTime struct {
	*time.Time
}

func (nt *NullTime) Scan(value interface{}) error {
	if value == nil {
		nt.Time = nil
		return nil
	}

	var t time.Time

	switch v := value.(type) {
	case time.Time:
		t = v
	case []byte:
		var err error
		t, err = time.Parse("2006-01-02 15:04:05", string(v))
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported type for NullTime")
	}

	nt.Time = &t
	return nil
}

func (nt NullTime) Value() (driver.Value, error) {
	if nt.Time == nil {
		return nil, nil
	}
	return *nt.Time, nil
}
