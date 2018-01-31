package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/davidmz/mustbe"
	_ "github.com/lib/pq"
)

const (
	commentTypeHidden = 3
	hiddenCommentBody = "Comment is in archive"
)

// Globals
var (
	infoLog  = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	errorLog = log.New(os.Stdout, "ERROR ", log.LstdFlags)
	fatalLog = log.New(os.Stdout, "FATAL ", log.LstdFlags)
)

const dateFormat = "2006-01-02"

func main() {
	var (
		cutDateString      string
		removeFromOwnPosts bool
	)

	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	flag.StringVar(&cutDateString, "before", "2015-05-01", "delete activities before this date")
	flag.BoolVar(&removeFromOwnPosts, "from-own-posts", false, "remove user's comments from their own posts")
	flag.Parse()

	if flag.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-rollback-activities [options] username")
		flag.PrintDefaults()
		os.Exit(1)
	}

	conf := mustbe.OKVal(config.Load()).(*config.Config)

	var (
		username = flag.Arg(0)
		cutDate  = mustbe.OKVal(time.Parse(dateFormat, cutDateString)).(time.Time)
		db       *sql.DB
		userID   string
	)

	db = mustbe.OKVal(sql.Open("postgres", conf.DbStr)).(*sql.DB)
	mustbe.OK(db.Ping())

	// Looking for userID
	err := mustbe.OKOr(db.QueryRow("select uid from users where username = $1", username).Scan(&userID), sql.ErrNoRows)
	if err != nil {
		fatalLog.Fatalf("Cannot find user '%s'", username)
	}

	var (
		likesFeedID    int
		commentsFeedID int
	)

	mustbe.OK(db.QueryRow("select id from feeds where user_id = $1 and name = $2", userID, "Likes").Scan(&likesFeedID))
	mustbe.OK(db.QueryRow("select id from feeds where user_id = $1 and name = $2", userID, "Comments").Scan(&commentsFeedID))

	var affectedPostIDs []string

	////////
	// LIKES
	////////

	infoLog.Printf("Trying to delete all %s's likes created before %s", username, cutDate.Format(dateFormat))

	var likes []struct {
		ID     int
		PostID string
		Date   time.Time
	}
	mustbe.OK(dbutil.QueryCols(
		db, &likes,
		"select id, post_id, created_at from likes where user_id = $1 and created_at < $2",
		userID, cutDate,
	))

	for n, like := range likes {
		dbutil.MustTransact(db, func(tx *sql.Tx) {
			dbutil.MustInsert(tx, "hidden_likes", dbutil.H{
				"post_id": like.PostID,
				"user_id": userID,
				"date":    like.Date,
			})
			mustbe.OKVal(tx.Exec("delete from likes where id = $1", like.ID))
			mustbe.OKVal(tx.Exec("update posts set feed_ids = feed_ids - $1::int where uid = $2", likesFeedID, like.PostID))
		})

		affectedPostIDs = append(affectedPostIDs, like.PostID)

		if (n+1)%100 == 0 {
			infoLog.Printf("%d posts was processed", n+1)
		}
	}

	////////
	// COMMENTS
	////////

	infoLog.Printf("Trying to delete all %s's comments created before %s", username, cutDate.Format(dateFormat))

	var postIDs []string
	if removeFromOwnPosts {
		mustbe.OK(dbutil.QueryCol(
			db, &postIDs,
			"select distinct post_id from comments where user_id = $1 and created_at < $2",
			userID, cutDate,
		))
	} else {
		mustbe.OK(dbutil.QueryCol(
			db, &postIDs,
			`select distinct c.post_id from
				comments c
				join posts p on p.uid = c.post_id and p.user_id <> c.user_id
			where c.user_id = $1 and c.created_at < $2`,
			userID, cutDate,
		))
	}

	for n, postID := range postIDs {
		dbutil.MustTransact(db, func(tx *sql.Tx) {
			var comments []struct {
				ID   string
				Body string
			}
			mustbe.OK(dbutil.QueryCols(
				tx, &comments,
				"select uid, body from comments where user_id = $1 and post_id = $2 and created_at < $3",
				userID, postID, cutDate,
			))

			for _, comm := range comments {
				dbutil.MustInsert(tx, "hidden_comments", dbutil.H{
					"comment_id": comm.ID,
					"body":       comm.Body,
					"user_id":    userID,
				})
				mustbe.OKVal(tx.Exec(
					"update comments set body = $1, user_id = null, hide_type = $2 where uid = $3",
					hiddenCommentBody,
					commentTypeHidden,
					comm.ID,
				))
				mustbe.OKVal(tx.Exec("delete from hashtag_usages where entity_id = $1", comm.ID))
			}

			var moreCommentsExists bool
			mustbe.OK(tx.QueryRow("select exists (select true from comments where post_id = $1 and user_id = $2)", postID, userID).Scan(&moreCommentsExists))
			if !moreCommentsExists {
				mustbe.OKVal(tx.Exec("update posts set feed_ids = feed_ids - $1::int where uid = $2", commentsFeedID, postID))
				affectedPostIDs = append(affectedPostIDs, postID)
			}
		})

		if (n+1)%100 == 0 {
			infoLog.Printf("%d posts was processed", n+1)
		}
	}

	////////
	// RoN's
	////////

	infoLog.Printf("Updating affected post's RoN feeds")

	affectedPostIDs = unique(affectedPostIDs)
	const chunkSize = 100
	for start := 0; start < len(affectedPostIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(affectedPostIDs) {
			end = len(affectedPostIDs)
		}
		postIDs := affectedPostIDs[start:end]
		mustbe.OKVal(db.Exec(
			`
			with
			--------------------------
			-- all existing posts feeds except RiverOfNews'es 
			--------------------------
			postFeeds as ( 
				select p.uid as post_id, f.* from 
					posts p 
					join feeds f on array[f.id] && p.feed_ids and f.name <> 'RiverOfNews' 
				where p.uid in (` + strings.Join(dbutil.QuoteStrings(postIDs), ",") + `)
			),
			--------------------------
			-- feeds that updates RiverOfNews'es 
			--------------------------
			srcFeeds as ( 
				select f.* from 
					postFeeds f
					join posts p on p.uid = f.post_id
				where f.name = any(
					case when p.is_propagable then array['Posts', 'Directs', 'Likes', 'Comments'] 
					else array['Posts', 'Directs'] 
					end
				)
			),
			--------------------------
			-- new feed_ids for these post (as column of integers) 
			--------------------------
			resultingFeedIds as ( 
				-- RoNs of users subscribed to post source feeds 
				select f.post_id, r.id from 
					srcFeeds f 
					join subscriptions s on feed_id = f.uid 
					join feeds r on r.user_id = s.user_id and r.name = 'RiverOfNews' 
				union 
				-- RoNs of users owned post source feeds 
				select f.post_id, r.id from 
					srcFeeds f 
					join feeds r on r.user_id = f.user_id and r.name = 'RiverOfNews' 
				union 
				-- post feeds 
				select post_id, id from postFeeds 
			),
			--------------------------
			-- new feed_ids for these post (as integer[]) 
			--------------------------
			newFeedsIds as ( 
				select post_id, array_agg(distinct id) as ids from resultingFeedIds group by post_id
			) 
			--------------------------
			-- performing update 
			--------------------------
			update posts set feed_ids = f.ids from newFeedsIds f where f.post_id = uid
			`,
		))
		infoLog.Printf("%d posts was processed", end)
	}
}

func unique(elements []string) []string {
	encountered := make(map[string]struct{})

	for _, element := range elements {
		encountered[element] = struct{}{}
	}

	result := []string{}
	for key := range encountered {
		result = append(result, key)
	}
	return result
}
