package util

import (
	"fmt"
	"reflect"
)

// CopyFields copies specified fields from src to dst using reflection.
func CopyFields(src, dst interface{}, fields []string) error {
	srcVal := reflect.ValueOf(src).Elem()
	dstVal := reflect.ValueOf(dst).Elem()

	if len(fields) == 0 {
		return fmt.Errorf("no fields specified for copying")
	}

	for _, field := range fields {
		srcField := srcVal.FieldByName(field)
		if !srcField.IsValid() {
			return fmt.Errorf("field %s not found in src", field)
		}

		dstField := dstVal.FieldByName(field)
		if !dstField.IsValid() {
			return fmt.Errorf("field %s not found in dst", field)
		}

		if dstField.CanSet() {
			dstField.Set(srcField)
		} else {
			return fmt.Errorf("field %s cannot be set in dst", field)
		}
	}

	return nil
}
