package main

import (
	"database/sql"
	"regexp"
	"strings"

	"github.com/FreeFeed/clio-restore/account"
	"github.com/FreeFeed/clio-restore/clio"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/lib/pq"
)

var (
	feedInfoRe   = regexp.MustCompile(`^[a-z0-9-]+/_json/data/feedinfo\.js$`)
	entryRe      = regexp.MustCompile(`^[a-z0-9-]+/_json/data/entries/[0-9a-f]{8}\.js$`)
	ffMediaURLRe = regexp.MustCompile(`^http://(m\.friendfeed-media\.com|i\.friendfeed\.com)/`)
)

var tx *sql.Tx

func getArchiveOwnerName() (string, error) {
	// Looking for feedinfo.js in files
	for _, f := range zipFiles {
		if feedInfoRe.MatchString(f.Name) {
			user := new(clio.UserJSON)
			if err := readZipObject(f, user); err != nil {
				return "", err
			}
			if user.Type != "user" {
				return "", errors.Errorf("@%s is not a user (%s)", user.UserName, user.Type)
			}
			return user.UserName, nil
		}
	}
	return "", errors.New("cannot find feedinfo.js")
}

func isViaAllowed(viaURL string) bool {
	for _, u := range viaToRestore {
		if u == viaURL {
			return true
		}
	}
	return false
}

func restoreEntry(entry *clio.Entry) {
	// check if entry already imported
	alreadyExists := false
	mustbe.OK(db.QueryRow(
		`select exists(select 1 from archive_post_names where old_post_name = $1)`,
		entry.Name,
	).Scan(&alreadyExists))

	if alreadyExists {
		errorLog.Println("entry already imported")
		return
	}

	tx = mustbe.OKVal(db.Begin()).(*sql.Tx)
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
		tx.Commit()
	}()

	// thumbnails & files
	var attachUIDs []string
	attachUIDs = append(attachUIDs, restoreThumbnails(entry, tx)...)
	attachUIDs = append(attachUIDs, restoreFiles(entry, tx)...)

	// TODO file attachments

	// create post
	createdAt := entry.Date
	updatedAt := createdAt
	for _, c := range entry.Comments {
		if c.Date.After(updatedAt) {
			updatedAt = c.Date
		}
	}

	postUID := ""
	mustbe.OK(insertAndReturn(tx, "posts", H{
		"body":                 entry.Body,
		"user_id":              entry.Author.UID,
		"created_at":           createdAt,
		"updated_at":           updatedAt,
		"bumped_at":            updatedAt,
		"comments_disabled":    entry.Author.DisableComments,
		"destination_feed_ids": pq.Array([]int{entry.Author.Feeds.Posts.ID}),
	}, "returning uid").Scan(&postUID))

	infoLog.Println("created post with UID", postUID)

	// register old post name
	mustbe.OKVal(insertRecord(tx, "archive_posts", H{"old_post_name": entry.Name, "post_id": postUID}))

	// register via
	if viaID := getViaID(entry.Via); viaID != 0 {
		mustbe.OKVal(insertRecord(tx, "archive_posts_via", H{"via_id": viaID, "post_id": postUID}))
	}

	// atach attachments
	if len(attachUIDs) > 0 {
		var qUIDs []string
		for _, uid := range attachUIDs {
			qUIDs = append(qUIDs, pgQuoteString(uid))
		}
		mustbe.OKVal(tx.Exec(
			`update attachments set post_id = $1 where uid in (`+strings.Join(qUIDs, ",")+`)`,
			postUID,
		))
	}

	incrementUserStat(entry.Author, statPosts)

	// post feed_ids - all UIDs of post's feeds
	feedIds := make(map[string]bool)
	feedIds[entry.Author.Feeds.Posts.UID] = true

	// add comments
	infoLog.Println("adding comments")
	for _, c := range entry.Comments {
		if commentPost(postUID, entry.Author, c) {
			feedIds[c.Author.Feeds.Comments.UID] = true
			incrementUserStat(c.Author, statComments)
		}
	}

	// add likes
	infoLog.Println("adding likes")
	for _, l := range entry.Likes {
		if likePost(postUID, l) {
			feedIds[l.Author.Feeds.Likes.UID] = true
			incrementUserStat(l.Author, statLikes)
		}
	}

	infoLog.Println("updating feed_ids")
	{ // update post's feed_ids
		var ids []string
		for id := range feedIds {
			ids = append(ids, pgQuoteString(id))
		}
		mustbe.OKVal(tx.Exec(
			`update posts set feed_ids = (
				select array_agg(distinct f.id) from 
					feeds f 
					join subscriptions s on 
						f.user_id = s.user_id and f.name = 'RiverOfNews'
						or f.uid = s.feed_id
				where s.feed_id in (`+strings.Join(ids, ",")+`)
			) where uid = $1`,
			postUID,
		))
	}
}

func likePost(postUID string, like *clio.Like) (restoredVisible bool) {
	restoredVisible = like.Author.RestoreCommentsAndLikes
	if restoredVisible {
		// like is visible
		mustbe.OKVal(insertRecord(tx, "likes", H{
			"post_id":    postUID,
			"user_id":    like.Author.UID,
			"created_at": like.Date,
		}))
	} else {
		// like is hidden
		if like.Author.UID != "" {
			mustbe.OKVal(insertRecord(tx, "hidden_likes", H{
				"post_id": postUID,
				"user_id": like.Author.UID,
				"date":    like.Date,
			}))
		} else {
			mustbe.OKVal(insertRecord(tx, "hidden_likes", H{
				"post_id":      postUID,
				"old_username": like.Author.OldUserName,
				"date":         like.Date,
			}))
		}
	}
	return
}

func commentPost(postUID string, postAuthor *account.Account, comment *clio.Comment) (restoredVisible bool) {
	restoredVisible = comment.Author.RestoreCommentsAndLikes ||
		comment.Author.OldUserName == postAuthor.OldUserName && comment.Author.RestoreSelfComments
	if restoredVisible {
		// comment is visible
		mustbe.OKVal(insertRecord(tx, "comments", H{
			"post_id":    postUID,
			"body":       comment.Body,
			"user_id":    comment.Author.UID,
			"created_at": comment.Date,
			"updated_at": comment.Date,
			"hide_type":  commentTypeVisible,
		}))
	} else {
		// comment is hidden
		commentID := ""
		mustbe.OK(insertAndReturn(tx, "comments", H{
			"post_id":    postUID,
			"body":       hiddenCommentBody,
			"user_id":    nil,
			"created_at": comment.Date,
			"updated_at": comment.Date,
			"hide_type":  commentTypeHidden,
		}, "returning uid").Scan(&commentID))

		if comment.Author.UID != "" {
			mustbe.OKVal(insertRecord(tx, "hidden_comments", H{
				"comment_id": commentID,
				"body":       comment.Body,
				"user_id":    comment.Author.UID,
			}))
		} else {
			mustbe.OKVal(insertRecord(tx, "hidden_comments", H{
				"comment_id":   commentID,
				"body":         comment.Body,
				"old_username": comment.Author.OldUserName,
			}))
		}
	}
	return
}

type statType string

const (
	statPosts    statType = "posts"
	statComments statType = "comments"
	statLikes    statType = "likes"
)

func incrementUserStat(a *account.Account, t statType) {
	colName := pq.QuoteIdentifier(string(t) + "_count")
	mustbe.OKVal(tx.Exec(
		`update user_stats set `+colName+` = `+colName+` + 1 where user_id = $1`,
		a.UID,
	))
}
