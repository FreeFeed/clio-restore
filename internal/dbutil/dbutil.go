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

// Querier is the interface for Query method
type Querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// QueryRower is the interface for QueryRow method
type QueryRower interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// H is a general map string->any
type H map[string]interface{}

// Args is a general argoments list
type Args []interface{}

// Insert inserts given values into the given database table
func Insert(db Execer, tableName string, vals H) (sql.Result, error) {
	sql, params := insertSQL(tableName, vals, "")
	return db.Exec(sql, params...)
}

// InsertWithoutConflict inserts given values into the given database table
// with 'on conflict do nothing'
func InsertWithoutConflict(db Execer, tableName string, vals H) (sql.Result, error) {
	sql, params := insertSQL(tableName, vals, "on conflict do nothing")
	return db.Exec(sql, params...)
}

// MustInsert inserts given values into the given database table
// and panics (in 'mustbe' way) if error
func MustInsert(db Execer, tableName string, vals H) sql.Result {
	return mustbe.OKVal(Insert(db, tableName, vals)).(sql.Result)
}

// MustInsertWithoutConflict inserts given values into the given database table
// with 'on conflict do nothing'
// and panics (in 'mustbe' way) if error
func MustInsertWithoutConflict(db Execer, tableName string, vals H) sql.Result {
	return mustbe.OKVal(InsertWithoutConflict(db, tableName, vals)).(sql.Result)
}

func insertSQL(tableName string, vals H, tail string) (query string, params []interface{}) {
	names, placeholders, params := SQLizeParams(vals)
	query = fmt.Sprintf(
		"insert into %s (%s) values (%s) %s",
		pq.QuoteIdentifier(tableName),
		names,
		placeholders,
		tail,
	)
	return
}

// SQLizeParams converts vals to
// 1) string of names,
// 2) string of placeholders and
// 3) slice of params values
func SQLizeParams(vals H) (names, placeholders string, params []interface{}) {
	var (
		aNames        []string
		aPlaceholders []string
	)
	n := 1
	for k, v := range vals {
		aNames = append(aNames, pq.QuoteIdentifier(k))
		params = append(params, v)
		aPlaceholders = append(aPlaceholders, fmt.Sprintf("$%d", n))
		n++
	}

	names = strings.Join(aNames, ", ")
	placeholders = strings.Join(aPlaceholders, ", ")

	return
}
