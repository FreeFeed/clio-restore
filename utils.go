package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"io/ioutil"

	"github.com/juju/errors"
)

func readZipObject(file *zip.File, v interface{}) error {
	r, err := file.Open()
	if err != nil {
		return errors.Annotate(err, "cannot open archived file")
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Annotate(err, "cannot read archived file")
	}

	err = json.Unmarshal(data, v)
	if err != nil {
		return errors.Annotate(err, "cannot parse JSON")
	}

	return nil
}

// JSONSqlScanner is a sql.Scanner that parses JSON data into Val
type JSONSqlScanner struct {
	Val interface{}
}

// Scan parses JSON from src into Val
// Src must be a valid JSON []byte or string
func (j *JSONSqlScanner) Scan(src interface{}) error {
	var source []byte
	switch t := src.(type) {
	case string:
		source = []byte(t)
	case []byte:
		source = t
	default:
		return errors.New("incompatible source for JSONSqlScanner")
	}
	return json.Unmarshal(source, j.Val)
}

// see https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-STRINGS
func pgQuoteString(s string) string {
	const Q = 0x27 // single quote
	var (
		in  = []byte(s)
		out []byte
	)
	out = append(out, Q)
	for _, b := range in {
		if b == Q {
			out = append(out, Q)
		}
		out = append(out, b)
	}
	out = append(out, Q)

	return string(out)
}

// Commnon interface for sql.DB and sql.Tx
type dbQ interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}
