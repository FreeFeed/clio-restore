package main

import "regexp"

const (
	commentTypeVisible = 0
	commentTypeHidden  = 3
	hiddenCommentBody  = "Comment is in archive"

	recoveryNotStarted = 0
	recoveryStarted    = 1
	recoveryFinished   = 2
)

type statType string

const (
	statPosts    statType = "posts"
	statComments statType = "comments"
	statLikes    statType = "likes"
)

var (
	feedInfoRe   = regexp.MustCompile(`^[a-z0-9-]+/_json/data/feedinfo\.js$`)
	entryRe      = regexp.MustCompile(`^[a-z0-9-]+/_json/data/entries/[0-9a-f]{8}\.js$`)
	ffMediaURLRe = regexp.MustCompile(`http://(?:(?:m\.)?friendfeed-media\.com|i\.friendfeed\.com)/([0-9a-f]+)`)

	imgurRe          = regexp.MustCompile(`http://(?:i\.)?imgur\.com/(\w+?)s\.jpg`)
	picasaImageRe    = regexp.MustCompile(`http://lh\d+\.ggpht\.com/`)
	instagramImageRe = regexp.MustCompile(`http://[^/]+\.instagram\.com/`)
	soupImageRe      = regexp.MustCompile(`http://asset-\w\.soup\.io\/asset\/`)
	flickrImageRe    = regexp.MustCompile(`https?://[^/]+\.static\.?flickr\.com/.+_[a-z]\.jpg`)

	fileIDRe      = regexp.MustCompile(`[0-9a-f]+$`)
	instagramIDRe = regexp.MustCompile(`http://(?:instagr\.am|instagram.com)/p/([^/]+)`)
)
