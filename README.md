# clio-restore

A program to restore Clio archives. Build: `go get github.com/FreeFeed/clio-restore/...`. 
This command builds two executables in `$GOPATH/bin`: `clio-restore` and `clio-restore-activities`.

## clio-restore

Usage: `clio-restore clio-archive.zip`

`clio-restore` restores archive from `clio-archive.zip` according to archive owners's settings in `archive` database table.

This program is configured via the environment. Call `clio-restore` without arguments to see all configuration options.

## clio-restore-activities

Usage: `clio-restore-activities -db DATABASE_CONNECTION_STRING`

`clio-restore-activities` restores comments and likes of users who allow this after `clio-restore` run. It makes sense to run this program via cron once per hour or so.

See https://godoc.org/github.com/lib/pq for the `DATABASE_CONNECTION_STRING` syntax.
