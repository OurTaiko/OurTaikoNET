# Deploying OurTaikoNet

Status: **in production since the switch on 2026-11-14, 04:06 UTC.** The one container
`ourtaiko-net` (`127.0.0.1:8096`) answers <https://net.ourtaiko.org> and the three old host
names: `fanmade.ourtaiko.org`, `database.ourtaiko.org` and `profile.ourtaiko.org` get their
old APIs from it and have their pages redirected to the new site (`internal/legacy`). Its
database `ourtaiko_net` is the only one written; files are in the bucket the old sites used,
each source signing with its old site's key. The three old containers, their images and
their deploy folders were removed on 2026-11-14, 07:30 UTC: there is no going back to them.
Their databases are still there, unused, as they were at the switch. A push to
`main` of this repository or OurTaikoNetWeb publishes to production. Nobody is the site's
administrator until the role is given in the account center's admin (Client roles), and
`rollback.yml` has not been run.

One container, `ourtaiko-net`, on the server that runs the three old sites, beside them until
the switch. It answers `net.ourtaiko.org`, and from the switch on the three old host names as
well (`internal/legacy`). The website is the other repository, OurTaikoNetWeb: its builds are
put in a directory on the server and this container serves the current one.

```
/opt/ourtaiko-net/
  compose.production.yml     put here by every release of this repository
  incoming/<commit>/         an uploaded image, removed once published
  incoming-web/<commit>/     an uploaded website build, removed once published
  web/releases/<commit>/     website builds, the five newest
  web/current -> releases/<commit>
/etc/ourtaiko-net/docker/
  api.env, storage.env       settings and secrets; see api.env.example
```

## A release

Push to `main`. `.github/workflows/deploy.yml` runs every test in the Dockerfile's `test` stage
against a throwaway PostgreSQL 18, builds the image, keeps it in `ourtaiko-private`
(`net/images/<commit>.tar.gz`), uploads it and runs `deploy/publish.sh` on the server, which
loads it, replaces the container and waits for `/readyz`.

The website is released the same way from its own repository; that changes a link on the
server and restarts nothing. When a change needs both, release this repository first.

`rollback.yml` republishes an archived image by commit. It does not touch the database. A
release that changes the schema applies the change as it starts (`db/README.md`, "Changing the
schema"), and an image from before that change refuses the newer database: to go back past
one, delete its row from `net.schema_migrations` first (the older program does not mind the
columns it does not know).

Publishing happens only while the repository variable `DEPLOY_ENABLED` is `true` (Settings >
Variables); otherwise a push runs the tests and stops. The same holds for OurTaikoNetWeb,
which stops after its build. Both are `true` since 2026-11-14: **a push to `main` of either
repository publishes to the server.**

Secrets: the server's address is the organization's `SERVER_ADDR`. Each of the two
repositories has `DEPLOY_USER` (`ourtaiko-ci`), `DEPLOY_FINGERPRINT` (the server's SSH host key)
and `DEPLOY_KEY`, a private key whose public half is in that user's `authorized_keys`: signing in
is by key only. Optionally `DEPLOY_PORT` and `DEPLOY_BASE_PATH`. This repository also
needs the organization's `PRIVATE_AWS_ACCESS_KEY_ID` and `PRIVATE_AWS_SECRET_ACCESS_KEY`, which
are given to selected repositories; this one was added on 2026-11-14.

## The account the workflows sign in as

`ourtaiko-ci` on the server (`deploy/server/create-ci-user.sh`, run on 2026-11-14) is used by
nothing else. It has no password and one key, which forwards nothing; it owns
`/opt/ourtaiko-net`; it cannot read the settings files; and it may run as root only the docker
commands `publish.sh` runs, on images named `ourtaiko-net` and the one Compose file
(`/etc/sudoers.d/ourtaiko-ci`). The private key exists only as the `DEPLOY_KEY` secret of the two
repositories: to replace it, make a new pair, put the new public half in the account's
`authorized_keys` and set the secret again.

This narrows what a leaked key can do, but not to nothing: the Compose file that says what the
container may do comes from this repository with every release, so whoever can push to `main`,
or holds the key, can still start a container of their choosing on the server.

## Trying the image on your own machine

```sh
docker build -t ourtaiko-net:local .
docker run --rm -p 127.0.0.1:8091:8080 \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=1g --cap-drop ALL \
  -e DATABASE_URL='postgres://<you>@host.docker.internal:5432/ourtaiko_net_dev?sslmode=disable' \
  -e APP_ORIGIN=http://127.0.0.1:8091 -e STORAGE_DIR=/tmp/staging \
  -e WEB_DIR=/srv/web -v <OurTaikoNetWeb>/dist:/srv/web:ro \
  ourtaiko-net:local
```

Without `SSO_ISSUER` it serves the catalogues and the website; signing in needs the account
center. The whole suite as CI runs it:

```sh
docker build --target test -t ourtaiko-net-test .
docker run --rm -e DATABASE_TEST_URL=postgres://postgres:postgres@<a PostgreSQL 18>/postgres ourtaiko-net-test
```

## The first time (preview at net.ourtaiko.org)

Each step touches production and is done with the owner's go-ahead.

1. **Database.** `deploy/server/create-database.sh` makes the role and the empty database
   `ourtaiko_net` in the shared PostgreSQL and writes the two settings files with a password it
   generates and never prints. `build-schema.sh` builds the schema as that role. `import.sh`
   fills it from the three old databases, which it only reads, and compares every table with
   its origin (see `db/README.md`); it can be repeated, so it can run nightly while the
   preview lasts. Each is copied to the server and run from a file there, beside the SQL
   files it names. **Done on 2026-11-14.**
2. **Settings.** `create-database.sh` wrote `/etc/ourtaiko-net/docker/api.env` (database,
   origin, secure cookies, the proxy's address) and `storage.env` (`STORAGE_BACKEND=local`: the
   preview stays away from the old sites' files, next section). The account center's values
   and the three `LEGACY_*_HOST` are added later; see `api.env.example`. **Done on 2026-11-14.**
3. **Account center.** `deploy/server/register-application.sh` registers the Application
   `OurTaikoNet` (website sign-in, token lookup, profiles, game sessions and account-token
   exchange) and appends its credentials and a session key to `api.env`; then the container is
   recreated. **Done on 2026-11-14**, after step 6.
4. **Directories.** `/opt/ourtaiko-net/{incoming,incoming-web,web/releases}`, owned by
   `ourtaiko-ci`; the container reads `web/` as user 10001. `/etc/ourtaiko-net/docker`, root
   only. **Done on 2026-11-14.**
5. **Proxy.** In 1Panel, a reverse proxy website for `net.ourtaiko.org` to
   `http://127.0.0.1:8096` with a Let's Encrypt certificate; then `net.locations.conf` is
   installed as `/opt/1panel/www/sites/net.ourtaiko.org/proxy/root.conf` in place of the file
   1Panel wrote (kept beside it as `root.conf.before-net-locations`), and nginx reloaded
   (`nginx -t`, `nginx -s reload` in the OpenResty container). **Done on 2026-11-14.**
6. **Release** this repository, then OurTaikoNetWeb. **Done on 2026-11-14**, before steps 3 and 5.

## Files before the switch

Until the switch the database here is a copy of the old ones, refreshed by the import, while
the old sites go on changing theirs. The server does not only read files: in the background it
deletes the files its database says nothing refers to any more, moves files to new names, and
writes previews. Run on a copy that is hours old, with the right to write where the old sites
keep their files, it would delete or move files the old sites still use.

So before the switch the container gets no credentials for the files at all. `storage.env`
says `STORAGE_BACKEND=public`, with each source's prefix and the public address of that prefix
(`FANMADE_S3_PREFIX`, `FANMADE_S3_PUBLIC_BASE_URL`, and the same two for `OFFICIAL`): the old
sites' files are read over the address everyone reads them at, and cannot be written, moved or
deleted, because nothing here could sign such a request. In that mode the server starts none
of the background work above and answers the routes that store a file with 503
`FILES_READ_ONLY`.

What the preview then shows is the old sites' files as its copy of the database names them: a
chart uploaded or replaced there since the last import is missing or shows its old files, and
a file the old site has since deleted answers 403 from the public address.

`STORAGE_BACKEND=local` (the first days of the preview) serves no files at all.

## The switch (done on 2026-11-14)

What was done, in order; the scripts ran on the server from `/tmp` and are described here
because they are not needed again.

Before the window, with the old sites still serving:
1. The three `LEGACY_*_HOST` were set in `api.env` (they matter only to a request that reaches
   the container under one of those names) and the import was repeated.
2. `tools/compare-answers.py` asked the old sites and this server the same questions on the
   server itself (old `127.0.0.1:8080`, `8081`, `8093`; new `127.0.0.1:8096` by old host name
   and by new address). Fanmade: 347 charts, 1644 questions, the differences were `coverHash`
   added to the game's lists (on purpose) and one avatar missed once. Official: 150 charts one
   by one and every list, 1285 questions, `sso.service` (on purpose: the game asks the account
   center for a session of the service the catalogue names) and one nickname missed once.
   Players: 564 questions; links now lead to the new site, and the official source is called
   `official` where the old players' site said `database`.

The window, 04:05:34 to 04:06:05 UTC:
3. `docker stop` of `ourtaiko-fanmade`, `ourtaiko-database`, `ourtaiko-profile`.
4. `deploy/server/import.sh` a last time: 55 tables, check passed, 47,034 rows.
5. `storage.env` set to `s3`: the bucket and region of the old sites, each source's prefix and
   public address, and for each source the access key its old site used
   (`FANMADE_AWS_*`, `OFFICIAL_AWS_*`); the container recreated.
6. `net.locations.conf` installed as the proxy file of the three old sites
   (`fanmade…/proxy/root.conf`, `database…/database.conf`, `profile…/proxy/root.conf`), each
   old file kept beside it as `<name>.before-net`; `nginx -t`, reload.

Afterwards: a last backup of each old database by its own timer's service, and the daily
backup of `ourtaiko_net` (`deploy/server/install-backup.sh`, `cmd/backup`; 04:30 Asia/Tokyo,
30 days kept, uploaded under `net/backups/postgresql/` of the private bucket).

Not done by anyone yet: a released game from sign-in to a score; an upload on the website; a
restore drill of an `ourtaiko_net` backup.

## The database console

CloudBeaver (<https://postgre.ourtaiko.org>, `/opt/ourtaiko-cloudbeaver`) has two connections
to `ourtaiko_net`, with the two accounts it already used for the old databases
(`deploy/server/grant-console-access.sql`, run once on 2026-11-14):

- **OurTaikoNET (read only)**, as `ourtaiko_viewer`: reads all three schemas. Its queries name
  the schema (`fanmade.charts`, `official.charts`, `net.users`).
- **OurTaikoNET official songs (read-write)**, as `ourtaiko_database_editor`: reads all three,
  writes the schema `official` only, and cannot change the schema. A name without a schema is
  the official source's.

Both give up a transaction left open for five minutes, so a forgotten one cannot hold a lock
against a migration. An edit made there goes around the server: the database's own triggers
still apply (replacing a chart's file queues the old one for deletion), the server's checks do
not. The connections named "old database (frozen …)" are to the databases nothing reads any
more.

## What is left of the old sites

Removed on 2026-11-14: the containers `ourtaiko-fanmade`, `ourtaiko-database` and
`ourtaiko-profile`, their images, `/opt/ourtaiko-{fanmade,database,profile}` and the daily
backup timers of their databases. The switch cannot be undone any more.

Still there, on purpose:

- The three old host names, their 1Panel sites and certificates, and `LEGACY_*_HOST` here:
  every released game before 2026-11-14 asks the old addresses.
- The databases `ourtaiko_fanmade`, `ourtaiko_database` and `ourtaiko_profile`, unused, and
  the last dump of each from the switch (locally under `/var/backups/ourtaiko-*`, and in the
  private bucket). Nothing backs them up any more, and nothing deletes those dumps.
- `/usr/local/libexec/ourtaiko-fanmade-backup`: the account center's daily backup runs it.
- `/etc/ourtaiko-{fanmade,database,profile}`: the old settings. The storage keys in them are
  the ones this server signs with (its own copy is in `storage.env`); the two IAM users they
  belong to must stay.

**Do not publish Fanmade, Database or OurTaikoProfile again.** Their workflows would now fail
at the server (their folders are gone), and must stay that way: a running old container would
delete files by a database that no longer changes.
