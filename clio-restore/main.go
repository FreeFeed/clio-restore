package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
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

	var (
		fromDateStr string
		toDateStr   string
	)

	flag.StringVar(&fromDateStr, "from-date", "", "restore entries created after this date (YYYY-MM-DD)")
	flag.StringVar(&toDateStr, "to-date", "", "restore entries created before this date (YYYY-MM-DD)")
	flag.Parse()

	if flag.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, "Usage: clio-restore [options] clio-archive.zip")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var (
		fromDate time.Time
		toDate   time.Time
	)

	const dateFormat = "2006-01-02"

	if fromDateStr != "" {
		fromDate = mustbe.OKVal(time.Parse(dateFormat, fromDateStr)).(time.Time)
	}
	if toDateStr != "" {
		toDate = mustbe.OKVal(time.Parse(dateFormat, toDateStr)).(time.Time)
	}

	conf := mustbe.OKVal(config.Load()).(*config.Config)

	archFile := flag.Arg(0)

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

		if !fromDate.IsZero() && entry.Date.Before(fromDate) || // entry was created before from-date
			!toDate.IsZero() && entry.Date.After(toDate) || // entry was created after to-date
			!app.ViaToRestore[entry.Via.URL] { // via source not allowed, skipping
			//
			continue
		}

		entry.Init(app.Accounts)

		infoLog.Printf("Processing entry %s [%d/%d]", entry.Name, processedPosts+1, app.PostsToRestore)
		app.restoreEntry(entry)

		processedPosts++
	}

	// all done
	app.FinishRestoration()
	infoLog.Println("Done.")
}
