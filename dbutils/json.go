package dbutils

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// ScanValuer interface combines sql.Scanner and driver.Valuer
type ScanValuer interface {
	sql.Scanner
	driver.Valuer
}

// JSONVal returns an object implementing ScanValuer
// interfaces to read/write JSON(b) values from/to DB.
// 'v' must be pointer for Scan use.
func JSONVal(v interface{}) ScanValuer { return &jsonVal{v} }

type jsonVal struct{ v interface{} }

// Value implements the driver.Valuer interface and marshals Val to []byte
func (j *jsonVal) Value() (driver.Value, error) { return json.Marshal(j.v) }

// Scan parses JSON from src into Val
// Src must be a valid JSON []byte or string (not nil)
func (j *jsonVal) Scan(src interface{}) error {
	var source []byte
	switch t := src.(type) {
	case string:
		source = []byte(t)
	case []byte:
		source = t
	default:
		return errors.New("incompatible source for JSONVal")
	}
	return json.Unmarshal(source, j.v)
}
