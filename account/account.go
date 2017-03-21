package account

import (
	"database/sql"

	"github.com/davidmz/mustbe"
)

type feedIDs struct {
	ID  int
	UID string
}

// Account reflects account and archive properties of user
type Account struct {
	OldUserName             string
	NewUserName             string
	UID                     string
	HasArchive              bool
	DisableComments         bool
	RestoreSelfComments     bool
	RestoreCommentsAndLikes bool
	Feeds                   struct {
		Posts    feedIDs
		Comments feedIDs
		Likes    feedIDs
	}
}

// IsExists returns true if account exists in new Freefeed
func (a *Account) IsExists() bool {
	return a.UID != ""
}

// Store is a db fetcher and cache of accounts
type Store struct {
	db    *sql.DB
	cache map[string]*Account
}

// NewStore returns a new Store instance
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:    db,
		cache: make(map[string]*Account),
	}
}

// Get returns Account by old user's username. Get always returns not-nil value
// even if account does not exists in DB.
func (s *Store) Get(oldUserName string) *Account {
	if a, ok := s.cache[oldUserName]; ok {
		return a
	}

	a := &Account{
		OldUserName: oldUserName,
	}

	mustbe.OKOr(s.db.QueryRow(
		`select
			u.username,
			a.old_username,
			a.user_id,
			a.has_archive,
			a.disable_comments,
			a.restore_self_comments,
			a.restore_comments_and_likes,
			pf.id, pf.uid,
			cf.id, cf.uid,
			lf.id, lf.uid
		from
			archives a
			join users u on a.user_id = u.uid
			join feeds pf on pf.user_id = u.uid and pf.name = 'Posts'
			join feeds cf on cf.user_id = u.uid and cf.name = 'Comments'
			join feeds lf on lf.user_id = u.uid and lf.name = 'Likes'
		where a.old_username = $1`,
		oldUserName,
	).Scan(
		&a.NewUserName,
		&a.OldUserName,
		&a.UID,
		&a.HasArchive,
		&a.DisableComments,
		&a.RestoreSelfComments,
		&a.RestoreCommentsAndLikes,
		&a.Feeds.Posts.ID, &a.Feeds.Posts.UID,
		&a.Feeds.Comments.ID, &a.Feeds.Comments.UID,
		&a.Feeds.Likes.ID, &a.Feeds.Likes.UID,
	), sql.ErrNoRows)

	s.cache[oldUserName] = a
	return a
}
