package clio

import (
	"encoding/json"
	"regexp"

	"github.com/FreeFeed/clio-restore/account"
)

var (
	twitterRe    = regexp.MustCompile(`^(http://twitter\.com/\w+)/statuses`)
	ffMediaURLRe = regexp.MustCompile(`^http://(m\.friendfeed-media\.com|i\.friendfeed\.com)/`)
)

// Entry represents archived FriendFeed entry
type Entry struct {
	entryJSON
	Author *account.Account
	Links  []string
}

// UnmarshalJSON unmarshalls Entry from the archive
func (entry *Entry) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &entry.entryJSON); err != nil {
		return err
	}
	entry.Body, entry.Links = deHTML(entry.Body)
	entry.Author = account.Get(entry.entryJSON.Author.UserName)

	if twitterRe.MatchString(entry.Via.URL) {
		// twitter is a special case
		entry.Body = entry.Body + " - " + entry.Via.URL
		entry.Links = append(entry.Links, entry.Via.URL)
		entry.Via.URL = twitterRe.FindStringSubmatch(entry.Via.URL)[1]
	}

	if entry.Via.URL == "" {
		entry.Via.URL = DefaultViaURL
		entry.Via.Name = DefaultViaName
	}

	return nil
}

// Comment represents archived comment
type Comment struct {
	commentJSON
	Author *account.Account
}

// UnmarshalJSON unmarshalls Comment from the archive
func (c *Comment) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &c.commentJSON); err != nil {
		return err
	}
	c.Body, _ = deHTML(c.Body)
	c.Author = account.Get(c.commentJSON.Author.UserName)
	return nil
}

// Like represents archived like
type Like struct {
	likeJSON
	Author *account.Account
}

// UnmarshalJSON unmarshalls Like from the archive
func (l *Like) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &l.likeJSON); err != nil {
		return err
	}
	l.Author = account.Get(l.likeJSON.Author.UserName)
	return nil
}
