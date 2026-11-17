# Deploying OurTaikoNet

Status: **in production since the switch on 2026-11-14, 04:06 UTC.** The one container
`ourtaiko-net` (`127.0.0.1:8096`) answers <https://net.ourtaiko.org> and the three old host
names: `fanmade.ourtaiko.org`, `database.ourtaiko.org` and `profile.ourtaiko.org` get their
old APIs from it and have their pages redirected to the new site (`internal/legacy`). Its
database `ourtaiko_net` is the only one written; files are in the bucket the old sites used,
each source signing with its old site's key. The three old containers, their images and
their deploy folders were removed on 2026-11-14, 07:30 UTC: there is no going back to them.
Their databases were dropped on 2026-11-14, 08:57 UTC, after every row of them was found in
`ourtaiko_net`; a last dump of each is kept. A push to
`main` of this repository publishes to production. Nobody is the site's
administrator until the role is given in the account center's admin (Client roles), and
`rollback.yml` has not been run.

One container, `ourtaiko-net`, on the server that runs the three old sites, beside them until
the switch. It answers `net.ourtaiko.org`, and from the switch on the three old host names as
well (`internal/legacy`). The website is `web/` in this repository: its builds are put in a
directory on the server and this container serves the current one. Until 2026-11-14 the
website was a repository of its own, OurTaikoNetWeb (archived on 2026-11-14); its history is kept here,
under `web/`, with the same commit hashes.

```
/opt/ourtaiko-net/
  compose.production.yml     put here by every release of the backend
  incoming/<commit>/         an uploaded image, removed once published
  incoming-web/<commit>/     an uploaded website build, removed once published
  web/releases/<commit>/     website builds, the five newest
  web/current -> releases/<commit>
/etc/ourtaiko-net/docker/
  api.env, storage.env       settings and secrets; see api.env.example
```

## A release

Push to `main`. `.github/workflows/deploy.yml` first looks at which part the push changed:
anything under `web/` (and `deploy/publish-web.sh`) is the website, everything else but
Markdown is the backend, and `contracts/` and the workflow itself are both. A run started by
hand does both. Then, for each part changed:

- **Backend.** Every test in the Dockerfile's `test` stage against a throwaway PostgreSQL 18;
  the image is built, kept in `ourtaiko-private` (`net/images/<commit>.tar.gz`) and uploaded,
  and `deploy/publish.sh` on the server loads it, replaces the container and waits for
  `/readyz`.
- **Website.** Lint, formatting, unit tests and the build in `web/`; the tested build is
  uploaded and `deploy/publish-web.sh` makes it the current one
  (`web/releases/<commit>`). That changes a link on the server and restarts nothing.

Nothing is published unless every test that ran passed. When a push changes both, the backend
is published first and the website only after it succeeded.

`rollback.yml` puts back one part, chosen when it is started. The backend: an archived image
by commit. It does not touch the database. A release that changes the schema applies the
change as it starts (`db/README.md`, "Changing the schema"), and an image from before that
change refuses the newer database: to go back past one, delete its row from
`net.schema_migrations` first (the older program does not mind the columns it does not
know). The website: one of the five builds the server keeps, by commit; builds published
before the website moved into this repository are named by OurTaikoNetWeb's commits.

Publishing happens only while the repository variable `DEPLOY_ENABLED` is `true` (Settings >
Variables); otherwise a push runs the tests and stops. It is `true` since 2026-11-14: **a push
to `main` publishes to the server.**

Secrets: the server's address is the organization's `SERVER_ADDR`. The repository has
`DEPLOY_USER` (`ourtaiko-ci`), `DEPLOY_FINGERPRINT` (the server's SSH host key) and
`DEPLOY_KEY`, a private key whose public half is in that user's `authorized_keys`: signing in is
by key only. Optionally `DEPLOY_PORT` and `DEPLOY_BASE_PATH`. It also needs the organization's
`PRIVATE_AWS_ACCESS_KEY_ID` and `PRIVATE_AWS_SECRET_ACCESS_KEY`, which are given to selected
repositories; this one was added on 2026-11-14.

## The account the workflows sign in as

`ourtaiko-ci` on the server (`deploy/server/create-ci-user.sh`, run on 2026-11-14) is used by
nothing else. It has no password and one key, which forwards nothing; it owns
`/opt/ourtaiko-net`; it cannot read the settings files; and it may run as root only the docker
commands `publish.sh` runs, on images named `ourtaiko-net` and the one Compose file
(`/etc/sudoers.d/ourtaiko-ci`). The private key exists only as the `DEPLOY_KEY` secret of this
repository (and of OurTaikoNetWeb, archived, where no workflow runs): to replace it, make a new pair, put the new
public half in the account's `authorized_keys` and set the secret again.

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
  -e WEB_DIR=/srv/web -v "$PWD/web/dist":/srv/web:ro \
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
6. **Release** the backend, then the website (then still OurTaikoNetWeb). **Done on 2026-11-14**,
   before steps 3 and 5.

## Multi-Enso (gRPC)

**On since 2026-11-14.** The steps below were done that day, in the order 2, 3, 1, 4, and
are kept for a server set up anew. Where the service is off the site answers
`404 MULTIPLAY_UNAVAILABLE` at `/api/v1/game/multiplay` and the game says online rooms are
not available.

Multi-Enso (the game's online rooms, `internal/multiplay`; root `README.md`, "Multi-Enso") is
a gRPC service inside the same container. `compose.production.yml` sets
`MULTIPLAY_LISTEN_ADDR=0.0.0.0:8081`, so the container listens there in clear text (h2c), and
maps it to `127.0.0.1:8097` on the server. The game is told to dial `MULTIPLAY_PUBLIC_URL`,
by default `APP_ORIGIN`: `https://net.ourtaiko.org`, port 443, where the proxy ends TLS and
passes the one path of the service on:

```
location ^~ /ourtaiko.multiplay.v1.MultiEnso/ { grpc_pass grpc://127.0.0.1:8097; … }
```

That location is `deploy/net.multiplay.conf`, a file of its own: `net.locations.conf` has to
stay one location for the panel to read it.

1. **Switch it on and release the backend.** Check first that nothing on the server listens
   on 8097 (`ss -ltn 'sport = :8097'`): a port that is taken stops the container from
   starting, and the site with it. Then, in `compose.production.yml`, set
   `MULTIPLAY_LISTEN_ADDR` and map 8097 in `ports`, and push to `main`.
   Afterwards `docker port ourtaiko-net` shows `8081/tcp -> 127.0.0.1:8097`, the log says
   `Multi-Enso (gRPC, h2c) listening on …`, and
   `curl https://net.ourtaiko.org/api/v1/game/multiplay` answers
   `{"url":"https://net.ourtaiko.org","protocol":1,"maxMembers":5}`. From here on the game
   tries to connect, so do steps 2 and 3 right after (or before: they do no harm while the
   port is closed).
2. **HTTP/2.** gRPC is HTTP/2: the 443 listener of the site `net.ourtaiko.org` must have it
   on (`http2 on;`, or `listen 443 ssl http2;`, in the site's server block).
   `curl -sI --http2 https://net.ourtaiko.org/healthz` must begin with `HTTP/2 200`. Without
   it the website works and the game cannot connect. (On 2026-11-14 the site's
   `/opt/1panel/www/conf.d/net.ourtaiko.org.conf` already had `http2 on;`.)
3. **The location.** Look at the site's server block first
   (`/opt/1panel/www/sites/net.ourtaiko.org/…`, in the panel "configuration file"): it
   should include every file of the site's `proxy/` folder (`include …/proxy/*.conf;`).
   Copy `deploy/net.multiplay.conf` there as `proxy/multiplay.conf`, beside `root.conf`;
   if the block includes `root.conf` by name only, add a line that includes the new file.
   Then `nginx -t` and `nginx -s reload` in the OpenResty container, as for
   `net.locations.conf`. The three old host names do not need the location. (On 2026-11-14
   the block included `proxy/*.conf`, and the file is there as `proxy/multiplay.conf`. It is
   a copy: a change to `deploy/net.multiplay.conf` has to be copied again by hand.)
4. **Check from outside**, with [grpcurl](https://github.com/fullstorydev/grpcurl) from a
   checkout of this repository (the service has no reflection, so it is given the protocol):

   ```sh
   grpcurl -import-path proto -proto ourtaiko/multiplay/v1/multiplay.proto \
     -d '{"join":{"quick_match":{}}}' net.ourtaiko.org:443 ourtaiko.multiplay.v1.MultiEnso/Connect
   ```

   Without a session it must answer `Code: Unauthenticated`, `Message: AUTH_REQUIRED`. With
   a made-up one (`-H 'authorization: Bearer <64 hex digits>'`) it must answer the same after
   a moment: the container asked the account center. `Unavailable` `AUTH_UNAVAILABLE` there
   means it could not. Any other answer (an unexpected HTTP status or content type, say)
   means the request went to the website's location: step 3 is not in effect. The same
   check without grpcurl: `go run ./tools/multiplay-smoke -do probe -site https://net.ourtaiko.org`.

Rooms are in memory, so every release of the backend closes them: players in a room are told
`ROOM_CLOSED` as the old container stops. `MULTIPLAY_PUBLIC_URL` and `MULTIPLAY_MIN_PLAYERS`
may be set in `api.env`; neither is needed. To switch the service off again, comment
`MULTIPLAY_LISTEN_ADDR` out in the Compose file: the site then answers
`404 MULTIPLAY_UNAVAILABLE` and the game does not try.

## Mirror the Don and nameplate art to Alibaba Cloud OSS

Stage the published catalogs and all objects they list with Python 3.11 or newer:

```sh
python3 -I tools/mirror-distrib.py /path/to/staging
```

Pass `3dchara` or `nameplate` after the staging directory to prepare only one collection.
The tool downloads the catalog from `distrib.ourtaiko.org`, checks object byte lengths and
SHA-256 hashes, and reuses matching local files on retry. It includes the Don base files and
any catalog-listed icons used by the website. Each catalog is staged after its objects are
verified. The staging directory can contain files left over from an earlier run. Use a fresh
directory for an exact snapshot.

1. Upload the staged objects at the same relative paths under `3dchara/` and `nameplate/` in
   the destination bucket. Upload each `catalog.json` after its objects are reachable. Keep
   the original bytes and use the appropriate content types.
2. Serve the destination through an HTTPS CDN hostname. For a mainland China OSS origin, use
   a CDN hostname eligible for mainland China acceleration. If the bucket is private, configure
   CDN origin access. Use a short cache lifetime for `catalog.json` and include the `v` query
   parameter in the cache key for asset requests so changed objects get a new cache entry.
   Allow cross-origin GET requests from the website's origin for the browser's Don renderer.
3. Fetch the mirrored catalogs and test an uncached model and icon with their versioned URLs.
   Verify the response bytes against the catalog. For the regional rollout, keep
   `DON_CATALOG_URL` on the canonical CloudFront catalog and configure the regional mirror
   separately once that backend support is deployed. Check the resulting API URLs, load an
   uncached outfit in the game, and check the website's Don preview from the configured site
   origin. The production OSS/ESA paths, cache policy and publishing requirements are in
   [aliyun-mirror.md](aliyun-mirror.md).
4. Already shipped game clients fetch `https://distrib.ourtaiko.org/nameplate/catalog.json`
   directly. To move their nameplate traffic, keep that public URL and route its prefix to the
   mirror. Using a different nameplate hostname requires a game build that requests it.

Run staging and upload again after publishing new Don or nameplate art to the canonical bucket.
To roll back regional Don downloads, clear `DON_MIRROR_CATALOG_URL` and recreate the container.
Chart downloads use the separate `FANMADE_S3_PUBLIC_BASE_URL` and
`OFFICIAL_S3_PUBLIC_BASE_URL` settings; keep those canonical until regional chart routing is
implemented.

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
not. The two connections to the old databases were removed with those databases.

## What is left of the old sites

Removed on 2026-11-14: the containers `ourtaiko-fanmade`, `ourtaiko-database` and
`ourtaiko-profile`, their images, `/opt/ourtaiko-{fanmade,database,profile}` and the daily
backup timers of their databases. The switch cannot be undone any more.

Dropped on 2026-11-14, 08:57 UTC: the databases `ourtaiko_fanmade`, `ourtaiko_database` and
`ourtaiko_profile`. Before that each of their tables was compared with its table here by
primary key (54 tables, 46,978 rows; the 56 players by id): nothing was missing but what this
server had removed since the switch — two chart files their authors replaced, and twelve
difficulty constants no chart used even in the old databases. The last dump of each, taken
minutes before, is `/var/backups/ourtaiko-{fanmade,database,profile}/daily/20261114T0856…/`
on the server and under `{fanmade,database,profile}/backups/postgresql/` in the private
bucket; nothing deletes them. `deploy/server/import.sh` has nothing left to read and must not
be pointed at a restored copy: it replaces what players have done since.

Removed after them, the same day: the old databases' roles (`ourtaiko`, `ourtaiko_database`,
`ourtaiko_profile`, `ourtaiko_database_viewer`), which owned nothing any more, and the units
`ourtaiko-{fanmade,database,profile}-backup.{service,timer}`. To read one of the last dumps,
restore it into a scratch database with `--no-owner`.

Still there, on purpose:

- The three old host names, their 1Panel sites and certificates, and `LEGACY_*_HOST` here:
  every released game before 2026-11-14 asks the old addresses.
- The roles `ourtaiko_viewer` and `ourtaiko_database_editor`: the console's accounts, in use.
- `/usr/local/libexec/ourtaiko-fanmade-backup`: the account center's daily backup runs it.
- The two IAM users of the old Fanmade and Database sites: this server signs with their keys
  (`FANMADE_AWS_*`, `OFFICIAL_AWS_*` in `storage.env`, now the only copy on the server).
- The old sites' backups under `/var/backups/ourtaiko-*`, the last dumps among them.

Also removed on 2026-11-14: the old settings `/etc/ourtaiko-{fanmade,database,profile}`
(after the storage keys in them were found equal to this server's own), and the unit
`ourtaiko-fanmade.service` from before the sites ran in containers.

The repositories Fanmade, Database, OurTaikoProfile and OurTaikoSiteKit are archived on GitHub
(2026-11-14), so their workflows cannot run and nothing can publish them again.
