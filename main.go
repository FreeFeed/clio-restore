package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime/debug"

	"github.com/FreeFeed/clio-restore/account"
	"github.com/FreeFeed/clio-restore/clio"
	"github.com/FreeFeed/clio-restore/dbutils"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/kelseyhightower/envconfig"
	"github.com/lib/pq"
)

// Config holds program configuration taken from env. vars
type Config struct {
	Db       string `desc:"Database connection string" required:"true"`
	GM       string `desc:"Path to the GraphicsMagick (gm) executable" required:"true"`
	GifSicle string `desc:"Path to the gifsicle executable" required:"true"`
	SRGB     string `desc:"Path to the sRGB ICM profile" required:"true"`
	AttDir   string `desc:"Directory to store attachments (S3 is not used if setted)"`
	S3Bucket string `desc:"S3 bucket name to store attachments (required if S3 is used)"`
	MP3Zip   string `desc:"Path to the zip-archive with mp3 files"`
	AttURL   string `desc:"Attachments root url" default:"https://media.freefeed.net"`
}

// Globals
var (
	infoLog  = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	errorLog = log.New(os.Stdout, "ERROR ", log.LstdFlags)
	fatalLog = log.New(os.Stdout, "FATAL ", log.LstdFlags)

	conf = new(Config)

	db         *sql.DB
	s3Client   *s3.S3
	zipFiles   []*zip.File
	mp3Files   map[string]*zip.File      // map ID -> *zip.File
	imageFiles map[string]localImageFile // map ID -> *zip.File

	acc          *account.Account // current user/archive information
	viaToRestore []string         // via sources (URLs) to restore
)

func main() {
	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: clio-restore clio-archive.zip")
		fmt.Fprintln(os.Stderr, "")
		envconfig.Usage("frf", conf)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Also you should set all variables required by AWS.")
		os.Exit(1)
	}

	mustbe.OK(envconfig.Process("frf", conf))

	if conf.AttDir == "" && conf.S3Bucket == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-restore clio-archive.zip")
		fmt.Fprintln(os.Stderr, "")
		envconfig.Usage("frf", conf)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Also you should set all variables required by AWS.")
		os.Exit(1)
	}

	archFile := os.Args[1]

	{ // Open zip
		archZip, err := zip.OpenReader(archFile)
		mustbe.OK(errors.Annotate(err, "cannot open archive file"))
		defer archZip.Close()
		zipFiles = archZip.File
	}

	mp3Files = make(map[string]*zip.File)
	if conf.MP3Zip != "" { // Open MP3 zip
		mp3zip, err := zip.OpenReader(conf.MP3Zip)
		mustbe.OK(errors.Annotate(err, "cannot open MP3 archive file"))
		defer mp3zip.Close()
		mp3Re := regexp.MustCompile(`([0-9a-f]+)\.mp3$`)
		for _, f := range mp3zip.File {
			m := mp3Re.FindStringSubmatch(f.Name)
			if m != nil {
				mp3Files[m[1]] = f
			}
		}
	}

	imageFiles = readImageFiles(zipFiles)

	if conf.AttDir == "" { // use S3
		awsSession, err := session.NewSession()
		mustbe.OK(errors.Annotate(err, "cannot create AWS session"))
		s3Client = s3.New(awsSession)
	}

	{ // Connect to DB
		var err error
		db, err = sql.Open("postgres", conf.Db)
		mustbe.OK(errors.Annotate(err, "cannot open DB"))
		mustbe.OK(errors.Annotate(db.Ping(), "cannot connect to DB"))
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
			dbutils.JSONVal(&viaStats),
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

		if len(entry.Thumbnails) == 0 {
			// continue
		}

		processedPosts++

		infoLog.Printf("Processing entry %s [%d/%d]", entry.Name, processedPosts, postsToRestore)

		restoreEntry(entry)
	}
}
