package main

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/FreeFeed/clio-restore/clio"
	"github.com/ascherkus/go-id3/src/id3"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
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
				id := m[1]
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

		// Write to DB
		attID := ""
		mustbe.OK(insertAndReturn(db, "attachments", H{
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
		}, "returning uid").Scan(&attID))

		if conf.AttDir != "" { // Save to disk
			dName := filepath.Join(conf.AttDir, "attachments")
			fName := filepath.Join(dName, attID+".mp3")

			mustbe.OK(func() error {
				if err := os.MkdirAll(dName, 0777); err != nil {
					return errors.Annotatef(err, "Cannot create directory %s", dName)
				}
				f, err := os.Create(fName)
				if err != nil {
					return errors.Annotatef(err, "Cannot open file %s to write attachment", fName)
				}
				defer f.Close()

				r, err := af.zipFile.Open()
				if err != nil {
					return errors.Annotatef(err, "Cannot open zip entry %s", af.zipFile.Name)
				}
				defer r.Close()

				_, err = io.Copy(f, r)
				if err != nil {
					return errors.Annotatef(err, "Cannot copy data %s -> %s", af.zipFile.Name, fName)
				}
				return nil
			}())

		} else { // Upload to S3
			// We must read file into memory because AWS required io.ReadSeeker
			// and zipFile.Open returns io.ReadCloser
			r := mustbe.OKVal(af.zipFile.Open()).(io.ReadCloser)
			body := mustbe.OKVal(ioutil.ReadAll(r)).([]byte)
			r.Close()

			mustbe.OKVal(s3Client.PutObject(
				new(s3.PutObjectInput).
					SetBody(bytes.NewReader(body)).
					SetBucket(conf.S3Bucket).
					SetKey("attachments/" + attID + ".mp3").
					SetContentType("audio/mpeg").
					SetACL("public-read").
					SetContentDisposition(contentDispositionString("inline", af.Name)),
			))
		}

		resUIDs = append(resUIDs, attID)
	}

	return
}
