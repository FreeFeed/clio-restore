package dbutil

import (
	"database/sql"

	"github.com/davidmz/mustbe"
)

// MustTransact opens transaction and call foo with it.
// It rollback transaction if panic happens in foo and re-panic that panic.
func MustTransact(db *sql.DB, foo func(*sql.Tx)) {
	tx := mustbe.OKVal(db.Begin()).(*sql.Tx)
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
		tx.Commit()
	}()
	foo(tx)
}
