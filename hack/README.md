# Hack

A bunch of files and scripts to aid in development of the repo

- `redis` - Dir to store helpers specific to redis
- `postgres` - Dir to store helpers specific to postgres
- `send_mail.go` - Can be used to send mail via a remote SMTP server. Used for quick verification of SMTP provider credentials.

CI-related scripts

- `codecov.sh` - A script for creating codecov.io code coverage reports via prow