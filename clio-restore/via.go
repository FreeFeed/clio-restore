package main

import (
	"database/sql"

	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/davidmz/mustbe"
)

var viaCache = make(map[string]int)

// getViaID returns ID of via-record or 0 if entry has really not 'via'
func (a *App) getViaID(via clio.ViaJSON) int {
	if via.URL == clio.DefaultViaURL {
		return 0
	}

	if id, ok := viaCache[via.URL]; ok {
		return id
	}

	var (
		id  int
		err = sql.ErrNoRows
	)
	for err != nil {
		err = mustbe.OKOr(a.Tx.QueryRow(`select id from archive_via where url = $1`, via.URL).Scan(&id), sql.ErrNoRows)
		if err != nil { // row not found
			err = mustbe.OKOr(a.Tx.QueryRow(
				`insert into archive_via (url, title) values ($1, $2) returning id`,
				via.URL, via.Name,
			).Scan(&id), sql.ErrNoRows)
		}
	}

	viaCache[via.URL] = id
	return id
}
