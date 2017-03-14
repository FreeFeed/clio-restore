package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/lib/pq"
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

// H is a general map string->any
type H map[string]interface{}

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

var nonASCIIRe = regexp.MustCompile(`[^\x20-\x7f]`)

// Get cross-browser Content-Disposition header for attachment
func contentDispositionString(disposition, name string) string {
	// Old browsers (IE8) need ASCII-only fallback filenames
	fileNameASCII := nonASCIIRe.ReplaceAllString(name, "_")

	// Modern browsers support UTF-8 filenames
	fileNameUTF8 := url.QueryEscape(name)

	// Inline version of 'attfnboth' method (http://greenbytes.de/tech/tc2231/#attfnboth)
	return fmt.Sprintf(`%s; filename="%s"; filename*=utf-8''%s`, disposition, fileNameASCII, fileNameUTF8)
}
