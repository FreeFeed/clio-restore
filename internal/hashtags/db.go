package hashtags

import (
	"database/sql"
	"strings"

	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/davidmz/mustbe"
)

var cache = make(map[string]int)

// GetID returns ID of given hashtag
func GetID(db dbutil.QueryRower, hashtag string) int {
	hashtag = strings.ToLower(hashtag)

	if id, ok := cache[hashtag]; ok {
		return id
	}

	var (
		id  int
		err = sql.ErrNoRows
	)
	for err != nil {
		err = mustbe.OKOr(db.QueryRow(`select id from hashtags where name = $1`, hashtag).Scan(&id), sql.ErrNoRows)
		if err != nil { // row not found
			err = mustbe.OKOr(db.QueryRow(`insert into hashtags (name) values ($1) returning id`, hashtag).Scan(&id), sql.ErrNoRows)
		}
	}
	cache[hashtag] = id
	return id
}
