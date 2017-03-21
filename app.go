package main

import (
	"archive/zip"
	"database/sql"
	"regexp"

	"github.com/FreeFeed/clio-restore/account"
	"github.com/FreeFeed/clio-restore/clio"
	"github.com/FreeFeed/clio-restore/dbutil"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/lib/pq"
)

// App is a main application
type App struct {
	*Config
	DB             *sql.DB
	Tx             *sql.Tx
	S3Client       *s3.S3
	Accounts       *account.Store
	Owner          *account.Account
	ZipFiles       []*zip.File
	Mp3Files       map[string]*zip.File  // map ID -> *zip.File
	ImageFiles     map[string]*localFile // map ID -> *zip.File
	ViaToRestore   map[string]bool       // via sources (URLs) to restore
	PostsToRestore int
}

// Init initialises App by Config
func (a *App) Init(zipFiles []*zip.File, conf *Config) {
	a.Config = conf

	a.ZipFiles = zipFiles

	a.Mp3Files = make(map[string]*zip.File)
	if a.MP3Zip != "" { // Open MP3 zip
		mp3zip, err := zip.OpenReader(conf.MP3Zip)
		mustbe.OK(errors.Annotate(err, "cannot open MP3 archive file"))
		defer mp3zip.Close()
		mp3Re := regexp.MustCompile(`([0-9a-f]+)\.mp3$`)
		for _, f := range mp3zip.File {
			m := mp3Re.FindStringSubmatch(f.Name)
			if m != nil {
				a.Mp3Files[m[1]] = f
			}
		}
	}

	a.readImageFiles()

	if a.AttDir == "" { // use S3
		awsSession, err := session.NewSession()
		mustbe.OK(errors.Annotate(err, "cannot create AWS session"))
		a.S3Client = s3.New(awsSession)
	}

	{ // Connect to DB
		var err error
		a.DB, err = sql.Open("postgres", a.DbStr)
		mustbe.OK(errors.Annotate(err, "cannot open DB"))
		mustbe.OK(errors.Annotate(a.DB.Ping(), "cannot connect to DB"))
	}

	a.Accounts = account.NewStore(a.DB)

	oldUserName, err := a.getArchiveOwnerName()
	mustbe.OK(errors.Annotate(err, "cannot get archive owner"))

	infoLog.Println("Archive belongs to", oldUserName)

	a.Owner = a.Accounts.Get(oldUserName)
	if !a.Owner.IsExists() {
		mustbe.OK(errors.Errorf("cannot find %s in new Freefeed", oldUserName))
	}

	infoLog.Printf("%s new username is %s", a.Owner.OldUserName, a.Owner.NewUserName)

	// posts statistics and sources
	{
		var (
			viaStats     []*clio.ViaStatItem
			viaToRestore []string
		)
		a.ViaToRestore = make(map[string]bool)
		err := a.DB.QueryRow(
			"select via_sources, via_restore from archives where old_username = $1",
			a.Owner.OldUserName,
		).Scan(
			dbutil.JSONVal(&viaStats),
			(*pq.StringArray)(&viaToRestore),
		)
		mustbe.OK(errors.Annotate(err, "error fetching via sources"))

		for _, v := range viaToRestore {
			a.ViaToRestore[v] = true
		}

		totalPosts := 0
		for _, s := range viaStats {
			totalPosts += s.Count
			if a.ViaToRestore[s.URL] {
				a.PostsToRestore += s.Count
			}
		}
		infoLog.Printf("%s wants to restore %d posts of %d total", a.Owner.OldUserName, a.PostsToRestore, totalPosts)
	}
}

func (a *App) getArchiveOwnerName() (string, error) {
	// Looking for feedinfo.js in files
	for _, f := range a.ZipFiles {
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
