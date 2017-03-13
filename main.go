package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/FreeFeed/clio-restore/account"
	"github.com/FreeFeed/clio-restore/clio"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/lib/pq"
)

// Globals
var (
	infoLog  = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	errorLog = log.New(os.Stdout, "ERROR ", log.LstdFlags)
	fatalLog = log.New(os.Stdout, "FATAL ", log.LstdFlags)

	db       *sql.DB
	zipFiles []*zip.File

	acc          *account.Account // current user/archive information
	viaToRestore []string         // via sources (URLs) to restore
)

func main() {
	defer mustbe.Catched(func(err error) { fatalLog.Println(err) })

	const dbEnv = "FRF_DB"

	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: clio-restore clio-archive.zip")
		fmt.Fprintln(os.Stderr, "Put database connection string into the "+dbEnv+" environment variable")
		fmt.Fprintln(os.Stderr, "See https://godoc.org/github.com/lib/pq for the connection string format")
		os.Exit(1)
	}

	archFile := os.Args[1]
	dbConnString := os.Getenv(dbEnv)

	{ // Connect to DB
		if dbConnString == "" {
			mustbe.OK(errors.Errorf("%s environment variable not found", dbEnv))
		}

		var err error
		db, err = sql.Open("postgres", dbConnString)
		mustbe.OK(errors.Annotate(err, "cannot open DB"))
		mustbe.OK(errors.Annotate(db.Ping(), "cannot connect to DB"))
	}

	{ // Open zip
		archZip, err := zip.OpenReader(archFile)
		mustbe.OK(errors.Annotate(err, "cannot open archive file"))
		defer archZip.Close()
		zipFiles = archZip.File
	}

	account.SetDBConnection(db)

	oldUserName, err := getArchiveOwnerName()
	mustbe.OK(errors.Annotate(err, "cannot get archive owner"))

	infoLog.Println("Archive belongs to", oldUserName)

	acc = account.Get(oldUserName)
	if acc.UID == "" {
		mustbe.OK(errors.Errorf("cannot find %s in new Freefeed", oldUserName))
	}

	infoLog.Printf("%s new username is %s", oldUserName, acc.NewUserName)

	// posts statistics and sources
	postsToRestore := 0
	{
		var viaStats []*clio.ViaStatItem
		err := db.QueryRow(
			"select via_sources, via_restore from archives where old_username = $1",
			oldUserName,
		).Scan(
			&JSONSqlScanner{&viaStats},
			(*pq.StringArray)(&viaToRestore),
		)
		mustbe.OK(errors.Annotate(err, "error fetching via sources"))

		totalPosts := 0
		for _, s := range viaStats {
			totalPosts += s.Count
			if isViaAllowed(s.URL) {
				postsToRestore += s.Count
			}
		}
		infoLog.Printf("%s wants to restore %d posts of %d total", oldUserName, postsToRestore, totalPosts)
	}

	processedPosts := 0

	for _, file := range zipFiles {
		if !entryRe.MatchString(file.Name) {
			continue
		}

		entry := new(clio.Entry)
		mustbe.OK(errors.Annotate(readZipObject(file, entry), "error reading entry"))

		if !isViaAllowed(entry.Via.URL) {
			// via source not allowed, skipping
			continue
		}

		processedPosts++

		infoLog.Printf("Processing entry %s [%d/%d]", entry.Name, processedPosts, postsToRestore)

		restoreEntry(entry)

		if processedPosts > 103 {
			break
		}
	}
}
