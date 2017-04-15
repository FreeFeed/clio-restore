package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
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
	var (
		cutDateString string
	)

	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	flag.StringVar(&cutDateString, "before", "2015-05-01", "delete records before this date")
	flag.Parse()

	if flag.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-rollback [options] username")
		flag.PrintDefaults()
		os.Exit(1)
	}

	conf := mustbe.OKVal(config.Load()).(*config.Config)

	var (
		username = flag.Arg(0)
		cutDate  = mustbe.OKVal(time.Parse(dateFormat, cutDateString)).(time.Time)
		db       *sql.DB
		s3Client *s3.S3
		userID   string
	)

	db = mustbe.OKVal(sql.Open("postgres", conf.DbStr)).(*sql.DB)
	mustbe.OK(db.Ping())

	// Looking for userID
	err := mustbe.OKOr(db.QueryRow("select uid from users where username = $1", username).Scan(&userID), sql.ErrNoRows)
	if err != nil {
		fatalLog.Fatalf("Cannot find user '%s'", username)
	}

	if conf.S3Bucket != "" {
		awsSession, err := session.NewSession()
		mustbe.OK(errors.Annotate(err, "cannot create AWS session"))
		s3Client = s3.New(awsSession)
	}

	infoLog.Printf("Trying to delete all %s's posts and files created before %s", username, cutDate.Format(dateFormat))

	var postIDs []string
	mustbe.OK(dbutil.QueryCol(
		db, &postIDs,
		"select uid from posts where user_id = $1 and created_at < $2",
		userID, cutDate,
	))

	infoLog.Printf("Found %d posts", len(postIDs))

	for n, postID := range postIDs {
		dbutil.MustTransact(db, func(tx *sql.Tx) {
			// Comments
			{
				var comStats []struct {
					UserID string
					Count  int
				}
				mustbe.OK(dbutil.QueryCols(
					tx, &comStats,
					"select user_id, count(*) from comments where post_id = $1 and user_id is not null group by user_id", postID,
				))
				for _, cs := range comStats {
					mustbe.OKVal(tx.Exec(
						`update user_stats set comments_count = comments_count - $1 where user_id = $2`,
						cs.Count, cs.UserID,
					))
				}
				mustbe.OKVal(tx.Exec("delete from comments where post_id = $1", postID))
			}

			// Likes
			{
				var likerIDs []string
				mustbe.OK(dbutil.QueryCol(tx, &likerIDs, "select user_id from likes where post_id = $1", postID))
				for _, likerID := range likerIDs {
					mustbe.OKVal(tx.Exec(`update user_stats set likes_count = likes_count - 1 where user_id = $1`, likerID))
				}
				mustbe.OKVal(tx.Exec("delete from likes where post_id = $1", postID))
			}

			// Post itself
			mustbe.OKVal(tx.Exec("delete from posts where uid = $1", postID))
		})

		if (n+1)%100 == 0 {
			infoLog.Printf("%d posts was processed", n+1)
		}
	}

	mustbe.OKVal(db.Exec(
		`update user_stats set posts_count = posts_count - $1 where user_id = $2`,
		len(postIDs), userID,
	))

	infoLog.Print("All posts was processed")

	var attachments []struct {
		ID        string
		Ext       string
		HasThumbs bool
	}
	mustbe.OK(dbutil.QueryCols(
		db, &attachments,
		"select uid, file_extension, not no_thumbnail from attachments where user_id = $1 and created_at < $2",
		userID, cutDate,
	))

	infoLog.Printf("Found %d files", len(attachments))
	for n, att := range attachments {
		name := att.ID + "." + att.Ext
		fileNames := []string{path.Join("attachments", name)}
		if att.HasThumbs {
			fileNames = append(fileNames, path.Join("attachments", "thumbnails", name))
			fileNames = append(fileNames, path.Join("attachments", "thumbnails2", name))
		}

		if conf.AttDir != "" {
			for _, fileName := range fileNames {
				if err := os.Remove(filepath.Join(conf.AttDir, fileName)); os.IsNotExist(err) {
					errorLog.Println("File not found:", fileName)
				} else {
					mustbe.OK(err)
				}
			}
		} else {
			del := new(s3.Delete)
			for _, fileName := range fileNames {
				del.Objects = append(del.Objects, new(s3.ObjectIdentifier).SetKey(fileName))
			}
			mustbe.OKVal(s3Client.DeleteObjects(
				new(s3.DeleteObjectsInput).
					SetBucket(conf.S3Bucket).
					SetDelete(del),
			))
		}

		mustbe.OKVal(db.Exec("delete from attachments where uid = $1", att.ID))

		if (n+1)%10 == 0 {
			infoLog.Printf("%d files was processed", n+1)
		}
	}

	infoLog.Print("All files was processed")

	mustbe.OKVal(db.Exec(
		"update archives set recovery_status = $1 where user_id = $2",
		1, userID,
	))

	infoLog.Printf("recovery_status resetted to %d", 1)
}
