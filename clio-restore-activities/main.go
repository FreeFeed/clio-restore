package main

import (
	"database/sql"
	"flag"
	"log"
	"os"
	"runtime/debug"

	"time"

	"github.com/FreeFeed/clio-restore/account"
	"github.com/FreeFeed/clio-restore/dbutil"
	"github.com/davidmz/mustbe"
	"github.com/lib/pq"
)

// Globals
var (
	infoLog  = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	errorLog = log.New(os.Stdout, "ERROR ", log.LstdFlags)
	fatalLog = log.New(os.Stdout, "FATAL ", log.LstdFlags)
)

func main() {
	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	var dbStr string
	flag.StringVar(&dbStr, "db", "", "database connection string")
	flag.Parse()

	if dbStr == "" {
		flag.Usage()
		os.Exit(1)
	}

	db := mustbe.OKVal(sql.Open("postgres", dbStr)).(*sql.DB)
	mustbe.OK(db.Ping())

	accStore := account.NewStore(db)

	// Looking for users who allow to restore their comments and likes
	var accounts []*account.Account
	mustbe.OK(dbutil.QueryRows(
		db, "select old_username from archives where restore_comments_and_likes", nil,
		func(r dbutil.RowScanner) error {
			var name string
			if err := r.Scan(&name); err != nil {
				return err
			}
			accounts = append(accounts, accStore.Get(name))
			return nil
		},
	))

	infoLog.Printf("Found %d users who allow to restore comments and likes", len(accounts))

	for _, acc := range accounts {
		var existsComments, existsLikes bool

		mustbe.OK(db.QueryRow(
			`select exists(select 1 from hidden_comments where user_id = $1 or old_username = $2)`,
			acc.UID, acc.OldUserName,
		).Scan(&existsComments))

		mustbe.OK(db.QueryRow(
			`select exists(select 1 from hidden_likes where user_id = $1 or old_username = $2)`,
			acc.UID, acc.OldUserName,
		).Scan(&existsLikes))

		if !existsComments && !existsLikes {
			continue
		}

		dbutil.MustTransact(db, func(tx *sql.Tx) {
			if existsComments {
				infoLog.Printf("Restoring hidden comments of %q (now %q)", acc.OldUserName, acc.NewUserName)
				restoreComments(tx, acc)
			}
			if existsLikes {
				infoLog.Printf("Restoring hidden likes of %q (now %q)", acc.OldUserName, acc.NewUserName)
				restoreLikes(tx, acc)
			}
		})
	}
}

const batchSize = 100

func restoreComments(tx *sql.Tx, acc *account.Account) {
	var (
		feeds pq.Int64Array
		count int
	)
	// Feeds to append commented post to
	mustbe.OK(tx.QueryRow(
		`select array_agg(distinct f.id) from
				feeds f join subscriptions s on 
					f.user_id = s.user_id and f.name = 'RiverOfNews' or f.uid = s.feed_id
				where s.feed_id = $1`,
		acc.Feeds.Comments.UID,
	).Scan(&feeds))

	processedPosts := make(map[string]bool) // postID is a key

	type commentInfo struct {
		ID     string
		PostID string
		Body   string
	}

	for {
		var comments []commentInfo
		dbutil.MustQueryRows(tx,
			`select hc.comment_id, c.post_id, hc.body from 
				hidden_comments hc
				join comments c on c.uid = hc.comment_id
				where hc.user_id = $1 or hc.old_username = $2
				limit $3`,
			dbutil.Args{acc.UID, acc.OldUserName, batchSize},
			func(r dbutil.RowScanner) {
				ci := commentInfo{}
				mustbe.OK(r.Scan(&ci.ID, &ci.PostID, &ci.Body))
				comments = append(comments, ci)
			})
		if len(comments) == 0 {
			break
		}

		for _, ci := range comments {
			mustbe.OKVal(tx.Exec(
				"update comments set (body, user_id, hide_type) = ($1, $2, $3) where uid = $4",
				ci.Body, acc.UID, 0, ci.ID,
			))
			mustbe.OKVal(tx.Exec("delete from hidden_comments where comment_id = $1", ci.ID))

			if !processedPosts[ci.PostID] {
				mustbe.OKVal(tx.Exec(
					"update posts set feed_ids = feed_ids | $1 where uid = $2",
					feeds, ci.PostID,
				))
				processedPosts[ci.PostID] = true
			}
			count++
		}
	}

	mustbe.OKVal(tx.Exec(
		`update user_stats set comments_count = comments_count + $1 where user_id = $2`,
		count, acc.UID,
	))

	infoLog.Printf("Restored %d comments in %d posts", count, len(processedPosts))
}

func restoreLikes(tx *sql.Tx, acc *account.Account) {
	var (
		feeds pq.Int64Array
		count int
	)
	// Feeds to append liked post to
	mustbe.OK(tx.QueryRow(
		`select array_agg(distinct f.id) from
				feeds f join subscriptions s on 
					f.user_id = s.user_id and f.name = 'RiverOfNews' or f.uid = s.feed_id
				where s.feed_id = $1`,
		acc.Feeds.Likes.UID,
	).Scan(&feeds))

	type likeInfo struct {
		ID     int
		PostID string
		Date   time.Time
	}

	for {
		var likes []likeInfo

		dbutil.MustQueryRows(tx,
			`select id, post_id, date from hidden_likes
			where user_id = $1 or old_username = $2`,
			dbutil.Args{acc.UID, acc.OldUserName},
			func(r dbutil.RowScanner) {
				li := likeInfo{}
				mustbe.OK(r.Scan(&li.ID, &li.PostID, &li.Date))
				likes = append(likes, li)
			},
		)
		if len(likes) == 0 {
			break
		}

		for _, li := range likes {
			dbutil.MustInsert(tx, "likes", dbutil.H{
				"post_id":    li.PostID,
				"user_id":    acc.UID,
				"created_at": li.Date,
			})
			mustbe.OKVal(tx.Exec("delete from hidden_likes where id = $1", li.ID))
			mustbe.OKVal(tx.Exec(
				"update posts set feed_ids = feed_ids | $1 where uid = $2",
				feeds, li.PostID,
			))
			count++
		}
	}

	mustbe.OKVal(tx.Exec(
		`update user_stats set likes_count = likes_count + $1 where user_id = $2`,
		count, acc.UID,
	))

	infoLog.Printf("Restored %d likes", count)
}