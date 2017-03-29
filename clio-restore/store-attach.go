package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/davidmz/mustbe"
)

func (a *App) storeAttachment(body []byte, path, name, contentType string) {
	if a.AttDir != "" {
		// Save to disk
		fileName := filepath.Join(a.AttDir, path)
		mustbe.OK(os.MkdirAll(filepath.Dir(fileName), 0777))
		mustbe.OK(ioutil.WriteFile(fileName, body, 0666))
	} else {
		// Upload to S3
		mustbe.OKVal(a.S3Client.PutObject(
			new(s3.PutObjectInput).
				SetBody(bytes.NewReader(body)).
				SetBucket(a.S3Bucket).
				SetKey(path).
				SetContentType(contentType).
				SetContentLength(int64(len(body))).
				SetACL(s3.ObjectCannedACLPublicRead).
				SetContentDisposition(contentDispositionString("inline", name)),
		))
	}
}

var nonASCIIRe = regexp.MustCompile(`[^\x20-\x7f]`)

// Get cross-browser Content-Disposition header for attachment
func contentDispositionString(disposition, name string) string {
	if name == "" {
		return disposition
	}
	// Old browsers (IE8) need ASCII-only fallback filenames
	fileNameASCII := nonASCIIRe.ReplaceAllString(name, "_")
	// Modern browsers support UTF-8 filenames
	fileNameUTF8 := url.QueryEscape(name)
	// Inline version of 'attfnboth' method (http://greenbytes.de/tech/tc2231/#attfnboth)
	return fmt.Sprintf(`%s; filename="%s"; filename*=utf-8''%s`, disposition, fileNameASCII, fileNameUTF8)
}
