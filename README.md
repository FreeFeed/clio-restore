# clio-restore

A program to restore Clio archives. Build: `go get github.com/FreeFeed/clio-restore/...`. 
This command builds the following executables in `$GOPATH/bin`: `clio-restore`, `clio-restore-activities` and `clio-rollback`.

## clio-restore

Usage: `clio-restore clio-archive.zip`

`clio-restore` restores archive from `clio-archive.zip` according to archive owners's settings in `archive` database table.

This program is configured via the environment. Call `clio-restore` without arguments to see all configuration options.

## clio-restore-activities

Usage: `clio-restore-activities -db DATABASE_CONNECTION_STRING`

`clio-restore-activities` restores comments and likes of users who allow this after `clio-restore` run. It makes sense to run this program via cron once per hour or so.

See https://godoc.org/github.com/lib/pq for the database connection string syntax.

## clio-rollback

Usage: `clio-rollback [options] username`

Options are:
```
  -attdir string
        directory to store attachments (S3 is not used if setted)
  -before string
        delete records before this date (default "2015-05-01")
  -bucket string
        S3 bucket name to store attachments (required if S3 is used)
  -db string
        database connection string
  -debug
        print stacktrace on failure
  -keep
        keep all posts and files, just set status
  -status int
        set 'recovery_status' to this value at the end (0, 1 and 2 are allowed) (default 1)

Also you should set all variables required by AWS if '-bucket' is used.
```

`clio-rollback` deletes posts and files restored by `clio-restore` (actually any posts and files created by `username` before `-before` date). `username` is the username in Freefeed, not in friendfeed (if there are difeerent).

`-status` options resets archive restoration status to given value at the end of rollback process (or immediately when `-keep`). There are three status values: 0 — process not yet started, user can set restoration options; 1 — user sent restoration request but process is not finished yet; 2 — process finished.


