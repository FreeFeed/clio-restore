package clio

import "time"

const (
	// DefaultViaURL is the via-URL for via-less posts (created on FriendFeed site)
	DefaultViaURL = "http://friendfeed.com"
	// DefaultViaName is the via-name for via-less posts (created on FriendFeed site)
	DefaultViaName = "FriendFeed"
)

// UserJSON is a json-data of user account
type UserJSON struct {
	UserName string `json:"id"`
	Type     string `json:"type"`
}

// ViaStatItem represents element of via statistic
type ViaStatItem struct {
	ViaJSON
	Count int `json:"count"`
}

// ViaJSON represents via-source of entry
type ViaJSON struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type entryJSON struct {
	Name       string    `json:"name"`
	Date       time.Time `json:"date"`
	Body       string    `json:"body"`
	Author     UserJSON  `json:"from"`
	Via        ViaJSON   `json:"via"` // always not nil but may have zero value
	Thumbnails []*struct {
		URL  string `json:"url"`
		Link string `json:"link"`
	} `json:"thumbnails"`
	Files []*struct {
		URL  string `json:"url"`
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"files"`
	Comments []*Comment `json:"comments"`
	Likes    []*Like    `json:"likes"`
}

type commentJSON struct {
	Date   time.Time `json:"date"`
	Author UserJSON  `json:"from"`
	Body   string    `json:"body"`
}

type likeJSON struct {
	Date   time.Time `json:"date"`
	Author UserJSON  `json:"from"`
}
