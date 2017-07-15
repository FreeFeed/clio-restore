package main

import (
	"archive/zip"
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/FreeFeed/clio-restore/internal/account"
	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/FreeFeed/clio-restore/internal/config"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"github.com/lib/pq"
	"gopkg.in/gomail.v2"
)

// App is a main application
type App struct {
	*config.Config
	DB       *sql.DB
	Tx       *sql.Tx
	S3Client *s3.S3
	Accounts *account.Store
	Owner    *account.Account
	ZipFiles zipFilesList
	// Mp3Files       map[string]*zip.File  // map ID -> *zip.File
	ImageFiles     map[string]*localFile // map ID -> *zip.File
	OtherFiles     map[string]*localFile // map ID -> *zip.File
	ViaToRestore   map[string]bool       // via sources (URLs) to restore
	PostsToRestore int
	AttOrd int

	mp3ZipReader *zip.ReadCloser
}

// Init initialises App by Config
func (a *App) Init(zipFiles []*zip.File, conf *config.Config) {
	a.Config = conf

	a.ZipFiles = zipFiles

	a.readImageFiles()
	a.readOtherFiles()

	if a.MP3Zip != "" { // Open MP3 zip
		var err error
		a.mp3ZipReader, err = zip.OpenReader(conf.MP3Zip)
		mustbe.OK(errors.Annotate(err, "cannot open MP3 archive file"))
		mp3Re := regexp.MustCompile(`([0-9a-f]+)\.mp3$`)
		for _, f := range a.mp3ZipReader.File {
			m := mp3Re.FindStringSubmatch(f.Name)
			if m != nil {
				if _, ok := a.OtherFiles[m[1]]; !ok {
					a.OtherFiles[m[1]] = &localFile{File: f, OrigName: path.Base(f.Name)}
				}
			}
		}
	}

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

	{
		var recStatus int
		mustbe.OK(a.DB.QueryRow(
			"select recovery_status from archives where user_id = $1", a.Owner.UID,
		).Scan(&recStatus))
		if recStatus == recoveryNotStarted {
			mustbe.OK(errors.New("user wasn't allow to restore his archive"))
		}
		if recStatus == recoveryFinished {
			mustbe.OK(errors.New("archive already restored"))
		}
	}

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

// Close closes opened resources
func (a *App) Close() {
	if a.mp3ZipReader != nil {
		a.mp3ZipReader.Close()
	}
}

func (a *App) getArchiveOwnerName() (string, error) {
	// Looking for feedinfo.js in files
	if f, ok := a.ZipFiles.FindByRe(feedInfoRe); ok {
		user := new(clio.UserJSON)
		if err := readZipObject(f, user); err != nil {
			return "", err
		}
		if user.Type != "user" {
			return "", errors.Errorf("@%s is not a user (%s)", user.UserName, user.Type)
		}
		return user.UserName, nil
	}
	return "", errors.New("cannot find feedinfo.js")
}

// FinishRestoration marks archive as restored
func (a *App) FinishRestoration() {
	mustbe.OKVal(a.DB.Exec(
		"update archives set recovery_status = $1 where user_id = $2",
		recoveryFinished, a.Owner.UID,
	))

	if a.SMTPHost != "" {
		dialer := gomail.NewDialer(a.SMTPHost, a.SMTPPort, a.SMTPUsername, a.SMTPPassword)
		mail := gomail.NewMessage()
		mail.SetHeader("From", a.SMTPFrom)
		mail.SetHeader("To", a.Owner.Email, a.SMTPBcc)
		mail.SetHeader("Subject", "Archive posts restoration request")
		mail.SetBody("text/plain",
			fmt.Sprintf(
				"Posts for FreeFeed user %q (FriendFeed username %q) have been restored from the archive.",
				a.Owner.NewUserName, a.Owner.OldUserName,
			),
		)
		if err := dialer.DialAndSend(mail); err != nil {
			errorLog.Printf("Cannot send email to %q: %v", a.Owner.Email, err)
		}
	}
}

func (a *App) readImageFiles() {
	a.ImageFiles = make(map[string]*localFile)
	name2id := make(map[string]string) // file name -> file UID

	var (
		tsvFileRe   = regexp.MustCompile(`^[a-z0-9-]+/_json/data/images\.tsv$`)
		mediaURLRe  = regexp.MustCompile(`[0-9a-f]+$`)
		imageFileRe = regexp.MustCompile(`^[a-z0-9-]+/images/media/([^/]+)$`)
		thumbFileRe = regexp.MustCompile(`^[a-z0-9-]+/images/media/thumbnails/(([0-9a-f]+).+)`)
	)

	// Looking for the TSV file
	if f, ok := a.ZipFiles.FindByRe(tsvFileRe); ok {
		r := mustbe.OKVal(f.Open()).(io.ReadCloser)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 2)
			if len(parts) != 2 {
				continue
			}
			m := mediaURLRe.FindStringSubmatch(parts[0])
			if m == nil {
				continue
			}
			name2id[parts[1]] = m[0]
		}
		r.Close()
	}

	// Now looking for images
	for _, f := range a.ZipFiles {
		if imageFileRe.MatchString(f.Name) {
			name := imageFileRe.FindStringSubmatch(f.Name)[1]
			if id, ok := name2id[name]; ok {
				a.ImageFiles[id] = &localFile{File: f, OrigName: name}
			}
		}
		if thumbFileRe.MatchString(f.Name) {
			m := thumbFileRe.FindStringSubmatch(f.Name)
			a.ImageFiles[m[2]] = &localFile{File: f, OrigName: m[1]}
		}
	}

	return
}

func (a *App) readOtherFiles() {
	a.OtherFiles = make(map[string]*localFile)
	name2id := make(map[string]string) // file name -> file UID

	var (
		tsvFileRe   = regexp.MustCompile(`^[a-z0-9-]+/_json/data/files\.tsv$`)
		mediaURLRe  = regexp.MustCompile(`[0-9a-f]+$`)
		otherFileRe = regexp.MustCompile(`^[a-z0-9-]+/files/([^/]+)$`)
	)

	// Looking for the TSV file
	if f, ok := a.ZipFiles.FindByRe(tsvFileRe); ok {
		r := mustbe.OKVal(f.Open()).(io.ReadCloser)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 2)
			if len(parts) != 2 {
				continue
			}
			m := mediaURLRe.FindStringSubmatch(parts[0])
			if m == nil {
				continue
			}
			name2id[parts[1]] = m[0]
		}
		r.Close()
	}

	// Now looking for files
	for _, f := range a.ZipFiles {
		if otherFileRe.MatchString(f.Name) {
			name := otherFileRe.FindStringSubmatch(f.Name)[1]
			if id, ok := name2id[name]; ok {
				a.OtherFiles[id] = &localFile{File: f, OrigName: name}
			}
		}
	}

	return
}
