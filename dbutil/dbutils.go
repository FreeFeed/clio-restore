package dbutil

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/davidmz/mustbe"
	"github.com/lib/pq"
)

// Execer is the interface for Exec method
type Execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// H is a general map string->any
type H map[string]interface{}

// Insert inserts given values into the given database table
func Insert(db Execer, tableName string, vals H) (sql.Result, error) {
	sql, params := insertSQL(tableName, vals)
	return db.Exec(sql, params...)
}

// MustInsert inserts given values into the given database table
// and panics (in 'mustbe' way) if error
func MustInsert(db Execer, tableName string, vals H) sql.Result {
	return mustbe.OKVal(Insert(db, tableName, vals)).(sql.Result)
}

func insertSQL(tableName string, vals H) (query string, params []interface{}) {
	var (
		names        []string
		placeholders []string
	)
	n := 1
	for k, v := range vals {
		names = append(names, pq.QuoteIdentifier(k))
		params = append(params, v)
		placeholders = append(placeholders, fmt.Sprintf("$%d", n))
		n++
	}
	query = fmt.Sprintf(
		"insert into %s (%s) values (%s)",
		pq.QuoteIdentifier(tableName),
		strings.Join(names, ", "),
		strings.Join(placeholders, ", "),
	)

	return
}
