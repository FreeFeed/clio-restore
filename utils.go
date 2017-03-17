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

// Commnon interface for sql.DB and sql.Tx
type dbQ interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}
