package driver_mysql

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util"
)

func (db *MySQLDriver) create(tableName string, obj interface{}) error {
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	var columns []string
	var values []interface{}
	var placeholders []string

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("db")

		if tag != "" {
			tag = strings.Replace(tag, ",pk", "", -1)
			if strings.Contains(tag, ",json") {
				tag = strings.Replace(tag, ",json", "", -1)
				jsonValue, err := json.Marshal(val.Field(i).Interface())
				if err != nil {
					return err
				}
				values = append(values, string(jsonValue))
			} else if field.Type == reflect.PointerTo(reflect.TypeOf(time.Time{})) {
				timePtr := val.Field(i).Interface().(*time.Time)
				if timePtr == nil {
					values = append(values, nil)
				} else {
					values = append(values, timePtr.Format("2006-01-02 15:04:05"))
				}
			} else {
				values = append(values, val.Field(i).Interface())
			}
			columns = append(columns, tag)
			placeholders = append(placeholders, "?")
		}
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := db.connection.Exec(query, values...)
	return err
}

func (db *MySQLDriver) update(tableName string, obj interface{}, fieldsToUpdate []string) error {
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	var setClauses []string
	var values []interface{}
	var pkValue interface{}
	var pkColumn string

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("db")

		if tag != "" {
			if strings.Contains(tag, ",pk") {
				pkValue = val.Field(i).Interface()
				pkColumn = strings.Replace(tag, ",pk", "", -1)
			} else if len(fieldsToUpdate) == 0 || util.InArray(fieldsToUpdate, field.Name) {
				if strings.Contains(tag, ",json") {
					tag = strings.Replace(tag, ",json", "", -1)
					jsonValue, err := json.Marshal(val.Field(i).Interface())
					if err != nil {
						return err
					}
					values = append(values, string(jsonValue))
				} else if field.Type == reflect.PointerTo(reflect.TypeOf(time.Time{})) {
					timePtr := val.Field(i).Interface().(*time.Time)
					if timePtr == nil {
						values = append(values, nil)
					} else {
						values = append(values, timePtr.Format("2006-01-02 15:04:05.000000"))
					}
				} else {
					fieldValue := val.Field(i).Interface()
					// Check if the field implements the driver.Valuer interface
					if valuer, ok := fieldValue.(driver.Valuer); ok {
						value, err := valuer.Value()
						if err != nil {
							return err
						}
						values = append(values, value)
					} else {
						values = append(values, fieldValue)
					}
				}
				setClauses = append(setClauses, fmt.Sprintf("%s = ?", tag))
			}
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	if pkColumn == "" {
		return fmt.Errorf("no primary key field found")
	}

	values = append(values, pkValue)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?",
		tableName,
		strings.Join(setClauses, ", "),
		pkColumn)

	_, err := db.connection.Exec(query, values...)
	return err
}

func (db *MySQLDriver) read(tableName string, results interface{}, fieldsToLoad []string, where string, args ...interface{}) error {
	resultsType := reflect.TypeOf(results)
	if resultsType.Kind() != reflect.Ptr || resultsType.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("results must be a pointer to a slice")
	}

	var isPtr bool = false
	objType := resultsType.Elem().Elem()

	var columns []string
	var fieldNames []string
	var jsonFields map[int]string = make(map[int]string)

	if len(fieldsToLoad) == 0 {
		var numFields int
		switch objType.Kind() {
		case reflect.Struct:
			numFields = objType.NumField()
		case reflect.Ptr:
			isPtr = true
			if objType.Elem().Kind() == reflect.Struct {
				numFields = objType.Elem().NumField()
				objType = objType.Elem() // Use the underlying struct type
			}
		}

		for i := 0; i < numFields; i++ {
			field := objType.Field(i)
			tag := field.Tag.Get("db")
			if tag != "" {
				if strings.Contains(tag, ",json") {
					tag = strings.Replace(tag, ",json", "", -1)
					jsonFields[len(columns)] = field.Name
				}
				columns = append(columns, strings.Replace(tag, ",pk", "", -1))
				fieldNames = append(fieldNames, field.Name)
			}
		}
	} else {
		if objType.Kind() == reflect.Ptr {
			objType = objType.Elem()
			isPtr = true
		}
		for _, fieldName := range fieldsToLoad {
			field, found := objType.FieldByName(fieldName)
			if found {
				tag := field.Tag.Get("db")
				if tag != "" {
					if strings.Contains(tag, ",json") {
						tag = strings.Replace(tag, ",json", "", -1)
						jsonFields[len(columns)] = fieldName
					}
					columns = append(columns, strings.Replace(tag, ",pk", "", -1))
					fieldNames = append(fieldNames, fieldName)
				}
			}
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(columns, ", "), tableName, where)
	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		obj := reflect.New(objType).Elem()
		columnPointers := make([]interface{}, len(columns))
		tempValues := make([]interface{}, len(columns))

		for i := range columnPointers {
			field := obj.FieldByName(fieldNames[i])
			if field.IsValid() {
				if _, ok := jsonFields[i]; ok {
					tempValues[i] = new(string)
					columnPointers[i] = tempValues[i]
				} else if field.Type() == reflect.TypeOf(time.Time{}) || field.Type() == reflect.PointerTo(reflect.TypeOf(time.Time{})) {
					tempValues[i] = new([]uint8)
					columnPointers[i] = tempValues[i]
				} else {
					columnPointers[i] = field.Addr().Interface()
				}
			} else {
				var dummy interface{}
				columnPointers[i] = &dummy
			}
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return err
		}

		// JSON decode fields if necessary
		for idx, fieldName := range jsonFields {
			field := obj.FieldByName(fieldName)
			if field.IsValid() {
				jsonStr := *(tempValues[idx].(*string))
				if err := json.Unmarshal([]byte(jsonStr), field.Addr().Interface()); err != nil {
					return err
				}
			}
		}

		// Convert []uint8 to time.Time or *time.Time if necessary
		for i, field := range fieldNames {
			fieldValue := obj.FieldByName(field)
			if fieldValue.Type() == reflect.TypeOf(time.Time{}) {
				timeBytes := *(tempValues[i].(*[]uint8))
				parsedTime, err := time.Parse("2006-01-02 15:04:05", string(timeBytes))
				if err != nil {
					return err
				}
				fieldValue.Set(reflect.ValueOf(parsedTime))
			} else if fieldValue.Type() == reflect.PointerTo(reflect.TypeOf(time.Time{})) {
				timeBytes := *(tempValues[i].(*[]uint8))
				if len(timeBytes) == 0 {
					fieldValue.Set(reflect.Zero(fieldValue.Type()))
				} else {
					parsedTime, err := time.Parse("2006-01-02 15:04:05", string(timeBytes))
					if err != nil {
						return err
					}
					fieldValue.Set(reflect.ValueOf(&parsedTime))
				}
			}
		}

		// Check if the result type is a pointer
		if isPtr {
			reflect.ValueOf(results).Elem().Set(reflect.Append(reflect.ValueOf(results).Elem(), obj.Addr()))
		} else {
			reflect.ValueOf(results).Elem().Set(reflect.Append(reflect.ValueOf(results).Elem(), obj))
		}
	}

	return nil
}

func (db *MySQLDriver) delete(tableName string, obj interface{}) error {
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	var pkValue interface{}
	var pkColumn string

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("db")
		if strings.Contains(tag, ",pk") {
			pkValue = val.Field(i).Interface()
			pkColumn = strings.Replace(tag, ",pk", "", -1)
			break
		}
	}

	if pkColumn == "" {
		return fmt.Errorf("no primary key field found")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableName, pkColumn)
	_, err := db.connection.Exec(query, pkValue)
	return err
}
