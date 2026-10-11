package driver_postgres

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util"
)

var (
	timeType    = reflect.TypeOf(time.Time{})
	timePtrType = reflect.PointerTo(timeType)
)

// quoteIdent quotes a table or column name, so names that are reserved words
// (e.g. groups) are always accepted.
func quoteIdent(name string) string {
	return `"` + name + `"`
}

type executor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// columnValue converts a struct field to the value stored in its column:
// JSON columns are marshalled and times are always stored in UTC.
func columnValue(field reflect.StructField, value reflect.Value, isJSON bool) (interface{}, error) {
	if isJSON {
		jsonValue, err := json.Marshal(value.Interface())
		if err != nil {
			return nil, err
		}
		return string(jsonValue), nil
	}

	switch field.Type {
	case timeType:
		return value.Interface().(time.Time).UTC(), nil
	case timePtrType:
		timePtr := value.Interface().(*time.Time)
		if timePtr == nil {
			return nil, nil
		}
		return timePtr.UTC(), nil
	}

	fieldValue := value.Interface()
	if valuer, ok := fieldValue.(driver.Valuer); ok {
		return valuer.Value()
	}
	return fieldValue, nil
}

func (db *PostgresDriver) create(tableName string, obj interface{}) error {
	return db.createWithExecutor(db.connection, tableName, obj)
}

func (db *PostgresDriver) createWithExecutor(executor executor, tableName string, obj interface{}) error {
	val := reflect.ValueOf(obj).Elem()
	typ := val.Type()

	var columns []string
	var values []interface{}
	var placeholders []string

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("db")

		if tag != "" {
			// A zero integer key is left for the column's sequence to fill.
			if strings.Contains(tag, ",pk") && field.Type.Kind() == reflect.Int64 && val.Field(i).Int() == 0 {
				continue
			}

			tag = strings.Replace(tag, ",pk", "", -1)
			isJSON := strings.Contains(tag, ",json")
			tag = strings.Replace(tag, ",json", "", -1)

			value, err := columnValue(field, val.Field(i), isJSON)
			if err != nil {
				return err
			}
			values = append(values, value)
			columns = append(columns, quoteIdent(tag))
			placeholders = append(placeholders, "?")
		}
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := executor.Exec(query, values...)
	return err
}

func (db *PostgresDriver) update(tableName string, obj interface{}, fieldsToUpdate []string) error {
	return db.updateWithExecutor(db.connection, tableName, obj, fieldsToUpdate)
}

func (db *PostgresDriver) updateWithExecutor(executor executor, tableName string, obj interface{}, fieldsToUpdate []string) error {
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
				isJSON := strings.Contains(tag, ",json")
				tag = strings.Replace(tag, ",json", "", -1)

				value, err := columnValue(field, val.Field(i), isJSON)
				if err != nil {
					return err
				}
				values = append(values, value)
				setClauses = append(setClauses, fmt.Sprintf("%s = ?", quoteIdent(tag)))
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
		quoteIdent(tableName),
		strings.Join(setClauses, ", "),
		quoteIdent(pkColumn))

	_, err := executor.Exec(query, values...)
	return err
}

func (db *PostgresDriver) read(tableName string, results interface{}, fieldsToLoad []string, where string, args ...interface{}) error {
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
				columns = append(columns, quoteIdent(strings.Replace(tag, ",pk", "", -1)))
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
					columns = append(columns, quoteIdent(strings.Replace(tag, ",pk", "", -1)))
					fieldNames = append(fieldNames, fieldName)
				}
			}
		}
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(columns, ", "), quoteIdent(tableName), where)
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
					tempValues[i] = new(sql.NullString)
					columnPointers[i] = tempValues[i]
				} else if field.Type() == timeType || field.Type() == timePtrType {
					tempValues[i] = new(sql.NullTime)
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
				str := tempValues[idx].(*sql.NullString)
				if str.Valid && len(str.String) > 0 {
					if err := json.Unmarshal([]byte(str.String), field.Addr().Interface()); err != nil {
						return err
					}
				}
			}
		}

		// Set the time fields, always in UTC
		for i, field := range fieldNames {
			fieldValue := obj.FieldByName(field)
			switch fieldValue.Type() {
			case timeType:
				if t := tempValues[i].(*sql.NullTime); t.Valid {
					fieldValue.Set(reflect.ValueOf(t.Time.UTC()))
				}
			case timePtrType:
				if t := tempValues[i].(*sql.NullTime); t.Valid {
					utc := t.Time.UTC()
					fieldValue.Set(reflect.ValueOf(&utc))
				} else {
					fieldValue.Set(reflect.Zero(fieldValue.Type()))
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

	return rows.Err()
}

// zonesLength orders rows by how many zones they're limited to. A row saved
// with no zones may hold JSON null rather than an empty array.
const zonesLength = "jsonb_array_length(CASE jsonb_typeof(zones) WHEN 'array' THEN zones ELSE '[]'::jsonb END)"
