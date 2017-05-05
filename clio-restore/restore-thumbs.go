package main

import (
	"archive/zip"
	"bufio"
	"io"
	"regexp"
	"strings"

	"github.com/FreeFeed/clio-restore/internal/clio"
	"github.com/davidmz/mustbe"
)

type localFile struct {
	*zip.File
	OrigName string
}

func (a *App) restoreThumbnails(entry *clio.Entry) (resUIDs []string) {
	if len(entry.Thumbnails) == 0 {
		return
	}

	bodyLinks := make(map[string]bool)
	for _, l := range entry.Links {
		bodyLinks[l] = true
	}

	// All images is of known types
	{
		handlableOnly := true
		for _, t := range entry.Thumbnails {
			if !(ffMediaURLRe.MatchString(t.Link) ||
				strings.HasPrefix(t.Link, "http://friendfeed.com/e/") ||
				strings.HasPrefix(t.URL, "http://twitpic.com/show/thumb/") ||
				imgurRe.MatchString(t.URL)) {
				handlableOnly = false
				break
			}
		}
		if handlableOnly {
			for _, t := range entry.Thumbnails {
				if ffMediaURLRe.MatchString(t.Link) {
					// get local file
					if uid, ok := a.createImageAttachment(t.Link); ok {
						resUIDs = append(resUIDs, uid)
					}
				}
				if t.Player != nil {
					// do nothing
				}
				if strings.HasPrefix(t.URL, "http://twitpic.com/show/thumb/") {
					url := strings.Replace(t.URL, "/thumb/", "/large/", 1)
					if uid, ok := a.createImageAttachment(url); ok {
						resUIDs = append(resUIDs, uid)
					}
				}
				if imgurRe.MatchString(t.URL) {
					code := imgurRe.FindStringSubmatch(t.URL)[1]
					if uid, ok := a.createImageAttachment("http://i.imgur.com/" + code + ".jpg"); ok {
						resUIDs = append(resUIDs, uid)
					}
				}
			}
			return
		}
	}

	// Dead services
	if strings.HasPrefix(entry.Via.URL, "http://filmfeed.ru/users/") ||
		strings.HasPrefix(entry.Via.URL, "http://www.zooomr.com/") ||
		strings.HasPrefix(entry.Via.URL, "http://meme.yahoo.com/") ||
		false {
		return
	}

	// Bookmarklet or direct post
	if entry.Via.URL == "http://friendfeed.com/share/bookmarklet" || entry.Via.URL == clio.DefaultViaURL {
		isSameURL := true
		isLocalThumbs := true
		for _, t := range entry.Thumbnails {
			if t.Link != entry.Thumbnails[0].Link {
				isSameURL = false
				break
			}
			if !ffMediaURLRe.MatchString(t.URL) {
				isLocalThumbs = false
				break
			}
		}

		if isSameURL && isLocalThumbs {
			// All links is the same
			if !bodyLinks[entry.Thumbnails[0].Link] {
				// Add link if body doesn't contan it
				entry.Body += " - " + entry.Thumbnails[0].Link
			}
			if !instagramImageRe.MatchString(entry.Thumbnails[0].Link) {
				// Use local thumbnails
				for _, t := range entry.Thumbnails {
					if uid, ok := a.createImageAttachment(t.URL); ok {
						resUIDs = append(resUIDs, uid)
					}
				}
				return
			}
		}
	}

	// Flickr
	{
		isAllFlickr := true
		isInBody := false
		for _, t := range entry.Thumbnails {
			if !strings.HasPrefix(t.Link, "http://www.flickr.com/photos/") {
				isAllFlickr = false
				break
			}
			if bodyLinks[t.Link] {
				isInBody = true
			}
		}
		if isAllFlickr && isInBody {
			// do nothing
			return
		}
	}

	// fotki.yandex.ru
	if strings.HasPrefix(entry.Via.URL, "http://fotki.yandex.ru/users/") {
		for _, t := range entry.Thumbnails {
			if strings.HasPrefix(t.URL, "http://img-fotki.yandex.ru/get/") && strings.HasPrefix(t.Link, "http://fotki.yandex.ru/users/") {
				imgURL := t.URL[:len(t.URL)-1] + "orig"
				if uid, ok := a.createImageAttachment(imgURL); ok {
					resUIDs = append(resUIDs, uid)
				}
			}
		}
		return
	}

	// http://picasaweb.google.com/
	if strings.HasPrefix(entry.Via.URL, "http://picasaweb.google.com/") {
		// импортируем картинку, которая присутствует в теле поста, в полном размере
		for _, t := range entry.Thumbnails {
			if picasaImageRe.MatchString(t.URL) && bodyLinks[t.Link] {
				url := strings.Replace(t.URL, "/s144/", "/", 1)
				if uid, ok := a.createImageAttachment(url); ok {
					resUIDs = append(resUIDs, uid)
				}
			}
		}
		return
	}

	// If there is only one thumb
	if len(entry.Thumbnails) == 1 {
		th := entry.Thumbnails[0]

		if strings.HasPrefix(th.Link, "http://www.youtube.com/watch") {
			// do nothing
			return
		}

		if strings.HasPrefix(th.Link, "http://vimeo.com/") && bodyLinks[th.Link] {
			// do nothing
			return
		}

		if instagramImageRe.MatchString(th.Link) {
			for _, l := range entry.Links {
				if strings.HasPrefix(l, "http://instagr.am/p/") || strings.HasPrefix(l, "http://instagram.com/p/") {
					// do nothing
					return
				}
			}
		}

		if strings.HasPrefix(th.Link, "http://behance.vo.llnwd.net/") {
			for _, l := range entry.Links {
				if strings.HasPrefix(l, "http://www.behance.net/gallery/") {
					// do nothing
					return
				}
			}
		}

		if strings.HasPrefix(th.Link, "http://b.vimeocdn.com/ts/") {
			for _, l := range entry.Links {
				if strings.HasPrefix(l, "http://vimeo.com/") || strings.HasPrefix(l, "https://vimeo.com/") {
					// do nothing
					return
				}
			}
		}
	}

	// Common case
	for _, t := range entry.Thumbnails {
		if ffMediaURLRe.MatchString(t.Link) {
			// get local file
			if uid, ok := a.createImageAttachment(t.Link); ok {
				resUIDs = append(resUIDs, uid)
			}
		} else if t.Player != nil {
			// do nothing
		} else if strings.HasPrefix(t.URL, "http://twitpic.com/show/thumb/") {
			url := strings.Replace(t.URL, "/thumb/", "/large/", 1)
			if uid, ok := a.createImageAttachment(url); ok {
				resUIDs = append(resUIDs, uid)
			}
		} else if strings.HasPrefix(t.Link, "http://pbs.twimg.com/media/") {
			if uid, ok := a.createImageAttachment(t.Link+":large", t.URL); ok {
				resUIDs = append(resUIDs, uid)
			}
		} else if imgurRe.MatchString(t.URL) {
			code := imgurRe.FindStringSubmatch(t.URL)[1]
			if uid, ok := a.createImageAttachment("http://i.imgur.com/" + code + ".jpg"); ok {
				resUIDs = append(resUIDs, uid)
			}
		} else if soupImageRe.MatchString(t.URL) {
			if uid, ok := a.createImageAttachment(strings.Replace(t.URL, "_400.gif", ".gif", 1)); ok {
				resUIDs = append(resUIDs, uid)
			}
		} else {
			if uid, ok := a.createImageAttachment(t.Link, t.URL); ok {
				resUIDs = append(resUIDs, uid)
			}
		}
	}

	return
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
	for _, f := range a.ZipFiles {
		if tsvFileRe.MatchString(f.Name) {
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
