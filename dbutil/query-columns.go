package dbutil

import (
	"reflect"

	"github.com/juju/errors"
)

// QueryCol fetches a single column from the query result.
// dest must be a pointer to slice of column type.
func QueryCol(db Querier, dest interface{}, query string, args ...interface{}) error {
	destType := reflect.TypeOf(dest)
	if destType.Kind() != reflect.Ptr || destType.Elem().Kind() != reflect.Slice {
		return errors.Errorf("Invalid type of dest: %T (slice pointer required)", dest)
	}

	aArgs := make(Args, len(args))
	for i, a := range args {
		aArgs[i] = a
	}

	destValue := reflect.ValueOf(dest)
	destSlice := reflect.Indirect(destValue)
	elemType := destType.Elem().Elem()

	err := QueryRows(db, query, aArgs, func(s RowScanner) error {
		el := reflect.New(elemType)
		if err := s.Scan(el.Interface()); err != nil {
			return err
		}
		destSlice = reflect.Append(destSlice, reflect.Indirect(el))
		return nil
	})
	if err != nil {
		return err
	}

	reflect.Indirect(destValue).Set(destSlice)
	return nil
}

// QueryCols fetches all columns from the query result.
// dest must be a pointer to slice of structs with fields
// ordered and typed as columns.
func QueryCols(db Querier, dest interface{}, query string, args ...interface{}) error {
	destType := reflect.TypeOf(dest)
	if destType.Kind() != reflect.Ptr ||
		destType.Elem().Kind() != reflect.Slice ||
		destType.Elem().Elem().Kind() != reflect.Struct {
		return errors.Errorf("Invalid type of dest: %T (slice of structs pointer required)", dest)
	}

	aArgs := make(Args, len(args))
	for i, a := range args {
		aArgs[i] = a
	}

	destValue := reflect.ValueOf(dest)
	destSlice := reflect.Indirect(destValue)
	elemType := destType.Elem().Elem()

	err := QueryRows(db, query, aArgs, func(s RowScanner) error {
		el := reflect.New(elemType)
		as := make(Args, elemType.NumField())

		for i := range as {
			as[i] = reflect.Indirect(el).Field(i).Addr().Interface()
		}

		if err := s.Scan(as...); err != nil {
			return err
		}
		destSlice = reflect.Append(destSlice, el.Elem())
		return nil
	})
	if err != nil {
		return err
	}

	reflect.Indirect(destValue).Set(destSlice)
	return nil
}
