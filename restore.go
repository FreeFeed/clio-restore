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

	mustbe.OK(tx.QueryRow(
		`insert into posts (
			body,
			user_id,
			created_at,
			updated_at,
			bumped_at,
			comments_disabled,
			destination_feed_ids
		) values ($1, $2, $3, $4, $5, $6, $7) returning uid`,
		entry.Body,
		entry.Author.UID,
		createdAt,
		updatedAt,
		updatedAt,
		entry.Author.DisableComments,
		pq.Array([]int{entry.Author.Feeds.Posts.ID}),
	).Scan(&postUID))

	infoLog.Println("created post with UID", postUID)

	// register old post name
	mustbe.OKVal(tx.Exec(
		`insert into archive_post_names (old_post_name, post_id) values ($1, $2)`,
		entry.Name, postUID,
	))

	// register via
	if viaID := getViaID(entry.Via); viaID != 0 {
		mustbe.OKVal(tx.Exec(`insert into archive_posts_via (via_id, post_id) values ($1, $2)`, viaID, postUID))
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
		mustbe.OKVal(tx.Exec(
			`insert into likes 
				(post_id, user_id, created_at) values ($1, $2, $3, $4, $5, $6)`,
			postUID, like.Author.UID, like.Date,
		))
	} else {
		// like is hidden
		if like.Author.UID != "" {
			mustbe.OKVal(tx.Exec(
				`insert into hidden_likes 
					(post_id, user_id, date) values ($1, $2, $3)`,
				postUID, like.Author.UID, like.Date,
			))
		} else {
			mustbe.OKVal(tx.Exec(
				`insert into hidden_likes 
					(post_id, old_username, date) values ($1, $2, $3)`,
				postUID, like.Author.OldUserName, like.Date,
			))
		}
	}
	return
}

func commentPost(postUID string, postAuthor *account.Account, comment *clio.Comment) (restoredVisible bool) {
	restoredVisible = comment.Author.RestoreCommentsAndLikes ||
		comment.Author.OldUserName == postAuthor.OldUserName && comment.Author.RestoreSelfComments
	if restoredVisible {
		// comment is visible
		mustbe.OKVal(tx.Exec(
			`insert into comments (
					post_id,
					body,
					user_id,
					created_at,
					updated_at,
					hide_type
				) values ($1, $2, $3, $4, $5, $6)`,
			postUID,
			comment.Body,
			comment.Author.UID,
			comment.Date,
			comment.Date,
			commentTypeVisible,
		))
	} else {
		// comment is hidden
		commentID := ""
		mustbe.OK(tx.QueryRow(
			`insert into comments (
					post_id,
					body,
					user_id,
					created_at,
					updated_at,
					hide_type
				) values ($1, $2, $3, $4, $5, $6) returning uid`,
			postUID,
			hiddenCommentBody,
			nil,
			comment.Date,
			comment.Date,
			commentTypeHidden,
		).Scan(&commentID))

		if comment.Author.UID != "" {
			mustbe.OKVal(tx.Exec(
				`insert into hidden_comments 
					(comment_id, body, user_id) values ($1, $2, $3)`,
				commentID, comment.Body, comment.Author.UID,
			))
		} else {
			mustbe.OKVal(tx.Exec(
				`insert into hidden_comments 
					(comment_id, body, old_username) values ($1, $2, $3)`,
				commentID, comment.Body, comment.Author.OldUserName,
			))
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
