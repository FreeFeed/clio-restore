package main

import "github.com/FreeFeed/clio-restore/clio"

func restoreThumbnails(entry *clio.Entry, db dbQ) (resUIDs []string) {
	if len(entry.Thumbnails) == 0 {
		return
	}
	return
}
