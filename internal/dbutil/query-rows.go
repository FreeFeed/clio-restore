package dbutil

import "github.com/davidmz/mustbe"

// RowScanner is an interface with a sql.Row(s) Scan function
type RowScanner interface {
	Scan(dest ...interface{}) error
}

// QueryRows performs Query and Scan's resulting Rows
func QueryRows(db Querier, query string, args Args, foo func(RowScanner) error) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := foo(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// MustQueryRows performs Query and Scan's resulting Rows
// and panics (in 'mustbe' way) if error.
// It also expects mustbe-panic in foo.
func MustQueryRows(db Querier, query string, args Args, foo func(RowScanner)) {
	mustbe.OK(QueryRows(db, query, args, func(r RowScanner) (outErr error) {
		defer mustbe.Catched(func(err error) { outErr = err })
		foo(r)
		return
	}))
}
