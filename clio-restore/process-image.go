package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"

	"github.com/FreeFeed/clio-restore/dbutil"
	"github.com/davidmz/mustbe"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/satori/go.uuid"
)

type imageSizes map[string]imageSizesEntry

type imageSizesEntry struct {
	Width   int    `json:"w"`
	Height  int    `json:"h"`
	DirName string `json:"-"`
	URL     string `json:"url"`
	Body    []byte `json:"-"`
}

var defaultImageSizes = map[string]imageSizesEntry{
	"o":  {0, 0, "attachments", "", nil},
	"t":  {525, 175, "attachments/thumbnails", "", nil},
	"t2": {1050, 350, "attachments/thumbnails2", "", nil},
}

var supportedFormats = map[string]struct {
	MIMEType string
	Ext      string
	GMFormat string
}{
	"jpeg": {"image/jpeg", "jpg", "jpeg"},
	"png":  {"image/png", "png", "png"},
	"gif":  {"image/gif", "gif", "gif"},
}

func (i imageSizes) setName(baseURL, uid, ext string) {
	for t, e := range i {
		e.URL = baseURL + "/" + e.DirName + "/" + uid + "." + ext
		i[t] = e
	}
}

// Create image attachment from the first suitable from provided URLs
func (a *App) createImageAttachment(URLs ...string) (uid string, ok bool) {
	for _, u := range URLs {
		uid, ok = a.processSingleImage(u)
		if ok {
			break
		}
	}
	return
}

func (a *App) processSingleImage(URL string) (uid string, ok bool) {
	if ffMediaURLRe.MatchString(URL) {
		// Local image
		id := ffMediaURLRe.FindStringSubmatch(URL)[1]
		if lf, exists := a.ImageFiles[id]; exists {
			r := mustbe.OKVal(lf.Open()).(io.ReadCloser)
			body := mustbe.OKVal(ioutil.ReadAll(r)).([]byte)
			r.Close()
			uid, ok = a.makeAttachment(filepath.Base(lf.Name), body)
		} else {
			errorLog.Printf("Local image not found: %s", URL)
		}
		return
	}

	// Trying to Load remote image
	resp, err := http.Get(URL)
	if err != nil {
		errorLog.Println("Cannot fetch URL", URL)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { // redirects?
		errorLog.Printf("Error fetching URL: %s (%s)", resp.Status, URL)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !(ct == "image/jpeg" || ct == "image/png" || ct == "image/gif") {
		errorLog.Printf("Unsupported content type: %s (%s)", resp.Header.Get("Content-Type"), URL)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errorLog.Printf("Cannot read URL data: %v (%s)", err, URL)
		return
	}

	uid, ok = a.makeAttachment("", body)

	return
}

func (a *App) makeAttachment(name string, body []byte) (uid string, ok bool) {
	// do not trust content-type
	cfg, fmtString, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		errorLog.Printf("Cannot decode image: %v", err)
		return
	}

	format, ok := supportedFormats[fmtString]
	if !ok {
		errorLog.Printf("Unsupported image format: %s", format)
		return
	}

	if format.MIMEType == "image/jpeg" {
		if orient := getEXIFOrientation(bytes.NewReader(body)); orient != 0 && orient != 1 {
			cmd := exec.Command(a.GM,
				"convert",
				"-", // stdin
				"-profile", a.SRGB,
				"-auto-orient",
				"-quality", "95",
				"jpeg:-", // stdout
			)
			cmd.Stdin = bytes.NewReader(body)
			newBody := new(bytes.Buffer)
			cmd.Stdout = newBody
			if err := cmd.Run(); err != nil {
				errorLog.Printf("Cannot auto-orient image: %s", err)
			} else {
				body = newBody.Bytes()
			}
		}
	}

	// Store original image size
	iSizes := make(imageSizes)

	szEntry := defaultImageSizes["o"]
	szEntry.Width = cfg.Width
	szEntry.Height = cfg.Height
	szEntry.Body = body
	iSizes["o"] = szEntry

	for szID, szEntry := range defaultImageSizes {
		if szID == "o" || cfg.Width <= szEntry.Width && cfg.Height <= szEntry.Height {
			continue
		}
		szEntry.Width, szEntry.Height = fitInto(cfg.Width, cfg.Height, szEntry.Width, szEntry.Height)

		// Do resize
		var cmd *exec.Cmd
		if format.MIMEType != "image/gif" {
			cmd = exec.Command(a.GM,
				"convert",
				"-", // stdin
				"-resize", fmt.Sprintf("%dx%d!", szEntry.Width, szEntry.Height),
				"-profile", a.SRGB,
				"-auto-orient",
				"-quality", "95",
				format.GMFormat+":-", // stdout
			)
		} else {
			cmd = exec.Command(a.GifSicle,
				"--resize", fmt.Sprintf("%dx%d", szEntry.Width, szEntry.Height),
				"-O3",
			)

		}
		cmd.Stdin = bytes.NewReader(body)
		newBodyBuf := new(bytes.Buffer)
		cmd.Stdout = newBodyBuf
		if err := cmd.Run(); err != nil {
			errorLog.Printf("Cannot resize image: %s", err)
			continue
		}
		if newBodyBuf.Len() == 0 {
			errorLog.Printf("Cannot resize image: empty result")
			continue
		}
		szEntry.Body = newBodyBuf.Bytes()
		iSizes[szID] = szEntry
	}

	uid, ok = uuid.NewV4().String(), true

	iSizes.setName(a.AttURL, uid, format.Ext)

	// Upload all versions
	for _, entry := range iSizes {
		a.storeAttachment(entry.Body, entry.DirName+"/"+uid+"."+format.Ext, name, format.MIMEType)
	}

	// Write to DB without post_id, user_id, created_at and updated_at
	dbutil.MustInsert(a.Tx, "attachments", dbutil.H{
		"uid":            uid,
		"file_name":      name,
		"file_size":      len(body),
		"mime_type":      format.MIMEType,
		"media_type":     "image",
		"file_extension": format.Ext,
		"no_thumbnail":   len(iSizes) == 1,
		"image_sizes":    dbutil.JSONVal(iSizes),
	})

	return
}

func getEXIFOrientation(r io.Reader) (orient int) {
	x, err := exif.Decode(r)
	if err != nil {
		return
	}
	oTag, err := x.Get(exif.Orientation)
	if err != nil {
		return
	}
	orient, _ = oTag.Int(0)
	return
}

func fitInto(w, h, fitW, fitH int) (newW, newH int) {
	if w*fitH > h*fitW {
		newW, newH = fitW, h*fitW/w
	} else {
		newW, newH = w*fitH/h, fitH
	}
	if newW == 0 {
		newW = 1
	}
	if newH == 0 {
		newH = 1
	}
	return
}
