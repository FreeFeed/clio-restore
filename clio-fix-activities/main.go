package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/davidmz/mustbe"
	_ "github.com/lib/pq"
)

// Globals
var (
	infoLog  = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	errorLog = log.New(os.Stdout, "ERROR ", log.LstdFlags)
	fatalLog = log.New(os.Stdout, "FATAL ", log.LstdFlags)
)

const dateFormat = "2006-01-02"

func main() {
	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	var (
		cutDateString string
	)

	flag.StringVar(&cutDateString, "before", "2015-05-01", "fix activities before this date")
	flag.Parse()

	if flag.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-fix-activities [options] username")
		flag.PrintDefaults()
		os.Exit(1)
	}

	conf := mustbe.OKVal(config.Load()).(*config.Config)

	var (
		username = flag.Arg(0)
		db       *sql.DB
		userID   string
		cutDate  = mustbe.OKVal(time.Parse(dateFormat, cutDateString)).(time.Time)
	)

	db = mustbe.OKVal(sql.Open("postgres", conf.DbStr)).(*sql.DB)
	mustbe.OK(db.Ping())

	// Looking for userID
	err := mustbe.OKOr(db.QueryRow("select uid from users where username = $1", username).Scan(&userID), sql.ErrNoRows)
	if err != nil {
		fatalLog.Fatalf("Cannot find user '%s'", username)
	}

	var (
		commentsFeedID int
		likesFeedID    int
	)

	mustbe.OK(db.QueryRow("select id from feeds where user_id = $1 and name = $2", userID, "Comments").Scan(&commentsFeedID))
	mustbe.OK(db.QueryRow("select id from feeds where user_id = $1 and name = $2", userID, "Likes").Scan(&likesFeedID))

	{
		var postIDs []string
		mustbe.OK(dbutil.QueryCol(
			db, &postIDs,
			"select distinct(post_id) from comments where user_id = $1 and created_at < $2",
			userID, cutDate,
		))
		infoLog.Printf("Found %d commented posts", len(postIDs))
		for n, postID := range postIDs {
			mustbe.OKVal(db.Exec(
				`update posts set feed_ids = feed_ids | $1::int where uid = $2`,
				commentsFeedID, postID,
			))
			if (n+1)%100 == 0 {
				infoLog.Printf("%d posts was processed", n+1)
			}
		}
	}

	{
		var postIDs []string
		mustbe.OK(dbutil.QueryCol(
			db, &postIDs,
			"select distinct(post_id) from likes where user_id = $1 and created_at < $2",
			userID, cutDate,
		))
		infoLog.Printf("Found %d liked posts", len(postIDs))
		for n, postID := range postIDs {
			mustbe.OKVal(db.Exec(
				`update posts set feed_ids = feed_ids | $1::int where uid = $2`,
				likesFeedID, postID,
			))
			if (n+1)%100 == 0 {
				infoLog.Printf("%d posts was processed", n+1)
			}
		}
	}

	infoLog.Print("All posts was processed")
}
