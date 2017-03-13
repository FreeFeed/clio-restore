package main

import (
	"database/sql"

	"github.com/FreeFeed/clio-restore/clio"
	"github.com/davidmz/mustbe"
)

var viaCache = make(map[string]int)

// getViaID returns ID of via-record or 0 if entry has really not 'via'
func getViaID(via clio.ViaJSON) int {
	if via.URL == clio.DefaultViaURL {
		return 0
	}

	if id, ok := viaCache[via.URL]; ok {
		return id
	}

	var id int
	err := mustbe.OKOr(db.QueryRow(`select id from archive_via where url = $1`, via.URL).Scan(&id), sql.ErrNoRows)
	if err != nil {
		// row not found
		mustbe.OK(db.QueryRow(`insert into archive_via (url, title) values ($1, $2)`, via.URL, via.Name).Scan(&id))
	}

	viaCache[via.URL] = id
	return id
}
