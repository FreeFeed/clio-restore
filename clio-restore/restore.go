package main

import (
	"database/sql"
	"strings"

	"github.com/FreeFeed/clio-restore/internal/account"
	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/FreeFeed/clio-restore/internal/hashtags"
	"github.com/davidmz/mustbe"
	"github.com/lib/pq"
	"github.com/satori/go.uuid"
)

func (a *App) restoreEntry(entry *clio.Entry) {
	// check if entry already imported
	alreadyExists := false
	mustbe.OK(a.DB.QueryRow(
		`select exists(select 1 from archive_post_names where old_post_name = $1)`,
		entry.Name,
	).Scan(&alreadyExists))

	if alreadyExists {
		errorLog.Println("entry already imported")
		return
	}

	a.Tx = mustbe.OKVal(a.DB.Begin()).(*sql.Tx)
	defer func() {
		if p := recover(); p != nil {
			a.Tx.Rollback()
			a.Tx = nil
			panic(p)
		}
		a.Tx.Commit()
		a.Tx = nil
	}()

	// thumbnails & files
	var attachUIDs []string
	attachUIDs = append(attachUIDs, a.restoreThumbnails(entry)...)
	attachUIDs = append(attachUIDs, a.restoreFiles(entry)...)

	// create post
	createdAt := entry.Date
	updatedAt := createdAt
	for _, c := range entry.Comments {
		if c.Date.After(updatedAt) {
			updatedAt = c.Date
		}
	}

	postUID := uuid.NewV4().String()
	dbutil.MustInsert(a.Tx, "posts", dbutil.H{
		"uid":                  postUID,
		"body":                 entry.Body,
		"user_id":              entry.Author.UID,
		"created_at":           createdAt,
		"updated_at":           updatedAt,
		"bumped_at":            updatedAt,
		"comments_disabled":    entry.Author.DisableComments,
		"destination_feed_ids": pq.Array([]int{entry.Author.Feeds.Posts.ID}),
	})

	infoLog.Println("created post with UID", postUID)

	// register old post name
	dbutil.MustInsert(a.Tx, "archive_post_names", dbutil.H{
		"post_id":       postUID,
		"old_post_name": entry.Name,
		"old_url":       entry.URL,
	})

	// register via
	if viaID := a.getViaID(entry.Via); viaID != 0 {
		dbutil.MustInsert(a.Tx, "archive_posts_via", dbutil.H{"via_id": viaID, "post_id": postUID})
	}

	// post hashtags
	for _, h := range entry.Hashtags {
		dbutil.MustInsertWithoutConflict(a.Tx, "hashtag_usages", dbutil.H{
			"hashtag_id": hashtags.GetID(a.Tx, h),
			"entity_id":  postUID,
			"type":       "post",
		})
	}

	// atach attachments
	if len(attachUIDs) > 0 {
		mustbe.OKVal(a.Tx.Exec(
			`update attachments set 
				post_id = $1,
				user_id = $2,
				created_at = $3,
				updated_at = $4
			where uid in (`+strings.Join(dbutil.QuoteStrings(attachUIDs), ",")+`)`,
			postUID,
			entry.Author.UID,
			entry.Date,
			entry.Date,
		))
	}

	a.incrementUserStat(entry.Author, statPosts)

	// post feed_ids - all UIDs/IDs of post's feeds
	feedIDs := make(map[string]int)
	feedIDs[entry.Author.Feeds.Posts.UID] = entry.Author.Feeds.Posts.ID

	// add comments
	infoLog.Println("adding comments")
	for _, c := range entry.Comments {
		if a.commentPost(postUID, entry.Author, c) {
			feedIDs[c.Author.Feeds.Comments.UID] = c.Author.Feeds.Comments.ID
			a.incrementUserStat(c.Author, statComments)
		}
	}

	// add likes
	infoLog.Println("adding likes")
	for _, l := range entry.Likes {
		if a.likePost(postUID, l) {
			feedIDs[l.Author.Feeds.Likes.UID] = l.Author.Feeds.Likes.ID
			a.incrementUserStat(l.Author, statLikes)
		}
	}

	infoLog.Println("updating feed_ids")
	{ // update post's feed_ids
		var (
			intIDs pq.Int64Array
			UIDs   []string
		)
		for uid := range feedIDs {
			UIDs = append(UIDs, dbutil.QuoteString(uid))
		}
		// 1) All 'RiverOfNews' feeds of users subscribed to activity feeds
		mustbe.OK(a.Tx.QueryRow(
			`select array_agg(distinct f.id) from
				subscriptions s
				join feeds f on f.user_id = s.user_id and f.name = 'RiverOfNews'
				where s.feed_id in (` + strings.Join(UIDs, ",") + `)`,
		).Scan(&intIDs))
		// 2) Activity feeds itself
		for _, intID := range feedIDs {
			intIDs = append(intIDs, int64(intID))
		}

		mustbe.OKVal(a.Tx.Exec(`update posts set feed_ids = $1 where uid = $2`, intIDs, postUID))
	}
}

func (a *App) likePost(postUID string, like *clio.Like) (restoredVisible bool) {
	restoredVisible = like.Author.RestoreCommentsAndLikes
	if restoredVisible {
		// like is visible
		dbutil.MustInsert(a.Tx, "likes", dbutil.H{
			"post_id":    postUID,
			"user_id":    like.Author.UID,
			"created_at": like.Date,
		})
	} else {
		// like is hidden
		if like.Author.UID != "" {
			dbutil.MustInsert(a.Tx, "hidden_likes", dbutil.H{
				"post_id": postUID,
				"user_id": like.Author.UID,
				"date":    like.Date,
			})
		} else {
			dbutil.MustInsert(a.Tx, "hidden_likes", dbutil.H{
				"post_id":      postUID,
				"old_username": like.Author.OldUserName,
				"date":         like.Date,
			})
		}
	}
	return
}

func (a *App) commentPost(postUID string, postAuthor *account.Account, comment *clio.Comment) (restoredVisible bool) {
	commentID := uuid.NewV4().String()
	restoredVisible = comment.Author.RestoreCommentsAndLikes ||
		comment.Author.OldUserName == postAuthor.OldUserName
	if restoredVisible {
		// comment is visible
		dbutil.MustInsert(a.Tx, "comments", dbutil.H{
			"uid":        commentID,
			"post_id":    postUID,
			"body":       comment.Body,
			"user_id":    comment.Author.UID,
			"created_at": comment.Date,
			"updated_at": comment.Date,
			"hide_type":  commentTypeVisible,
		})

		// comment hashtags
		for _, h := range comment.Hashtags {
			dbutil.MustInsertWithoutConflict(a.Tx, "hashtag_usages", dbutil.H{
				"hashtag_id": hashtags.GetID(a.Tx, h),
				"entity_id":  commentID,
				"type":       "comment",
			})
		}

	} else {
		// comment is hidden
		dbutil.MustInsert(a.Tx, "comments", dbutil.H{
			"uid":        commentID,
			"post_id":    postUID,
			"body":       hiddenCommentBody,
			"user_id":    nil,
			"created_at": comment.Date,
			"updated_at": comment.Date,
			"hide_type":  commentTypeHidden,
		})

		if comment.Author.UID != "" {
			dbutil.MustInsert(a.Tx, "hidden_comments", dbutil.H{
				"comment_id": commentID,
				"body":       comment.Body,
				"user_id":    comment.Author.UID,
			})
		} else {
			dbutil.MustInsert(a.Tx, "hidden_comments", dbutil.H{
				"comment_id":   commentID,
				"body":         comment.Body,
				"old_username": comment.Author.OldUserName,
			})
		}
	}
	return
}

func (a *App) incrementUserStat(acc *account.Account, t statType) {
	colName := pq.QuoteIdentifier(string(t) + "_count")
	mustbe.OKVal(a.Tx.Exec(
		`update user_stats set `+colName+` = `+colName+` + 1 where user_id = $1`,
		acc.UID,
	))
}
