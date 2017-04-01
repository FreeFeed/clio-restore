package main

import (
	"archive/zip"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/FreeFeed/clio-restore/clio"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/kelseyhightower/envconfig"
)

// Config holds program configuration taken from env. vars
type Config struct {
	DbStr    string `desc:"Database connection string" required:"true"`
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
)

func main() {
	defer mustbe.Catched(func(err error) {
		fatalLog.Println(err)
		debug.PrintStack()
	})

	conf := new(Config)

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

	// Open zip
	archZip, err := zip.OpenReader(archFile)
	mustbe.OK(errors.Annotate(err, "cannot open archive file"))
	defer archZip.Close()

	app := new(App)
	app.Init(archZip.File, conf)
	defer app.Close()

	processedPosts := 0

	for _, file := range app.ZipFiles {
		if !entryRe.MatchString(file.Name) {
			continue
		}

		entry := new(clio.Entry)
		mustbe.OK(errors.Annotate(readZipObject(file, entry), "error reading entry"))

		if !app.ViaToRestore[entry.Via.URL] {
			// via source not allowed, skipping
			continue
		}

		entry.Author = app.Accounts.Get(entry.AuthorName)
		for _, c := range entry.Comments {
			c.Author = app.Accounts.Get(c.AuthorName)
		}
		for _, l := range entry.Likes {
			l.Author = app.Accounts.Get(l.AuthorName)
		}

		processedPosts++

		infoLog.Printf("Processing entry %s [%d/%d]", entry.Name, processedPosts, app.PostsToRestore)

		app.restoreEntry(entry)
	}

	// all done
	app.FinishRestoration()
}
