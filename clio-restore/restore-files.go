package main

import (
	"io"
	"io/ioutil"
	"strings"

	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/FreeFeed/clio-restore/internal/dbutil"
	"github.com/ascherkus/go-id3/src/id3"
	"github.com/davidmz/mustbe"
	"github.com/gofrs/uuid"
)

func (a *App) restoreFiles(entry *clio.Entry) (resUIDs []string) {
	var foundFiles []*fileInfo
	for _, f := range entry.Files {
		m := fileIDRe.FindStringSubmatch(f.URL)
		if m == nil { // no file ID
			continue
		}
		id := m[0]
		of, ok := a.OtherFiles[id]
		if !ok { // file not found in local files
			continue
		}

		foundFiles = append(foundFiles, &fileInfo{
			zipFile:     of.File,
			ContentType: f.Type,
			Name:        f.Name,
		})
	}

	for _, af := range foundFiles {
		var (
			title  string
			artist string
		)
		if af.isMP3() {
			// Read ID3 metadata
			func() {
				defer func() { recover() }() // due to issue https://github.com/ascherkus/go-id3/issues/1
				r, err := af.zipFile.Open()
				if err != nil {
					return
				}
				defer r.Close()

				meta := id3.Read(r)
				if meta == nil {
					return
				}

				title = strings.Replace(meta.Name, "\u0000", "", -1)
				artist = strings.Replace(meta.Artist, "\u0000", "", -1)
			}()
		}

		attID := mustbe.OKVal(uuid.NewV4()).(uuid.UUID).String()

		// We must read file into memory because AWS required io.ReadSeeker
		// and zipFile.Open returns io.ReadCloser
		r := mustbe.OKVal(af.zipFile.Open()).(io.ReadCloser)
		body := mustbe.OKVal(ioutil.ReadAll(r)).([]byte)
		r.Close()
		a.storeAttachment(body, "attachments/"+attID+af.dotExt(), af.Name, af.ContentType)

		// Write to DB
		dbutil.MustInsert(a.Tx, "attachments", dbutil.H{
			"uid":            attID,
			"ord":            a.AttOrd,
			"created_at":     entry.Date,
			"updated_at":     entry.Date,
			"file_name":      af.Name,
			"file_size":      af.size(),
			"mime_type":      af.ContentType,
			"media_type":     af.attachType(),
			"file_extension": af.ext(),
			"user_id":        entry.Author.UID,
			"artist":         artist,
			"title":          title,
		})
		a.AttOrd++

		resUIDs = append(resUIDs, attID)
	}

	return
}
