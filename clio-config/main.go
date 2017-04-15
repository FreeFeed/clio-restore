package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime/debug"

	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/davidmz/mustbe"
)

type archConfig struct {
	OldUserName             string `json:"old_username"`
	RecoveryStatus          int    `json:"recovery_status"`
	HasArchive              bool   `json:"has_archive"`
	DisableComments         bool   `json:"disable_comments"`
	RestoreCommentsAndLikes bool   `json:"restore_comments_and_likes"`
}

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

	flagVars := dbutil.H{}
	flagVars["old_username"] =
		flag.String("old_username", "", "set old (friendfeed) username of user")
	flagVars["recovery_status"] =
		flag.Int("recovery_status", 0, "set recovery_status for user (0, 1 or 2)")
	flagVars["has_archive"] =
		flag.Bool("has_archive", false, "set has_archive flag for user (t or f)")
	flagVars["disable_comments"] =
		flag.Bool("disable_comments", false, "set disable_comments flag for user (t or f)")
	flagVars["restore_comments_and_likes"] =
		flag.Bool("restore_comments_and_likes", false, "set restore_comments_and_likes flag for user (t or f)")
	flag.Parse()

	if flag.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-config [options] username")
		flag.PrintDefaults()
		os.Exit(1)
	}

	conf := mustbe.OKVal(config.Load()).(*config.Config)

	var (
		username = flag.Arg(0)
		db       *sql.DB
		userID   string
	)

	db = mustbe.OKVal(sql.Open("postgres", conf.DbStr)).(*sql.DB)
	mustbe.OK(db.Ping())

	// Looking for userID
	err := mustbe.OKOr(
		db.QueryRow("select uid from users where username = $1", username).Scan(&userID),
		sql.ErrNoRows,
	)
	if err != nil {
		fatalLog.Fatalf("Cannot find user '%s'", username)
	}

	archConf := getArchConfig(db, username)

	bytes, _ := json.MarshalIndent(archConf, "", "  ")
	fmt.Printf("Archive config for '%s':\n", username)
	fmt.Println(string(bytes))

	vals := dbutil.H{}
	flag.Visit(func(f *flag.Flag) {
		if v, ok := flagVars[f.Name]; ok {
			vals[f.Name] = reflect.ValueOf(v).Elem().Interface()
		}
	})

	if len(vals) > 0 {
		names, placeholders, params := dbutil.SQLizeParams(vals)
		lastPH := fmt.Sprintf("$%d", len(vals)+1)
		params = append(params, userID)
		mustbe.OKVal(db.Exec(
			"update archives set ("+names+") = ("+placeholders+") where user_id = "+lastPH,
			params...,
		))

		archConf := getArchConfig(db, username)

		bytes, _ := json.MarshalIndent(archConf, "", "  ")
		fmt.Println("Updated, now archive config is:")
		fmt.Println(string(bytes))
	}
}

func getArchConfig(db *sql.DB, username string) *archConfig {
	archConf := new(archConfig)
	err := mustbe.OKOr(db.QueryRow(
		`select
			a.old_username,
			a.recovery_status,
			a.has_archive,
			a.disable_comments,
			a.restore_comments_and_likes
		from
			archives a
			join users u on u.uid = a.user_id
		where u.username = $1`,
		username,
	).Scan(
		&archConf.OldUserName,
		&archConf.RecoveryStatus,
		&archConf.HasArchive,
		&archConf.DisableComments,
		&archConf.RestoreCommentsAndLikes,
	), sql.ErrNoRows)

	if err != nil {
		fatalLog.Fatalf("Cannot find any archive information for '%s'", username)
	}
	return archConf
}
