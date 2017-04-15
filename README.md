# clio-restore

A set of programs to restore Clio archives. Build: `go get github.com/FreeFeed/clio-restore/...`. 
This command builds the following executables in `$GOPATH/bin`:
 * clio-restore
 * clio-restore-activities
 * clio-rollback
 * clio-config

All these programs read the common settings from the _clio.ini_ file (see example _clio.ini_ in this repository).

This file is searched by default in the program's directory, but can be specified explicitly through the _-conf_ flag.

Also you should set all variables required by AWS for the _clio-restore_ and _clio-rollback_.

## clio-restore

Usage: `clio-restore [-conf /path/to/clio.ini] clio-archive.zip`

`clio-restore` restores archive from `clio-archive.zip` according to archive owners's settings in `archive` database table.

This program is configured via the environment. Call `clio-restore` without arguments to see all configuration options.

## clio-restore-activities

Usage: `clio-restore-activities [-conf /path/to/clio.ini]`

`clio-restore-activities` restores comments and likes of users who allow this after `clio-restore` run. It makes sense to run this program via cron once per hour or so.

See https://godoc.org/github.com/lib/pq for the database connection string syntax.

## clio-rollback

Usage: `clio-rollback [options] username`

Options are:
```
  -before string
        delete records before this date (default "2015-05-01")
  -conf string
        path to ini file (default is PROGRAM_DIR/clio.ini)
```

`clio-rollback` deletes any posts and files created by `username` before `-before` date. `username` is the username in Freefeed (new), not in Friendfeed (if there are difeerent).

## clio-config

Usage: `clio-config [options] username`

Options are:
```
  -conf string
        path to ini file (default is PROGRAM_DIR/clio.ini)
  -disable_comments
        set disable_comments flag for user (t or f)
  -has_archive
        set has_archive flag for user (t or f)
  -old_username string
        set old (friendfeed) username of user
  -recovery_status int
        set recovery_status for user (0, 1 or 2)
  -restore_comments_and_likes
        set restore_comments_and_likes flag for user (t or f)
```

`clio-rollback` show and changes archive settings for the `username`. `username` is the username in Freefeed (new), not in Friendfeed (if there are difeerent).

If program is called without options, it just prints the current configuration. If any of 'set' option is defined then program changes this option. For example, `clio-config -disable_comments=t username` will set `disable_comments` flag to true.

There are three `recovery_status` values: 0 — process not yet started, user can fill archive options form; 1 — user sent restoration request but process is not finished yet; 2 — process finished.


