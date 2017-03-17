package main

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/FreeFeed/clio-restore/clio"
	"github.com/FreeFeed/clio-restore/dbutils"
	"github.com/ascherkus/go-id3/src/id3"
	"github.com/davidmz/mustbe"
	"github.com/satori/go.uuid"
)

var fileIDRe = regexp.MustCompile(`[0-9a-f]+$`)

type audioFile struct {
	zipFile *zip.File
	Name    string
	Size    int
	Artist  string
	Title   string
}

func restoreFiles(entry *clio.Entry, db dbQ) (resUIDs []string) {
	var foundFiles []*audioFile
	for _, f := range entry.Files {
		if f.Type == "audio/mpeg" {
			m := fileIDRe.FindStringSubmatch(f.URL)
			if m != nil {
				id := m[0]
				if zf, ok := mp3Files[id]; ok {
					foundFiles = append(foundFiles, &audioFile{zipFile: zf, Name: f.Name})
				}
			}
		}
	}

	for _, af := range foundFiles {
		af.Size = int(af.zipFile.FileHeader.UncompressedSize64)

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

			af.Title = meta.Name
			af.Artist = meta.Artist
		}()

		attID := uuid.NewV4().String()

		// We must read file into memory because AWS required io.ReadSeeker
		// and zipFile.Open returns io.ReadCloser
		r := mustbe.OKVal(af.zipFile.Open()).(io.ReadCloser)
		body := mustbe.OKVal(ioutil.ReadAll(r)).([]byte)
		r.Close()
		storeAttachment(body, "attachments/"+attID+".mp3", af.Name, "audio/mpeg")

		// Write to DB
		dbutils.MustInsert(db, "attachments", dbutils.H{
			"uid":            attID,
			"created_at":     entry.Date,
			"updated_at":     entry.Date,
			"file_name":      af.Name,
			"file_size":      af.Size,
			"mime_type":      "audio/mpeg",
			"media_type":     "audio",
			"file_extension": "mp3",
			"user_id":        entry.Author.UID,
			"artist":         af.Artist,
			"title":          af.Title,
		})

		resUIDs = append(resUIDs, attID)
	}

	return
}
